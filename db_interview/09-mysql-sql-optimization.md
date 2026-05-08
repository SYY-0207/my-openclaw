# MySQL SQL 优化面试题集

> 共 20 题，覆盖索引优化、查询优化、执行计划分析、慢查询处理、SQL 改写等核心面试场景

---

## 1. 一条 SQL 执行很慢，你从哪些维度排查？

**回答要点：**

首先确认「慢」的定义——是偶尔慢还是一直慢。

**偶尔慢：**
- **刷脏页 (flush)**：redo log 写满、内存不足、MySQL 空闲时刷脏页，SQL 被阻塞
- **等锁**：`lock in share mode` / `for update` 等行锁、MDL 锁
- **Buffer Pool 数据未预热**：刚重启后缓存命中率低

**一直慢：**
- **没走索引**：全表扫描，`EXPLAIN` 看 `type=ALL`
- **索引失效**：隐式类型转换、函数操作、LIKE `'%xxx'`、OR 条件等
- **选错索引**：优化器选错，`force index` 或重新收集统计信息
- **回表过多**：普通索引回表聚簇索引，考虑覆盖索引
- **大数据量**：`limit 100000,10` 深分页
- **关联查询笛卡尔积**：缺少 join 条件或关联字段无索引
- **返回数据过多**：一次查询大量字段/记录

**排查工具：**
```sql
-- 查看当前执行的语句
SHOW PROCESSLIST;
SHOW FULL PROCESSLIST;

-- 执行计划
EXPLAIN SELECT ...;
EXPLAIN FORMAT=JSON SELECT ...;  -- 详细成本信息

-- 表结构
SHOW CREATE TABLE t\G
SHOW INDEX FROM t;
```

---

## 2. EXPLAIN 各字段含义及如何分析？

```
+----+-------------+-------+--------+------+---------+------+------+-------------+
| id | select_type | table |  type  | key  | key_len | ref  | rows | Extra       |
+----+-------------+-------+--------+------+---------+------+------+-------------+
```

| 字段 | 含义 | 关注点 |
|------|------|--------|
| **id** | 执行顺序 | id 相同从上到下；id 越大越先执行 |
| **select_type** | 查询类型 | SIMPLE/PRIMARY/SUBQUERY/DERIVED/UNION |
| **type** ⭐ | 访问方式 | system > const > eq_ref > ref > range > index > ALL |
| **possible_keys** | 可能用到的索引 | 候选索引列表 |
| **key** ⭐ | 实际使用的索引 | NULL 表示未使用索引 |
| **key_len** | 索引使用长度 | 越长使用越多的索引列 |
| **ref** | 索引比较的列 | const/字段名/函数 |
| **rows** ⭐ | 预估扫描行数 | 越小越好 |
| **filtered** | 过滤百分比 | 被索引过滤后还需回表过滤的比例 |
| **Extra** ⭐ | 额外信息 | Using index / Using where / Using filesort / Using temporary |

**Extra 关键信息：**
- `Using index`：✅ 覆盖索引，最优
- `Using index condition`：ICP 索引条件下推
- `Using where`：存储引擎层返回后在 Server 层过滤
- `Using filesort`：⚠️ 需要额外排序，考虑优化
- `Using temporary`：⚠️ 使用临时表，GROUP BY/DISTINCT/UNION 常见
- `Using join buffer`：⚠️ join 字段无索引，使用连接缓冲

---

## 3. type 访问类型从优到劣排序

```
system  >  const  >  eq_ref  >  ref  >  fulltext  >  ref_or_null
  >  index_merge  >  unique_subquery  >  index_subquery
  >  range  >  index  >  ALL
```

**实战解释：**
- **system**：表只有一行（系统表）
- **const**：主键或唯一索引等值查询，最多一行
- **eq_ref**：联表时用主键或唯一索引关联，每行只匹配一行
- **ref**：普通索引等值查询，可能多行
- **range**：索引范围扫描 `>` `<` `BETWEEN` `IN`
- **index**：全索引扫描（比全表扫描快，因为索引文件小）
- **ALL**：❌ 全表扫描，必须优化

**面试加分项：** 至少优化到 range 级别，最好 ref 及以上。

---

## 4. 哪些情况会导致索引失效？

```
口诀：模糊查询 % 开头，类型转换函数走
      OR 条件一边无，复合索引不带头
     IS NULL 不走索引，不等于 (!= / <>) 也发愁
```

