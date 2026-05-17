# DuckDB 嵌入 MySQL：开发学习路线

> 将 DuckDB 作为 MySQL 存储引擎集成——从零到一的全链路学习计划

---

## 一、前置认知：这到底在做什么

### 同类项目参考

| 项目 | 说明 | 代码量 | 状态 |
|------|------|--------|------|
| **MyRocks** | Facebook：RocksDB 作 MySQL 引擎 | ~3 万行 | 生产级 |
| **MyDuck** | 创业项目：DuckDB 作 MySQL 分析引擎 | ~2 万行 | 早期 |
| **HeatWave** | Oracle：MySQL 内置分析引擎 | 闭源 | 商用 |
| **pg_duckdb** | DuckDB 嵌入 PostgreSQL | ~1 万行 | 活跃 |
| **duckdb-mysql** | DuckDB 官方 MySQL scanner（反向） | ~5000 行 | 官方 |

**你要做的本质：**

```
MySQL Server
  └── Handler API（存储引擎接口）
        └── ha_duckdb（你的自定义引擎） ← 核心
              └── DuckDB C++ Library
                    └── DuckDB 数据库文件
```

### 核心挑战

```
MySQL 侧：
  - Handler API 是行式接口（逐行读写）
  - 事务、锁、MVCC 由引擎层负责
  - 需要支持 InnoDB 兼容的 DDL/DML

DuckDB 侧：
  - 列式存储，批量向量化执行
  - 事务模型跟 MySQL 不完全一致
  - 不支持 MySQL 的全部数据类型

阻抗不匹配 ← 这是最难的部分
```

---

## 二、先修知识清单（按顺序学）

### 阶段 0：C/C++ 基础（如果是弱项）

```
时间：2-4 周（因人而异）
重点：
  ✅ C++ 类、继承、虚函数
  ✅ 内存管理（智能指针、RAII）
  ✅ 模板、STL（vector/map/string）
  ✅ CMake 构建系统
  ✅ GDB 调试
```

### 阶段 1：MySQL 内核入门（2-3 周）

```
目标：理解一条 SQL 从网络协议到存储引擎的全路径
```

**必读源码（按阅读顺序）：**

```
sql/sql_parse.cc          — SQL 解析入口
sql/sql_select.cc          — SELECT 执行流程
sql/handler.h              — 存储引擎 Handler 接口定义 ⭐
sql/handler.cc             — Handler 实现逻辑
storage/example/ha_example.cc  — 官方示例引擎 ⭐ 入门起点
sql/sql_table.cc           — CREATE TABLE / DDL
```

**核心概念：**

| 概念 | 位置 | 作用 |
|------|------|------|
| **handler** | `sql/handler.h` | 引擎抽象基类，定义 ~80 个虚函数 |
| **ha_xxx** | `storage/xxx/` | your handler implementation |
| **THD** | `sql/sql_class.h` | 线程描述符，每个连接一个 |
| **TABLE** | `sql/table.h` | 打开的表结构 |
| **Field** | `sql/field.h` | 列描述符 |
| **key_range** | `sql/key.h` | 索引范围扫描参数 |

**实践：**
```bash
# 编译 MySQL 源码
git clone https://github.com/mysql/mysql-server.git
cd mysql-server
mkdir build && cd build
cmake .. -DDOWNLOAD_BOOST=1 -DWITH_BOOST=/tmp/boost
make -j$(nproc)

# 看 example 引擎
ls storage/example/
# ha_example.cc  ha_example.h
```

---

### 阶段 2：DuckDB 内部机制（3-4 周）

```
目标：理解 DuckDB 存储层和查询引擎
```

**必读源码：**

```
src/main/database.cpp      — 数据库初始化
src/main/connection.cpp    — 连接管理
src/storage/storage_manager.cpp  — 存储管理器
src/storage/data_table.cpp — 表操作
src/execution/operator/    — 算子实现
src/include/duckdb.hpp     — 主头文件
```

**核心 API 你需要掌握的：**

