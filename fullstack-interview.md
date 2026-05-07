# 全栈技术面试题集

> 覆盖：MySQL / Oracle&DB2&PG / Redis / 编程语言 / 运维自动化 / 容器&K8s / AI大模型

---

## 🔹 MySQL 深度

**Q1: MySQL 8.0 的优化器在 5.7 基础上做了哪些关键改进？遇到过优化器选错索引的情况吗，怎么排查和解决？**

A:

8.0 优化器改进：
- **直方图统计**（Histogram）：5.7 只有索引基数，8.0 可对非索引列建直方图，解决数据倾斜场景估算偏差
- **Hash Join**（8.0.18+）：替代部分 Nested Loop，大数据集关联不再依赖索引
- **成本模型重做**：InnoDB 的 cost constants 更精确
- **EXPLAIN ANALYZE**：显示实际执行时间 vs 估算代价，一目了然

排查优化器选错索引：
```sql
-- 1. 看执行计划
EXPLAIN FORMAT=JSON SELECT ...;
-- 看 cost_info，对比各候选索引的 cost

-- 2. 对比强制索引的实际耗时
SELECT ... FORCE INDEX(idx_a) WHERE ...;
SELECT ... FORCE INDEX(idx_b) WHERE ...;

-- 3. 检查统计信息是否过期
SELECT * FROM mysql.innodb_index_stats WHERE table_name = 't';

-- 4. 更新统计信息
ANALYZE TABLE t;

-- 5. 如果统计信息对但优化器还是选错 → 用 optimizer_trace
SET optimizer_trace='enabled=on';
SELECT ...;
SELECT * FROM information_schema.optimizer_trace\G
-- 可以看到优化器逐个评估每个索引的 cost 和 rejected 原因
```

**生产处置：**
- 短期：`FORCE INDEX` 或 `USE INDEX`
- 中期：直方图 `ANALYZE TABLE t UPDATE HISTOGRAM ON col`
- 长期：调整 `optimizer_search_depth`，或调整成本常量

---

**Q2: MySQL 线上出现大量 `MDL` 锁等待，DDL 被阻塞，怎么排查和紧急处理？**

A:

**MDL (Metadata Lock) 机制：** DDL 需要 MDL_EXCLUSIVE，等所有 MDL_SHARED_READ 释放。如果有未提交的长事务持有 SR 锁，DDL 永远排不到队。

**排查：**
```sql
-- 8.0 精确查 MDL 锁
SELECT * FROM performance_schema.metadata_locks
WHERE OBJECT_SCHEMA = 'mydb' AND OBJECT_NAME = 't';

-- 看谁在等 (waiting) 和谁在持 (granted)
-- 5.7 可以用 sys.schema_table_lock_waits
SELECT * FROM sys.schema_table_lock_waits;

-- 找持有 SR 锁但未提交的长事务
SELECT trx_id, trx_started, trx_mysql_thread_id, trx_query
FROM information_schema.innodb_trx
WHERE trx_started < NOW() - INTERVAL 5 MINUTE;
```

**紧急处理：**
```sql
-- 1. 找到阻塞源的事务/连接
-- 2. kill 掉阻塞的连接
KILL <connection_id>;

-- 3. 如果 kill 不掉（waiting for commit），kill -9 对应 OS 进程
--    先找到:
SELECT * FROM performance_schema.threads WHERE processlist_id = <id>;

-- 4. 必要时设置锁等待超时
SET SESSION lock_wait_timeout = 5;
```

**预防：**
- `wait_timeout` / `interactive_timeout` 设合理值，自动踢空闲连接
- 监控长事务（`innodb_trx` 中 `trx_started` 太久）
- 上线 DDL 前先查活跃事务
- pt-online-schema-change 替代直操 DDL

---

**Q3: `redo log` 和 `binlog` 的区别？两阶段提交的 prepare 和 commit 各发生了什么？崩溃恢复时怎么用？**

A:

| | Redo Log | Binlog |
|---|---|---|
| 层面 | InnoDB 引擎层 | Server 层 |
| 内容 | 物理日志（页修改） | 逻辑日志（SQL 语句/行变更） |
| 产生 | 事务过程中持续写 | 事务提交时写 |
| 用途 | 崩溃恢复 | 主从复制、PITR 时间点恢复 |
| 管理 | 大小固定、循环写（一组文件） | 递增、按大小/时间切 |

