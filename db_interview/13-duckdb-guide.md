# DuckDB 使用指南

> 嵌入式 OLAP 数据库——SQLite 的分析型兄弟，进程内运行，零配置，列式存储

---

## 一、是什么

**DuckDB** 是一个嵌入式的、面向分析工作负载（OLAP）的数据库。C++ 编写，2019 年开源。

```
一句话记住：
  SQLite 之于 OLTP = DuckDB 之于 OLAP
  PostgreSQL 语法兼容 + 列式存储 + 向量化执行
```

**核心特性：**

| 特性 | 说明 |
|------|------|
| **嵌入式** | 无服务进程，链接为库文件，应用内运行 |
| **零配置** | 无需安装、无守护进程、无端口监听 |
| **列式存储** | 天生适合聚合分析 |
| **向量化引擎** | 批量处理数据（非逐行），充分利用 CPU 缓存 |
| **SQL 方言** | 兼容 PostgreSQL 语法，扩展了友好的分析函数 |
| **多源查询** | 直接查询 CSV/Parquet/JSON/SQLite/PostgreSQL |
| **ACID** | 支持事务，MVCC |
| **跨平台** | Linux/macOS/Windows，C++/Python/R/Java/Node.js/Go 绑定 |

---

## 二、安装

### CLI 安装

```bash
# Linux / macOS
curl -O https://github.com/duckdb/duckdb/releases/download/v1.1.3/duckdb_cli-linux-amd64.zip
unzip duckdb_cli-linux-amd64.zip
./duckdb

# macOS Homebrew
brew install duckdb

# 或直接下载预编译二进制
```

### Python 安装

```bash
pip install duckdb

# 可选：Parquet/Arrow 支持
pip install duckdb[parquet]
```

### R 安装

```r
install.packages("duckdb")
```

### 其他语言

```bash
# Node.js
npm install duckdb

# Go
go get github.com/marcboeker/go-duckdb

# Java
# Maven: org.duckdb / duckdb_jdbc
```

---

## 三、快速上手

### CLI 模式

```bash
$ ./duckdb
v1.1.3 19864453f7
Enter ".help" for usage hints.
Connected to a transient in-memory database.
Use ".open FILENAME" to reopen on a persistent database.
```

```sql
-- 直接写 SQL（不需要建表就能查询数据）
SELECT 'Hello, DuckDB!';

-- 查询 CSV 文件（零导入）
SELECT * FROM read_csv('orders.csv');

-- 查询 Parquet（列式格式，超快）
SELECT * FROM read_parquet('logs.parquet');

-- 链式操作
SELECT category, SUM(amount) AS total
FROM read_csv('sales.csv')
WHERE year = 2024
GROUP BY category
ORDER BY total DESC;
```

### Python 模式

```python
import duckdb

# 1. 内存数据库（最常用）
conn = duckdb.connect()
# 或直接用默认连接
duckdb.sql("SELECT 42").show()

# 2. 持久化文件
conn = duckdb.connect('my_database.duckdb')

# 3. 直接查询 DataFrame！🔥
import pandas as pd
df = pd.DataFrame({'name': ['Alice', 'Bob'], 'age': [30, 25]})
result = duckdb.sql("SELECT name, age FROM df WHERE age > 26").df()
print(result)
#     name  age
# 0  Alice   30

# 4. 查询 CSV
duckdb.sql("SELECT * FROM 'sales.csv'").show()

# 5. 查询 Parquet
duckdb.sql("SELECT * FROM 'data/*.parquet' LIMIT 10").show()
```

### 执行模式

```python
# 方式 1：直接 sql()
duckdb.sql("CREATE TABLE t AS SELECT * FROM read_csv('data.csv')")
duckdb.sql("SELECT count(*) FROM t")

# 方式 2：conn.execute()
conn = duckdb.connect('my.db')
conn.execute("CREATE TABLE users (id INTEGER, name VARCHAR)")
conn.execute("INSERT INTO users VALUES (1, 'Alice')")
result = conn.execute("SELECT * FROM users").fetchall()
```

---

## 四、数据导入/导出

### 从文件查询（零拷贝）

