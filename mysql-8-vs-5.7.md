# MySQL 5.6 → 5.7 → 8.0 版本演进与核心差异

---

## 一、版本演进时间线

| 版本 | 发布时间 | 状态 |
|------|----------|------|
| 5.6 | 2013.02 | EOL（2021.02 停止支持） |
| 5.7 | 2015.10 | EOL（2023.10 停止支持） |
| 8.0 | 2018.04 | 当前主版本（支持中） |

---

## 二、5.6 → 5.7 主要变化

### 性能提升

| 特性 | 5.6 | 5.7 |
|------|-----|-----|
| 读写性能 | 基准 | 3-4x 更快（官方数据） |
| 并行复制 | 基于 Schema 并行 | 基于 LOGICAL_CLOCK（组提交并行） |
| InnoDB 临时表 | 磁盘临时表在 ibtmp1 | 优化元数据管理 |
| Full-Text 索引 | 仅 InnoDB | InnoDB FTS 全面优化 |

### 复制增强

```
5.6: 从库单线程回放 → 主从延迟严重
5.7: slave_parallel_workers > 1 → 基于组提交的并行回放
```

- **GTID 增强**：支持在线启用 GTID，不需要重启
- **多源复制**：一个从库可以同时从多个主库复制
- **增强半同步**：`AFTER_SYNC` 模式，零丢失半同步

### 数据字典 & 系统

- `sys` schema 首次引入（Performance Schema 的易读视图）
- JSON 类型引入（用 `JSON_EXTRACT`、`JSON_SET` 等函数）
- 生成列（Generated Columns）：虚拟列和存储列
- InnoDB 空间加密（表空间级）

### SQL & 优化器

- `EXPLAIN FOR CONNECTION <id>`（分析正在运行的 SQL）
- query rewrite plugin（查询重写插件）
- 优化器增强：MySQL 5.7 的 cost model 大幅改善

### 安全性

- `mysql.user` 表 `password` 列被移除，`authentication_string` 取代
- `mysql_ssl_rsa_setup` 自动初始化 SSL
- 密码过期策略 `default_password_lifetime`

---

## 三、5.7 → 8.0 重大变化 ⭐

### 🔴 破坏性变化（升级必看）

| 变化 | 说明 |
|------|------|
| `utf8mb4` 变默认字符集 | 之前 `utf8` 是 utf8mb3，现在默认 `utf8mb4` |
| `utf8` = utf8mb3 废弃 | `SHOW CREATE TABLE` 会显示 `utf8mb3` |
| `query_cache` 完全移除 | 5.7 已废弃，8.0 直接删掉 |
| SQL_MODE 更严格 | `ONLY_FULL_GROUP_BY` 等默认开启 |
| 授权语法变更 | `GRANT ... IDENTIFIED BY` 不再支持，必须先 `CREATE USER` |
| InnoDB 内部元数据表改名 | `mysql.innodb_table_stats` → 数据字典重构 |
| 默认认证插件 | `caching_sha2_password` 替代 `mysql_native_password` |
| 保留字增加 | `rank`、`system`、`window` 等成为保留字 |

---

## 四、MySQL 8.0 核心新特性详讲

### 1. 原子 DDL（Atomic DDL）

**问题：** 5.7 中 DDL 中途崩溃会导致元数据与数据不一致（如 `DROP TABLE` 崩溃后只剩一半数据文件）。

**8.0 方案：** DDL 操作写入同一个事务日志（data dictionary + storage engine），要么全成功，要么全回滚。

```sql
-- 8.0 中这些操作都能原子回滚：
DROP TABLE t1, t2;           -- 删两张表，中途崩溃 → 两张表都在
ALTER TABLE t ADD COLUMN x;  -- DDL 失败 → 表回滚到改前状态
CREATE TABLE t ...;          -- 中途崩溃 → 表不存在
```

### 2. INSTANT DDL（参见单独文档）

- `ADD COLUMN` 秒级完成（仅改元数据）
- 8.0.29+ 支持 `DROP COLUMN`、任意位置加列
- 行记录头部记录 INSTANT 列信息，读取时自动补齐

### 3. 数据字典重构（Data Dictionary）

```
5.7:
  frm 文件（表定义）+ InnoDB 内部系统表 + mysql.* 系统表
  → 多源、不同步、易损坏

8.0:
  统一的数据字典（InnoDB 表，存在 mysql.ibd）
  → 单一真相源、事务性、原子 DDL
```

- 删除了 `.frm` 文件（表定义存在数据字典中）
- 删除了 `mysql.innodb_table_stats`（改为 `innodb_stats` 系统表）
- `INFORMATION_SCHEMA` 查询快 100 倍（之前要打开 frm 文件读取）

