# 数据库面试 Q&A — 第三轮：锁机制、死锁、并发控制

---

## Q21: MySQL InnoDB 有哪些锁？分别什么粒度？

按粒度分：

| 锁类型 | 粒度 | 说明 |
|--------|------|------|
| 表锁（Table Lock） | 表 | `LOCK TABLES t READ/WRITE` |
| 行锁（Row Lock） | 行 | 对索引记录操作时加锁 |
| 间隙锁（Gap Lock） | 索引间隙 | 锁定索引记录之间的间隙，防幻读 |
| Next-Key Lock | 行+间隙 | Row Lock + Gap Lock，RR 级别默认 |

按模式分：共享锁(S) / 排他锁(X) / 意向锁(IS/IX)

Oracle 和 PG 没有 Gap Lock：Oracle 通过 undo 实现读一致性，写不阻塞读。PG 行锁信息存在 tuple 的 xmax 字段中。

---

## Q22: 什么是死锁？如何避免？三个数据库分别如何处理？

死锁 = 事务互相等待对方持有的锁。

**避免策略：**
1. 按相同顺序访问资源（最重要）
2. 尽量缩短事务
3. 减少锁定范围
4. 降低隔离级别

| 数据库 | 检测方式 | 处理 |
|--------|---------|------|
| MySQL | `innodb_deadlock_detect=ON` 检测等待图环路 | 回滚 undo log 最少的事务，报错 1213 |
| PostgreSQL | 死锁检测器定期检查 | 回滚其中一个，报 `deadlock detected` |
| Oracle | 自动检测 | 回滚导致死锁的最后一条语句（非整个事务），报 ORA-00060 |

查看：MySQL `SHOW ENGINE INNODB STATUS`，PG `pg_stat_activity`，Oracle alert log。

---

## Q23: 什么是乐观锁和悲观锁？

| | 悲观锁 | 乐观锁 |
|------|--------|--------|
| 思路 | 冲突概率高，先锁再操作 | 冲突概率低，操作时检查版本 |
| 实现 | `SELECT ... FOR UPDATE` | 版本号/时间戳 CAS |
| 适用 | 冲突多、短事务 | 冲突少、读多写少 |

```sql
-- 悲观锁
SELECT balance FROM account WHERE id = 1 FOR UPDATE;
UPDATE account SET balance = balance - 100 WHERE id = 1;

-- 乐观锁
UPDATE account SET balance = balance - 100, version = version + 1
WHERE id = 1 AND version = 5;  -- affected_rows=0 → 重试
```

---

## Q24: MySQL 的 MDL 锁是什么？

MDL（Metadata Lock）保护表结构不被并发修改：
- DML 加 MDL 读锁
- DDL 加 MDL 写锁

潜在风险：长事务持有 MDL 读锁 → DDL 阻塞 → DDL 阻塞后续 DML → 连接池打满。

排查：`performance_schema.metadata_locks` / `SHOW PROCESSLIST` 看 "Waiting for table metadata lock"。

Oracle 用 DDL_LOCK 和 Library Cache Lock。PG DDL 通过 AccessExclusiveLock。

---

## Q25: FOR UPDATE 和 FOR SHARE 的区别？

| | FOR SHARE | FOR UPDATE |
|------|------|------|
| 锁类型 | S 锁 | X 锁 |
| 兼容性 | 多事务可同时持有 | 只能一个 |
| 场景 | 读依赖、不修改数据 | 读后要更新 |

---

## Q26: 一条 UPDATE 语句加了哪些锁？

```sql
UPDATE t SET age = 30 WHERE age = 25;  -- idx_age(age), RR级别
```

1. 二级索引 idx_age：对每条符合 age=25 记录加 Next-Key Lock
2. 回表到聚簇索引：加 X Record Lock
3. age 列从 25 改 30：旧值位置标记删除，新值位置插入，产生额外锁

RC 级别：不加 Gap Lock，只加 Record Lock，并发更好。

Oracle/PG：无此复杂间隙锁机制。

---

## Q27: MVCC 和锁的关系是什么？

- MVCC 解决"读-写"冲突（快照读看到历史版本）
- 锁解决"写-写"冲突（串行化写操作）
- 两者配合，MVCC 负责读一致性，锁负责写冲突

```sql
-- 两个事务同时更新同一行，MVCC 不够用，必须靠锁
-- 事务A 看到 version=10，update 成 11
-- 事务B 也看到 version=10，update 成 11  ← 锁阻塞
```

---

## Q28: 什么是两阶段锁协议（2PL）？

1. **增长阶段**：事务执行中不断加锁
2. **缩减阶段**：提交或回滚时一次性释放所有锁

InnoDB 严格遵守 2PL：事务中获取的锁直到 COMMIT/ROLLBACK 才释放。Oracle 和 PG 同样遵循。

---

## Q29: PostgreSQL 的 SSI（Serializable Snapshot Isolation）怎么实现？

PG 9.1+ 基于 SSI，不阻塞读：
1. 跟踪事务间依赖关系
2. 检测串行化冲突（Serialization Anomaly）
3. 无法被序列化 → 回滚其中一个

优势：不阻塞读。代价：可能被回滚，需要重试逻辑。

MySQL SERIALIZABLE 则是用 Next-Key Lock 把读也变成锁定读。

---

## Q30: Oracle UNDO 表空间 vs Redo Log？

| | Redo Log | Undo |
|------|------|------|
| 作用 | 持久性（崩溃恢复） | 回滚 + 读一致性 |
| 记录内容 | "做了什么修改" | "修改前的数据" |
| 写了之后 | 数据页后写 | 先写 undo 再改数据 |

MySQL InnoDB：Redo Log（`ib_logfile`）/ Undo Log（独立 undo 表空间）
Oracle：Redo → archived redo / Undo 表空间
PostgreSQL：只有 WAL，无独立 undo，旧版本在数据页 tuple 中，VACUUM 回收