```cpp
#include <duckdb.hpp>

// 1. 打开/创建数据库
duckdb::DuckDB db("/path/to/mydb.duckdb");
duckdb::Connection conn(db);

// 2. 执行 SQL
auto result = conn.Query("CREATE TABLE t (id INTEGER, name VARCHAR)");
conn.Query("INSERT INTO t VALUES (1, 'Alice')");

// 3. 遍历结果（这个是高频操作，性能关键）
auto result = conn.Query("SELECT * FROM t");
// result->collection.ChunkCount()  块数
// result->collection.GetChunk(i)   取第 i 个 DataChunk

// 4. DataChunk 操作（列式批量）
for (idx_t i = 0; i < result->collection.ChunkCount(); i++) {
    auto &chunk = result->collection.GetChunk(i);
    // chunk.data[0] → Vector of column 0
    // chunk.size() → row count in this chunk
}

// 5. 从 Vector 取数据
auto &vec = chunk.data[0];  // 第 0 列
auto data = FlatVector::GetData<int32_t>(vec);
for (idx_t j = 0; j < chunk.size(); j++) {
    int32_t value = data[j];
}
```

**学习路径：**
```
1. 用 DuckDB C API 写几个小程序（1 天）
2. 读 src/main/connection.cpp → 理解查询生命周期（2 天）
3. 读 src/storage/ → 理解表和数据页（3 天）
4. 读 src/execution/ → 理解向量化执行引擎（5 天）
5. 写 benchmark → 对比 DuckDB 直接调 vs MySQL handler（3 天）
```

---

### 阶段 3：存储引擎 API 深入（2 周）

MySQL Handler API 是你需要实现的核心接口。`handler` 是一个抽象类，约 80 个虚函数，你至少要实现以下关键函数：

```cpp
// ===== 必须实现的 Handler 虚函数 =====

// -- 基础 --
int open(const char *name, int mode, uint test_if_locked);  // 打开表
int close(void);                                              // 关闭表
int rnd_init(bool scan);                                     // 全表扫描初始化
int rnd_next(uchar *buf);                                    // 全表扫描下一行 ⭐
int rnd_pos(uchar *buf, uchar *pos);                         // 按位置读取
void position(const uchar *record);                          // 记录当前位置

// -- 索引 --
int index_init(uint idx, bool sorted);                       // 索引扫描初始化
int index_read(uchar *buf, const uchar *key, ...);           // 索引等值查询 ⭐
int index_next(uchar *buf);                                  // 索引扫描下一行
int index_read_last(uchar *buf, const uchar *key, ...);      // 索引范围结束

// -- 写入 --
int write_row(uchar *buf);                                   // 插入行 ⭐
int update_row(const uchar *old_data, uchar *new_data);     // 更新行 ⭐
int delete_row(const uchar *buf);                            // 删除行 ⭐

// -- 元数据 --
int create(const char *name, TABLE *form, HA_CREATE_INFO *info);  // 建表
int delete_table(const char *name);                                 // 删表
ulonglong table_flags() const;                                      // 引擎特性
const char *table_type() const;                                     // 引擎名字
```

**核心难点：行式 ↔ 列式转换**

```cpp
// MySQL 给的是行式 buffer → 你要存到 DuckDB 列式存储
// 这是你要写的核心代码

int ha_duckdb::write_row(uchar *buf) {
    // MySQL 调用你，buf 是一行数据的 MySQL 格式
    
    // 1. 从 buf 中提取各列的值
    std::vector<std::string> values;
    for (uint i = 0; i < table->s->fields; i++) {
        Field *field = table->field[i];
        String val;
        field->val_str(&val);  // 转为字符串
        values.push_back(std::string(val.ptr(), val.length()));
    }
    
    // 2. 拼装 DuckDB INSERT SQL（简单方案）
    //    或使用 DuckDB Appender API（高性能方案）
    std::string sql = "INSERT INTO " + table_name + " VALUES (";
    for (size_t i = 0; i < values.size(); i++) {
        if (i > 0) sql += ", ";
        sql += "'" + values[i] + "'";  // 生产环境要做 SQL 注入防护
    }
    sql += ")";
    
    duckdb_conn->Query(sql);
    return 0;
}
```

---

## 四、分阶段开发计划

### Phase 1：最小可行引擎（4 周）

**目标**：MySQL 能 `CREATE TABLE ... ENGINE=DUCKDB` 并读写数据

