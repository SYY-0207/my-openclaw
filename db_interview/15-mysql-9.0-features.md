# MySQL 9.0 版本特性全解

> MySQL 9.0 Innovation Release — 2024 年 7 月发布，首个创新版本，非 LTS

---

## 一、版本背景：创新版 vs LTS 版

MySQL 8.4 开始引入双轨发布模型：

```
Innovation Release（创新版）
  9.0 → 9.1 → 9.2 → ...
  每季度发布，包含最新特性
  只维护到下一个创新版发布
  适合：尝鲜、开发测试

LTS Release（长期支持版）
  8.4 LTS（支持到 2032 年）
  → 9.7 LTS（下一个）
  每 2 年一个大版本
  适合：生产环境
```

**重要**：MySQL 9.0 是创新版，**不建议直接上生产**。生产环境用 8.0 LTS（8.0.37+）或 8.4 LTS。

---

## 二、CREATE/ALTER EVENT 语法增强

### 新增 IF NOT EXISTS / IF EXISTS

```sql
-- 9.0 之前：重复创建会报错
CREATE EVENT daily_cleanup
  ON SCHEDULE EVERY 1 DAY
  DO DELETE FROM logs WHERE created_at < NOW() - INTERVAL 30 DAY;
-- ERROR 1537: Event 'daily_cleanup' already exists

-- 9.0：新语法
CREATE EVENT IF NOT EXISTS daily_cleanup
  ON SCHEDULE EVERY 1 DAY
  DO DELETE FROM logs WHERE created_at < NOW() - INTERVAL 30 DAY;
-- ✅ 已存在则跳过

DROP EVENT IF EXISTS daily_cleanup;  -- 同 DROP TABLE IF EXISTS
```

```sql
-- 新增 COMMENT 和 PRESERVE 属性修改
ALTER EVENT daily_cleanup
  COMMENT '每日清理 30 天前日志'
  ON COMPLETION PRESERVE;
```

---

## 三、EXPLAIN ANALYZE INTO（JSON 输出）

9.0 之前 EXPLAIN ANALYZE 只能打印到终端，现在可以输出为 JSON：

```sql
-- 9.0：将分析结果存入变量
EXPLAIN ANALYZE FORMAT=JSON INTO @analysis
  SELECT u.name, COUNT(*) AS cnt
  FROM users u
  JOIN orders o ON u.id = o.user_id
  GROUP BY u.name;

-- 查询结果
SELECT @analysis\G
-- 返回完整 JSON，包含：
--   - query_block
--   - cost_info
--   - table 级别的操作
--   - 实际行数 vs 预估行数
--   - 各算子实际耗时
```

**用途**：自动化慢查询分析——程序可以用 JSON 解析 EXPLAIN ANALYZE 输出了。

---

## 四、JavaScript 存储程序（企业版 MLE）

MySQL 9.0 企业在企业版中引入了 **Multi-Language Engine (MLE)** 组件，支持用 JavaScript 编写存储过程和函数：

```javascript
// 用 JavaScript 写存储过程
CREATE FUNCTION js_fibonacci(n INT) 
RETURNS BIGINT 
LANGUAGE JAVASCRIPT
AS $$
  function fibonacci(n) {
    if (n <= 1) return n;
    let a = 0, b = 1;
    for (let i = 2; i <= n; i++) {
      [a, b] = [b, a + b];
    }
    return b;
  }
  return fibonacci(n);
$$;

SELECT js_fibonacci(10);  -- 55
```

**使用场景**：
- SQL 难以实现的复杂逻辑（循环、递归）
- HTTP 调用（在存储过程中调外部 API）
- JSON 复杂处理
- 数据处理管道

**注意**：
- 仅企业版可用
- 基于 GraalVM 运行 JavaScript
- 需要先安装 MLE 组件：`INSTALL COMPONENT 'file://component_mle'`

---

## 五、authentication_openid_connect 插件

新增 OpenID Connect (OIDC) 认证插件，支持单点登录：

```
工作原理：
  客户端 → MySQL Server
    ↓
  MySQL → OpenID Provider（如 Okta/Auth0/Keycloak）
    ↓ 验证 JWT token
  返回认证结果

配置：
  CREATE USER 'alice'@'%' 
    IDENTIFIED WITH authentication_openid_connect
    BY '{"issuer": "https://accounts.google.com", "client_id": "xxx"}';
```

