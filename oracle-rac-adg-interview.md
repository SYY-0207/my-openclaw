# Oracle RAC & ADG 面试题集

---

## 🔹 RAC 基础架构

**Q1: Oracle RAC 的核心组件有哪些？各自的角色是什么？**

A:

| 组件 | 角色 |
|------|------|
| **Clusterware (Grid Infrastructure)** | 集群管理层，负责节点成员管理、资源管理、故障检测 |
| **ASM (Automatic Storage Management)** | 共享存储管理，替代文件系统和卷管理器 |
| **OCR (Oracle Cluster Registry)** | 存储集群配置信息（节点列表、资源定义、ASM 磁盘组信息等） |
| **Voting Disk** | 集群仲裁盘，决定节点成员资格，防止脑裂 |
| **Interconnect (Private Network)** | 节点间高速通信，传输 Cache Fusion 数据块和心跳 |
| **SCAN Listener** | 单一客户端接入名，屏蔽节点变化 |
| **GCS (Global Cache Service)** | 管理数据块在节点间的一致性（Cache Fusion 核心） |
| **GES (Global Enqueue Service)** | 管理全局锁（非数据块锁，如 DDL、表锁、字典锁） |

---

**Q2: Cache Fusion 的工作流程是什么？当一个节点要修改另一个节点持有的数据块时会发生什么？**

A:

完整流程（假设 Node 1 要修改 Block A，但 Block A 正被 Node 2 持有）：

1. Node 1 向 GCS 请求 Block A 的 X 锁
2. GCS 查到 Block A 的 Current 版本在 Node 2
3. GCS 通知 Node 2 释放锁 + 把 Block A 传给 Node 1
4. Node 2 将 dirty block 通过 Interconnect 直接发送给 Node 1（**不走磁盘！**）
5. Node 2 保留一个 Past Image (PI) 用于恢复
6. Node 1 收到 block 后进行修改
7. 提交时写 redo log（各自节点的）

**关键点：**
- 块传输走的是 **Interconnect（私有网络）**，延迟极低（微秒级）
- Node 2 保留 PI 是为了故障恢复——如果 Node 1 挂了，可以用 PI + redo 恢复
- 这就是 RAC 能做到"写扩展"的原因——多个节点可以同时写不同的块，块有争用时虽然需要传输但不走磁盘

---

**Q3: GCS 和 GES 的区别？各管什么？**

A:

| | GCS (Global Cache Service) | GES (Global Enqueue Service) |
|---|---|---|
| 管理对象 | 数据块（Buffer Cache 中的块） | 非块级资源（表锁、DDL 锁、字典缓存、序列等） |
| 锁类型 | Global Cache Lock (Buffer Lock) | Global Enqueue |
| 后台进程 | LMS (Lock Manager Server) | LMD (Lock Manager Daemon) |
| 性能影响 | LMS 忙说明块争用严重 | LMD 忙说明锁竞争严重 |

**等待事件对照：**
- `gc buffer busy` → GCS 相关的块争用
- `gc cr grant 2-way` → Cache Fusion 传输 CR 块的时间
- `enq: TX - row lock contention` → GES 管理的事务锁竞争

---

**Q4: RAC 中为什么要有 Private Interconnect？对网络有什么要求？**

A:

Private Interconnect 是 RAC 节点间专用的高速网络，承载两类流量：

1. **Cache Fusion**（数据块传输）—— 占大头，延迟直接影响 OLTP 性能
2. **Cluster Heartbeat**（心跳检测，每秒一次）

**要求：**
- 带宽：建议 10GbE 起步，生产环境推荐 25GbE 或 InfiniBand
- 延迟：越低越好，建议 < 200 微秒
- 冗余：至少两条物理链路做 bonding/HAIP（Oracle High Availability IP）
- UDP/RDS 协议：Cache Fusion 默认用 UDP（低开销）、也可以用 RDS（RDMA）
- 网络包大小：Jumbo Frame (MTU 9000) 能显著提升吞吐

