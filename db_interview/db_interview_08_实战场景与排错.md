# 数据库面试 Q&A — 第八轮：实战场景、性能调优、排错案例

---

## Q71: 线上 CPU 打满 100%，怎么排查？

**MySQL：**
```sql
-- 当前执行中的 SQL
SELECT * FROM information_schema.processlist WHERE command != 'Sleep' ORDER BY time DESC;

-- 用 sys 库分析
SELECT * FROM sys.statement_analysis ORDER BY avg_timer_wait DESC LIMIT 10;
SELECT * FROM sys.user_summary_by_statement_type ORDER BY total_latency DESC;

-- 抓单条 SQL
SET profiling = 1;
SHOW PROFILE CPU, BLOCK IO, SWAPS FOR QUERY 12;
```

**PostgreSQL：**
```sql
SELECT pid, query, state, wait_event_type, wait_event
FROM pg_stat_activity WHERE state = 'active';
```

**Oracle：**
```sql
SELECT sql_id, cpu_time, elapsed_time, executions, sql_text
FROM v$sql ORDER BY cpu_time DESC FETCH FIRST 10 ROWS ONLY;
```

关键思路：找执行次数多 + 单次耗时长 + 没走索引的 SQL。

---

## Q72: 主从延迟突然变大，怎么排查？

```sql
SHOW SLAVE STATUS\G
-- Seconds_Behind_Master / Retrieved_Gtid_Set vs Executed_Gtid_Set
SHOW FULL PROCESSLIST;  -- 是否 Waiting for table metadata lock
SELECT * FROM sys.innodb_lock_waits;  -- Slave 锁等待
```

常见原因：
1. 大事务 → 拆小
2. Slave 没有主键 → 加主键（行回放变全表扫）
3. Slave 查询阻塞复制 → 读写分离
4. 单线程跟不上 → 开并行复制

---

## Q73: 连接数打满怎么办？

**紧急：**
```sql
-- 清理睡眠连接
SELECT COUNT(*) FROM information_schema.processlist WHERE command='Sleep';
SET GLOBAL wait_timeout = 600;
```

**治本：**
1. 连接池（HikariCP/Druid）
2. MySQL 端 `max_connections` + 代理层（ProxySQL/MySQL Router）
3. 短查询快速释放
4. 监控使用率 >70%

PG 用 PgBouncer。Oracle 有 DRCP 和共享服务器。

---

## Q74: 如何用 Performance Schema 定位瓶颈？

```sql
-- 确认开启
SHOW VARIABLES LIKE 'performance_schema';

-- 最耗时的 SQL
SELECT * FROM sys.statement_analysis ORDER BY total_latency DESC LIMIT 10;

-- 全表扫描的表
SELECT * FROM sys.schema_tables_with_full_table_scans;

-- 未使用的索引
SELECT * FROM sys.schema_unused_indexes;

-- IO 等待
SELECT * FROM sys.io_global_by_wait_by_latency LIMIT 10;

-- 锁等待
SELECT * FROM sys.innodb_lock_waits;
```

开销约 5-10%，生产环境值得开。

---

## Q75: 数据误删恢复？没有备份怎么办？

**有备份（PITR）：** 全量恢复 + Binlog 回放，跳过误删的 GTID。

**没有备份 — MySQL：**
- Binlog 还在 → `mysqlbinlog` 反向解析
- ROW 格式 Binlog → MyFlash / `binlog2sql` 反向生成回滚 SQL

**Oracle（最强）：**
```sql
SELECT * FROM t AS OF TIMESTAMP (SYSTIMESTAMP - INTERVAL '10' MINUTE);  -- 闪回查询
FLASHBACK TABLE t TO TIMESTAMP ...;                    -- 闪回表
FLASHBACK TABLE t TO BEFORE DROP;                     -- 闪回删除
```

预防：`sql_safe_updates=ON`、DELETE 前先 SELECT 确认。

---

## Q76: 什么是长事务？怎么发现和处理？

危害：锁长期持有、Undo Log 膨胀（阻止 purge）、主从延迟加剧。