**两阶段提交：**
```
1. Prepare 阶段：
   └── 写 redo log，标记为 prepare 状态（尚未提交）

2. Commit 阶段：
   ├── 写 binlog（先写 binlog！）
   └── 写 redo log commit 标记
```

**为什么 binlog 在 redo commit 之前写：**
- 保证 binlog 和 redo 一致
- 如果先 commit redo 再写 binlog，中间崩溃 → redo 已提交但 binlog 没有 → 备库丢数据

**崩溃恢复流程：**
```
1. 扫描 redo log
2. 对于 prepare 状态的事务：
   ├── 如果 binlog 中有对应记录 → 提交（redo log commit）
   └── 如果 binlog 中没有     → 回滚
3. 对于已 commit 的事务：已提交，无需处理
```

---

## 🔹 Oracle / DB2 / PostgreSQL 跨库

**Q4: Oracle RAC 和 PostgreSQL 的 Citus/Greenplum 在分布式扩展思路上有什么本质不同？**

A:

| | Oracle RAC | Citus (PG) | Greenplum |
|---|---|---|---|
| 架构 | Shared-Disk | Shared-Nothing | Shared-Nothing |
| 数据分布 | 共享磁盘，各节点都可访问所有数据 | 按分布键分片（hash/range） | 按分布键分片 |
| 扩展方式 | 加节点但共享磁盘 | 加节点，数据重分布 | 加节点，数据重分布 |
| Cache Fusion | 核心机制（块跨节点传输） | 无（每节点独立） | 无 |
| 写扩展 | 有限（块争用） | 好（并行写各自分片） | 好 |
| SQL 兼容 | 完整 | 部分（分布式 JOIN 受限） | 部分 |

**核心思想差异：**
- Oracle RAC：多个脑共用一个硬盘，靠高速内联网协调
- PG 分布式：各自独立，靠 Coordinator 拆 SQL 再汇总

---

**Q5: DB2 的隔离级别和锁机制与 Oracle 有什么核心差异？**

A:

| | DB2 | Oracle |
|---|---|---|
| 默认隔离级别 | Cursor Stability (≈ Read Committed) | Read Committed |
| 读阻塞写 | RC 下 SELECT 可能被阻塞 | **永不**（undo 读一致性） |
| 写阻塞读 | 会阻塞（默认） | **永不** |
| 锁升级 | 自动（行→表） | 自动（行→表，但罕见） |
| 隔离级别数 | CS / RS / RR / UR | RC / Serializable / Read Only |
| 多版本 | MVCC（从 9.7 开始部分支持） | 一直是 MVCC（undo） |

**关键差异：**
- DB2 的 Cursor Stability：只保证当前游标指向的行不变，next row 可能已被修改
- Oracle 的读从不被阻塞的原因是 undo 的读一致性（CR block）
- DB2 生产上经常遇到锁升级导致并发行骤降，需要 `LOCKMAX` 参数控制

---

**Q6: PostgreSQL 的 Autovacuum 和 Oracle 的 SMON/SMONCO 在职责上有什么异同？**

A:

| | PG Autovacuum | Oracle SMON |
|---|---|---|
| 死元组清理 | ✅ 核心职责 | ❌（Oracle 用 undo 不用 vacuum） |
| 空间回收 | 标记可重用 | 合并空闲空间（coalesce） |
| 事务恢复 | ❌ | ✅ 崩溃恢复后清理 |
| 临时段清理 | ❌ | ✅ |
| 触发方式 | 自动（按阈值）/ 手动 | 定期自动 |
| 阻塞业务 | 可能（VACUUM FULL 锁表） | 几乎不 |

**本质区别：**
- PG 的 MVCC 旧版本存数据页，vacuum 是必须的垃圾回收
- Oracle 旧版本存 undo tablespace，SMON 不回收"死行"，undo 过期自动覆盖

---

## 🔹 Redis

**Q7: Redis 的 RDB 和 AOF 持久化，各自的优缺点？生产环境混合方案怎么配？**

A:

| | RDB | AOF |
|---|---|---|
| 原理 | 定时快照整个内存 → dump.rdb | 追加每条写命令 → appendonly.aof |
| 恢复速度 | 快（二进制加载） | 慢（逐条重放命令） |
| 数据丢失 | 最多丢两次快照间的数据 | 最少丢 1 条（everysec） |
| 文件大小 | 小（压缩二进制） | 大（文本命令，持续增长） |
| 对性能影响 | fork 子进程时可能卡顿（大内存） | everysec 时几乎无影响 |

