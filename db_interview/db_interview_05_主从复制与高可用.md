# 数据库面试 Q&A — 第五轮：主从复制、高可用、分库分表

---

## Q41: MySQL 主从复制的原理？延迟原因？

**复制流程（基于 Binlog）：**

```
Master: 写Binlog
           ↓ IO 线程拉取
Slave:  IO Thread → Relay Log → SQL Thread 回放
```

**延迟原因：**
1. Slave 单线程回放（5.6 以下最严重）→ 5.7 并行复制（group commit）
2. Master 大事务
3. Slave 机器性能差
4. 网络带宽不足

```sql
SHOW SLAVE STATUS\G  -- Seconds_Behind_Master
```

Oracle：Data Guard / GoldenGate。PostgreSQL：Streaming Replication（WAL 流复制），延迟更低。

---

## Q42: GTID 复制和传统复制有什么区别？

| | 传统复制 | GTID 复制 |
|------|------|------|
| 定位方式 | MASTER_LOG_FILE + MASTER_LOG_POS | 全局唯一事务 ID |
| 主从切换 | 手动计算位点 | 自动定位，MASTER_AUTO_POSITION=1 |
| 故障恢复 | 麻烦，可能重复/丢失 | 自动跳过已执行事务 |

GTID 格式：`server_uuid:transaction_id`

---

## Q43: MySQL 8.0 并行复制怎么实现？

基于 WRITESET：判断事务操作的行是否冲突，不冲突即可并行回放。

```sql
SET GLOBAL binlog_transaction_dependency_tracking = WRITESET;
SET GLOBAL slave_parallel_type = LOGICAL_CLOCK;
SET GLOBAL slave_parallel_workers = 4;
```

---

## Q44: MySQL Group Replication (MGR) 是什么？

MySQL 5.7.17+ 多主复制方案，基于 Paxos 协议。
- 自动选主 + 自动故障切换
- 单主模式（Single-Primary）/ 多主模式（Multi-Primary）
- InnoDB Cluster = MGR + MySQL Router + MySQL Shell

对比：Oracle RAC（多主、共享存储）、PG Patroni+etcd（不原生支持多主）。

---

## Q45: 分库分表怎么做？什么时候分？

**时机：** 单表 > 2000万行 / > 5-10GB / 索引优化无效

| 方式 | 说明 | 示例 |
|------|------|------|
| 垂直拆分 | 按列拆 | 用户主表 + 用户扩展表 |
| 水平拆分 | 按行拆 | 按 user_id 取模 |

中间件：ShardingSphere、Vitess、MyCat/DBLE

分片键原则：80% SQL 能带上、数据均匀分布、有业务意义。

```sql
-- 16 库 × 4 表 = 64 分片
-- 库：user_id % 16，表：(user_id / 16) % 4
```

---

## Q46: 分区表和分库分表有什么区别？

| | 分区表 | 分库分表 |
|------|------|------|
| 层面 | 数据库内部 | 中间件/应用层 |
| 对业务透明 | ✅ | ❌ 需路由 |
| 跨实例 | ❌ | ✅ |
| 扩展性 | 有限 | 理论无限 |

三种数据库都支持分区表（Oracle 最强：范围/列表/哈希/复合/引用/间隔）。

---

## Q47: 数据迁移怎么做到不停机？

**双写方案：**
1. 双写老库 + 新库
2. 后台任务逐批迁移历史数据（按主键分片）
3. 数据核对
4. 灰度切读
5. 停止写老库

工具：gh-ost（基于 Binlog）、Canal/Maxwell（实时同步）
Oracle：RMAN + Data Guard。PG：pg_dump + logical replication。

---

## Q48: Paxos / Raft 在数据库中有哪些应用？

| 场景 | 协议 | 实现 |
|------|------|------|
| MySQL MGR | Paxos | InnoDB Cluster |
| TiDB | Raft | TiKV |
| PG 高可用 | Raft | Patroni + etcd |
| CockroachDB | Raft | 完整实现 |

解决：选主、日志复制、成员变更。

---

## Q49: 什么是读写分离？怎么实现？

写走 Master → 读走 Slave

实现：客户端层（代码分数据源）、中间件层（ProxySQL/ShardingSphere）、驱动层。

最大问题：复制延迟 → 写后立刻读可能读到旧数据
解决：时效性要求高的读走 Master，或强制等待 Slave 同步。

---

## Q50: 数据库备份策略？PITR 怎么实现？

| 类型 | MySQL 工具 |
|------|-----------|
| 逻辑备份 | `mysqldump` |
| 物理备份 | `xtrabackup`、`mysqlbackup`（企业版） |

**PITR = 全量备份 + Binlog 回放到指定时间点：**
```bash
xtrabackup --prepare --target-dir=/backup/full
mysqlbinlog --start-datetime=... --stop-datetime=... mysql-bin.000001 | mysql -u root
```

Oracle：RMAN + 归档日志 + 闪回（独有，不需备份即可回退）。
PG：`pg_basebackup` + WAL 归档 = PITR。
