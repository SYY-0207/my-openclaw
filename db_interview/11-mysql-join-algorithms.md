# MySQL JOIN 算法：SNLJ、INLJ、BNLJ 详解

## 前置准备：演示表和数据

```sql
-- 订单表（大表，3 行演示）
CREATE TABLE orders (
  id INT PRIMARY KEY,
  user_id INT,
  amount DECIMAL(10,2),
  INDEX idx_uid (user_id)     -- user_id 有索引
);
INSERT INTO orders VALUES
(1, 101, 99.00),
(2, 102, 199.00),
(3, 101, 299.00);

-- 用户表（小表，2 行演示）
CREATE TABLE users (
  id INT PRIMARY KEY,
  name VARCHAR(20),
  level INT
  -- 注意：level 没有索引
);
INSERT INTO users VALUES
(101, '张三', 1),
(102, '李四', 2);
```

**以上数据为下文所有 SQL 的基准。**

---

## 一、Simple Nested-Loop Join（简单嵌套循环）

### 原理

```
for each row in 驱动表:
    for each row in 被驱动表:
        if (join条件匹配):
            返回结果
```

**特征**：
- 最原始、最笨的算法
- 笛卡尔积式扫描，复杂度 O(M × N)
- **被驱动表没有任何索引可用时走这个**
- 实际上 MySQL 很少直接走 SNLJ，**通常走的是 BNLJ**（见第三节）

### 如何强制演示（关闭 BNLJ）

```sql
SET SESSION optimizer_switch='block_nested_loop=off';

EXPLAIN SELECT * FROM orders o JOIN users u ON o.user_id = u.id;
```

```
+----+-------------+-------+------+---------------+------+---------+------+------+--------------------------------+
| id | select_type | table | type | possible_keys | key  | key_len | ref  | rows | Extra                          |
+----+-------------+-------+------+---------------+------+---------+------+------+--------------------------------+
|  1 | SIMPLE      | users | ALL  | PRIMARY       | NULL | NULL    | NULL |    2 | NULL                           |
|  1 | SIMPLE      | orders| ALL  | idx_uid       | NULL | NULL    | NULL |    3 | Using where; Using join buffer |
+----+-------------+-------+------+---------------+------+---------+------+------+--------------------------------+
```

> 注意：即使关闭 BNLJ，MySQL 8.0 优化器也可能自动选 hash join（见第四节）。

### 执行过程（手工模拟）

```
外循环（驱动表）users： 2 行
  遍历 users 第 1 行： id=101, 张三
    内循环（被驱动表）orders： 3 行
      比较 orders.user_id=101 → 匹配订单 1 和 3 ✅
  遍历 users 第 2 行： id=102, 李四
    内循环（被驱动表）orders： 3 行
      比较 orders.user_id=102 → 匹配订单 2 ✅

总扫描次数 = 2 × 3 = 6 次
```

### 性能特征

```
扫描行数 = 驱动表行数 × 被驱动表行数
IO次数  = 驱动表行数 × 被驱动表行数（最坏情况）
结论    = ❌ 极差，生产不可接受
```

---

## 二、Index Nested-Loop Join（索引嵌套循环）⭐ 最优

### 原理

```
for each row in 驱动表:
    用 join 条件字段的值 -->
    通过被驱动表的【索引】直接定位匹配行  ← 关键区别
    返回结果
```

**特征**：
- 内层循环不走全表扫描，走**索引查找**
- 复杂度 O(M × log N)
- 被驱动表的 **join 字段必须有索引**

### SQL 演示

```sql
-- orders.user_id 有索引 idx_uid ✅
EXPLAIN SELECT * FROM users u JOIN orders o ON u.id = o.user_id;
```

```
+----+-------------+-------+------+---------------+---------+---------+-----------------+------+-------+
| id | select_type | table | type | possible_keys | key     | key_len | ref             | rows | Extra |
+----+-------------+-------+------+---------------+---------+---------+-----------------+------+-------+
|  1 | SIMPLE      | users | ALL  | PRIMARY       | NULL    | NULL    | NULL            |    2 | NULL  |
|  1 | SIMPLE      | orders| ref  | idx_uid       | idx_uid | 5       | test.u.id       |    1 | NULL  |
+----+-------------+-------+------+---------------+---------+---------+-----------------+------+-------+
```

**关键信息**：
- `orders.type = ref` → 走了索引，每次只定位到匹配行
- `orders.key = idx_uid` → 使用的索引
- `orders.ref = test.u.id` → 用 users.id 的值去索引中查找
- `orders.rows = 1` → 预估每次索引查找只返回 1 行

### 执行过程（手工模拟）

```
外循环（驱动表）users： 2 行
  遍历 users 第 1 行： id=101
    内循环：用 101 查 idx_uid 索引 → 直接定位 orders 第 1 行和第 3 行 ✅
  遍历 users 第 2 行： id=102
    内循环：用 102 查 idx_uid 索引 → 直接定位 orders 第 2 行 ✅

总扫描次数 = 2（外循环）+ 3（索引定位的行）= 5 次 IO
```

### 性能特征

```
扫描行数 = 驱动表行数 + 匹配行数（远小于 M×N）
IO次数  = M + 匹配行数
结论    = ⭐⭐⭐ 最优，生产目标
```

**优化铁律**：被驱动表的 JOIN 字段必须建索引，否则立刻退化到 BNLJ。

---

## 三、Block Nested-Loop Join（块嵌套循环）⚠️ 常见坑

### 原理