**生产混合方案：**
```bash
# redis.conf
save 900 1              # RDB: 900s 内至少 1 次修改 → 快照
save 300 10
save 60 10000

appendonly yes          # AOF: 开启
appendfsync everysec    #      每秒 fsync

# 恢复时优先用 AOF（数据更完整）
```

**AOF 重写避免无限膨胀：**
```bash
auto-aof-rewrite-percentage 100  # AOF 体积翻倍时触发重写
auto-aof-rewrite-min-size 64mb
```

---

**Q8: Redis Sentinel 和 Cluster 的区别？什么场景选哪个？二者的故障转移流程是怎样的？**

A:

| | Sentinel | Cluster |
|---|---|---|
| 数据分片 | ❌ 不分片（单主多从） | ✅ 自动分片（16384 个 slot） |
| 存储容量 | 单节点内存上限 | 线性扩展（N × 节点内存） |
| 客户端 | 普通客户端即可 | 需要 smart client |
| 故障转移 | Sentinel 自动判断 + 选举 | 节点间 Gossip 协议自治 |
| 复杂度 | 低 | 高 |
| 适用 | 读多写少、数据量 < 单机内存 | 海量数据、高并发写 |

**Sentinel 故障转移流程：**
```
1. Sentinel 定期 PING
2. 主观下线 (SDOWN)：单个 Sentinel 认为 Master 挂了
3. 客观下线 (ODOWN)：≥ quorum 个 Sentinel 都认为挂了
4. 选举 Sentinel Leader（Raft 式投票）
5. Leader 选新 Master（replication offset 最大 + runid 最小的从库）
6. SLAVEOF NO ONE 提升新 Master
7. 其他从库切到新 Master
```

**Cluster 故障转移流程：**
```
1. 节点间 Gossip 协议持续交换状态
2. 一个 Master 被 ≥半数 Master 标记为 PFAIL → FAIL
3. 该 Master 的从库发现 Master FAIL 后等待 cluster-node-timeout
4. 从库发起选举（向其他 Master 拉票）
5. 获得 ≥半数票的从库成为新 Master
6. 接管旧 Master 的 slot
```

---

**Q9: Redis 大 Key 和热 Key 怎么发现和解决？**

A:

**大 Key 发现：**
```bash
# redis-cli --bigkeys（内置，遍历扫描）
redis-cli --bigkeys -i 0.1

# MEMORY USAGE 精确大小（8.0+）
redis-cli MEMORY USAGE keyname

# 更安全：rdb 工具离线分析
rdb -c memory dump.rdb --bytes 10240 > memory_report.csv
```

**大 Key 解决：**
- String：拆分（`key:1, key:2, ...`）或压缩后再存
- Hash/Set/ZSet/List：拆成小 key（每个存 5000 个元素）
- 删除：不要直接 `DEL`（阻塞），用 `UNLINK`（异步删除）或分批删

**热 Key 发现：**
```bash
# 统计热点，找频繁访问的 key
redis-cli --hotkeys

# 客户端主动统计（各语言的 SDK 埋点）
```

**热 Key 解决：**
- 读热 Key：多副本（每个从库一份），客户端随机选副本读
- 写热 Key：业务层限流、合并、降级
- 终极：`key:shard_1, key:shard_2, ...`，前端 hash 分散压力

---

## 🔹 编程语言

**Q10: Java 中 `synchronized` 和 `ReentrantLock` 的区别？AQS 的核心原理是什么？**

A:

| | synchronized | ReentrantLock |
|---|---|---|
| 实现 | JVM 层面（monitorenter/exit） | JDK 层面（AQS + CAS） |
| 可中断 | ❌ | ✅ `lockInterruptibly()` |
| 超时获取 | ❌ | ✅ `tryLock(timeout)` |
| 公平锁 | ❌ 非公平 | ✅ 可选公平/非公平 |
| 条件变量 | 1 个（wait/notify） | 多个 Condition |
| 自动释放 | ✅（退出代码块自动） | ❌ 必须 `unlock()`（finally 里） |

