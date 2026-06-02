# Data Validation Tool (DVT) 使用指南

> Google Cloud 开源的数据校验工具——跨异构数据源自动比对，支持 16+ 种数据库

---

## 一、是什么

**DVT (Data Validation Tool)** 是 Google Cloud 开源的 Python CLI 工具，用于在数据迁移、同步、ETL 场景中校验源端和目标端数据是否一致。

**一句话**：给你两个数据库的表，DVT 自动比较它们的数据量、列聚合值、逐行哈希值，告诉你两边是不是一样的。

### 核心能力

| 校验类型 | 说明 |
|---------|------|
| **Column Validation** ⭐ | 列级别聚合比较（COUNT/SUM/AVG/MIN/MAX/STD + GROUP BY） |
| **Row Validation** ⭐ | 逐行哈希/字段比较，精确定位差异行 |
| **Schema Validation** | 比较表结构（列名、类型、是否可空） |
| **Custom Query Validation** | 自定义 SQL 查询结果比较 |
| **Ad-hoc SQL Exploration** | 临时 SQL 查询探索 |

### 支持的数据库（16+ 种）

```
GCP 原生：BigQuery、Spanner、AlloyDB、FileSystem (GCS)
传统关系型：MySQL、PostgreSQL、Oracle、SQL Server、Db2 (LUW+z/OS)
大数据：Hive、Impala、Teradata
云数仓：Snowflake、Redshift
其他：Sybase ASE
```

---

## 二、原理机制

### 整体架构

```
CLI (data-validation)
    ↓
Ibis 框架（统一 SQL 方言抽象层）
    ├→ BigQuery SQL  → BigQuery
    ├→ PostgreSQL SQL → PostgreSQL
    ├→ MySQL SQL     → MySQL
    └→ Oracle SQL    → Oracle
    ↓
比较结果 → stdout / BigQuery 表 / PostgreSQL 表
```

### 核心原理：基于 Ibis 框架的 SQL 生成

DVT 不读取原始数据到本地，而是：

```
1. 根据校验类型生成对应 SQL
   - Column: SELECT COUNT(*) FROM t UNION ALL SELECT COUNT(*) FROM t2
   - Row: SELECT hash_cols FROM t ↔ SELECT hash_cols FROM t2

2. 分别向源端和目标端发送 SQL

3. 在 Python 端比较两份结果集（内存中 diff）
   → 不搬全量数据，只搬聚合结果或 diff 后的差异行

4. 输出校验报告
```

### 三种校验的 SQL 生成原理

**Column Validation（列校验）**：
```
源：SELECT COUNT(*), SUM(amount), AVG(price) FROM src_table
目标：SELECT COUNT(*), SUM(amount), AVG(price) FROM tgt_table
比较：源 COUNT vs 目标 COUNT，计算 pct_difference
```

**Row Validation（行级校验）**：
```
源：SELECT pk, SHA256(CONCAT(col1, '|', col2, '|', ...)) AS hash FROM src_table
目标：SELECT pk, SHA256(CONCAT(col1, '|', col2, '|', ...)) AS hash FROM tgt_table
比较：FULL OUTER JOIN 找出 mismatch 和 missing 行
```

**Schema Validation（表结构校验）**：
```
查询 information_schema.columns → 比较列名、类型、nullable
```

### 关键设计特性

- **不传输全量数据**：只搬聚合结果或差异行
- **数据库原生计算**：SUM/AVG/HASH 都在数据库内执行
- **Ibis 中间层**：统一 16 种数据库的 SQL 方言差异
- **增量校验**：支持 filter 指定校验范围

---

## 三、安装

### 方式一：pip 安装（推荐）

```bash
# 前置条件
sudo apt-get install python3 python3-dev gcc -y

# 创建虚拟环境
python3 -m venv venv
source venv/bin/activate
pip install --upgrade pip

# 安装 DVT
pip install google-pso-data-validator

# 验证
data-validation --help
```

### 方式二：源码安装（开发/调试）

```bash
git clone https://github.com/GoogleCloudPlatform/professional-services-data-validator.git
cd professional-services-data-validator
pip install .
```

### 方式三：Docker

```bash
# 参考 samples/docker/ 中的 Dockerfile
docker build -t data-validation .
docker run data-validation --help
```

### 特定数据库依赖

```bash
# Oracle
pip install oracledb

# SQL Server
pip install pyodbc

# Teradata（需授权）
pip install teradatasql

# Snowflake
pip install snowflake-connector-python

# MySQL
pip install pymysql

# PostgreSQL（默认已包含）
# psycopg2 自动安装
```

---

## 四、创建连接

### 连接管理机制

```
连接配置存储位置：
  ~/.config/google-pso-data-validator/    （本地默认）
  gs://bucket/path/                        （GCS，推荐）
  由环境变量 PSO_DV_CONN_HOME 控制
```