```sql
-- CSV
SELECT * FROM read_csv('file.csv');
SELECT * FROM read_csv('file.csv', header=true, delim='|');
SELECT * FROM read_csv('data/*.csv');  -- 通配符批量

-- Parquet ⭐ 推荐格式
SELECT * FROM read_parquet('file.parquet');
SELECT * FROM read_parquet('logs/2024/*.parquet');  -- 文件扫描
SELECT * FROM read_parquet('s3://bucket/data.parquet');  -- S3 直读（需 httpfs 扩展）

-- JSON
SELECT * FROM read_json('file.json');
SELECT * FROM read_json_auto('file.json');  -- 自动推断 schema

-- Excel
INSTALL excel; LOAD excel;
SELECT * FROM st_read('file.xlsx');

-- SQLite
INSTALL sqlite; LOAD sqlite;
SELECT * FROM sqlite_scan('other.db', 'table_name');

-- PostgreSQL
INSTALL postgres; LOAD postgres;
SELECT * FROM postgres_scan('host=localhost dbname=mydb', 'public', 'users');
```

### 导入到 DuckDB 表

```sql
-- 从文件创建表（指定类型）
CREATE TABLE sales AS
SELECT * FROM read_csv('sales.csv', columns={
    'id': 'INTEGER',
    'amount': 'DECIMAL(10,2)',
    'date': 'DATE'
});

-- 从 Parquet 创建表（快速）
CREATE TABLE logs AS SELECT * FROM read_parquet('logs.parquet');
```

### 导出数据

```sql
-- 导出为 CSV
COPY sales TO 'output.csv' (HEADER, DELIMITER ',');

-- 导出为 Parquet
COPY sales TO 'output.parquet' (FORMAT PARQUET);

-- 导出为 JSON
COPY (SELECT * FROM sales LIMIT 100) TO 'output.json';

-- Python 导出到 DataFrame
df = duckdb.sql("SELECT * FROM sales").df()        # pandas
df = duckdb.sql("SELECT * FROM sales").arrow()     # PyArrow
df = duckdb.sql("SELECT * FROM sales").fetchnumpy() # NumPy
```

---

## 五、SQL 特性亮点

### 友好的 SQL 语法（比 PostgreSQL 更宽松）

```sql
-- 1. SELECT 中引用 SELECT 中定义的别名（传统 DB 不行）
SELECT
    amount * 1.1 AS with_tax,
    ROUND(with_tax, 2) AS rounded       -- ✅ 可以引用 with_tax
FROM orders;

-- 2. GROUP BY 中引用 SELECT 中的别名
SELECT
    EXTRACT(YEAR FROM date) AS year,
    SUM(amount) AS total
FROM orders
GROUP BY year;  -- ✅ 可以用别名

-- 3. 友好方括号访问
SELECT ['a', 'b', 'c'][2];  -- 返回 'b'（1-indexed）

-- 4. 隐式字符串拼接
SELECT 'Hello' ' ' 'World';  -- 'Hello World'
```

### 透视表（PIVOT/UNPIVOT）

```sql
-- PIVOT：行转列
PIVOT sales
ON EXTRACT(MONTH FROM date)  -- 按月份转列
USING SUM(amount);            -- 聚合方式

-- 结果：product | jan | feb | mar | ...

-- UNPIVOT：列转行
UNPIVOT monthly_sales
ON jan, feb, mar
INTO NAME month VALUE amount;
```

### 窗口函数

```sql
-- 完整窗口函数支持
SELECT
    product,
    date,
    amount,
    SUM(amount) OVER (PARTITION BY product ORDER BY date) AS cumulative,
    ROW_NUMBER() OVER (PARTITION BY product ORDER BY amount DESC) AS rank,
    LAG(amount) OVER (PARTITION BY product ORDER BY date) AS prev_amount
FROM sales;

-- 简化的窗口语法
SELECT
    product,
    amount,
    SUM(amount) OVER rolling_3_days
FROM sales
WINDOW rolling_3_days AS (
    PARTITION BY product
    ORDER BY date
    ROWS BETWEEN 3 PRECEDING AND CURRENT ROW
);
```

### ASOF Join（时序数据关联）🔥