```
1. 从驱动表读取一批行 → 放入 join buffer（内存块）
2. 扫描被驱动表的每一行 →
    在 join buffer 中逐行比较（内存操作）
3. 重复直到驱动表读完
```

**特征**：
- 被驱动表**没有可用索引**时 MySQL 自动选用
- 把驱动表分批加载到内存（join buffer），减少被驱动表的扫描次数
- 虽然比 SNLJ 好，但**仍然是全表扫描被驱动表**

### SQL 演示

```sql
-- ⚠️ 用无索引的 level 字段关联
-- users.level 没有索引！
EXPLAIN SELECT * FROM orders o JOIN users u ON o.user_id = u.level;
```

```
+----+-------------+-------+------+---------------+------+---------+------+------+------------------------------------------------+
| id | select_type | table | type | possible_keys | key  | key_len | ref  | rows | Extra                                          |
+----+-------------+-------+------+---------------+------+---------+------+------+------------------------------------------------+
|  1 | SIMPLE      | users | ALL  | NULL          | NULL | NULL    | NULL |    2 | Using where                                    |
|  1 | SIMPLE      | orders| ALL  | idx_uid       | NULL | NULL    | NULL |    3 | Using where; Using join buffer (Block Nested)  |
+----+-------------+-------+------+---------------+------+---------+------+------+------------------------------------------------+
```

**关键信号**：`Extra = Using join buffer (Block Nested Loop)` ← 面试高频考点

**为什么不走 INLJ？**
users.level 没有索引 → 无法用索引快速定位 → 只能全表扫描 orders → 但用 BNLJ 比 SNLJ 少扫驱动表。

### 执行过程（手工模拟）

```
join_buffer_size = 可以放 2 行 users 数据

第 1 轮：
  把 users 全部 2 行放入 join buffer（驱动表小，一次装完）
  扫描 orders 的全表 3 行：
    orders 第 1 行 → 在 buffer 里比较 level → 匹配成功 ✅
    orders 第 2 行 → 在 buffer 里比较 level → 匹配成功 ✅
    orders 第 3 行 → 在 buffer 里比较 level → 匹配成功 ✅

总扫描次数 = 2（读驱动表）+ 3（扫被驱动表）= 5 次
如果不用 BNLJ（即 SNLJ）= 2 × 3 = 6 次
```

虽然有优化，但被驱动表仍然全表扫。

### join_buffer_size 的作用

```sql
SHOW VARIABLES LIKE 'join_buffer_size';  -- 默认 256K

-- 调大
SET SESSION join_buffer_size = 4 * 1024 * 1024;  -- 4MB
```

| buffer 太小 | buffer 够大 |
|------------|------------|
| 驱动表分 N 批 → 被驱动表扫 N 次 | 驱动表一次装入 → 被驱动表扫 1 次 |

**建议**：BNLJ 是救急的，不是追求的。**优化目标永远是让它变成 INLJ**。

---

## 四、三种算法对比总结

```
被驱动表 join 字段有索引吗？
  ├── YES → INLJ（Index Nested-Loop）⭐⭐⭐ 最优
  │     被驱动表每次走索引查找
  │     Extra: 无 "Using join buffer"
  │
  └── NO  → BNLJ（Block Nested-Loop）⚠️ 次优
          驱动表分批装入 join buffer
          被驱动表仍然全表扫（但少扫了驱动表）
          Extra: "Using join buffer (Block Nested Loop)"

特殊情况：SNLJ（Simple Nested-Loop）❌ 几乎不会出现
         仅在 MySQL 没有 join buffer 且被驱动表无索引时
         MySQL 5.6+ 默认开启 BNLJ，不会退化成 SNLJ
```

### 一表对比

| 特性 | SNLJ | INLJ | BNLJ |
|------|------|------|------|
| 内层扫描方式 | 全表 | 索引查找 | 全表 + buffer |
| 被驱动表需要索引 | ❌ | ✅ 必须 | ❌ |
| 复杂度 | O(M×N) | O(M×logN) | O(M + N×batches) |
| Extra 关键词 | Using where | 无特殊标记 | Using join buffer ⚠️ |
| 生产可用 | ❌ | ✅ | ⚠️ 临时方案 |
| MySQL 版本 | 古老 | 全版本 | 5.6+ |

---

## 五、MySQL 8.0：Hash Join 来了

MySQL 8.0.18+ 引入了 **Hash Join**，在等值 JOIN 且被驱动表无索引时，替代 BNLJ。

```sql
-- MySQL 8.0.18+
EXPLAIN FORMAT=TREE
SELECT * FROM orders o JOIN users u ON o.user_id = u.level;
```

```
-> Inner hash join (u.level = o.user_id)  ← Hash Join！
    -> Table scan on u
    -> Hash
        -> Table scan on o
```

**Hash Join 原理**：
1. 把驱动表放入 hash 表（key = join 字段）
2. 扫描被驱动表，对每行做 hash 查找
3. 复杂度 O(M + N)，比 BNLJ 的 O(M × N) 快一个量级

**但即使有 Hash Join，对 DBA 来说原则不变：**
> `Extra: Using join buffer` 出现 → 看被驱动表 join 字段有没有索引 → 没有就加上

---

## 六、面试一句话总结

> **SNLJ** 两个 for 循环硬扫，**INLJ** 外层 for + 内层索引查，**BNLJ** 外层装 buffer 分批 + 内层全表扫。DBA 的目标是让所有 JOIN 都走 INLJ：**被驱动表的 join 字段必须建索引**，看到 `Using join buffer` 就要警觉，这是索引缺失的信号。