**替换方向**：取代旧的 `authentication_fido` 和 `authentication_ldap_sasl` 部分场景。

---

## 六、移除 mysql_native_password（重大变更）⚠️

```sql
-- 9.0：mysql_native_password 已移除
-- 以下语句报错
ALTER USER 'root'@'localhost' IDENTIFIED WITH mysql_native_password BY 'xxx';
-- ERROR: Plugin 'mysql_native_password' is not loaded

-- 只能使用
ALTER USER 'root'@'localhost' IDENTIFIED WITH caching_sha2_password BY 'xxx';
```

**影响**：
- 所有旧客户端（mysql client 8.0 之前）无法连接
- PHP、Python、Java 的旧 connector 需要升级
- 如果必须兼容旧客户端，需要在配置文件里手动加载旧插件（MySQL 仍保留实现但默认不加载）

---

## 七、向量数据类型（VECTOR）

为 AI/ML 场景新增 VECTOR 类型：

```sql
-- 创建向量表
CREATE TABLE articles (
  id INT PRIMARY KEY,
  title VARCHAR(200),
  embedding VECTOR(384)  -- 384 维向量（可指定维度）
);

-- 插入向量
INSERT INTO articles VALUES
(1, 'MySQL 索引优化', 
  TO_VECTOR('[0.1, 0.5, -0.3, ...]'));
INSERT INTO articles VALUES
(2, 'PostgreSQL 性能调优',
  TO_VECTOR('[0.2, 0.4, -0.1, ...]'));

-- 相似度搜索（余弦相似度）
SELECT 
  title,
  VECTOR_COSINE_SIMILARITY(embedding, TO_VECTOR('[0.15, 0.45, -0.25, ...]')) AS similarity
FROM articles
ORDER BY similarity DESC
LIMIT 5;
```

**支持的距离/相似度函数**：
| 函数 | 含义 |
|------|------|
| `VECTOR_COSINE_SIMILARITY(a, b)` | 余弦相似度（越大越相似） |
| `VECTOR_DOT_PRODUCT(a, b)` | 点积 |
| `VECTOR_L2_DISTANCE(a, b)` | 欧几里得距离（越小越相似） |

**与 pgvector 对比**：

| 特性 | MySQL 9.0 VECTOR | pgvector |
|------|-----------------|----------|
| 数据类型 | VECTOR(N) | vector(N) |
| 索引 | VARBINARY 上建索引（不直接支持 ANN） | IVFFlat / HNSW |
| 最大维度 | 16383 | 16000（HNSW） / 2000（IVFFlat） |
| 成熟度 | 初版 | 成熟 |

**注意**：MySQL 9.0 的 VECTOR 还不支持内置 ANN 向量索引，大规模向量检索性能不如专用向量数据库（Milvus/Qdrant）。

---

## 八、系统变量变更

### 新增

| 变量 | 说明 |
|------|------|
| `explain_json_format_version` | EXPLAIN JSON 输出格式版本 |
| `mle_java_heap_size` | MLE Java 堆大小（企业版） |
| `openid_connect_xxx` 系列 | OIDC 认证配置 |

### 废弃

| 变量 | 替代 |
|------|------|
| `log_bin_use_v1_row_events` | 强制使用 v2，已移除 |
| `transaction_write_set_extraction` | 默认 XXHASH64 |

### 默认值变化

| 变量 | 旧默认 | 新默认 |
|------|--------|--------|
| `binlog_transaction_dependency_tracking` | COMMIT_ORDER | WRITESET |
| `group_replication_consistency` | EVENTUAL | BEFORE_ON_PRIMARY_FAILOVER |

---

## 九、弃用项（预计后续版本移除）

| 弃用项 | 替代方案 |
|--------|---------|
| **mysql_native_password 插件** | caching_sha2_password |
| **SHA-1 相关函数和插件** | SHA-2 |
| `FLUSH PRIVILEGES` 以读锁方式执行 | 正常 FLUSH PRIVILEGES |
| `--skip-host-cache` 参数 | `host_cache_size=0` |
| **ReplicaSet**（MySQL InnoDB ReplicaSet） | InnoDB Cluster / ClusterSet |
| `max_digest_length=0` | 设正值 |

---

## 十、移除项

| 移除项 | 影响 |
|--------|------|
| **mysql_native_password** | 默认不加载，旧客户端需升级 |
| `--character-set-client-handshake` | 不再可关闭 |
| `keyring_aws` 插件 | 用 `keyring_aws_sdk` 替代 |
| `innodb_log_files_in_group` | 系统变量已移除 |
| `innodb_log_file_size` | 已被 `innodb_redo_log_capacity` 替代 |