```sql
-- 股市交易表 + 报价表，按时间近似匹配
SELECT t.trade_time, t.price, q.bid, q.ask
FROM trades t
ASOF JOIN quotes q
  ON t.symbol = q.symbol
 AND t.trade_time >= q.quote_time;
-- 取 trade_time 之前最近的报价
```

### LATERAL Join

```sql
-- 对每行执行子查询
SELECT
    u.name,
    o.*
FROM users u,
LATERAL (
    SELECT * FROM orders WHERE user_id = u.id ORDER BY date DESC LIMIT 3
) o;
```

### 列式操作（COLUMNS 表达式）

```sql
-- 对所有匹配的列应用函数
SELECT COLUMNS('amount|price') * 1.1 FROM invoices;
-- 展开为 SELECT amount*1.1, price*1.1

-- 排除某些列
SELECT * EXCLUDE (id, created_at) FROM users;

-- 替换某些列
SELECT * REPLACE (UPPER(name) AS name) FROM users;
```

---

## 六、Python 深度集成

### 直接查询 DataFrame

```python
import duckdb
import pandas as pd

# DataFrame 就是表
orders = pd.read_csv('orders.csv')
users = pd.read_parquet('users.parquet')

# Join 两个 DataFrame（无导入）
result = duckdb.sql("""
    SELECT u.name, SUM(o.amount) AS total
    FROM orders o
    JOIN users u ON o.user_id = u.id
    GROUP BY u.name
    ORDER BY total DESC
""").df()
```

### Python 函数注册（UDF）

```python
import duckdb

conn = duckdb.connect()

# 注册 Python 函数为 SQL 函数
@conn.create_function('add_tax')
def add_tax(amount):
    return amount * 1.13

conn.sql("SELECT add_tax(100)").show()  # 113.0

# 或者使用 lambda
duckdb.create_function('double', lambda x: x * 2)
```

### 与 Polars/Arrow 互操作

```python
import polars as pl
import pyarrow as pa

# Polars → DuckDB
pl_df = pl.read_parquet('data.parquet')
result = duckdb.sql("SELECT * FROM pl_df WHERE amount > 100").pl()

# PyArrow → DuckDB
arrow_table = pa.Table.from_pandas(df)
result = duckdb.sql("SELECT * FROM arrow_table").arrow()
```

### 查询 Pandas DataFrame（不复制数据）

```python
# duckdb 对 Pandas 的查询是零拷贝或高效转换的
df = pd.read_parquet('huge_file.parquet')  # 2GB DataFrame
# 直接在 DataFrame 上查，不需要先导入到 DuckDB 表
result = duckdb.sql("""
    SELECT category, COUNT(*) AS cnt, SUM(amount) AS total
    FROM df
    GROUP BY category
""").df()
```

---

## 七、扩展（Extensions）

DuckDB 通过扩展机制支持额外功能：

```sql
-- 安装扩展（自动从官方仓库下载）
INSTALL httpfs;     -- HTTP/S3 文件系统访问
INSTALL spatial;    -- 地理空间支持
INSTALL postgres;   -- PostgreSQL 连接
INSTALL sqlite;     -- SQLite 连接
INSTALL excel;      -- Excel 读取
INSTALL json;       -- JSON 增强

-- 加载扩展
LOAD httpfs;
```

### 常用扩展

| 扩展 | 功能 |
|------|------|
| **httpfs** | 直接查询 S3/HTTP 上的文件 |
| **spatial** | PostGIS 风格的地理空间函数 |
| **fts** | 全文搜索 |
| **postgres_scanner** | 直接查询 PostgreSQL 表 |
| **sqlite_scanner** | 直接查询 SQLite 数据库 |
| **iceberg** | Apache Iceberg 表格式 |
| **delta** | Delta Lake 表格式 |

### S3 示例

```sql
INSTALL httpfs; LOAD httpfs;

-- 设置 S3 凭证
SET s3_access_key_id='AKIAIOSFODNN7EXAMPLE';
SET s3_secret_access_key='...';
SET s3_region='us-east-1';

-- 直接查询 S3 中的文件
SELECT COUNT(*) FROM read_parquet('s3://my-bucket/logs/2024/*.parquet');

-- 将结果写入 S3
COPY (SELECT * FROM summary) TO 's3://my-bucket/output.parquet';
```