### 4. 窗口函数（Window Functions）

5.7 实现排名、累计只能靠 `@变量` 黑魔法，8.0 原生支持：

```sql
-- 按部门排名
SELECT dept, name, salary,
  ROW_NUMBER()   OVER (PARTITION BY dept ORDER BY salary DESC) AS row_num,
  RANK()         OVER (PARTITION BY dept ORDER BY salary DESC) AS rank_val,
  DENSE_RANK()   OVER (PARTITION BY dept ORDER BY salary DESC) AS dense_val,
  SUM(salary)    OVER (PARTITION BY dept ORDER BY hire_date) AS running_total
FROM employees;

-- 窗口函数一览
ROW_NUMBER | RANK | DENSE_RANK | PERCENT_RANK
CUME_DIST | NTILE | LAG | LEAD | FIRST_VALUE | LAST_VALUE
NTH_VALUE | SUM | AVG | COUNT | MIN | MAX (窗口聚合)
```

### 5. CTE（公用表表达式）

```sql
-- 普通 CTE（WITH 子句）
WITH dept_avg AS (
  SELECT dept_id, AVG(salary) avg_sal
  FROM employees
  GROUP BY dept_id
)
SELECT e.name, e.salary, d.avg_sal
FROM employees e
JOIN dept_avg d ON e.dept_id = d.dept_id
WHERE e.salary > d.avg_sal;

-- 递归 CTE（查组织树）
WITH RECURSIVE org_tree AS (
  SELECT id, name, manager_id, 1 AS level
  FROM employees WHERE manager_id IS NULL
  UNION ALL
  SELECT e.id, e.name, e.manager_id, t.level + 1
  FROM employees e
  JOIN org_tree t ON e.manager_id = t.id
)
SELECT * FROM org_tree;
```

**递归 CTE 替代了以前必须用存储过程/循环才能做的树形查询。**

### 6. 不可见索引（Invisible Index）

```sql
ALTER TABLE t ALTER INDEX idx_name INVISIBLE;
-- 优化器不再使用它，但索引仍然维护

-- 测试索引是否真的需要
SELECT * FROM t FORCE INDEX(idx_name) WHERE ...;
-- vs
SELECT * FROM t WHERE ...;  -- 优化器自己选

-- 确认无效后删除
ALTER TABLE t ALTER INDEX idx_name VISIBLE;  -- 恢复
-- 或 DROP INDEX idx_name;                  -- 确认无用再删除
```

**场景：** 不确定一个索引是否还有用？先 invisible 观察几天，没问题再删除。

### 7. 降序索引（Descending Index）

5.7 中 `ORDER BY a ASC, b DESC` 无法同时利用复合索引排序，8.0 支持：

```sql
CREATE INDEX idx ON t (a ASC, b DESC);

-- 下面查询能用索引排序，不再 Using filesort
SELECT * FROM t ORDER BY a ASC, b DESC;
```

### 8. JSON 增强

```sql
-- JSON_TABLE：把 JSON 转成关系表
SELECT jt.*
FROM t,
JSON_TABLE(
  t.data,
  '$.items[*]' COLUMNS (
    id INT PATH '$.id',
    name VARCHAR(50) PATH '$.name',
    price DECIMAL(10,2) PATH '$.price'
  )
) AS jt;

-- JSON 聚合函数
SELECT JSON_ARRAYAGG(name) FROM t;       -- ["a","b","c"]
SELECT JSON_OBJECTAGG(key, value) FROM t; -- {"k1":"v1","k2":"v2"}

-- JSON 操作符
-- 5.7: JSON_EXTRACT(col, "$.name")
-- 8.0: col->>"$.name"  (等价于 JSON_UNQUOTE)
```

### 9. 正则表达式增强

```sql
-- 完整 ICU 正则库，支持所有现代正则语法
-- 5.7 只支持基础正则 (REGEXP_LIKE 等函数)
-- 8.0 新增：
SELECT REGEXP_SUBSTR('abc123def', '[0-9]+');   -- "123"
SELECT REGEXP_REPLACE('abc123', '[0-9]+', 'X'); -- "abcX"
SELECT REGEXP_INSTR('abc123', '[0-9]+');        -- 4
```

### 10. 资源组（Resource Groups）

```sql
-- 创建资源组，绑定 CPU 核心
CREATE RESOURCE GROUP rpt TYPE=USER VCPU=2,3;
CREATE RESOURCE GROUP oltp TYPE=USER VCPU=0,1;

-- 把会话绑到特定资源组
SET RESOURCE GROUP oltp FOR CURRENT_THREAD;

-- 做报表时切到低优先级
SET RESOURCE GROUP rpt;
SELECT ...;  -- 大型分析查询
SET RESOURCE GROUP oltp;
```