### BigQuery 连接

```bash
# 创建 BigQuery 连接
data-validation connections add \
  --connection-name bq_prod BigQuery \
  --project-id my-gcp-project

# 使用 GCP Secret Manager 存密码
data-validation connections add \
  --secret-manager-type GCP \
  --secret-manager-project-id my-project \
  --connection-name bq_secure BigQuery \
  --project-id my-gcp-project
```

### MySQL 连接

```bash
data-validation connections add \
  --connection-name mysql_prod MySQL \
  --host 10.0.0.1 \
  --port 3306 \
  --user dvt_user \
  --password 'S3cr3t!' \
  --database mydb
```

### PostgreSQL 连接

```bash
data-validation connections add \
  --connection-name pg_prod Postgres \
  --host 10.0.0.2 \
  --port 5432 \
  --user dvt_user \
  --password 'S3cr3t!' \
  --database mydb
```

### Oracle 连接

```bash
data-validation connections add \
  --connection-name ora_prod Oracle \
  --host 10.0.0.3 \
  --port 1521 \
  --user dvt_user \
  --password 'S3cr3t!' \
  --database ORCLPDB1

# TLS 安全连接
data-validation connections add \
  --connection-name ora_secure Oracle \
  --user dvt_user --password 'S3cr3t!' \
  --connect-args='{"wallet_location":"/path/wallet","config_dir":"/path/tns"}'
```

### 查看/删除连接

```bash
# 列出所有连接
data-validation connections list

# 查看连接详情
data-validation connections describe -c bq_prod

# 删除连接
data-validation connections delete -c bq_prod
```

---

## 五、核心用法：三种校验

### 5.1 Column Validation（列校验）⭐ 最常用

**用途**：快速验证数据量是否一致，某列聚合值是否匹配。

```bash
# 基础行数校验（默认 COUNT *）
data-validation validate column \
  --source-conn bq_prod \
  --target-conn mysql_prod \
  --tables-list my_dataset.orders=orders

# 指定聚合函数
data-validation validate column \
  --source-conn bq_prod \
  --target-conn mysql_prod \
  --tables-list my_dataset.orders=orders \
  --sum amount,tax \
  --count id,user_id \
  --min amount \
  --max amount \
  --avg amount

# 分组校验（按日期分组统计）
data-validation validate column \
  --source-conn bq_prod \
  --target-conn mysql_prod \
  --tables-list my_dataset.orders=orders \
  --grouped-columns date \
  --sum amount \
  --count id

# 对所有数值列做 sum（* 通配符）
data-validation validate column \
  --source-conn bq_prod \
  --target-conn mysql_prod \
  --tables-list my_dataset.orders=orders \
  --sum '*'
```

**输出示例**：
```
+-----------------+------------+------------+----------+-----------+----------+-------------+
| validation_name | source_agg | target_agg | pct_diff | pct_thold | status   | columns     |
+-----------------+------------+------------+----------+-----------+----------+-------------+
| count           |   500000   |   499995   |  0.001%  |    0%     |  fail    | id,user_id  |
| sum_amount      |   12345678 |   12345678 |    0%    |    0%     |  pass    | amount      |
| sum_tax         |   1234567  |   1234000  |  0.046%  |    0%     |  fail    | tax         |
+-----------------+------------+------------+----------+-----------+----------+-------------+
```

### 5.2 Row Validation（行级校验）⭐ 精确校验

**用途**：精确定位哪些行不一致，需要主键或唯一键。

```bash
# 哈希比较（所有列）
data-validation validate row \
  --source-conn bq_prod \
  --target-conn mysql_prod \
  --tables-list my_dataset.orders=orders \
  --primary-keys id \
  --hash '*'

# 只比较指定字段的原始值
data-validation validate row \
  --source-conn bq_prod \
  --target-conn mysql_prod \
  --tables-list my_dataset.orders=orders \
  --primary-keys id \
  --comparison-fields order_no,amount,status,create_time

# 哈希指定列（不比较全部列）
data-validation validate row \
  --source-conn bq_prod \
  --target-conn mysql_prod \
  --tables-list my_dataset.orders=orders \
  --primary-keys id \
  --hash order_no,amount,status,create_time
```

**工作原理**：
```
源端：
  SELECT id, SHA256(CONCAT(IFNULL(col1,''), '|', IFNULL(col2,''), '|', ...)) AS hash
  FROM src_table

目标端：
  SELECT id, SHA256(CONCAT(IFNULL(col1,''), '|', IFNULL(col2,''), '|', ...)) AS hash
  FROM tgt_table

Python 端：
  FULL OUTER JOIN src_hash JOIN tgt_hash ON id
  → 输出：mismatch 的行（hash 不同）、missing 的行（一侧有另一侧无）
```