---

## 八、性能优化

### 1. 使用 Parquet 格式 ⭐

```sql
-- ❌ CSV：慢，文本解析
SELECT * FROM read_csv('data.csv') WHERE id = 100;

-- ✅ Parquet：列式存储 + 压缩 + 谓词下推
SELECT * FROM read_parquet('data.parquet') WHERE id = 100;
-- 只读 id 列，其他列不碰
```

### 2. 列裁剪

```sql
-- ✅ 只读需要的列（列式存储天生支持）
SELECT name, amount FROM sales WHERE date > '2024-01-01';
-- 不会读 sales 表的其他列

-- ❌ 别这么做
SELECT * FROM sales WHERE date > '2024-01-01';
```

### 3. 谓词下推

DuckDB 自动将过滤条件下推到文件扫描层：

```sql
-- 这个查询读取 Parquet 时只读匹配的行组（Row Group）
SELECT * FROM read_parquet('logs.parquet')
WHERE date = '2024-06-15' AND status = 'error';
```

### 4. 内存设置

```sql
-- 查看当前设置
SELECT * FROM duckdb_settings();

-- 内存限制
SET memory_limit = '4GB';

-- 线程数（默认 = CPU 核数）
SET threads = 4;

-- 临时目录（大数据溢出到磁盘）
SET temp_directory = '/fast_ssd/duckdb_tmp';
```

### 5. 物化 vs 流式

```sql
-- 中间结果物化（默认）
CREATE TABLE filtered AS SELECT * FROM raw WHERE status = 'valid';

-- 物化后多次查询更快
SELECT ... FROM filtered WHERE ...
```

---

## 九、DuckDB vs 其他系统

### DuckDB vs SQLite

| 特性 | DuckDB | SQLite |
|------|--------|--------|
| 目标场景 | **OLAP 分析** | **OLTP 事务** |
| 存储模型 | 列式 | 行式 |
| 执行引擎 | 向量化（批量） | 逐行 |
| 大聚合查询 | ⭐⭐⭐ | ⭐ |
| 点查询（单行） | ⭐⭐ | ⭐⭐⭐ |
| 数据类型 | 更丰富 | 基础 |
| CSV/Parquet | 原生支持 | 需要导入 |
| 并发 | 单写多读 | 单写多读 |

### DuckDB vs Pandas

| 特性 | DuckDB | Pandas |
|------|--------|--------|
| 内存效率 | 超出内存可溢出磁盘 | 全部内存 |
| 语法 | SQL | Python API |
| 大数据集 | ⭐⭐⭐ | ⭐（慢） |
| 多表 Join | ⭐⭐⭐ 优化器 | ⭐⭐ 手动 |
| 列式计算 | 自动并行 | 需手动优化 |

### DuckDB vs ClickHouse

| 特性 | DuckDB | ClickHouse |
|------|--------|------------|
| 架构 | 嵌入式库 | 独立服务器 |
| 部署 | 零配置 | 需部署运维 |
| 并发 | 弱 | 强 |
| 数据量 | GB-TB | TB-PB |
| 适合 | 单机分析 / 笔记本 | 分布式分析平台 |

**一句话选型**：
> DuckDB = 笔记本上的 ClickHouse  
> 10GB 以下数据 → Pandas  
> 100MB-500GB → DuckDB  
> 500GB+ → ClickHouse / Spark

---

## 十、实战示例

### 示例 1：CSV 分析一条龙

```bash
$ duckdb

# 从多个 CSV 文件加载
CREATE TABLE sales AS
SELECT * FROM read_csv('2024-*.csv', union_by_name=true);

# 快速探索
SELECT COUNT(*), COUNT(DISTINCT customer_id) FROM sales;

# 按月份和品类分析
SELECT
    EXTRACT(MONTH FROM sale_date) AS month,
    category,
    SUM(amount) AS revenue,
    COUNT(*) AS orders
FROM sales
GROUP BY month, category
ORDER BY month, revenue DESC;

# 导出结果
COPY (SELECT * FROM monthly_summary ORDER BY month) TO 'report.parquet';
```

### 示例 2：JSON 日志分析