**10 大索引失效场景：**

```sql
-- 1. LIKE 左模糊
SELECT * FROM t WHERE name LIKE '%abc';  -- ❌
SELECT * FROM t WHERE name LIKE 'abc%';  -- ✅

-- 2. 隐式类型转换
SELECT * FROM t WHERE phone = 13800138000;  -- ❌ phone 是 varchar
SELECT * FROM t WHERE phone = '13800138000'; -- ✅

-- 3. 对索引列用函数
SELECT * FROM t WHERE DATE(create_time) = '2024-01-01';  -- ❌
SELECT * FROM t WHERE create_time >= '2024-01-01' AND create_time < '2024-01-02'; -- ✅

-- 4. OR 条件一边无索引
SELECT * FROM t WHERE id = 1 OR name = 'test';  -- ❌ name 无索引则全表扫

-- 5. 复合索引不带头（最左前缀）
INDEX idx_a_b_c (a, b, c)
WHERE b = 1 AND c = 2;  -- ❌ 没用到 a
WHERE a = 1 AND c = 2;  -- ✅ 用到 a，但 c 没用上

-- 6. NOT / != / <> 主键外基本不走索引
SELECT * FROM t WHERE status != 1;  -- ❌ 大多数情况不走

-- 7. IS NULL / IS NOT NULL (看情况)
-- InnoDB 下 IS NULL 可能走，IS NOT NULL 可能不走

-- 8. 范围查询右侧列失效
INDEX idx_a_b_c (a, b, c)
WHERE a = 1 AND b > 2 AND c = 3;  -- c 没用上

-- 9. 参与运算
SELECT * FROM t WHERE age + 1 = 20;  -- ❌
SELECT * FROM t WHERE age = 19;       -- ✅

-- 10. 优化器认为全表更快
-- 数据量小 / 回表成本高，优化器拒绝走索引
```

---

## 5. 什么是覆盖索引？为什么能优化查询？

**定义**：查询的字段全部在索引中，不需要回表查聚簇索引。

```sql
INDEX idx_name_age (name, age)

-- 覆盖索引
SELECT name, age FROM t WHERE name = 'Tom';  -- Extra: Using index ✅

-- 非覆盖索引
SELECT name, age, phone FROM t WHERE name = 'Tom';  -- 需要回表取 phone
```

**为什么快：**
1. 索引通常比数据小，扫描更快
2. 减少磁盘 IO（不回表）
3. 减少 Buffer Pool 污染

**实战建议：**
- 高频查询字段建联合索引
- 别建超大宽索引（维护成本）
- 善用 `Extra: Using index` 验证

---

## 6. 什么是索引下推（ICP）？

**Index Condition Pushdown**：MySQL 5.6+ 功能。

把 Server 层的索引过滤条件下推到存储引擎层，减少回表次数。

```sql
INDEX idx_name_age (name, age)

SELECT * FROM t WHERE name LIKE '张%' AND age = 25;
```

**无 ICP（5.5）：**
1. 存储引擎用 name 前缀扫索引 → 拿到所有以「张」开头的主键 id
2. 全部回表取完整行
3. Server 层再过滤 `age = 25`

**有 ICP（5.6+）：**
1. 存储引擎用 name 前缀扫索引
2. **在索引层直接判断 `age = 25`** ← 下推
3. 只回表取满足两个条件的行

减少了大量回表 IO。

---

## 7. 慢查询怎么定位和优化？

**① 开启慢查询日志：**
```sql
SET GLOBAL slow_query_log = ON;
SET GLOBAL long_query_time = 1;  -- 超过1秒记录
SET GLOBAL log_queries_not_using_indexes = ON;  -- 记录未用索引的查询
```

**② 分析工具：**
```bash
# mysqldumpslow - 自带
mysqldumpslow -s t -t 10 /var/lib/mysql/slow.log  # Top 10 按时间

# pt-query-digest - Percona 工具
pt-query-digest slow.log > report.txt
```

**③ 优化流程：**
```
慢查询日志 → pt-query-digest 分析 → 找 Top N 
→ EXPLAIN 看执行计划 → 针对性优化（加索引/SQL重写/表结构调整）
```

**④ 生产环境注意：**
- 用 `pt-query-digest` 而不是直接 tail 日志
- 关注 `Rows_examined`（扫描行数）和 `Rows_sent`（返回行数）比值
- 比值大 = 效率低