### 11. 直方图（Histogram Statistics）

5.7 优化器只有索引基数统计，8.0 可以给单列建直方图：

```sql
-- 给字段创建等宽直方图
ANALYZE TABLE t UPDATE HISTOGRAM ON col1, col2 WITH 256 BUCKETS;

-- 查看直方图信息
SELECT * FROM information_schema.column_statistics
WHERE table_name = 't';

-- 删除
ANALYZE TABLE t DROP HISTOGRAM ON col1;
```

**作用：** 当列没有索引，但数据分布又不均匀时，直方图帮优化器估算更准。

### 12. 复制增强

| 特性 | 5.7 | 8.0 |
|------|-----|-----|
| 并行复制 | LOGICAL_CLOCK | **WRITESET** 并行（粒度更细） |
| 二进制日志 | 单个 | 支持 binlog 加密 + 事务性 DDL |
| 组复制 (MGR) | 5.7.17 引入（GA 外） | 生产就绪 |
| Clone Plugin | 无 | 物理克隆，不用 xtrabackup |

**WRITESET 并行：** 基于行级冲突检测，大幅提升从库并行度。

### 13. 安全增强

- `caching_sha2_password` 默认认证（比 `mysql_native_password` 更安全）
- SQL Roles（角色管理，类似 Oracle 的角色）
- 密码强度策略 `cracklib`
- 支持 FIPS 模式
- ACL 细粒度控制（`partial_revokes`：`REVOKE INSERT ON testdb.*`）

```sql
-- 角色管理示例
CREATE ROLE 'app_read', 'app_write';
GRANT SELECT ON mydb.* TO 'app_read';
GRANT INSERT,UPDATE,DELETE ON mydb.* TO 'app_write';
GRANT 'app_read' TO 'appuser'@'%';
```

### 14. 其他实用特性

| 特性 | 说明 |
|------|------|
| **CHECK 约束** | `CREATE TABLE t (age INT CHECK (age > 0))` — 8.0.16+ |
| **Hash Join** | 8.0.18+ 支持 Hash Join（替代部分 Nested Loop） |
| **NOWAIT / SKIP LOCKED** | `SELECT ... FOR UPDATE NOWAIT` |
| **SET PERSIST** | `SET PERSIST max_connections=500;`（重启保留） |
| **快速加列** | INSTANT DDL |
| **透视表** | JSON_TABLE |
| **Schema 版本** | 不需要再看到 `.frm` 文件 |
| **EXPLAIN ANALYZE** | 8.0.18+ 真实执行并返回各步骤实际耗时（一棵树 + 时间） |

---

## 五、EXPLAIN 演进对比

```sql
-- 5.6: 基础 EXPLAIN
-- 5.7: EXPLAIN FORMAT=JSON（关键进步）
-- 8.0: EXPLAIN ANALYZE（王炸）
```

```sql
-- 8.0 EXPLAIN ANALYZE 示例输出：
mysql> EXPLAIN ANALYZE SELECT * FROM t WHERE a BETWEEN 1 AND 100;
+-----------------------------------------------------------------+
| -> Filter: (t.a between 1 and 100)  (cost=...) (rows=99)
    -> Index range scan on t using PRIMARY  (cost=...) (rows=99)
       (actual time=0.025..0.089 rows=99 loops=1)
+-----------------------------------------------------------------+
-- 既显示估代价 (cost)，也显示实际时间 (actual time)
```

---

## 六、升级建议

### 从 5.6/5.7 → 8.0 升级 checklist

```
☐ 1. 检查器：mysqlcheck -u root -p --all-databases --check-upgrade
☐ 2. utf8mb3 → utf8mb4：查看所有 utf8 列是否需要转换
☐ 3. 保留字冲突：GROUP BY rank → 改列名或加引号
☐ 4. query_cache 移除：确认应用不依赖 query cache
☐ 5. 认证插件：确认客户端支持 caching_sha2_password
☐ 6. SQL_MODE：适配 ONLY_FULL_GROUP_BY
☐ 7. 系统表升级：自动升级数据字典，需要重启一次
☐ 8. 用 mysqldump 全量备份后再操作！
```

---

## 七、一句话总结

| 版本 | 一句话 |
|------|--------|
| **5.6 → 5.7** | 性能 3x + GTID + 半同步零丢失 + JSON 起步 |
| **5.7 → 8.0** | 原子 DDL + 窗口函数 + CTE + 数据字典统一 + 复制 WRITESET + INSTANT DDL |

**5.6 和 5.7 已 EOL，生产跑这两个版本的该升级了。**