```sql
-- 直接查询 JSON 日志文件
SELECT
    json_extract_string(line, '$.level') AS level,
    json_extract_string(line, '$.message') AS message,
    json_extract_string(line, '$.timestamp') AS ts,
    COUNT(*) AS cnt
FROM read_json('app.log', format='newline_delimited', columns={line: 'VARCHAR'})
GROUP BY level
ORDER BY cnt DESC;
```

### 示例 3：多源关联查询（Federated Query）🔥

```sql
-- 同时查询 Parquet、CSV、PostgreSQL
INSTALL postgres; LOAD postgres;

SELECT
    p.name,
    p.category,
    s.amount,
    s.sale_date,
    u.email
FROM read_parquet('products.parquet') p
JOIN read_csv('sales.csv') s ON p.id = s.product_id
JOIN postgres_scan('host=prod-db dbname=app', 'public', 'users') u
    ON s.user_id = u.id
WHERE s.sale_date >= '2024-01-01'
ORDER BY s.amount DESC
LIMIT 100;
```

### 示例 4：Python ETL 管道

```python
import duckdb

conn = duckdb.connect('warehouse.duckdb')

# 1. 从 S3 加载原始数据
conn.execute("""
    INSTALL httpfs; LOAD httpfs;
    SET s3_region='us-east-1';
    
    CREATE OR REPLACE TABLE raw_events AS
    SELECT * FROM read_parquet('s3://datalake/events/2024/*.parquet');
""")

# 2. 清洗和转换
conn.execute("""
    CREATE OR REPLACE TABLE clean_events AS
    SELECT
        event_id,
        user_id,
        event_type,
        PARSE_TIMESTAMP(timestamp_str) AS event_time,
        json_extract(payload, '$.page') AS page,
        json_extract(payload, '$.duration_ms')::INTEGER AS duration_ms
    FROM raw_events
    WHERE event_type IN ('pageview', 'click', 'purchase')
      AND user_id IS NOT NULL;
""")

# 3. 聚合
summary = conn.execute("""
    SELECT
        user_id,
        COUNT(*) AS total_events,
        COUNT(DISTINCT DATE(event_time)) AS active_days,
        SUM(CASE WHEN event_type = 'purchase' THEN 1 ELSE 0 END) AS purchases
    FROM clean_events
    GROUP BY user_id
""").df()

print(summary.head())
```

---

## 十一、常见问题

**Q: DuckDB 能处理多大数据？**
> 单机 100GB-1TB 比较舒适。超出内存也能工作（溢出到磁盘），但最佳实践是内存里的 50%-80%。

**Q: 能并发读写吗？**
> 单写多读（类似 SQLite）。高并发写入不适合 DuckDB，建议用 ClickHouse。

**Q: 支持索引吗？**
> 不支持传统 B-Tree 索引。列式存储 + 自动分区消除 + 压缩 + 向量化执行已经很快，不需要额外索引。

**Q: 数据存在哪里？**
> 内存模式（临时）或单个 `.duckdb` 文件（持久化）。

**Q: 能不能替代 PostgreSQL？**
> 分析场景可以，事务场景不行。常见做法是 OLTP 用 PostgreSQL，ETL 到 DuckDB 做分析。

**Q: DuckDB 和 MotherDuck 有什么关系？**
> MotherDuck 是 DuckDB 的云端版本，把 DuckDB 的计算扩展到云端，数据存 S3，计算分布式执行。本地 DuckDB + 云端 MotherDuck 是推荐架构。

---

## 十二、速查命令

```sql
-- 元数据
.tables                    -- 列出表
SHOW TABLES;
DESCRIBE sales;            -- 表结构
SUMMARIZE sales;           -- 快速统计摘要（min/max/avg/null_count/...）
PRAGMA table_info('sales');

-- 导出
EXPORT DATABASE 'backup/'; -- 导出整个数据库为 SQL + CSV
IMPORT DATABASE 'backup/'; -- 从目录导入

-- 设置
SET memory_limit = '8GB';
SET threads = 8;
SELECT * FROM duckdb_settings();

-- 查看查询计划
EXPLAIN SELECT ...;
EXPLAIN ANALYZE SELECT ...;  -- 带实际执行时间

-- .duckdb 文件信息
PRAGMA database_size;
PRAGMA version;
```
