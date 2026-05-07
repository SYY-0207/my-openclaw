# 数据库面试 Q&A — 第二轮：索引优化、执行计划、慢查询

---

## Q11: 什么是索引的最左前缀原则？

复合索引 `(a, b, c)` 相当于创建了三个索引：`(a)`、`(a,b)`、`(a,b,c)`。只有查询条件从最左列开始且连续，才能用到索引。

```sql
-- 索引 idx(a, b, c)
WHERE a = 1                        -- ✅
WHERE a = 1 AND b = 2              -- ✅
WHERE a = 1 AND b = 2 AND c = 3    -- ✅
WHERE a = 1 AND c = 3              -- ⚠️ 只用a列
WHERE b = 2                        -- ❌ 完全用不到
WHERE a = 1 AND b > 2 AND c = 3    -- ⚠️ a,b用到，c断了
```

MySQL / PostgreSQL / Oracle 都遵循此原则。

---

## Q12: 什么是索引下推（ICP）？

MySQL 5.6+ 将 WHERE 过滤条件下推到存储引擎层，在索引层面过滤，减少回表次数。

```sql
-- 索引 idx(name, age)
SELECT * FROM users WHERE name LIKE '张%' AND age = 25;
-- 引擎直接在索引中检查 age=25 → 符合条件的才回表
```

判断：EXPLAIN Extra 列 `Using index condition`。

---

## Q13: 什么是索引跳跃扫描（Index Skip Scan）？

跳过复合索引前导列时，优化器自动枚举前导列的 distinct 值。

```sql
-- 索引 idx(gender, age)
SELECT * FROM users WHERE age = 25;  -- 优化器拆分 gender='男' AND age=25 UNION ALL gender='女' AND age=25
```

- Oracle 9i+：原生支持 INDEX SKIP SCAN
- MySQL 8.0.13+：支持 INDEX SKIP SCAN
- PostgreSQL：不原生支持

---

## Q14: 什么情况下索引会失效？

1. **对索引列使用函数或计算**：
   ```sql
   WHERE YEAR(create_time) = 2024    -- ❌
   WHERE create_time >= '2024-01-01'  -- ✅ 用范围
   WHERE salary + 100 > 5000          -- ❌
   WHERE salary > 5000 - 100          -- ✅
   ```

2. **隐式类型转换**：
   ```sql
   WHERE phone = 13800138000   -- ❌ varchar列给数字
   WHERE phone = '13800138000' -- ✅
   ```

3. **LIKE 前置百分号**：`LIKE '%强'` ❌

4. OR 条件中有非索引列

5. != / NOT IN / NOT EXISTS（大多数情况）

---

## Q15: MySQL EXPLAIN 怎么看？

| 字段 | 含义 |
|------|------|
| **type** | ALL(全表) < index(全索引) < range(范围) < ref < eq_ref < const |
| **key** | 实际使用的索引 |
| **key_len** | 索引使用长度（判断复合索引用了几列） |
| **rows** | 预估扫描行数 |
| **Extra** | Using index(覆盖), Using filesort(文件排序⚠️), Using temporary(临时表⚠️) |

---

## Q16: PostgreSQL EXPLAIN 有什么不同？

PG 格式更丰富：
```sql
EXPLAIN ANALYZE SELECT * FROM users WHERE age > 25;
-- Index Scan using idx_age on users  (cost=0.29..8.35 rows=100 width=36)
--         (actual time=0.025..0.052 rows=98 loops=1)
```

- **cost=0.29..8.35**：启动成本..总成本（单位是磁盘页读取估算值）
- **actual time**：实际执行时间ms
- 支持 `EXPLAIN (ANALYZE, BUFFERS)` 看缓存命中
- Oracle 用 `EXPLAIN PLAN` + `DBMS_XPLAN.DISPLAY`

---

## Q17: MySQL 的 JOIN 算法？

| 算法 | 说明 |
|------|------|
| Nested Loop Join (NLJ) | 外层每行扫内层，适合驱动表小+内层有索引 |
| Block Nested Loop Join (BNL) | 用 Join Buffer 批量读驱动表 |
| Hash Join | MySQL 8.0.18+，大表 JOIN 性能显著提升 |

Oracle 和 PG 很早就有 Hash Join、Merge Join。

---

## Q18: 什么是索引合并（Index Merge）？

一个查询同时使用多个索引：
- Index Merge Intersection（AND 条件交集）
- Index Merge Union（OR 条件并集）

```sql
SELECT * FROM users WHERE name = '张三' OR age = 25;  -- 可能走 Index Merge Union
```

注意：出现索引合并通常说明该建复合索引。Oracle 有类似 BITMAP CONVERSION。

---

## Q19: 统计信息怎么看？

| 数据库 | 收集统计信息 | 查看 |
|--------|-------------|------|
| MySQL | `ANALYZE TABLE t;` | `SHOW INDEX FROM t;` |
| PostgreSQL | `ANALYZE t;` | `pg_stats` |
| Oracle | `DBMS_STATS.GATHER_TABLE_STATS(...)` | `DBA_TAB_STATISTICS` |

过时的统计信息是执行计划变坏的常见原因。

---

## Q20: 慢查询怎么定位？

**MySQL：**
```sql
SET GLOBAL slow_query_log = ON;
SET GLOBAL long_query_time = 1;
SET GLOBAL log_queries_not_using_indexes = ON;
```
用 `mysqldumpslow` 或 `pt-query-digest` 分析。

**PostgreSQL：**
- `log_min_duration_statement = 1s`
- `pg_stat_statements` 扩展

**Oracle：**
- `V$SQLAREA` / `DBA_HIST_SQLSTAT`（AWR）
- SQL Trace + TKPROF