### 5.3 Schema Validation（表结构校验）

```bash
data-validation validate schema \
  --source-conn bq_prod \
  --target-conn mysql_prod \
  --tables-list my_dataset.orders=orders
```

**输出示例**：
```
+--------------+------------------+-------------+---------------+------------+
| column_name  | source_type      | target_type | validation_status | notes  |
+--------------+------------------+-------------+-------------------+
| id           | INT64            | INT         | pass              |        |
| order_no     | STRING           | VARCHAR(50) | pass              |        |
| amount       | FLOAT64          | DECIMAL(10,2)| fail            | 类型不匹配 |
| created_at   | TIMESTAMP        | DATETIME    | pass              |        |
+--------------+------------------+-------------+-------------------+
```

---

## 六、高级特性

### 6.1 过滤器（增量校验）

```bash
# 只校验昨天的数据
data-validation validate column \
  --source-conn bq_prod \
  --target-conn mysql_prod \
  --tables-list my_dataset.orders=orders \
  --filters 'date="2024-06-01":date="2024-06-01"' \
  --count id --sum amount

# 源和目标不同过滤条件
data-validation validate row \
  --source-conn bq_prod \
  --target-conn mysql_prod \
  --tables-list my_dataset.orders=orders \
  --primary-keys id --hash '*' \
  --filters 'date>="2024-01-01":date>="2024-01-01"'
```

### 6.2 结果输出（Result Handler）

```bash
# 输出到 BigQuery
data-validation validate column \
  --source-conn bq_prod \
  --target-conn mysql_prod \
  --tables-list my_dataset.orders=orders \
  --sum amount \
  --result-handler bq_prod.pso_data_validator.results

# 输出到 PostgreSQL
data-validation validate column \
  --source-conn bq_prod \
  --target-conn pg_prod \
  --tables-list my_dataset.orders=public.orders \
  --sum amount \
  --result-handler pg_prod.pso_data_validator.results
```

### 6.3 输出格式

```bash
# 表格格式（默认）
--format table

# CSV 格式（机器解析）
--format csv

# JSON 格式
--format json

# 文本格式
--format text
```

### 6.4 YAML 配置文件（批量校验）

```yaml
# validations.yaml
result_handler:
  project_id: my-project
  table_id: pso_data_validator.results

source: bq_prod
target: mysql_prod

validations:
  # 校验 1：行数
  - type: Column
    tables:
      - source: my_dataset.orders
        target: orders
    aggregates:
      - field: id
        type: count

  # 校验 2：金额总和 + 分组
  - type: Column
    tables:
      - source: my_dataset.orders
        target: orders
    aggregates:
      - field: amount
        type: sum
    grouped_columns:
      - date

  # 校验 3：行级精确比较
  - type: Row
    tables:
      - source: my_dataset.orders
        target: orders
    primary_keys:
      - id
    comparison_fields:
      - order_no
      - amount
      - status
      - create_time
```

```bash
# 运行配置文件
data-validation run-config --config-file validations.yaml
```

### 6.5 自定义 SQL 校验

```bash
# 比较两个自定义查询的结果
data-validation validate custom-query \
  --source-conn bq_prod \
  --target-conn mysql_prod \
  --source-query "SELECT date, SUM(amount) FROM orders WHERE status='done' GROUP BY date" \
  --target-query "SELECT date, SUM(amount) FROM orders WHERE status='done' GROUP BY date" \
  --count '*'
```

### 6.6 阈值控制

```bash
# 允许 1% 的差异（适合浮点数比较）
data-validation validate column \
  --source-conn bq_prod \
  --target-conn mysql_prod \
  --tables-list my_dataset.orders=orders \
  --sum amount \
  --threshold 1.0
```

### 6.7 通配符批量表

```bash
# 校验 schema 下所有表
data-validation validate column \
  --source-conn bq_prod \
  --target-conn mysql_prod \
  --tables-list my_dataset.* \
  --count '*'
```

---

## 七、部署与集成

### 7.1 Airflow 集成

```python
# samples/airflow/dvt_airflow_dag.py
from airflow import DAG
from airflow.operators.python import PythonVirtualenvOperator

def run_dvt_validation():
    import subprocess
    subprocess.run([
        "data-validation", "validate", "column",
        "--source-conn", "bq_prod",
        "--target-conn", "mysql_prod",
        "--tables-list", "orders=orders",
        "--count", "*",
        "--result-handler", "bq_prod.pso_data_validator.results"
    ], check=True)

with DAG("dvt_daily_validation", schedule_interval="@daily") as dag:
    validate_task = PythonVirtualenvOperator(
        task_id="validate_orders",
        python_callable=run_dvt_validation,
        requirements=["google-pso-data-validator"],
    )
```

### 7.2 Cloud Run 部署

参考 `samples/` 目录下的 Cloud Run 和 Cloud Functions 示例。