```
Week 1: 搭框架
  - 从 storage/example/ 复制模板
  - 改名为 storage/duckdb/
  - 链接 DuckDB 静态库
  - 实现 create() / open() / close()

Week 2: 实现全表扫描
  - rnd_init() / rnd_next() / rnd_pos()
  - 把 DuckDB SELECT 结果逐行喂给 MySQL
  - MySQL 端能 SELECT * FROM duckdb_table

Week 3: 实现写入
  - write_row() / delete_row() / update_row()
  - 处理字段类型转换
  - MySQL 端能 INSERT/UPDATE/DELETE

Week 4: 测试 + 修复
  - 基本 CRUD 自动化测试
  - 性能对比（vs InnoDB for simple queries）
```

**开发环境准备：**

```bash
# 1. 安装 DuckDB 开发库
git clone https://github.com/duckdb/duckdb.git
cd duckdb
make -j$(nproc)
# 库文件：build/release/src/libduckdb.a
# 头文件：src/include/duckdb.hpp

# 2. 创建引擎目录
cd mysql-server/storage/
mkdir duckdb
cp -r example/* duckdb/
mv duckdb/ha_example.cc duckdb/ha_duckdb.cc
mv duckdb/ha_example.h duckdb/ha_duckdb.h

# 3. 修改 CMakeLists.txt
# 加入 DuckDB 的 include 路径和库链接
```

**CMakeLists.txt 关键部分：**

```cmake
# storage/duckdb/CMakeLists.txt
SET(DUCKDB_ROOT "/path/to/duckdb")

INCLUDE_DIRECTORIES(${DUCKDB_ROOT}/src/include)
LINK_DIRECTORIES(${DUCKDB_ROOT}/build/release/src)

MYSQL_ADD_PLUGIN(duckdb
    ha_duckdb.cc
    STORAGE_ENGINE
    LINK_LIBRARIES duckdb pthread
)
```

### Phase 2：索引支持（3 周）

```
DuckDB 目前不支持传统 B-Tree 索引。
你要创建一个「影子索引」机制：

方案 A（简单）：
  - 用一个独立的内存 B-Tree（如 std::map 或 tlx/btree）
  - 作为 DuckDB 行号 → 主键/索引 key 的映射表
  - 数据仍存 DuckDB，索引存 MySQL 侧

方案 B（高效）：
  - 用 DuckDB 内部 table 按索引 key 排序存储
  - 利用列式排序后的二分查找
  - 需要理解 DuckDB 的 RowGroup 布局
```

```cpp
int ha_duckdb::index_read(uchar *buf, const uchar *key, ...) {
    // 1. 从 key 中提取索引值
    uint idx_value = *(uint*)key;
    
    // 2. 从影子索引查对应的 DuckDB rowid
    int rowid = shadow_index[idx_value];
    
    // 3. 从 DuckDB 按 rowid 取行
    auto result = conn->Query(
        "SELECT * FROM " + table_name + " WHERE rowid = " + std::to_string(rowid)
    );
    
    // 4. 返回给 MySQL
    fill_buffer_from_result(buf, result);
    return 0;
}
```

### Phase 3：事务支持（3 周）

```
DuckDB 的事务模型和 MySQL 不同：
  - DuckDB：单写多读，快照隔离
  - MySQL：需要支持 BEGIN/COMMIT/ROLLBACK

实现策略：
  1. 用 DuckDB 的 Catalog + Schema 机制
  2. 每个 MySQL 事务对应一个 DuckDB 快照
  3. 写操作暂存在 WAL/Temp 表中
  4. COMMIT 时 apply 到主表
```

```cpp
// 简化示例
class DuckDBTransaction {
    duckdb::Connection *conn;
    std::string temp_table;  // 暂存写操作
    bool active;
    
    void commit() {
        conn->Query("INSERT INTO main_table SELECT * FROM " + temp_table);
        conn->Query("DROP TABLE " + temp_table);
    }
    
    void rollback() {
        conn->Query("DROP TABLE " + temp_table);
    }
};
```

### Phase 4：SQL 下推优化（4 周）

```
这是发挥 DuckDB 列式优势的关键一步。

目标：MySQL 尽量把查询下推到 DuckDB 执行
  - WHERE 条件下推
  - GROUP BY / ORDER BY 下推
  - 聚合函数下推
  - JOIN 能不能下推？

这个阶段的代码在 handler::cond_push() 等函数中
```

