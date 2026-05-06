# MySQL Online DDL 三种算法详解：COPY / INPLACE / INSTANT

---

## 一、三种算法概览

| 算法 | 原理 | 是否阻塞 DML | 空间需求 | 速度 | 引入版本 |
|------|------|:---:|------|------|------|
| **COPY** | 全表复制 | ❌ 阻塞（早期版本）/ 允许（Online） | 额外 ~1 倍空间 | 最慢 | 5.5- |
| **INPLACE** | 原地重建 | ✅ 不阻塞 | 少量额外空间（临时文件） | 快 | 5.6+ |
| **INSTANT** | 仅改元数据 | ✅ 不阻塞 | 无 | 极快（秒级） | 8.0+ |

---

## 二、COPY 算法

### 原理

```
┌──────────┐                  ┌──────────┐
│  原表 A   │ ───逐行拷───▶  │  临时表 B  │
│  (旧结构) │                  │  (新结构)  │
└──────────┘                  └──────────┘
                                    │
                              DDL 执行期间
                              增量写入 redo log
                                    │
                                    ▼
                              重放增量数据
                                    │
                                    ▼
                              原子 rename
                              A ← B
```

### 详细流程

1. **创建临时表** —— 按新结构创建一个隐式临时表（`#sql-xxx`）
2. **加共享锁** —— 对原表加 S 锁（`LOCK=NONE` 时不做，`LOCK=SHARED` 时加）
3. **全量拷贝** —— 逐行从原表读到内存 → 按新结构写进临时表
4. **增量重放** —— 拷贝期间原表产生的 DML 写进 redo log，拷贝完成后重放到临时表
5. **原子切换** —— `RENAME TABLE` 原子地把临时表替换原表
6. **删除旧表**

### 什么时候走 COPY

- MySQL 5.5 及之前：**所有 DDL 都走 COPY**
- MySQL 5.6+：INPLACE 不支持时降级为 COPY，比如：
  - `ALTER TABLE ... ENGINE=MyISAM`（跨引擎）
  - `OPTIMIZE TABLE`（实际上是重建表）
  - 修改列类型导致数据必须转换（如 `VARCHAR(50) → VARCHAR(200)` 要扩展空间）
  - 删除列的主键属性（涉及聚簇索引重建）

### COPY 的痛点

```
空间：峰值需要原表 2 倍磁盘空间
时间：大表可能跑几小时甚至几天
IO：全表读写各一遍 + 增量重放
阻塞：虽然 Online，但性能下降严重
```

---

## 三、INPLACE 算法

### 原理

INPLACE 不是真的"原位不动"，而是**在同一个表空间内完成异构操作**，不创建独立的临时表。

```
原表文件 (t.ibd)
    │
    ├── 旧索引 (B+Tree)       ← 逐页读取
    │
    ├── 临时排序缓冲区 (tmp)   ← sort_buffer / tmpdir
    │
    ├── 新索引 (B+Tree)        ← 逐页写入新索引
    │
    └── row_log (增量 DML 记录)  ← 建索引期间的 DML 记录在这里
```

### 详细流程（以 `ADD INDEX` 为例）

```
Phase 1: 准备 (Prepare)
├── 获取 MDL（metadata lock）
├── 在内存中创建新索引结构定义
├── 初始化 row_log（记录并发 DML）
└── 释放 MDL，允许 DML

Phase 2: 构建 (Build) ← 最耗时
├── 扫描聚簇索引（主键）
├── 提取索引列的值
├── 排序（如果必要）
├── 批量构建 B+Tree
└── DML 产生的变更同时写入 row_log

Phase 3: 提交 (Commit)
├── 获取 MDL（短暂阻塞）
├── 重放 row_log 到新索引
├── 原子提交，新索引生效
└── 释放 MDL
```

### 关键机制：row_log

INPLACE 在构建阶段不阻塞 DML，靠的是 row_log 机制：

```
时间线:
  ────────────────────────────────────────────────────▶
  
  DML:  INSERT x ──── UPDATE y ────── DELETE z ────
         │              │                 │
         ▼              ▼                 ▼
  row_log: [rec1]  [rec2, rec3]      [rec4]
         │
         ▼ (commit 阶段)
  重放到新索引中
```

