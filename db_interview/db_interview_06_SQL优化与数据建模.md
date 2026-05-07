# 数据库面试 Q&A — 第六轮：SQL 优化实战、数据建模、范式与反范式

---

## Q51: 慢查询排查和优化流程？

**第一步——定位：**
```sql
SHOW FULL PROCESSLIST;                          -- MySQL
SELECT * FROM pg_stat_activity WHERE state='active';  -- PG
```

**第二步——分析执行计划：**
```sql
EXPLAIN SELECT ...;
EXPLAIN (ANALYZE, BUFFERS) SELECT ...;  -- PG
```

**第三步——排查：**
1. type=ALL → 缺索引或索引失效
2. rows 特别大 → 统计信息过时
3. Extra=Using filesort/temporary → 排序或 GROUP BY 没走索引
4. 大表驱动大表 → 驱动表选反

**第四步——常用优化：**
```sql
-- 覆盖索引
CREATE INDEX idx_covering ON t(a, b, c);

-- 大分页：延迟关联
SELECT * FROM t INNER JOIN
(SELECT id FROM t ORDER BY create_time LIMIT 1000000, 20) AS tmp USING(id);

-- IN 拆分或改用 EXISTS
SELECT * FROM t WHERE EXISTS (SELECT 1 FROM s WHERE s.t_id = t.id AND s.val > 100);

-- COUNT 优化：COUNT(*) 选最短二级索引
```

---

## Q52: COUNT(*)、COUNT(1)、COUNT(column) 有什么区别？

| 写法 | 行为 |
|------|------|
| COUNT(*) | 统计所有行（包含 NULL 行） |
| COUNT(1) | 与 COUNT(*) 完全等价 |
| COUNT(column) | 统计非 NULL 值的行数 |

性能：COUNT(*) = COUNT(1)，优化器选最短二级索引全扫。MyISAM 中 COUNT(*) 是 O(1)。

大表优化：汇总表 / Redis 计数器 / EXPLAIN 估算行数。

---

## Q53: JOIN 驱动表是什么？选错怎么办？

驱动表是 Nested Loop Join 中先扫描的外层表，选结果集小的做驱动表。

```sql
-- MySQL：STRAIGHT_JOIN 强制驱动表
SELECT STRAIGHT_JOIN ... FROM users u JOIN orders o ON u.id = o.user_id;

-- 或用 hint（MySQL 8.0+）
SELECT /*+ JOIN_ORDER(u, o) */ ...
```

PG：`SET enable_nestloop = off` 控制。Oracle：`/*+ ORDERED */` hint 或 `LEADING()`。

---

## Q54: Batched Key Access (BKA) 是什么？

传统 NLJ：驱动表每次一行 → 被驱动表查一次（随机 IO）

BKA：驱动表批量取多行 → 一起查被驱动表（批量 + MRR）

需要 JOIN Buffer 配合，适合大表 JOIN。

---

## Q55: EXPLAIN type 从最优到最差排列

```
system > const > eq_ref > ref > fulltext > ref_or_null > index_merge
> unique_subquery > index_subquery > range > index > ALL
```

| type | 示例 |
|------|------|
| system | 表只有一行 |
| const | WHERE id=1（主键等值） |
| eq_ref | JOIN 唯一索引匹配 |
| ref | 非唯一索引等值 |
| range | WHERE id>1 AND id<100 |
| index | 全索引扫描 |
| ALL | 全表扫描（最差） |

---

## Q56: 三范式分别是什么？

| 范式 | 规则 | 反例 |
|------|------|------|
| 1NF | 列不可再分（原子性） | 一个字段存 `tag1,tag2,tag3` |
| 2NF | 消除部分依赖 | 复合主键，部分列只依赖主键的一部分 |
| 3NF | 消除传递依赖 | 非主键列依赖其他非主键列 |

```
2NF 反例：order_detail(order_id, product_id, order_date, product_name)
→ order_date 只依赖 order_id → 拆出 order 表
→ product_name 只依赖 product_id → 拆出 product 表

3NF 反例：user(id, city_id, city_name)
→ city_name 依赖 city_id → 拆出 city 表
```

---

## Q57: 什么时候该反范式化？

1. **高频 JOIN → 冗余字段**：订单表冗余 user_name
2. **聚合计算开销大 → 汇总表/物化视图**
3. **历史快照**：订单冗余当时的 product_name、price

代价：一致性问题、存储增加、写操作变复杂。

---

## Q58: 什么是物化视图？三个数据库如何支持？

| 数据库 | 支持 | 刷新 |
|--------|------|------|
| MySQL | 无原生支持，触发器+表模拟 | 实时（触发器方式） |
| PostgreSQL | `CREATE MATERIALIZED VIEW` | `REFRESH MATERIALIZED VIEW`（全量） |
| Oracle | 最强支持 | `REFRESH ON COMMIT`（增量）/ FAST |

```sql
-- Oracle（最强）
CREATE MATERIALIZED VIEW mv_order_summary
REFRESH FAST ON COMMIT  -- 提交时增量刷新
AS SELECT user_id, COUNT(*), SUM(amount) FROM orders GROUP BY user_id;
```

---

## Q59: CHAR 和 VARCHAR 怎么选？INT 怎么选？

| 类型 | 特点 | 适用 |
|------|------|------|
| CHAR(N) | 定长，读取快 | 固定长度：哈希、UUID、手机号 |
| VARCHAR(N) | 变长，省空间 | 姓名、地址、备注 |

| INT 类型 | 范围 | 字节 |
|----------|------|------|
| TINYINT | 0~255 | 1 |
| SMALLINT | 0~65535 | 2 |
| MEDIUMINT | 0~1677万 | 3 |
| INT | 0~42亿 | 4 |
| BIGINT | 0~2^64-1 | 8 |

原则：够用就好。主键 INT→BIGINT 改动痛苦（锁表重建）。

---

## Q60: 为什么阿里巴巴规范说不要用外键？

1. 写性能损耗（每次 INSERT/UPDATE 子表都检查父表）
2. 死锁风险（父子表插入顺序）
3. 分库分表不兼容（跨库无法维护）
4. 在线 DDL 阻塞
5. 运维复杂（恢复、切换不一致）

替代：应用层保证 + 定期对账。Oracle/PG 通常保留外键（实现更高效），但分库分表同样不适用。
