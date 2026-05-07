# 数据库面试 Q&A — 第七轮：存储引擎深入、日志系统深入、参数调优

---

## Q61: InnoDB 数据页结构是什么样的？

16KB 页结构：

| 区域 | 大小 | 说明 |
|------|------|------|
| File Header | 38 字节 | 页号、页类型、LSN |
| Page Header | 56 字节 | 槽数量、堆记录数、垃圾大小 |
| Infimum + Supremum | 26 字节 | 最小/最大虚拟行（边界） |
| User Records | 动态 | 实际数据行（按主键顺序） |
| Free Space | 动态 | 空闲空间 |
| Page Directory | 动态 | 槽数组，每 4-8 行一个槽，用于二分查找 |
| File Trailer | 8 字节 | 校验和 + LSN |

Page Directory 作用：页内行按主键单向链表连接，Page Directory 是稀疏目录。查找时先二分查 Page Directory，再在槽对应链表段顺序扫描。

PG 默认 8KB 页。Oracle 块大小可配（2/4/8/16/32KB）。

---

## Q62: Double Write Buffer 是什么？

解决**部分页写入（Torn Page）**问题：OS 页 4KB，InnoDB 页 16KB → 刷脏页时 OS 崩溃可能造成半个新页+半个旧页。

过程：
1. 脏页先顺序写入 double write buffer（2MB 连续区域）
2. 再随机写入表空间实际位置
3. 崩溃恢复时检查完整性，损坏则用 double write buffer 副本覆盖

开销约 5-10%。SSD 可关闭（`innodb_doublewrite=off`，8.0.20+）。

Oracle：更大 Redo 格式避免。PG：full_page_writes 实现。

---

## Q63: Insert Buffer 为什么改名为 Change Buffer？

5.1 只支持 INSERT 缓冲，5.5+ 扩展到 INSERT / DELETE MARK / PURGE，所以改名为 Change Buffer。

所有操作只针对**非唯一二级索引**（唯一索引需要读盘验证唯一性）。

---

## Q64: LSN（Log Sequence Number）是什么？

LSN 是单调递增的逻辑序列号，标识 Redo Log 位置。

```
Log sequence number     29200344567  ← 当前写入位置
Log flushed up to       29200344567  ← 已刷到磁盘
Pages flushed up to     29200344567  ← 脏页已刷
Last checkpoint at      29200344499  ← 最近检查点
```

崩溃恢复：从 Last checkpoint 回放到 Log flushed up to。

PG 也用 LSN 概念（WAL 位置）。Oracle 用 SCN（System Change Number）。

---

## Q65: `innodb_flush_log_at_trx_commit` 三个值的区别？

| 值 | 策略 | 持久性 | 性能 |
|----|------|--------|------|
| 0 | 每秒刷一次 | 丢 1 秒数据 | 最快 |
| 1 | 每次 COMMIT 刷盘 | 不丢数据 | 最慢 |
| 2 | COMMIT 时写 OS cache | MySQL 崩溃不丢，OS 崩溃丢 1 秒 | 中等 |

金融/支付：1。日志/点击：2。

PG `synchronous_commit` 可设为 off/remote_write。Oracle `COMMIT_WRITE` 参数。

---

## Q66: 什么是 CDC（Change Data Capture）？有哪些方案？

| 方案 | 原理 |
|------|------|
| 基于查询 | 轮询时间戳列 |
| 基于触发器 | 触发器写变更日志表 |
| 基于日志解析 | 解析 Redo/Binlog/WAL（主流） |

工具：Canal/Maxwell（MySQL Binlog 解析）、Debezium（Kafka Connect，多数据库）、PG 内置逻辑复制、Oracle GoldenGate。

```sql
-- PG 逻辑复制
CREATE PUBLICATION mypub FOR TABLE users, orders;
CREATE SUBSCRIPTION mysub CONNECTION 'host=...' PUBLICATION mypub;
```

---

## Q67: DOUBLE 精度问题？DECIMAL 怎么存？

DOUBLE 用 IEEE 754，无法精确表示某些十进制小数：`0.1+0.2≠0.3`。

DECIMAL 以二进制形式存储精确十进制整数。`DECIMAL(18,4)` 约 9 字节。

金额存储：DECIMAL 或 BIGINT 存分（100.50→10050，性能最好）。

---

## Q68: 自增 ID 空洞问题？在线 DDL 怎么做安全？

空洞原因：INSERT 失败、DELETE 不回收、主从切换偏移。MySQL 8.0 修复重启丢失自增值问题。

| 操作 | 风险 | 安全工具 |
|------|------|---------|
| 加字段 | 低（8.0+ INSTANT） | ALTER ... ALGORITHM=INSTANT |
| 加索引 | 中（INPLACE 不锁表） | 直接执行或 gh-ost |
| 改字段类型 | 高（COPY 锁表） | gh-ost / pt-osc |
| 修改主键 | 极高 | 停机！ |

gh-ost 原理：影子表 → 分批拷贝 → Binlog 增量 → 原子 rename。

PG 在线 DDL 更强（ALTER TABLE 基本不锁表）。Oracle 类似。

---

## Q69: Thread Pool 和 one-thread-per-connection 区别？

| | One-Thread-Per-Connection | Thread Pool |
|------|------|------|
| 模型 | 每连接一个线程 | 固定线程数 |
| 适用 | 连接数 < 200 | 连接数 > 500 |

企业版 Plugin / MariaDB / Percona Server 支持。

PG 默认一进程一连接，高并发靠 PgBouncer。Oracle 支持共享服务器模式。

---

## Q70: MySQL 直方图（Histogram）对优化有什么帮助？

MySQL 8.0+ 支持，解决数据分布不均匀时优化器预估不准问题。

```sql
ANALYZE TABLE t UPDATE HISTOGRAM ON status, age;
SELECT * FROM information_schema.column_statistics;
ANALYZE TABLE t DROP HISTOGRAM ON status;
```

PG 一直有直方图（`pg_stats.histogram_bounds`）。Oracle 支持 Height-Balanced 和 Hybrid 直方图。