**row_log 大小限制：** `innodb_online_alter_log_max_size` 控制（默认 128MB）。如果 DML 太密集导致 row_log 满了，DDL 会失败回滚。

### INPLACE 适用的场景

- `ADD INDEX` / `CREATE INDEX`（最经典）
- `DROP INDEX`
- `ALTER TABLE ... ADD COLUMN`（部分场景 8.0 走 INSTANT）
- `ALTER TABLE ... MODIFY COLUMN`（部分场景）
- `ALTER TABLE ... AUTO_INCREMENT=xxx`
- 修改列默认值

### INPLACE 不适用（会回退到 COPY）

- 修改列类型导致存储格式变化（`INT → BIGINT`）
- 重排列表序（`MODIFY COLUMN` 改变了列位置）
- 修改字符集（`utf8 → utf8mb4` 需要扩展存储）
- 删除主键

---

## 四、INSTANT 算法 ⭐

### 原理

INSTANT 是 8.0 的革命，**只修改表的元数据（数据字典），不碰任何数据页**。

```
修改前                                  修改后
┌──────────────────────┐     ┌──────────────────────┐
│ MySQL Data Dictionary │     │ MySQL Data Dictionary │
│                      │     │                      │
│ column: id, name     │ ──▶ │ column: id, name,     │
│ n_cols = 2           │     │         email         │
│                      │     │ n_cols = 3            │
└──────────────────────┘     └──────────────────────┘

表数据文件 (.ibd) — 完全没有动！
```

### 怎么做到不改数据页？

**核心设计：InnoDB 8.0 在行头中引入了 `info_bits` 和 `instant bit`**

```
行记录的物理格式:

┌──────────────────────────────────────────────────┐
│ 头部 (Header)                                     │
│  ├── info_bits: 标识字段                          │
│  ├── instant_flag: 该表是否是 INSTANT 修改的      │
│  └── n_instant_cols: INSTANT 添加列的个数         │
├──────────────────────────────────────────────────┤
│ 列数据 (Column Data)                              │
│  ├── col_1: 值（原始列）                          │
│  ├── col_2: 值（原始列）                          │
│  └── col_3: 值（INSTANT 加的列，可能为空或默认值） │
└──────────────────────────────────────────────────┘
```

**读取时：** InnoDB 知道数据页上的行可能有"缺失列"（INSTANT 加的），读取时自动补上默认值或 NULL。

### 详细流程

```
Phase 1: 元数据更新 (毫秒级)
├── 获取 MDL
├── 更新数据字典中表的列定义
├── 记录 `n_instant_cols` 和 `instant_col_defaults`
├── 释放 MDL
└── 完成！不 touch 任何数据行

Phase 2: 后续访问的适配
├── 读旧行时：自动补齐 INSTANT 列为默认值/NULL
├── 新插入的行：按新结构完整写入
├── UPDATE 旧行时：触发行重建，填入 INSTANT 列的真实值
```

### INSTANT 适用场景

| 版本 | 支持的操作 |
|------|-----------|
| **MySQL 8.0.12** | `ADD COLUMN`（最后位置）、`ADD/DROP VIRTUAL COLUMN` |
| **MySQL 8.0.29** | `ADD COLUMN` 支持指定任意位置（`AFTER col`, `FIRST`） |
| **MySQL 8.0.29** | `DROP COLUMN` |
| **MySQL 8.0.29** | 列重命名（`RENAME COLUMN`） |

### INSTANT 的限制

- ⚠️ 一个表最多做 **64 次 INSTANT ADD/DROP COLUMN**（8.0.29 之前的限制）
- ⚠️ 表不能是旧格式（用 `ALTER TABLE ... FORCE` 重建后重置计数）
- ⚠️ `DROP COLUMN` 在 8.0.29+ 才支持 INSTANT
- ❌ 删除的列是主键的一部分 → 不行
- ❌ 修改列类型 → 不行
- ❌ 修改列顺序（非 ADD/DROP）→ 不行

**查看 INSTANT 计数：**
```sql
SELECT NAME, INSTANT_COLS 
FROM information_schema.innodb_tables 
WHERE NAME LIKE '%your_table%';
```

---

## 五、三种算法的选择策略