---

## 8. LIMIT 深分页怎么优化？

```sql
-- 问题查询
SELECT * FROM orders WHERE status = 1 ORDER BY id LIMIT 1000000, 10;
-- MySQL 需要扫描前 1000010 行然后丢弃前 1000000 行
```

**优化方案：**

```sql
-- 方案1：延迟关联（推荐）
SELECT o.* FROM orders o
INNER JOIN (
    SELECT id FROM orders WHERE status = 1 ORDER BY id LIMIT 1000000, 10
) tmp ON o.id = tmp.id;

-- 方案2：游标分页（记住上次 id）
SELECT * FROM orders WHERE id > 1000000 AND status = 1 ORDER BY id LIMIT 10;
-- 但要求 id 自增连续且有索引

-- 方案3：业务层限制
-- 不允许翻到 100 页以后，用搜索+过滤替代翻页
```

**原理**：延迟关联把「扫描+丢弃」变成了「索引覆盖扫描 + 主键关联 10 行」。

---

## 9. COUNT(*) / COUNT(1) / COUNT(字段) 区别？

| 写法 | 含义 | 性能 |
|------|------|------|
| `COUNT(*)` | 统计行数（不展开字段） | ✅ 最优（有优化） |
| `COUNT(1)` | 同 COUNT(*) | ✅ 等价（MySQL 已优化） |
| `COUNT(id)` | 统计 id 不为 NULL 的行 | ⚠️ 略慢（需判断 NULL） |
| `COUNT(col)` | 统计 col 不为 NULL 的行 | ⚠️ 略慢 |

**真实性能**：MySQL 8.0 中 `COUNT(*)` 和 `COUNT(1)` 完全等价，优化器已做处理。

**MyISAM vs InnoDB**：
- MyISAM 有计数器，`COUNT(*)` O(1)
- InnoDB 需要遍历索引，无计数器

**大表 COUNT 优化：**
```sql
-- 方案1：用 EXPLAIN 估算
EXPLAIN SELECT COUNT(*) FROM orders;  -- 看 rows 列

-- 方案2：查 information_schema（近似值）
SELECT TABLE_ROWS FROM information_schema.TABLES
WHERE TABLE_NAME = 'orders';
-- 注意：这是统计值，误差 10%-50%

-- 方案3：业务缓存（Redis 计数）
-- 适用于对精确度要求不高的场景
```

---

## 10. JOIN 优化有哪些技巧？

**① 小表驱动大表（核心原则）：**
```sql
-- ✅ 好：小表 left join 大表
SELECT * FROM small_table s LEFT JOIN big_table b ON s.id = b.sid;

-- ❌ 差：大表驱动小表
SELECT * FROM big_table b LEFT JOIN small_table s ON b.sid = s.id;
```
优化器会自动判断，但写 SQL 时也应有意识。

**② 关联字段建索引：**
```sql
-- 被驱动表（join 的右边）的关联字段必须有索引
ALTER TABLE orders ADD INDEX idx_uid (user_id);
```

**③ 减少 JOIN 的表数量：**
- 阿里规范：不超过 3 表 JOIN
- 太多表 JOIN 容易膨胀，考虑拆分成多条简单查询

**④ 用 STRAIGHT_JOIN 控制驱动表：**
```sql
-- 强制指定驱动表
SELECT STRAIGHT_JOIN * FROM small s JOIN big b ON s.id = b.sid;
```

**⑤ BNL vs NLJ：**
- Nested Loop Join：被驱动表关联字段有索引 → 走索引
- Block Nested Loop Join：无索引 → 用 join buffer → 性能差

---

## 11. IN 和 EXISTS 怎么选？

**核心原则：外层小用 IN，内层小用 EXISTS。**

```sql
-- 外层小表 → IN
SELECT * FROM orders WHERE user_id IN (SELECT id FROM users WHERE vip = 1);
-- 先执行子查询，结果集小

-- 内层小表 → EXISTS  
SELECT * FROM users u WHERE EXISTS (
    SELECT 1 FROM orders o WHERE o.user_id = u.id AND o.amount > 1000
);
-- 外层 users 逐行去内层 orders 查是否存在
```

**MySQL 8.0 优化**：很多 IN 子查询自动转为 semi-join，性能大幅提升，不再需要手动改写。