```cpp
// 条件推送接口
const COND *ha_duckdb::cond_push(const COND *cond) {
    // MySQL 会把 WHERE 条件传给你
    // 你可以转换成 DuckDB SQL 的一部分
    // 返回 NULL 表示全部下推，返回 cond 表示未下推部分
    
    pushed_condition = convert_mysql_cond_to_duckdb(cond);
    return nullptr;  // 全部下推
}

int ha_duckdb::rnd_init(bool scan) {
    // 带上之前下推的条件
    std::string sql = "SELECT * FROM " + table_name;
    if (!pushed_condition.empty()) {
        sql += " WHERE " + pushed_condition;
    }
    current_result = duckdb_conn->Query(sql);
    current_chunk_idx = 0;
    current_row_in_chunk = 0;
    return 0;
}
```

---

## 五、完整学习计划时间表

| 阶段 | 内容 | 时间 | 产出 |
|------|------|------|------|
| **先修** | C++ 强化 | 2-4 周 | 能读 MySQL 源码 |
| **阶段 1** | MySQL 内核入门 | 2-3 周 | 理解 Handler API |
| **阶段 2** | DuckDB 内部机制 | 3-4 周 | 能用 DuckDB C API |
| **阶段 3** | Handler API 深入 | 2 周 | 实现关键接口 |
| **Phase 1** | 最小可行引擎 | 4 周 | CRUD 可用 |
| **Phase 2** | 索引支持 | 3 周 | 索引扫描可用 |
| **Phase 3** | 事务支持 | 3 周 | BEGIN/COMMIT/ROLLBACK |
| **Phase 4** | SQL 下推优化 | 4 周 | 分析查询加速 |
| **Phase 5** | 线上化打磨 | 4 周 | 稳定 + 测试 |

```
总计：约 6-8 个月（全职），12+ 个月（兼职）

最短路径（只做 MVP）：
  阶段 1+2+3 + Phase 1 = 12 周 ≈ 3 个月
```

---

## 六、关键资源清单

### 必读

```
1. MySQL Internals Manual（官方）
   https://dev.mysql.com/doc/internals/en/

2. Understanding MySQL Internals (O'Reilly 书)

3. DuckDB 源码阅读指南
   https://duckdb.org/docs/internals/overview

4. pg_duckdb 源码（最佳参考）⭐
   https://github.com/duckdb/pg_duckdb
   → DuckDB 嵌入 PG 的实现，架构最接近你的目标

5. MyRocks 源码
   https://github.com/facebook/mysql-5.6
   storage/rocksdb/

6. MySQL Handler API 官方文档
   https://dev.mysql.com/doc/refman/8.0/en/pluggable-storage-overview.html
```

### 社区

```
- MySQL Internals mailing list
- DuckDB Discord: https://discord.gg/duckdb
- 中文社区：Greenplum/MySQL 内核交流群
```

---

## 七、给你避坑的十句话

```
1. 不要一上来就写代码，先跑通 example 引擎
2. table_flags() 的返回值是关键，决定 MySQL 对你的信任程度
3. 字段类型转换（Field → DuckDB Value）是最容易出 bug 的环节
4. rnd_next() 的性能直接决定 SELECT * 的速度，用 DataChunk 批量取
5. DuckDB 不支持行级锁，你需要自己模拟（乐观锁）
6. 先做全表扫描，再做索引，最后做事务——递进开发
7. 参考 pg_duckdb 而不是 MyDuck（代码质量差别大）
8. MySQL 8.0 的 Handler API 比 5.7 多了很多接口，以 8.0 为目标
9. 别试图 100% 兼容 InnoDB 特性（外键、全文索引等），先做核心功能
10. 先跑通 EXPLAIN + SELECT + INSERT，能在屏幕上看到数据就成功了 50%
```

---

## 八、一个可以立刻跑的 Hello World