```sql
-- 显式指定算法
ALTER TABLE t ADD COLUMN email VARCHAR(100), ALGORITHM=INSTANT;
ALTER TABLE t ADD INDEX idx_name (name), ALGORITHM=INPLACE;
ALTER TABLE t MODIFY COLUMN name VARCHAR(200), ALGORITHM=COPY;

-- 指定不允许哪种算法
ALTER TABLE t ADD COLUMN email VARCHAR(100), ALGORITHM=INSTANT;  
-- 如果 INSTANT 不行就报错，不降级

-- 配合锁级别
ALTER TABLE t ADD INDEX idx_name (name), ALGORITHM=INPLACE, LOCK=NONE;
```

### 决策流程

```
你想做 DDL
    │
    ▼
支持 INSTANT 吗？
    ├── 是 → ALGORITHM=INSTANT   （秒级，零影响）✅
    └── 否 ↓
支持 INPLACE 吗？
    ├── 是 → ALGORITHM=INPLACE   （分-时级，允许 DML）✅
    └── 否 ↓
只能 COPY 了
    └── ALGORITHM=COPY          （时-天级，大表慎用）⚠️
        └── 考虑用 pt-online-schema-change 替代
```

---

## 六、在线 DDL 的锁级别

| LOCK | 含义 |
|------|------|
| `NONE` | 完全不阻塞 DML ✅ |
| `SHARED` | 允许读，阻塞写 |
| `DEFAULT` | 让 MySQL 自己选 |
| `EXCLUSIVE` | 阻塞所有 DML ❌ |

**INSTANT → 只需要元数据锁（极短，毫秒级）**
**INPLACE → 准备和提交阶段短暂 MDL，构建阶段无锁**
**COPY → 历史上阻塞，现在可以 NONE（但性能影响大）**

---

## 七、实战：如何判断一个 DDL 走哪个算法（不用真的跑）

```sql
-- 模拟执行，看会用什么算法（不真实执行）
-- 但 MySQL 没有 EXPLAIN DDL，得自己判断：

-- 1. 检查 INSTANT 是否可用
--    问：是否是 ADD COLUMN(8.0.12+)? 是否超过 64 次限制?
--    
-- 2. 检查 INPLACE 是否可用
--    问：是否只是 ADD/DROP INDEX? 是否需要类型转换?
--
-- 3. 看官方文档矩阵：
--    https://dev.mysql.com/doc/refman/8.0/en/innodb-online-ddl-operations.html

-- 实际上只能试探：
ALTER TABLE t ADD COLUMN col INT, ALGORITHM=INSTANT;
-- 如果成功 → INSTANT 可用
-- 如果报错 "ALGORITHM=INSTANT is not supported" → 换 INPLACE
ALTER TABLE t ADD COLUMN col INT, ALGORITHM=INPLACE;
-- 如果报错 → 只能 COPY
```

---

## 八、pt-online-schema-change（COPY 的救星）

当 COPY 不可避免时，用 Percona 的 `pt-osc` 替代：

```bash
pt-online-schema-change \
  --alter "MODIFY COLUMN name VARCHAR(500)" \
  --execute \
  h=localhost,D=test,t=users
```

**原理：** 创建影子表 → 触发器同步增量 → 分批复制 → 原子切换。全程不阻塞，但比原生 COPY 更快、影响更小。

---

📝 **一句话总结：** INSTANT 改字典，INPLACE 原地重建，COPY 全表复制。能用 INSTANT 绝不用 INPLACE，能用 INPLACE 绝不用 COPY。

---

Add by chatGPT:
MySQL 在 InnoDB 在线建索引时，通过“全量构建 + 增量日志”的方式保证一致性：

1. 先扫描聚簇索引，对已有数据进行排序并批量构建新索引（bulk load）
2. 在构建过程中，所有 DML 操作不会直接更新新索引，而是记录到 row log（online DDL log）
3. 全量构建完成后，再将 row log 中的增量操作应用到新索引
4. 最后通过短暂锁切换索引，保证一致性

这种方式避免了随机写带来的性能问题，同时保证了在线 DDL 期间的数据一致性


👉 面试官会问：
如果在构建过程中，row log 非常大，会发生什么？

你要答：
👉
MySQL 会限制 row log 大小
超过阈值后，会退化为 COPY 算法（锁表重建）
避免内存或临时空间爆炸