**实战建议**：
- MySQL 5.6 及以前：IN 子查询性能坑，推荐 EXISTS 替代
- MySQL 5.7/8.0：优化器已改善，差异不大
- 能用 JOIN 解决的优先用 JOIN

---

## 12. 为什么 SELECT * 不好？

1. **回表/IO 浪费**：不需要的字段也返回，网络传输+内存消耗
2. **无法覆盖索引**：`SELECT *` 必须回表，无法利用覆盖索引优化
3. **表结构变更影响**：加列/删列可能导致业务代码出错
4. **不方便索引优化**：DBA 无法判断哪些字段能建覆盖索引
5. **大字段拖慢**：TEXT/BLOB 等大字段在 `SELECT *` 时全部拉出

```sql
-- ❌
SELECT * FROM users WHERE status = 1;

-- ✅ 精确定义需要的字段
SELECT id, name, email FROM users WHERE status = 1;
```

---

## 13. ORDER BY 如何高效排序？

**两种排序方式：**

| 方式 | 条件 | 性能 | Extra |
|------|------|------|-------|
| **Using index** | 排序列有索引，且覆盖 | ⭐⭐⭐ 最优 | Using index |
| **Using filesort** | 内存/磁盘排序 | ⭐ 差 | Using filesort |

**优化 filesort：**

```sql
-- 联合索引 (a, b, c)
-- ✅ 索引排序
WHERE a = 1 ORDER BY b, c;

-- ❌ filesort（跳过了 b）
WHERE a = 1 ORDER BY c;

-- ❌ filesort（a 是范围）
WHERE a > 1 ORDER BY b, c;

-- ❌ filesort（排序方向不一致，8.0 支持降序索引可解决）
WHERE a = 1 ORDER BY b ASC, c DESC;
```

**sort_buffer_size 调优：**
```sql
SHOW STATUS LIKE 'Sort_merge_passes';
-- 值大 → 增大 sort_buffer_size（默认 256K，可调到 1M-4M）
```

---

## 14. GROUP BY 优化思路？

**执行方式：**
- 有合适索引 → 松散索引扫描（Loose Index Scan）
- 无合适索引 → 临时表 + filesort

```sql
-- 联合索引 (a, b)
-- ✅ 松散索引扫描
SELECT a, COUNT(*) FROM t GROUP BY a;

-- ✅ 紧密索引扫描
SELECT a, b, COUNT(*) FROM t GROUP BY a, b;

-- ❌ 临时表
SELECT b, COUNT(*) FROM t GROUP BY b;  -- 未以 a 开头
```

**优化策略：**
1. 建合适的联合索引：`WHERE 条件列 + GROUP BY 列`
2. 大数据量 GROUP BY 考虑在从库执行
3. 实时要求不高用汇总表 + 定时任务

---

## 15. 大表如何做 DDL（添加字段/索引）？

**问题**：大表加索引会锁表，阻塞业务读写。

| 方案 | 原理 | 适用版本 |
|------|------|---------|
| **ALGORITHM=INPLACE** | 不重建表，原地操作 | 5.6+ |
| **ALGORITHM=INSTANT** | 只改元数据 | 8.0+（加列） |
| **pt-online-schema-change** | 创建新表+触发器同步 | 所有版本 |
| **gh-ost** | 无触发器，binlog 解析 | GitHub 开源 |

**pt-osc 原理：**
```
1. 创建新表 _t_new（目标结构）
2. 在旧表上创建触发器（INSERT/UPDATE/DELETE）
3. 分批拷贝数据（chunk 方式，减少锁等待）
4. 同步完成后原子性 rename
5. 删除旧表和触发器
```

**面试要点：**
- MySQL 5.6 前 DDL 是灾难（COPY 算法全表锁）
- 5.6 加入 Online DDL，但仍有短暂 MDL 锁
- 8.0 支持 INSTANT 加列（秒级）
- 生产环境推荐 `pt-osc` 或 `gh-ost`

---

## 16. 什么是 MRR（Multi-Range Read）优化？

**MRR**：MySQL 5.6+，将随机磁盘 IO 转换为顺序 IO。

**没有 MRR：**
```
回表查询 → 按索引顺序 → 主键随机 → 大量随机 IO
```

**使用 MRR：**
```
回表查询 → 先收集主键 → 排序 → 顺序 IO 批量读取
```

```sql
-- 开启 MRR
SET optimizer_switch = 'mrr=on,mrr_cost_based=off';
```