**为什么不用 Public 网络：**
- 数据块传输的流量极大且对延迟敏感，必须和客户端请求流量隔离
- 公共网络有防火墙/路由等额外开销

**排查命令：**
```sql
-- 查看 Interconnect 延迟
SELECT * FROM v$sysmetric WHERE metric_name LIKE '%gc cr%';
```

---

**Q5: 脑裂 (Split Brain) 是什么？RAC 如何防止？**

A:

**脑裂场景：** 集群中节点之间的私有网络全部断开，每个节点都认为自己是唯一存活的，同时尝试访问共享存储——数据损坏。

**RAC 的防止机制——Voting Disk (仲裁盘)：**

1. 当心跳丢失时，节点发起"reconfiguration"
2. 能访问 Voting Disk 的节点获得投票
3. 获得多数票的子集群存活，被驱逐的节点被强制 reboot（STONITH 行为，叫 Node Fencing）
4. 被驱逐的节点通过 `reboot` 或 `cssd abort` 来确保它不再访问数据

**两个关键进程：**
- **CSSD (Cluster Synchronization Services Daemon)**：管理心跳和成员资格
- **OHASD (Oracle High Availability Services Daemon)**：守护进程的守护进程，CSSD 挂了由它拉起

**脑裂保护使用规则：**
- Voting Disk 必须是奇数个（3 或 5），保证"多数"可决
- Voting Disk 放在独立的、高可用的共享存储上

---

**Q6: ASM 是什么？为什么 RAC 要用 ASM 而不是裸设备或集群文件系统？**

A:

**ASM (Automatic Storage Management)** 是 Oracle 自带的卷管理器 + 类文件系统：

- 将磁盘（或 LUN）组成磁盘组
- 自动 striping + mirroring
- 自动负载均衡（数据在磁盘组内均匀分布）
- 支持在线扩盘/缩盘（Rebalance）

**对比其他方案：**

| | ASM | 集群文件系统 (OCFS2/GFS2) | 裸设备 |
|---|---|---|---|
| 管理复杂度 | 低（SQL 操作） | 中（需 OS 层管理） | 高（需手动映射） |
| 性能 | 高（直写块设备） | 中（文件系统开销） | 最高 |
| 动态扩缩 | ✅ 在线 rebalance | 取决于 FS | ❌ 困难 |
| Stripe/Mirror | 内置 | 需单独配置 | 需存储层做 |
| 多路径 | 自动管理 | 需多路径软件 | 需多路径软件 |

**生产推荐：ASM**。11g R2 后已成为 RAC 标配，Oracle 也停止了对裸设备的支持。

---

**Q7: RAC 中的连接负载均衡有哪些方式？各有什么优缺点？**

A:

**1. 客户端负载均衡 (Client-Side LB)**

`tnsnames.ora` 中配置多个节点地址，客户端随机选择：
```
RAC_DB =
  (DESCRIPTION =
    (LOAD_BALANCE=ON)
    (ADDRESS=(PROTOCOL=TCP)(HOST=node1-vip)(PORT=1521))
    (ADDRESS=(PROTOCOL=TCP)(HOST=node2-vip)(PORT=1521))
    (CONNECT_DATA=(SERVICE_NAME=orcl))
  )
```
- ✅ 简单直接
- ❌ 不知道各节点真实负载，可能把连接打到最忙的节点

**2. 服务端负载均衡 (Server-Side LB)**

通过 SCAN Listener + Listener 做基于节点负载的分配：
- Remote Listener 向 SCAN 注册
- 结合 `SERVICE_TIME`、`THROUGHPUT` 等指标分配
- PMON 定期更新负载信息

**3. 连接池/TAC (Transparent Application Continuity)**

应用层的 Runtime 连接负载均衡 + 连接失败透明切换

**最佳实践：**
- 客户端 LB（`LOAD_BALANCE=ON`）+ 服务端 LB（`CLB_GOAL`）
- 配合 TAFF (Transparent Application Failover) 做故障时切换

---

**Q8: RAC 节点的驱逐 (Eviction) 流程是怎样的？什么原因会导致被驱逐？**

A:

**常见驱逐原因：**
- CSSD 心跳超时（Interconnect 问题或节点过载，CSSD 得不到 CPU 导致 missed heartbeat）
- 节点 hang（OS 层负载过高导致核心进程无响应）
- 网络分区
- 存储访问中断导致 I/O fencing

**驱逐流程：**
1. CSSD 超时未收到心跳
2. 发起 reconfiguration
3. OCR + Voting Disk 仲裁
4. 被驱逐节点上 `cssdagent` 执行 reboot 或进程级 fencing
5. 存活节点恢复全局资源，重新分配被驱逐节点的锁和事务

**排查方法：**
```bash
# 查看驱逐日志
$GRID_HOME/log/<hostname>/cssd/ocssd.log
$GRID_HOME/log/<hostname>/alert<hostname>.log

# 关键字：reboot、voting disk、timeout
```

---

## 🔹 RAC 高级问题

**Q9: RAC 中的 Sequence 缓存问题怎么处理？`CACHE` + `NOORDER` vs `ORDER` 的区别？**

A:

这是 RAC 中的经典性能陷阱。每个节点独立从序列缓存拿号。

| 属性 | 行为 | 性能 | 顺序 |
|------|------|------|------|
| CACHE + NOORDER | 每节点独立缓存，不保证全局顺序 | 最佳 ✅ | 交错（RAC 典型） |
| CACHE + ORDER | 需要向 GES 请求全局顺序，有竞争 | 差 ❌ | 全局有序 |
| NOCACHE | 每次 nextval 都写磁盘 + 锁 | 极差 ❌❌ | 有序 |

**问题：** `CACHE + NOORDER` 时，Node1 拿了 1-20，Node2 拿了 21-40，两个节点并发插入，ID 交叉出现（1, 21, 2, 22...），不是严格递增。

**解决方案：**
- 绝大多数场景用 `CACHE + NOORDER`（接受 ID 不严格有序）
- 必须严格有序（如日志表）考虑用 timestamp 或者用 `ORDER` 但承担性能代价
- 12c+ 支持 `SCALE` 扩展序列，减少跨节点争用

---

**Q10: RAC 性能调优的核心指标有哪些？怎么看？**

A:

**等待事件（最重要）：**

| 等待事件 | 含义 | 正常阈值 |
|----------|------|----------|
| `gc cr block 2-way` | CR 块跨节点传输延迟 | < 5ms |
| `gc current block 2-way` | Current 块跨节点传输延迟 | < 5ms |
| `gc buffer busy acquire/release` | 跨节点块争用 | 越低越好 |
| `enq: TX - row lock contention` | 行锁竞争（分布式死锁可能） | - |
| `ges remote message` | GES 远程消息延迟 | 越低越好 |

**关键视图：**
```sql
-- 1. 全局Cache命中率
SELECT * FROM gv$sysstat WHERE name LIKE '%global cache%';

-- 2. 各节点的 Cache Fusion 流量
SELECT inst_id, name, value 
FROM gv$sysstat 
WHERE name IN (
  'gc cr blocks received', 'gc cr blocks served',
  'gc current blocks received', 'gc current blocks served'
);

-- 3. 查看热块竞争
SELECT owner, object_name, subobject_name
FROM gv$segment_statistics
WHERE statistic_name = 'gc buffer busy'
ORDER BY value DESC;

-- 4. 全局等待事件
SELECT inst_id, event, total_waits, time_waited_micro/1000 ms_waited
FROM gv$system_event
WHERE event LIKE 'gc%'
ORDER BY time_waited_micro DESC;
```

**黄金法则：如果 `gc cr` + `gc current` 占总等待事件的 5%-10% 以内，RAC 跑得很健康。**

---

## 🔹 ADG 基础架构

**Q11: Oracle Data Guard 的三种保护模式各自是什么？区别在哪？**

A:

| 保护模式 | Redo 传输 | 提交确认 | 数据丢失风险 | 性能影响 |
|----------|-----------|----------|-------------|----------|
| **最大可用 (Max Availability)** | SYNC | 备库确认后才返回提交成功 | 零丢失（正常时） | 中等（SYNC 延迟） |
| **最大保护 (Max Protection)** | SYNC | 必等备库确认，否则主库挂起 | **零丢失** | 最大 |
| **最大性能 (Max Performance)** | ASYNC / LGWR ASYNC | 主库不等备库 | 可能丢失最后若干事务 | 最小 |

**详细解释：**

**Max Availability（生产最常用）：**
- 正常时：行为 = Max Protection（SYNC 传输，零丢失）
- 备库不可达时：自动降级为 Max Performance（不等备库，继续运行）
- 备库恢复后：自动切回 SYNC 模式

**Max Protection（金融级零丢失）：**
- 主库提交必须等至少一个 SYNC 备库写入 standby redo log
- 如果备库不可达 → **主库 shutdown**（宁可不服务也不丢数据）
- 要求至少两个 SYNC 备库（单备库挂了主库就停）

**Max Performance（默认模式，99% 场景用这个）：**
- ASYNC 传输，主库不等
- 性能最高，但可能丢少量数据
- 配合 Far Sync Instance 可做到零丢失（见后续问题）

---

**Q12: Redo 传输模式 SYNC / ASYNC / ARCH 的区别？LGWR 和 ARCH 进程各自什么场景用？**

A:

| 传输模式 | 发送进程 | 网络 | 数据丢失窗口 | 适用 |
|----------|----------|------|-------------|------|
| **SYNC (LGWR)** | LGWR → LNS | 低延迟 | 零丢失 | 同城/同机房 |
| **ASYNC (LGWR ASYNC)** | LGWR → LNS | 任何 | 少量 | 异地灾备 |
| **ARCH** | ARCn | 任何 | 一个归档日志 | DBFS/测试 |

**LGWR vs ARCH：**
- **LGWR 模式**：LGWR 同时写 online redo log + 发给 LNS（Log Network Server）传给备库，实时性强
- **ARCH 模式**：只有归档完成后，ARCn 才把归档日志发给备库，延迟大（至少一个 redo log 的窗口）

**生产推荐：同城用 SYNC，异地用 ASYNC + Far Sync。基本不用 ARCH 模式。**

---

**Q13: Active Data Guard 和普通 Data Guard 的区别？ADG 带来的最大价值是什么？**

A:

| | 普通 Data Guard | Active Data Guard |
|---|---|---|
| 备库可读？ | ❌ Mount 状态，不能读 | ✅ 只读打开，可实时查询 |
| 实时报表 | ❌ | ✅ |
| 实时应用 redo | - | ✅ Real-Time Apply |
| 延迟 | 等日志 apply 完才能看 | 最小延迟 |
| 授权 | 企业版包含 | **额外 license (需付费)** |

**ADG 的核心价值：**

1. **读写分离** — 报表、备份、批处理跑在备库，主库只承担 OLTP
2. **实时数据** — 备库在 apply redo 时就可以查询，延迟秒级
3. **零停机迁移/升级** — 建个 ADG → switchover → 原主库变备库 → 升级 → switchover 回来
4. **Far Sync 实现异地零丢失** — 见下题

---

**Q14: Switchover 和 Failover 的区别？各自操作流程？**

A:

| | Switchover (切换) | Failover (故障转移) |
|---|---|---|
| 触发 | **计划内**（维护、升级） | **计划外**（主库故障） |
| 数据丢失 | 零丢失 | 可能丢失 ASYNC 未传输部分 |
| 可逆性 | 原主库变备库，可切回 | 原主库需要重建 |
| 操作方式 | DGMGRL/Broker 一键 | 手动或 DGMGRL |

**Switchover 流程：**
```bash
# DGMGRL 一键切换
DGMGRL> validate database 'orcl_stby'  -- 先校验
DGMGRL> switchover to 'orcl_stby'
# 原主库自动变备库
```

**Failover 流程（手动）：**
```sql
-- 在备库执行
ALTER DATABASE RECOVER MANAGED STANDBY DATABASE FINISH;
ALTER DATABASE COMMIT TO SWITCHOVER TO PRIMARY;
ALTER DATABASE OPEN;
-- 原主库恢复了需要重建为备库
```