**AQS (AbstractQueuedSynchronizer) 核心原理：**
```
┌─────────────────────────────┐
│          AQS                 │
│  state (volatile int)        │  ← CAS 操作改状态
│  ┌───────────────────────┐  │
│  │   CLH 队列（双向链表）  │  │
│  │   Head ← Node ← Node  │  │  ← 排队等待的线程
│  │   (持有锁) (等待)      │  │
│  └───────────────────────┘  │
└─────────────────────────────┘

获取锁: CAS 尝试改 state 0→1
       成功 → 拿锁走人
       失败 → 构造 Node 放入 CLH 队列尾部，park 挂起

释放锁: state 1→0，unpark 唤醒队列头部等待线程
```

**锁升级机制（synchronized 优化）：**
```
无锁 → 偏向锁 → 轻量级锁 → 重量级锁
               ↑           ↑
          CAS 自旋     队列等待+park
```

---

**Q11: Go 的 goroutine 和 channel 的底层原理？`select` 多个 case 同时就绪时选哪个？**

A:

**Goroutine 调度模型 (GMP)：**
```
G (Goroutine)   → 用户态轻量线程（2KB 初始栈）
M (Machine)     → OS 线程
P (Processor)   → 逻辑处理器（GOMAXPROCS 个）

┌─── P0 ──────┐ ┌─── P1 ──────┐
│ G1 G2 G3    │ │ G4 G5 G6    │
│ G-Runnable Q│ │ G-Runnable Q│
└─────┬───────┘ └─────┬───────┘
      ↓               ↓
     M0              M1
      ↓               ↓
   OS Thread       OS Thread
```

**核心特点：**
- **用户态调度**：G 切换不走内核态，微秒级
- **Work Stealing**：空闲 P 会从其他 P 的队列偷 G
- **阻塞处理**：G 阻塞时（如 channel recv），M 会自动去其他 P 取 G 跑

**Channel 底层结构：**
```go
type hchan struct {
    buf      unsafe.Pointer  // 环形缓冲区
    sendx    uint            // 发送索引
    recvx    uint            // 接收索引
    sendq    *waitq          // 等待发送的 goroutine 队列
    recvq    *waitq          // 等待接收的 goroutine 队列
    lock     mutex
}
```

**select 多 case 同时就绪：**
> **伪随机均匀选择**。Go runtime 打乱 case 的顺序，伪随机选一个可执行的 case，保证公平性（不饿死任何一个 case）。**不是按代码书写顺序选。**

---

**Q12: Python GIL 是什么？什么时候是真正的瓶颈？3.12/3.13 有什么改善？**

A:

**GIL (Global Interpreter Lock)：** CPython 解释器层面的互斥锁，保证同一时刻只有一个线程执行 Python 字节码。

**什么时候 GIL 是真正瓶颈：**
- ✅ **CPU 密集型**多线程：比如纯 Python 的数学计算、JSON 解析、numpy 外的矩阵运算
- ❌ **IO 密集型**多线程（网络请求、文件读写）：GIL 在 IO 等待时自动释放，多线程有效
- ❌ 用了 C 扩展：`numpy`、`pandas` 等 C 库释放 GIL 后才执行，多线程也有效

**绕过 GIL：**
```python
# 1. CPU 密集用 multiprocessing（每个进程独立 GIL）
from multiprocessing import Pool
with Pool(8) as p:
    results = p.map(heavy_compute, data)

# 2. 用 Cython/C 扩展，在计算段释放 GIL
with nogil:
    # 纯 C 代码

# 3. 用 asyncio（协程，单线程，但不阻塞）
```

**Python 3.13 (PEP 703)：** 实验性支持 `--disable-gil` 编译选项，GIL 可关闭，但还不是默认。

---

## 🔹 运维自动化

**Q13: 用 Ansible 管理 200+ 节点，有哪些组织和性能优化的经验？**

A:

**组织层面：**
```yaml
# inventory/production/
├── hosts.yml        # 环境分组
├── group_vars/      # 组级别变量
│   ├── all.yml
│   ├── db_servers.yml
│   └── web_servers.yml
└── host_vars/       # 节点级变量
    └── node01.yml
```

**性能优化：**
```bash
# 1. 加大并发
ansible-playbook site.yml -f 50  # 默认 5，200 节点至少 50

# 2. 关闭 fact gathering（如果不需要）
gather_facts: no

# 3. 用 Mitogen 插件（大幅加速，减少 SSH 往返）
# pip install mitogen && ansible-playbook -c mitogen_ssh

# 4. 用 async + poll 处理耗时任务
- name: long task
  shell: /tmp/long_running.sh
  async: 3600
  poll: 0      # fire and forget

# 5. 用 free strategy 代替 linear（不需要顺序执行时）
strategy: free
```

