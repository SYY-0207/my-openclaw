# 数据库面试 Q&A — 第一轮：SQL 基础与核心概念

---

## Q1: WHERE 和 HAVING 的区别？

- `WHERE` 在**聚合之前**过滤行数据，不能使用聚合函数
- `HAVING` 在**聚合之后**过滤分组数据，必须配合 `GROUP BY` 使用，可以使用聚合函数

执行顺序：`FROM → WHERE → GROUP BY → HAVING → SELECT → ORDER BY`

```sql
SELECT dept_id, AVG(salary)
FROM employees
WHERE hire_date > '2020-01-01'   -- 先过滤行
GROUP BY dept_id
HAVING AVG(salary) > 10000;       -- 再过滤分组
```

---

## Q2: 四种 JOIN 的区别？

| JOIN 类型 | 结果 |
|-----------|------|
| INNER JOIN | 两表匹配的行 |
| LEFT JOIN | 左表全部 + 右表匹配（无匹配填 NULL） |
| RIGHT JOIN | 右表全部 + 左表匹配（无匹配填 NULL） |
| FULL OUTER JOIN | 两表全部（无匹配填 NULL） |

注意：MySQL 不直接支持 `FULL OUTER JOIN`，需要用 `LEFT JOIN UNION RIGHT JOIN` 模拟。PostgreSQL 和 Oracle 都原生支持。

---

## Q3: UNION 和 UNION ALL 的区别？

- `UNION` 会去重（内部做排序+比较），`UNION ALL` 不去重
- `UNION ALL` 更快，省去了去重的排序开销
- 如果能确定结果不会重复，用 `UNION ALL`
- 三个数据库行为一致

---

## Q4: 窗口函数 ROW_NUMBER、RANK、DENSE_RANK 的区别？

假设某部门薪资：20000, 15000, 15000, 10000

| 值 | ROW_NUMBER | RANK | DENSE_RANK |
|----|-----------|------|------------|
| 20000 | 1 | 1 | 1 |
| 15000 | 2 | 2 | 2 |
| 15000 | 3 | 2 | 2 |
| 10000 | 4 | 4 | 3 |

三个数据库都支持（MySQL 8.0+ 才支持，5.7 不支持）。

---

## Q5: 什么是相关子查询（Correlated Subquery）？

相关子查询引用外层查询的列，外层每一行都要执行一次内层查询，性能较差。通常可以用 JOIN 或窗口函数重写。

```sql
-- 相关子查询（差）
SELECT name, salary
FROM employees e1
WHERE salary > (SELECT AVG(salary) FROM employees e2 WHERE e2.dept_id = e1.dept_id);

-- 窗口函数重写（好）
SELECT name, salary FROM (
    SELECT name, salary, AVG(salary) OVER (PARTITION BY dept_id) AS avg_sal
    FROM employees
) t WHERE salary > avg_sal;
```

---

## Q6: 事务 ACID 和隔离级别？

**ACID：**
- **A**tomicity（原子性）：事务要么全做，要么全不做
- **C**onsistency（一致性）：事务前后数据满足约束
- **I**solation（隔离性）：并发事务互不干扰
- **D**urability（持久性）：已提交事务不丢失

**四种隔离级别：**

| 级别 | 脏读 | 不可重复读 | 幻读 |
|------|------|-----------|------|
| READ UNCOMMITTED | ✓ | ✓ | ✓ |
| READ COMMITTED | ✗ | ✓ | ✓ |
| REPEATABLE READ | ✗ | ✗ | ✓ |
| SERIALIZABLE | ✗ | ✗ | ✗ |

关键差异：
- **MySQL InnoDB** 默认 RR，通过 Next-Key Lock 解决了幻读
- **PostgreSQL** 默认 RC，SERIALIZABLE 通过 SSI 实现
- **Oracle** 默认 RC，不提供 READ UNCOMMITTED

---

## Q7: MySQL InnoDB 的 MVCC 是怎么实现的？

1. **隐藏列**：每行有 `trx_id`（最近修改事务ID）和 `roll_pointer`（undo log 指针）
2. **Undo Log**：旧版本存 undo log，roll_pointer 形成版本链
3. **ReadView**：快照读时判断版本可见性

RC vs RR：
- RC：每次 SELECT 生成新 ReadView
- RR：事务中第一次 SELECT 生成 ReadView，后续复用

**PostgreSQL 差异：** 旧版本存在数据页 tuple 中（非独立 undo），通过 xmin/xmax 标记可见性，VACUUM 清理死元组。

**Oracle 差异：** undo 存储在 undo 表空间，通过 SCN 判断可见性。读操作永不被写阻塞。

---

## Q8: 索引底层为什么用 B+Tree 而不是 B-Tree 或 Hash？

**B+Tree vs B-Tree：**
- B+Tree 数据只存叶子节点 → 扇出更大，树更矮
- 叶子节点链表连接 → 范围查询极快

**B+Tree vs Hash：**
- Hash 不支持范围查询、排序、LIKE 前缀匹配

**Oracle：** 默认也是 B*Tree，还支持 Bitmap 索引、Function-Based Index。

---

## Q9: 什么是覆盖索引（Covering Index）？

覆盖索引 = 查询需要的所有列都在索引中，不需要回表。

```sql
-- 索引 idx_dept_salary(dept_id, salary, name)
SELECT dept_id, salary, name FROM employees WHERE dept_id = 10;  -- Using index
SELECT dept_id, salary, name, hire_date FROM ...;                 -- 需要回表
```

**EXPLAIN 判断：**
- MySQL：`Extra` 列显示 `Using index`
- PostgreSQL：`Index Only Scan`
- Oracle：`TABLE ACCESS BY INDEX ROWID` 说明回表了

---

## Q10: InnoDB 聚簇索引 vs 二级索引？

**聚簇索引（主键索引）：**
- 叶子节点存储完整行数据
- 数据按主键顺序物理存储
- 没有显式主键时 InnoDB 自动生成 6 字节 row_id

**二级索引：**
- 叶子节点存储索引列 + 主键值
- 需要**回表**（两次查找）

**Oracle：** 默认堆表，无聚簇索引概念，ROWID 直接定位。IOT（Index-Organized Table）类似聚簇索引。

**PostgreSQL：** 堆表，所有索引都是二级索引，叶子存 CTID（页号+行偏移）。