**发现：**
```sql
-- MySQL
SELECT trx_id, trx_started,
       TIMESTAMPDIFF(SECOND, trx_started, NOW()) AS duration_sec,
       trx_mysql_thread_id, trx_query
FROM information_schema.innodb_trx ORDER BY trx_started;

-- PostgreSQL
SELECT pid, xact_start,
       EXTRACT(EPOCH FROM (now() - xact_start)) AS duration_sec, query
FROM pg_stat_activity WHERE xact_start IS NOT NULL AND state != 'idle'
ORDER BY xact_start;

-- Oracle
SELECT s.sid, s.serial#, t.start_time, sql.sql_text
FROM v$transaction t JOIN v$session s ON t.ses_addr = s.saddr
LEFT JOIN v$sql sql ON s.sql_id = sql.sql_id;
```

处理：`KILL <thread_id>`、`SET max_execution_time`、监控告警。

---

## Q77: !=, IS NULL, OR 为什么容易全表扫描？

- `!=` / `<>`：排除一个值 ≈ 全表
- `IS NULL`：优化器倾向全表扫描
- `OR` 分散条件 → 不同列不同索引，合并不如全表扫

```sql
-- ❌
WHERE status != 'DONE';
WHERE name IS NULL;
WHERE phone = '138' OR email = 'a@b.com';

-- ✅
WHERE status IN ('NEW', 'PROCESSING', 'SHIPPED');
WHERE name = '';  -- 业务空值用空串
SELECT * FROM t WHERE phone = '138' UNION SELECT * FROM t WHERE email = 'a@b.com';
```

---

## Q78: LIMIT 1000000,20 为什么慢？怎么优化？

MySQL 需先扫描前 1000000 行才拿到 20 行。

```sql
-- 延迟关联（最佳）
SELECT t1.* FROM t t1 INNER JOIN
(SELECT id FROM t ORDER BY create_time LIMIT 1000000, 20) t2 ON t1.id = t2.id;

-- 业务上改用上一页最后 id
SELECT * FROM t WHERE id > 1000000 ORDER BY id LIMIT 20;
```

---

## Q79: INSERT ON DUPLICATE KEY UPDATE 和 REPLACE 区别？

| | ON DUPLICATE KEY UPDATE | REPLACE |
|------|------|------|
| 冲突行为 | UPDATE 指定列 | DELETE + INSERT |
| 自增 ID | 不变 | 变化 |
| 外键级联 | 不影响 | DELETE 触发级联 |
| 性能 | 好（原地更新） | 差（删除+插入） |

```sql
-- 推荐
INSERT INTO stats (date, pv) VALUES ('2024-01-01', 100)
ON DUPLICATE KEY UPDATE pv = pv + 100;
```

PG/Oracle 用 `INSERT ON CONFLICT` / `MERGE`。

---

## Q80: 三个数据库各自杀手锏特性总结

| 领域 | MySQL | PostgreSQL | Oracle |
|------|-------|------------|--------|
| 复制 | GTID + MGR | 逻辑复制 + Streaming | Data Guard + GoldenGate |
| MVCC | Undo Log + ReadView | Tuple 版本链 + xmin/xmax | Undo 表空间 + SCN |
| 索引 | 自适应哈希索引 | GIN/GiST/BRIN | Bitmap / Function-Based |
| 分库分表 | 生态最强 | 原生分区 + FDW | 基本不分（RAC 纵向） |
| 闪回 | 无原生 | 无原生 | ✅ Flashback Query/Table/Drop |
| 物化视图 | 无原生 | ✅ 手动刷新 | ✅ 自动+增量刷新 |
| 并行查询 | 有限（8.0+ 并行回放） | ✅ 并行扫描/聚合/Hash | ✅ Parallel Query |
| GIS | 基础 | ✅ PostGIS（最强） | 支持 |
| 全文搜索 | FULLTEXT | ✅ GIN + tsvector | ✅ Oracle Text |
| JSON | ✅ JSON 类型 | ✅ JSONB（最强） | ✅ JSON 类型 |
| 外键 | 不推荐 | ✅ 推荐 | ✅ 推荐 |