**Flashback + Reinstantiate：**
```bash
# 12c+ 支持 - 原主库用 flashback 回到失败前 SCN，重新作为备库启动
DGMGRL> reinstate database 'orcl';
```

---

**Q15: Far Sync Instance 是什么？怎么用 ADG + Far Sync 实现异地零丢失？**

A:

**Far Sync Instance 是 Oracle 12c 引入的轻量级实例：**

- 不 mount 数据库（没有数据文件）
- 只接收主库的 redo，然后转发给异地备库
- 距离主库近（同城，低延迟），距离备库远（异地，高延迟）

**架构：**
```
主库 (北京) ──SYNC──▶ Far Sync (北京) ──ASYNC──▶ 备库 (上海)
          < 1ms                   < 50ms>
```

**原理：**
- 主库 → Far Sync 用 **SYNC**（同城，延迟 < 1ms）→ 零丢失
- Far Sync → 异地备库用 **ASYNC**（长距离延迟高，但不丢数据——因为 Far Sync 已经持久化了）
- 即使异地备库挂了或网络断了，Far Sync 会暂存 redo，网络恢复后继续传

**配置要点：**
- Far Sync 实例放在离主库 < 100km 的数据中心
- Far Sync 仅需 ASM + control file + standby redo log
- 备库接收的 redo 可能比主库晚几秒但不会丢

---

## 🔹 ADG 高级问题

**Q16: ADG 的延迟如何监控和排查？备库追不上主库怎么办？**

A:

**监控延迟：**
```sql
-- 主库查传输延迟
SELECT dest_id, applied_scn - archived_scn gap
FROM gv$archive_dest_status 
WHERE dest_id = 2;

-- 备库查 apply 延迟
SELECT name, value, unit 
FROM v$dataguard_stats;

-- 关键指标：
-- transport lag    : 传输延迟（redo 从产生到到备库的时间）
-- apply lag        : 应用延迟（redo 到备库到 apply 完的时间）
-- apply finish time: 预估完全追上的时间
```

**排查步骤：**
1. 看网络带宽/延迟是否够（SYNC 模式下网络是瓶颈）
2. 看备库的 redo apply 是否有瓶颈——MRP 进程只有 1 个（默认）
3. 看 standby redo log 大小是否够（太小导致频繁切换）
4. 看备库是否有大事务回滚或检查点阻塞 apply

**优化方向：**
- 加大 standby redo log（和主库 online redo log 一样大或更大）
- 开并行 recovery：`RECOVER MANAGED STANDBY DATABASE PARALLEL 8`
- 12.2+ 支持多实例 recovery（ADG 节点可以 RAC 多节点并行 apply）
- 网络：确保带宽足够（ASYNC 的话也要关注，积压太多后面追不上）

---

**Q17: Data Guard Broker 是什么？DGMGRL 常用命令有哪些？**

A:

**Broker** 是 DG 的集中管理框架，通过 `DMON` 后台进程管理配置。

**核心概念：**
- Configuration: 整个 DG 环境
- Database: 主库/备库
- Observer: 可选的第三方监控节点（用于自动 Failover）

**DGMGRL 常用命令：**
```bash
DGMGRL> connect sys/password@primary_db
DGMGRL> show configuration                -- 查看整体状态
DGMGRL> show database 'orcl'              -- 查看某库详情
DGMGRL> show database 'orcl_stby'         -- 备库状态
DGMGRL> validate database 'orcl_stby'     -- 切换前校验
DGMGRL> switchover to 'orcl_stby'         -- 切换！
DGMGRL> edit database 'orcl_stby' set property 'ApplyParallel'=8;
DGMGRL> enable fast_start failover        -- 开启自动故障转移
```

**Observer 的作用（Fast-Start Failover）：**
- Observer 是独立的轻量进程，持续监控主库存活
- 主库宕机时自动触发 Failover（无需人工干预）
- 启动：`DGMGRL> start observer`
- 建议 Observer 放在第三个数据中心