**关键细节：**
- Serial 控制：一次只操作 N 台，滚动更新不留白
- Delegate_to：操作只在一台执行（如 MySQL 主库切从库）
- Dynamic Inventory：对接 CMDB/云 API 自动生成清单

---

**Q14: 200+ 节点 MySQL 集群，需要批量给所有从库加一个参数，怎么安全地操作？给出 Ansible Playbook 核心逻辑。**

A:

```yaml
- name: 安全批量修改 MySQL 从库参数
  hosts: mysql_slaves
  serial: 5          # 每次 5 台，5x5=25 台依次，不是一口气
  gather_facts: no
  
  vars:
    param_key: "slave_parallel_workers"
    param_value: "8"
  
  tasks:
    - name: 检查从库延迟
      shell: >
        mysql -e "SELECT Seconds_Behind_Master FROM performance_schema.replication_applier_status"
      register: slave_lag
      
    - name: 延迟过高则跳过
      fail:
        msg: "Slave lag too high, skipping this batch"
      when: slave_lag.stdout | int > 10
      
    - name: 停止 IO 线程
      shell: mysql -e "STOP SLAVE IO_THREAD"
      
    - name: 等待回放完成
      shell: >
        until mysql -e "SHOW SLAVE STATUS\G" | grep -c "Seconds_Behind_Master: 0"; do sleep 1; done
      timeout: 60
      
    - name: 修改参数
      lineinfile:
        path: /etc/my.cnf.d/custom.cnf
        regexp: "^{{ param_key }}"
        line: "{{ param_key }} = {{ param_value }}"
      notify: restart mysql
      
    - name: 确认参数生效
      shell: mysql -e "SHOW VARIABLES LIKE '{{ param_key }}'"
      register: verify_param
      
    - name: 启动复制
      shell: mysql -e "START SLAVE"

  handlers:
    - name: restart mysql
      systemd:
        name: mysql
        state: restarted
```

---

## 🔹 Docker & Kubernetes

**Q15: Dockerfile 中 CMD 和 ENTRYPOINT 如何配合？给出一个生产级 MySQL+初始化脚本的 Dockerfile。**

A:

```dockerfile
FROM mysql:8.0.33

# 自定义配置
COPY my.cnf /etc/mysql/conf.d/

# 初始化脚本（容器第一次启动时执行）
COPY init/*.sql /docker-entrypoint-initdb.d/

# 健康检查
HEALTHCHECK --interval=10s --timeout=5s --retries=3 \
  CMD mysqladmin ping -h localhost || exit 1

# 用 ENTRYPOINT 定主进程，CMD 给默认参数
ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["mysqld"]

# 非 root 运行
USER mysql
```

---

**Q16: K8s StatefulSet 和 Deployment 的区别？什么时候必须用 StatefulSet？StatefulSet 的 Pod 名称、PVC、网络标识有什么特点？**

A:

| | Deployment | StatefulSet |
|---|---|---|
| Pod 标识 | 随机后缀 (`app-xyz123`) | 固定序号 (`app-0, app-1, app-2`) |
| 启动顺序 | 并发 | 顺序（0→1→2） |
| 终止顺序 | 并发 | 倒序（2→1→0） |
| 存储 | 共享 PVC 或无状态 | 每个 Pod 独立 PVC |
| 网络 | 共享 Service IP | 独立 DNS（`pod-0.svc.ns.svc.cluster.local`） |
| 扩缩容 | 随机删 | 从高序号删 |

**必须用 StatefulSet 的场景：**
- **数据库/有状态中间件**：MySQL、Redis Cluster、ZK、Kafka
- **需要固定的 Pod 名**：主从架构中，主 Pod 名不变，从 Pod 按固定号扩容
- **独立持久存储**：每个 Pod 有自己的 PV/PVC