**适用场景**：普通索引范围扫描后大量回表时效果明显。

---

## 17. UNION 和 UNION ALL 区别？

| 操作 | 去重 | 性能 |
|------|------|------|
| `UNION ALL` | ❌ 不去重 | ⭐⭐⭐ 快 |
| `UNION` | ✅ 去重（额外排序） | ⭐ 慢 |

```sql
-- 如果确定无重复或可接受重复，用 UNION ALL
SELECT id FROM t1
UNION ALL
SELECT id FROM t2;
```

**为什么 UNION 慢？**
`UNION = UNION ALL + DISTINCT`，去重需要额外排序或用临时表。

---

## 18. 分库分表后 SQL 查询有哪些限制？

1. **JOIN 不能跨库**：需要应用层组装
2. **ORDER BY / GROUP BY 跨分片**：需要中间件归并，性能差
3. **分页麻烦**：`LIMIT 100,10` 需从每个分片取 110 条然后归并
4. **COUNT 不准**：需要额外计数表
5. **分布式事务**：需要 XA 或最终一致性方案
6. **自增 ID**：不能用自增主键，需雪花算法等分布式 ID
7. **UNIQUE 约束失效**：唯一索引只能在单个分片内生效

---

## 19. SQL 优化黄金法则

```
1. 能用索引解决的不用全表
2. 能精确查询的不做模糊
3. 能少返回的不多返回
4. 能一次查的不多次查
5. 能覆盖索引的不回表
6. 能小表驱动的不大表驱动
7. 能 WHERE 过滤的不 HAVING 过滤
8. 能 JOIN 的不子查询（视版本）
9. 能用数值比较的不做字符串比较
10. 能批量插入的不逐条插入
```

---

## 20. 给出一个真实 SQL 优化案例

**场景**：订单表 500 万行，用户表 50 万行。

**慢 SQL：**
```sql
SELECT o.*, u.name, u.level
FROM orders o
LEFT JOIN users u ON o.user_id = u.id
WHERE DATE(o.create_time) = '2024-06-01'
  AND o.status IN (1, 2, 3)
ORDER BY o.create_time DESC
LIMIT 20;
```
执行时间：**8.5 秒**

**问题分析（EXPLAIN）：**
1. `DATE(create_time)` 导致索引失效，type=ALL 全表扫描
2. `orders.user_id` 无索引，join type=ALL
3. filesort 排序 500 万行
4. `SELECT *` 拉出全部字段

**优化步骤：**

```sql
-- Step 1: 改 DATE 函数为范围查询
WHERE o.create_time >= '2024-06-01 00:00:00'
  AND o.create_time < '2024-06-02 00:00:00'

-- Step 2: 建联合索引
ALTER TABLE orders ADD INDEX idx_ctime_status (create_time, status);

-- Step 3: users.id 是主键已有索引，确认 user_id 有索引
ALTER TABLE orders ADD INDEX idx_uid (user_id);  -- 如果没有

-- Step 4: 只查必要字段
SELECT o.id, o.order_no, o.amount, o.create_time, u.name, u.level
FROM orders o
LEFT JOIN users u ON o.user_id = u.id
WHERE o.create_time >= '2024-06-01 00:00:00'
  AND o.create_time < '2024-06-02 00:00:00'
  AND o.status IN (1, 2, 3)
ORDER BY o.create_time DESC
LIMIT 20;
```

优化后执行时间：**0.02 秒**（提升 425 倍）

---

## 附：常见面试追问

**Q: 什么情况下即使有索引也不走？**
> 优化器估算全表扫描更快时（表小、回表代价高、数据分布不均）

**Q: 如何强制使用/忽略索引？**
> `FORCE INDEX(idx_name)` / `USE INDEX(idx_name)` / `IGNORE INDEX(idx_name)`

**Q: 索引是不是越多越好？**
> 不是。索引有维护成本（INSERT/UPDATE/DELETE 需更新索引），占用磁盘空间，优化器选择成本增加。一般单表索引 < 5-6 个。

**Q: CHAR 和 VARCHAR 如何选择？**
> 定长用 CHAR（如 MD5、手机号），变长用 VARCHAR。CHAR 查询效率高但占空间。

**Q: TRUNCATE vs DELETE 区别？**
> TRUNCATE = DDL，不可回滚，速度快（直接删除数据文件重建）；DELETE = DML，可回滚，逐行删除写 undo log。