---

**Q18: Snapshot Standby 是什么？和物理备库、逻辑备库有什么区别？**

A:

| | 物理备库 (Physical) | 逻辑备库 (Logical) | 快照备库 (Snapshot) |
|---|---|---|---|
| 同步方式 | 块对块 redo apply | SQL apply（挖掘 redo → SQL 执行） | 类似 Physical，但可临时读写 |
| 可读可写 | ❌ 只读（ADG） | ✅ 读写 | ✅ 临时读写 |
| 数据类型支持 | 全部 | 大部分（有限制） | 全部 |
| 用途 | 灾备/只读报表 | 报表/数据转换/滚动升级 | 临时测试/QA 环境 |
| 恢复 | N/A | N/A | 丢弃变更可以切回 Physical |

**Snapshot Standby 使用场景：**
```sql
-- 把物理备库切为快照备库
ALTER DATABASE CONVERT TO SNAPSHOT STANDBY;
-- 在快照备库上做测试、跑批
-- 测试完，切换回物理备库（测试期间的修改全部丢弃）
ALTER DATABASE CONVERT TO PHYSICAL STANDBY;
```

---

**Q19: ADG 下做备份有哪些策略？什么备份应该在哪边做？**

A:

**最佳实践：在备库做备份（而不是主库）：**

- 全量备份 (RMAN Full) → 备库做，不消耗主库资源
- 增量备份 → 备库做
- 归档日志备份 → 主库和备库都可以，通常备库统一管理

```bash
# RMAN 备库全量备份
rman target sys/password@stby
RMAN> BACKUP DATABASE PLUS ARCHIVELOG DELETE INPUT;
```

**注意事项：**
- 备库备份必须和主库兼容（主库出问题后，备库备份要能 restore 到主库）
- 使用 Recovery Catalog 统一管理备份元数据
- 12c+ 支持 RMAN 的 Multi-Section Backup 从备库加速

---

**Q20（实战）: 凌晨 3 点 RAC 一个节点被驱逐，ADG 备库也在报 apply lag。你怎么排查？**

A:

```
第一步：确认现状
├── RAC: 剩余节点是否正常服务？被驱逐节点 reboot 了吗？
│   crsctl stat res -t           -- 看集群资源状态
│   $GRID_HOME/log/<node>/cssd/ocssd.log  -- 看驱逐原因
├── ADG: apply lag 多大？
│   SELECT * FROM v$dataguard_stats;  -- 看具体延迟值
│   SELECT process, status FROM v$managed_standby; -- MRP 是否在跑

第二步：找根因
├── 被驱逐节点：
│   1. 检查 ocssd.log 最后 100 行，找 "reboot" / "timeout" / "missed heartbeat"
│   2. top/htop → OS 负载是否过高（CSSD 需要实时调度）
│   3. Interconnect 丢包率 → netstat -s / ifconfig error 统计
│   4. 存储路径是否短暂不可达
│   常见原因：CPU 被耗尽导致 CSSD 心跳丢失被踢
│
├── ADG apply lag：
│   1. 主库日志切换频率 → 如果 RAC 节点驱逐导致主库负载升高
│   2. 备库 MRP 是否挂了？→ ALTER DATABASE RECOVER MANAGED STANDBY DATABASE DISCONNECT;
│   3. 网络带宽被占用？

第三步：处置
├── RAC: 被驱逐节点 rebooting → 等它回来，查看是否有硬件故障
│   如果起不来 → 先保证剩余节点服务，被驱逐节点从集群踢出维护
├── ADG: 重启 MRP → ALTER DATABASE RECOVER MANAGED STANDBY DATABASE PARALLEL 8;

第四步：天亮后复盘
├── 驱逐根因 → 是否需要增加 CPU、调整 CSSD 超时参数
├── ADG → 是否需要增加备库节点或调整传输模式
└── 监控阈值 → 提前预警
```

---

📝 共 20 题，覆盖 RAC 架构/性能/故障 + ADG 架构/切换/监控/实战。大宝贝觉得深度如何？