```yaml
# 精简版 StatefulSet + Headless Service
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: mysql
spec:
  serviceName: mysql-headless   # 关联 Headless Service
  replicas: 3
  podManagementPolicy: OrderedReady  # 或 Parallel
  selector:
    matchLabels:
      app: mysql
  template:
    metadata:
      labels:
        app: mysql
    spec:
      containers:
      - name: mysql
        image: mysql:8.0
        volumeMounts:
        - name: data
          mountPath: /var/lib/mysql
  volumeClaimTemplates:         # 每个 Pod 自动创建独立 PVC
  - metadata:
      name: data
    spec:
      accessModes: ["ReadWriteOnce"]
      resources:
        requests:
          storage: 100Gi
---
apiVersion: v1
kind: Service
metadata:
  name: mysql-headless
spec:
  clusterIP: None               # Headless
  selector:
    app: mysql
  ports:
  - port: 3306
```

**Pod 网络标识：**
```
mysql-0.mysql-headless.default.svc.cluster.local
mysql-1.mysql-headless.default.svc.cluster.local
mysql-2.mysql-headless.default.svc.cluster.local
```

---

**Q17: K8s 中一个 Pod 从创建到运行的完整流程？画出核心组件交互。**

A:

```
kubectl apply -f pod.yaml
         │
         ▼
   ┌─────────────┐
   │  API Server  │ ← 认证/鉴权/准入控制(Admission Webhooks)
   └──────┬──────┘
          │ 写入 etcd
          ▼
   ┌─────────────┐
   │     etcd     │ ← Pod 对象持久化，状态 Pending
   └──────┬──────┘
          │ Watch 机制通知
          ▼
   ┌─────────────┐
   │   Scheduler  │ ← 过滤(资源够吗?)+打分(哪个节点最合适?)
   └──────┬──────┘
          │ 绑定 Pod → Node
          ▼
   ┌─────────────┐
   │    Kubelet   │ ← 收到新 Pod
   │   (Node上)   │
   └──────┬──────┘
          │ CRI (Container Runtime Interface)
          ▼
   ┌──────────────┐
   │ containerd /  │ ← 拉镜像 → 创建 Sandbox (pause容器)
   │   CRI-O       │    创建 Init 容器 → 启动主容器
   └──────┬──────┘
          │ CNI (Container Network Interface)
          ▼
   ┌─────────────┐
   │  Calico/     │ ← 分配 IP → 配置路由/网络策略
   │  Flannel     │
   └──────┬──────┘
          │ CSI (Container Storage Interface)
          ▼
   ┌─────────────┐
   │   CSI 驱动   │ ← 挂载 Volume/PVC
   └──────┬──────┘
          ▼
   Pod 状态 → Running
   └── Kubelet 汇报状态 → API Server → etcd
```

---

## 🔹 AI / 大模型

**Q18: Transformer 的自注意力机制 (Self-Attention) 的计算流程？为什么需要 Q、K、V 三个矩阵？**

A:

**计算流程：**
```
输入: X ∈ R^(n×d)   (n 个 token，每个 d 维)

1. 线性投影:
   Q = X·Wq     Query: 我要找什么?
   K = X·Wk     Key:   我有什么?
   V = X·Wv     Value: 我的内容是什么?

2. 注意力分数:
   Score = Q·K^T / √dk     (除以 √dk 防止梯度消失)

3. Softmax 归一化:
   Attention = Softmax(Score)    (每行概率分布)

4. 加权求和:
   Output = Attention · V
```

**为什么 Q/K/V 分开：**
- Q·K^T 算"相关性"：当前 token 和所有 token 匹配
- V 是实际信息：通过 Attention 权重加权聚合所有 token 的信息
- 如果 Q=K=V（单矩阵），搜索和信息源混在一起，表达能力受限
- 类比：查字典 → 你的查询词 (Q) ≠ 词条标题 (K) ≠ 词条内容 (V)

**Multi-Head Attention：** 多套 Q/K/V 投影，不同 head 关注不同模式（语法/语义/位置关系）。

---

**Q19: PyTorch 如何搭建一个自定义的 Transformer 模型？给出核心代码结构。**

A:

```python
import torch
import torch.nn as nn

class MultiHeadAttention(nn.Module):
    def __init__(self, d_model=512, n_heads=8):
        super().__init__()
        self.d_model = d_model
        self.n_heads = n_heads
        self.d_k = d_model // n_heads
        
        self.Wq = nn.Linear(d_model, d_model)
        self.Wk = nn.Linear(d_model, d_model)
        self.Wv = nn.Linear(d_model, d_model)
        self.Wo = nn.Linear(d_model, d_model)
        
    def forward(self, x, mask=None):
        batch, seq_len, _ = x.shape
        
        # 投影 + 切分成多头
        Q = self.Wq(x).view(batch, seq_len, self.n_heads, self.d_k).transpose(1,2)
        K = self.Wk(x).view(batch, seq_len, self.n_heads, self.d_k).transpose(1,2)
        V = self.Wv(x).view(batch, seq_len, self.n_heads, self.d_k).transpose(1,2)
        
        # Scaled Dot-Product Attention
        scores = (Q @ K.transpose(-2, -1)) / (self.d_k ** 0.5)
        if mask is not None:
            scores = scores.masked_fill(mask == 0, -1e9)
        attn = torch.softmax(scores, dim=-1)
        
        # 加权 + 合并多头
        out = (attn @ V).transpose(1, 2).contiguous()
        out = out.view(batch, seq_len, self.d_model)
        return self.Wo(out)

class TransformerBlock(nn.Module):
    def __init__(self, d_model=512, n_heads=8, d_ff=2048, dropout=0.1):
        super().__init__()
        self.attn = MultiHeadAttention(d_model, n_heads)
        self.norm1 = nn.LayerNorm(d_model)
        self.norm2 = nn.LayerNorm(d_model)
        self.ff = nn.Sequential(
            nn.Linear(d_model, d_ff),
            nn.ReLU(),
            nn.Linear(d_ff, d_model),
            nn.Dropout(dropout)
        )
        
    def forward(self, x):
        # Pre-LN 结构（更稳定）
        x = x + self.attn(self.norm1(x))
        x = x + self.ff(self.norm2(x))
        return x

class MyTransformer(nn.Module):
    def __init__(self, vocab_size, d_model=512, n_heads=8, 
                 n_layers=6, d_ff=2048, max_len=512):
        super().__init__()
        self.embed = nn.Embedding(vocab_size, d_model)
        self.pos_embed = nn.Parameter(torch.zeros(1, max_len, d_model))
        self.blocks = nn.ModuleList([
            TransformerBlock(d_model, n_heads, d_ff) 
            for _ in range(n_layers)
        ])
        self.norm = nn.LayerNorm(d_model)
        self.lm_head = nn.Linear(d_model, vocab_size)
        
    def forward(self, x):
        x = self.embed(x) + self.pos_embed[:, :x.size(1), :]
        for block in self.blocks:
            x = block(x)
        return self.lm_head(self.norm(x))
```

---

**Q20: 大模型的推理优化有哪些手段？vLLM 的 PagedAttention 是什么原理？**

A:

**推理优化全景：**

| 方法 | 原理 | 效果 |
|------|------|------|
| **KV Cache** | KV 值缓存，避免重复计算 | 减少重复矩阵运算 |
| **量化 (Quantization)** | INT8/INT4 代替 FP16 | 内存减半，速度 1.5-2x |
| **Flash Attention** | 分块计算，减少 HBM 读写 | 2-4x 加速 |
| **Continuous Batching** | 动态拼接请求，提高 GPU 利用率 | 吞吐量 10x+ |
| **Speculative Decoding** | 小模型先猜，大模型校验 | 延迟 2-3x 降低 |
| **Tensor Parallelism** | 权重按层/头拆分到多 GPU | 支持大模型超单卡内存 |

**vLLM / PagedAttention：**

传统推理的问题是 KV Cache 管理粗糙——每个请求预留最大长度的连续内存，实际只用了一部分，**碎片化严重，显存浪费率 > 60%**。

PagedAttention 把 KV Cache 切成固定大小的 **Block**（类似 OS 的分页）：

```
传统: [Req1 KV Cache ——————— 全预留 ——————— 只用了 30%]
      [Req2 KV Cache ——————— 全预留 ——————— 只用了 20%]
      → 碎片严重

PagedAttention:
  Block Table: [Block_0, Block_2, Block_5]  ← 动态分配
  Block Pool:  [ | B0 | B1 | B2 | B3 | B4 | B5 | ...]  ← 共享池
  → 按需分配，无碎片，利用率 ~100%
```

**效果：** 同样的 GPU 显存，vLLM 可以服务 2-4x 的并发请求，吞吐量提升 24x。

---

📝 全 20 题，覆盖七大技术领域，既有八股也有实战。大宝贝满意吗？
