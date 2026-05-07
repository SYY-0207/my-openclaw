# 数据库面试 Q&A — 第四轮：架构、存储引擎、日志系统

---

## Q31: InnoDB 和 MyISAM 有什么区别？

| 特性 | InnoDB | MyISAM |
|------|--------|--------|
| 事务 | ✅ ACID | ❌ |
| 行级锁 | ✅ | ❌ 仅表锁 |
| MVCC | ✅ | ❌ |
| 外键 | ✅ | ❌ |
| 崩溃恢复 | ✅ Redo + Undo | ❌ 需修复表 |
| 聚簇索引 | ✅ | ❌ 堆表 |

InnoDB 成为默认原因：ACID、行锁高并发、崩溃自动恢复。MyISAM 崩溃后需 REPAIR TABLE。

---

## Q32: MySQL 一条 SQL 的执行流程？

```
客户端 → 连接器 → 查询缓存(8.0已移除) → 分析器 → 优化器 → 执行器 → 存储引擎
```

- 分析器：词法分析 → 语法分析(AST)
- 优化器：选索引、定 JOIN 顺序、生成执行计划
- 执行器：调用存储引擎接口获取数据

Oracle：解析 → CBO 优化 → 执行，库缓存存储解析计划。
PostgreSQL：解析 → 重写(规则系统) → 优化(CBO) → 执行。

---

## Q33: 什么是 Buffer Pool？

InnoDB 内存中缓存数据页和索引页的区域。所有读写先经过它。

LRU 变种：新页先放 old 区（避免全表扫描刷掉热数据），再次访问才移到 young 区。

配置：通常设物理内存 50-80%（`innodb_buffer_pool_size`）。

- PG `shared_buffers` 通常设 25%（还依赖 OS page cache）
- Oracle Buffer Cache 是 SGA 一部分（`DB_CACHE_SIZE`）

---

## Q34: 什么是 WAL（Write-Ahead Logging）？

先写日志，再写数据。数据页修改必须先记录日志，日志落盘后才刷数据页。

原因：崩溃恢复、顺序写性能（COMMIT 只等日志落盘）、数据页延迟批量刷盘。

| MySQL | PG | Oracle |
|-------|-----|--------|
| Redo Log | WAL | Online Redo Log |

---

## Q35: Redo Log 和 Binlog 有什么区别？

| | Redo Log | Binlog |
|------|------|------|
| 层级 | InnoDB 引擎层 | MySQL Server 层 |
| 内容 | 物理日志(页上偏移量改动) | 逻辑日志(SQL语句) |
| 用途 | 崩溃恢复 | 主从复制+数据恢复 |
| 写入 | 循环写 | 追加写 |

两阶段提交（XA）保证一致性：Prepare(写Redo) → Commit(写Binlog → 标记Redo commit)。

对比：Oracle 只有 Redo Log（无 Server 层需求），PG 只有 WAL（可配 logical 做复制）。

---

## Q36: 什么是 Checkpoint？为什么需要它？

将 Buffer Pool 脏页批量刷新到磁盘。

目的：
1. 加速崩溃恢复（Checkpoint 之前的 Redo 可丢弃）
2. Redo Log 空间回收（循环使用）
3. 控制脏页比例

触发：MySQL `innodb_max_dirty_pages_pct`，PG `checkpoint_timeout`，Oracle `FAST_START_MTTR_TARGET`。

---

## Q37: Change Buffer 是什么？

对非唯一二级索引的写缓存。索引页不在 Buffer Pool 时，修改先记录到 Change Buffer，后续 Merge。

适用：批量插入辅助索引（如 `idx_user_id`），写多读少的索引。

不适用：唯一索引（需立即检查唯一性）、即写即读的索引。

---

## Q38: 什么是 Adaptive Hash Index（AHI）？

InnoDB 在高频等值查询时自动在内存构建哈希索引，完全自动，无需管理。

开关：`innodb_adaptive_hash_index`。

---

## Q39: PostgreSQL VACUUM 是什么？

PG 旧版本在数据页 tuple 中（无独立 undo），需 VACUUM 清理死元组：
1. 标记死元组空间可复用
2. 更新 visibility map（Index-Only Scan 用）
3. 冻结事务 ID（防 wraparound）

```sql
VACUUM t;           -- 手动清理（不锁表）
VACUUM FULL t;      -- 整理碎片+回收空间（锁表）
```

每个 tuple 有 xmin/xmax 标记版本，代价是需要 VACUUM 定期清理。

---

## Q40: Oracle SGA 和 PGA 分别是什么？

**SGA（共享内存）：**
| 组件 | 作用 |
|------|------|
| Database Buffer Cache | 缓存数据块 |
| Shared Pool | 库缓存+数据字典缓存 |
| Redo Log Buffer | Redo 写入缓冲 |
| Large Pool | 备份/共享服务器会话 |

**PGA（进程私有内存）：** 排序区、Hash Join 区、会话变量、游标状态。

MySQL 无此概念，类似的是 Buffer Pool + per-thread buffers。