### 7.3 BigQuery 结果表初始化

```bash
# 创建 BigQuery 结果表
bq mk pso_data_validator
bq mk --table \
  --time_partitioning_field start_time \
  --clustering_fields validation_name,run_id \
  pso_data_validator.results \
  terraform/results_schema.json
```

---

## 八、典型使用场景

### 场景 1：MySQL → BigQuery 迁移校验

```bash
# 第 1 步：建连接
data-validation connections add -c mysql_src MySQL \
  --host 10.0.0.1 --user dvt --password 'xxx' --database prod
data-validation connections add -c bq_tgt BigQuery \
  --project-id my-project

# 第 2 步：表结构校验
data-validation validate schema \
  -sc mysql_src -tc bq_tgt \
  -tbls prod.orders=my_dataset.orders

# 第 3 步：行数 + 关键列聚合校验
data-validation validate column \
  -sc mysql_src -tc bq_tgt \
  -tbls prod.orders=my_dataset.orders \
  -sum amount,tax -count id -grouped-columns order_date

# 第 4 步：抽样行级精确校验（昨天数据）
data-validation validate row \
  -sc mysql_src -tc bq_tgt \
  -tbls prod.orders=my_dataset.orders \
  -pk id -hash '*' \
  -filters 'order_date="2024-06-01":order_date="2024-06-01"'
```

### 场景 2：每日增量数据校验（Airflow 定时）

```bash
# 校验当天新增数据
TODAY=$(date +%Y-%m-%d)

data-validation validate column \
  -sc bq_prod -tc mysql_report \
  -tbls dataset.daily_summary=daily_summary \
  -sum revenue,orders,customers \
  -filters "date='$TODAY':date='$TODAY'" \
  -rh bq_prod.pso_data_validator.results \
  -l env=prod,run=daily_validation \
  -th 0.5
```

### 场景 3：ETL 转换后校验

```bash
# 自定义查询比较 ETL 转换结果
data-validation validate custom-query \
  -sc mysql_src -tc bq_tgt \
  --source-query "
    SELECT user_id, COUNT(*) as order_cnt, SUM(amount) as total
    FROM orders WHERE status='done'
    GROUP BY user_id
  " \
  --target-query "
    SELECT user_id, order_count, total_amount
    FROM my_dataset.user_summary
  " \
  --count '*'
```

---

## 九、最佳实践

```
1. 分层校验
   先 column（快，秒级）→ 有问题再 row（慢，定位差异）
   不要上来就 hash '*' 全表

2. 增量优先
   大表用 filter 按日期分批校验，避免一次扫描全表

3. 阈值合理设置
   FLOAT/DOUBLE 类型设置 threshold=0.01（浮点误差）
   INT/DECIMAL 设置 threshold=0

4. 权限最小化
   DVT 只需要 SELECT 权限 + 结果表 INSERT 权限
   不要给 root/admin 账号

5. 结果持久化
   务必用 --result-handler 存 BigQuery
   不要依赖 stdout，历史记录丢了没法回溯

6. 敏感信息保护
   密码用 GCP Secret Manager，不要硬编码

7. 避开高峰期
   大表的 column validation 也会扫全表
   安排在业务低峰期或只查增量

8. GROUP BY 慎用
   高基数列（如 user_id）分组会产生巨量结果
   先确认分组列基数
```

---

## 十、故障排查

```bash
# 1. 开启详细日志
data-validation -v validate column ...

# 2. 指定日志级别
data-validation --log-level DEBUG validate column ...

# 3. 测试连接
data-validation connections describe -c CONN_NAME

# 4. 验证 SQL 生成（加 -v 看生成的 SQL）

# 5. 手动验证 SQL（把生成的 SQL 直接到数据库执行）

# 6. 常见错误
# "Connection refused" → 网络不通/防火墙
# "Permission denied" → 数据库权限不足
# "Table not found" → schema.table 名称写错
# "Type mismatch" → 源目标列类型不兼容（CAST 或 skip）
```

---

## 十一、与同类工具对比

| 工具 | 类型 | 异构支持 | 行级校验 | 规模 | 成本 |
|------|------|---------|---------|------|------|
| **DVT** | Google 开源 | ✅ 16+ 种 | ✅ hash | 任意 | 免费 |
| **Great Expectations** | 通用数据质量 | ✅ 有限 | ❌ | 中小 | 免费 |
| **dbt tests** | 转换测试 | ❌ 单库 | ❌ | 中小 | 免费 |
| **Monte Carlo** | 商业数据可观测 | ❌ 单库 | ❌ | 大 | 付费 |
| **Soda** | 数据可靠性 | ✅ 有限 | ❌ | 中大 | 免费/付费 |

**DVT 的独特价值**：跨异构数据库的表级别精确比对，尤其适合云迁移场景。