```cpp
// storage/duckdb/ha_duckdb.h
#pragma once
#include "sql/handler.h"
#include "duckdb.hpp"

class ha_duckdb : public handler {
public:
    ha_duckdb(handlerton *hton, TABLE_SHARE *table_arg);
    ~ha_duckdb();

    // 必须实现
    int create(const char *name, TABLE *form, HA_CREATE_INFO *create_info) override;
    int open(const char *name, int mode, uint test_if_locked) override;
    int close(void) override;
    int rnd_init(bool scan) override;
    int rnd_next(uchar *buf) override;
    int write_row(uchar *buf) override;
    int update_row(const uchar *old_data, uchar *new_data) override;
    int delete_row(const uchar *buf) override;
    
    // 元数据
    const char *table_type() const override { return "DUCKDB"; }
    ulonglong table_flags() const override;
    
private:
    std::unique_ptr<duckdb::DuckDB> duckdb_db;
    std::unique_ptr<duckdb::Connection> duckdb_conn;
    std::unique_ptr<duckdb::QueryResult> current_result;
    idx_t current_chunk_idx;
    idx_t current_row_in_chunk;
    std::string table_name;
    std::string data_path;
};
```

```cpp
// storage/duckdb/ha_duckdb.cc（核心骨架）
#include "ha_duckdb.h"

// 新建表
int ha_duckdb::create(const char *name, TABLE *form, HA_CREATE_INFO *create_info) {
    // 1. 构建 CREATE TABLE SQL
    std::string sql = "CREATE TABLE " + std::string(name) + " (";
    for (uint i = 0; i < form->s->fields; i++) {
        Field *f = form->field[i];
        if (i > 0) sql += ", ";
        sql += f->field_name;
        sql += " ";
        sql += mysql_type_to_duckdb(f->type());  // 你要实现的类型映射
    }
    sql += ")";
    
    // 2. 在 DuckDB 中执行
    auto conn = duckdb::Connection(*duckdb_db);
    auto result = conn.Query(sql);
    return result->HasError() ? HA_ERR_GENERIC : 0;
}

// 打开表
int ha_duckdb::open(const char *name, int mode, uint test_if_locked) {
    table_name = name;
    duckdb_db = std::make_unique<duckdb::DuckDB>(data_path, nullptr);
    duckdb_conn = std::make_unique<duckdb::Connection>(*duckdb_db);
    return 0;
}

// 全表扫描下一行
int ha_duckdb::rnd_next(uchar *buf) {
    if (!current_result || current_result->HasError()) {
        return HA_ERR_END_OF_FILE;
    }
    
    auto &collection = current_result->Collection();
    
    // 检查是否有更多行
    if (current_chunk_idx >= collection.ChunkCount()) {
        return HA_ERR_END_OF_FILE;
    }
    
    auto &chunk = collection.GetChunk(current_chunk_idx);
    
    // 清除 MySQL buffer 的 NULL bitmap
    memset(buf, 0, table->s->null_bytes);
    
    // 填充各列
    for (idx_t col = 0; col < chunk.ColumnCount(); col++) {
        Field *field = table->field[col];
        auto &vec = chunk.data[col];
        
        // 从列式 Vector 取第 current_row 个值 → 写回 MySQL 行式 buffer
        store_duckdb_value_to_field(field, buf, vec, current_row_in_chunk);
    }
    
    current_row_in_chunk++;
    if (current_row_in_chunk >= chunk.size()) {
        current_chunk_idx++;
        current_row_in_chunk = 0;
    }
    
    return 0;
}

// 插入行
int ha_duckdb::write_row(uchar *buf) {
    std::string sql = "INSERT INTO " + table_name + " VALUES (";
    for (uint i = 0; i < table->s->fields; i++) {
        Field *f = table->field[i];
        if (i > 0) sql += ", ";
        // 从 MySQL buffer 取字段值转为 SQL 字面量
        sql += field_to_sql_literal(f, buf);
    }
    sql += ")";
    
    auto result = duckdb_conn->Query(sql);
    return result->HasError() ? HA_ERR_GENERIC : 0;
}
```

---

## 九、如果不想从零写——替代方案

| 方案 | 难度 | 适用 |
|------|------|------|
| **完全手写引擎** | ⭐⭐⭐⭐⭐ | 学习内核、深入理解 |
| **基于 MyDuck 二次开发** | ⭐⭐⭐ | 快速出产品 |
| **MySQL + DuckDB FDW** | ⭐⭐ | 利用现有工具 |
| **DuckDB 查询 MySQL** | ⭐ | 反向集成（已有官方 scanner） |

**推荐策略**：先用官方 `duckdb-mysql` 或 `mysql-duckdb-fdw` 理解两者的桥接模式，再决定要不要手写引擎。