---

## 十一、性能与优化器改进

### 1. 批量插入优化

```sql
-- 大批量 INSERT 性能提升约 10-15%
INSERT INTO large_table VALUES
  (1, 'a'), (2, 'b'), /* ... 数千行 */;
```

### 2. JSON 函数性能提升

```sql
-- JSON_EXTRACT 和 -> 操作符内部优化
SELECT JSON_EXTRACT(doc, '$.items[*].name') FROM json_table;
```

### 3. 子查询物化优化

优化器对某些子查询自动选择物化策略，减少重复执行。

### 4. InnoDB 改进

- 自适应哈希索引（AHI）重构，减少锁竞争
- Redo log 容量动态调整更稳定
- 并行索引创建（`innodb_parallel_read_threads`）

---

## 十二、复制增强

### 1. SOURCE_RETRY_COUNT 默认值变化

```
8.0：SOURCE_RETRY_COUNT = 86400（约 24 小时）
9.0：SOURCE_RETRY_COUNT = 10（更合理）
→ 网络故障 10 次后停止重连，避免长时间等待
```

### 2. 二进制日志事务压缩

```sql
-- 压缩 binlog，减少磁盘和网络占用
SET GLOBAL binlog_transaction_compression = ON;
SET GLOBAL binlog_transaction_compression_level_zstd = 3;
```

### 3. GTID 增强

- GTID 跳过事务更灵活
- `ASSIGN_GTIDS_TO_ANONYMOUS_TRANSACTIONS` 改进

---

## 十三、安全增强

| 改进 | 说明 |
|------|------|
| **OIDC 认证** | 支持标准 SSO |
| **FIPS 模式** | 更完善的 FIPS 140-2 合规 |
| **TLS 1.3 完善** | 支持更多密码套件 |
| **AES 硬件加速** | 利用 CPU AES-NI 指令集 |
| **审计日志增强** | 更细粒度的审计规则 |

---

## 十四、MySQL 8.0 → 8.4 → 9.0 演进总结

| 特性 | 8.0 LTS | 8.4 LTS | 9.0 创新版 |
|------|---------|---------|-----------|
| 发布模型 | 传统的 GA | LTS | Innovation |
| mysql_native_password | 默认支持 | 废弃 | **默认禁用** |
| 认证默认 | caching_sha2 | caching_sha2 | caching_sha2 + OIDC |
| JavaScript 存储过程 | ❌ | ❌ | ✅（企业版） |
| VECTOR 类型 | ❌ | ❌ | ✅ |
| EXPLAIN JSON 输出 | FORMAT=JSON | FORMAT=JSON | + ANALYZE + INTO |
| ReplicaSet | 支持 | 废弃 | 废弃 |
| GTID 默认 | OFF | ON | ON |
| binlog 压缩 | ❌ | 可选 | 更成熟 |
| 生产建议 | ✅ | ✅ | ❌（创新版） |

---

## 十五、升级建议

```
当前版本 → 目标版本：

8.0.x → 8.0.37 → 8.4 LTS（推荐的生产路径）
               ↘ 9.0（适合测试/学习，不推荐生产）

8.4 LTS → 保持不动 → 等 9.7 LTS

5.7 → 8.0.37 → 8.4 LTS（不要直接跳 9.0）

升级前必须检查：
  ✅ 所有用户改为 caching_sha2_password
  ✅ 客户端驱动升级到 mysql 8.0+ connector
  ✅ 废弃功能不用了
  ✅ mysqlcheck --check-upgrade 跑一遍
```

---

## 面试要点

**Q: MySQL 9.0 最大的变化是什么？**
> 双轨发布模型（Innovation vs LTS）+ 移除 mysql_native_password + 新增 VECTOR 类型 + JavaScript 存储程序（企业版）

**Q: 为什么不建议生产用 9.0？**
> 9.0 是创新版，只维护到 9.1 发布。生产用 8.0.x 或 8.4 LTS，支持到 2032 年。

**Q: VECTOR 类型能替代专用向量数据库吗？**
> 不能。MySQL VECTOR 目前不支持 ANN 索引，大规模向量检索性能远不如 Milvus/Qdrant。适合小规模 RAG + 已有 MySQL 基础设施不想多部署一套的场景。
