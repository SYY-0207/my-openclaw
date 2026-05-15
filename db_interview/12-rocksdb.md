# RocksDB 全解

> Facebook 开源的嵌入式 KV 存储引擎，基于 Google LevelDB，LSM-Tree 架构

---

## 一、是什么

**RocksDB** 是 Facebook 2013 年 fork LevelDB 后重构的高性能嵌入式键值存储引擎，C++ 编写，提供 C++/Java/Python 等多语言绑定。

**定位**：

```
不是一个数据库（无 SQL、无网络协议）
是一个存储引擎（类似 InnoDB 之于 MySQL）

应用层
  ↓
RocksDB（排序 KV 存储 + 事务）
  ↓
文件系统（XFS/ext4/NVMe/云盘）
```

**谁在用**：
- **TiKV**（TiDB 的存储层）
- **MyRocks**（Facebook 的 MySQL 引擎）
- **Apache Flink**（状态后端）
- **Kafka Streams**
- **CockroachDB**（早期版本）
- **YugabyteDB**

---

## 二、LSM-Tree 核心架构

### 为什么不用 B+ Tree？

| 特性 | B+ Tree (InnoDB) | LSM-Tree (RocksDB) |
|------|------------------|---------------------|
| 写放大 | 高（随机写，页分裂） | 低（顺序写 WAL + 批量刷） |
| 读放大 | 低（缓存 + 二分查找） | 高（多层查找 + Bloom Filter） |
| 空间放大 | 低 | 高（旧版本待压缩） |
| 适合场景 | 读多写少 | 写多读少 / 大写入量 |

### 写入流程

```
             Write(Key, Value)
                  │
                  ▼
         ┌──────────────┐
         │    WAL       │  ← 1. 先写日志（持久化，防止崩溃丢失）
         │ (Write-Ahead │
         │   Log)       │
         └──────────────┘
                  │
                  ▼
         ┌──────────────┐
         │ Active       │  ← 2. 写入活跃 MemTable（SkipList，内存）
         │ MemTable     │
         └──────────────┘
                  │ (MemTable 写满)
                  ▼
         ┌──────────────┐
         │ Immutable    │  ← 3. 变为只读 MemTable，等待 Flush
         │ MemTable     │
         └──────────────┘
                  │ (Flush 到磁盘)
                  ▼
         ┌──────────────┐
         │  SST File    │  ← 4. 生成 L0 层 SST 文件（有序，不可变）
         │  (Level 0)   │
         └──────────────┘
                  │ (Compaction 合并)
                  ▼
         ┌──────────────┐
         │  SST Files   │  ← 5. 压缩到 L1, L2, ... Ln
         │  (Level 1+)  │     每层容量是上一层的 10 倍
         └──────────────┘
```

### 读取流程

```
Read(Key):
  1. 查 Active MemTable          ← 最新
  2. 查 Immutable MemTable       ← 次新
  3. 查 L0 SST（可能多个文件有重叠 key）
     ├── Bloom Filter 快速判断 key 是否存在
     └── 不存在 → 跳过该文件
  4. 查 L1 SST（一层一个文件，key 范围不重叠）
  5. 查 L2, L3 ... 直到找到或确认不存在
```

**为什么读会慢？** 需要从上层往下查，层级越多读放大越严重。

---

## 三、核心数据结构

### 1. MemTable（内存表）

| 实现 | 特点 |
|------|------|
| **SkipList**（默认） | 并发写入性能好 |
| **HashSkipList** | 前缀查询更快 |
| **HashLinkList** | 内存占用更小 |
| **Vector** | 批量导入场景 |

**写满条件**：
- `write_buffer_size`（默认 64MB）写满
- WAL 文件达到 `max_total_wal_size`

### 2. SST File（Sorted String Table）

```
┌──────────────────────┐
│     Data Blocks      │  ← 有序 key-value 数据块（默认 4KB）
│  (多个，可压缩)       │
├──────────────────────┤
│    Meta Blocks       │  ← Bloom Filter / Properties
├──────────────────────┤
│   Index Block        │  ← Data Block 的偏移 + 起始 key
├──────────────────────┤
│    Footer            │  ← 元数据，指向 Index Block 和 Meta Block
└──────────────────────┘
```

**压缩算法**：Snappy（默认）/ LZ4 / ZSTD / Zlib

### 3. WAL（Write-Ahead Log）

```
目的：崩溃恢复
模式：
  - 每个 ColumnFamily 独立 WAL
  - 或 单个 WAL（atomic flush）

生命周期：
  创建 → 写入 → MemTable flush 后归档 → 可删除
```

### 4. Bloom Filter

```
作用：快速判断 key 是否可能存在
实现：每 SST 文件一个 Bloom Filter

查询时：
  Bloom Filter 说「不存在」 → 直接跳过该 SST（减少 IO） ✅
  Bloom Filter 说「可能存在」 → 真正读 SST（存在误判可能）

bits_per_key = 10  → 误判率约 1%  → 内存约占 key 总量的 10 bit × key数量
```

---

## 四、Compaction（压缩合并）

**为什么需要 Compaction？**
- SST 文件越来越多 → 读性能下降
- 相同 key 的旧版本浪费空间
- 删除标记需要清理

### Leveled Compaction（默认）

```
L0:  SST SST SST SST    ← key 范围重叠（因为直接来自 MemTable flush）
       ↓  Compaction
L1:  [a-d] [e-k] [l-z]  ← key 不重叠，按范围分裂，10× 于 L0
       ↓  Compaction
L2:  [a-c] [c-g] ...    ← 10× 于 L1
...
L6:  10× 于 L5           ← 最底层

每当 Li 大小超过阈值：
  → 选择 Li 的一个文件
  → 与 Li+1 中 key 范围重叠的文件合并
  → 生成新的 Li+1 文件
```

| 优点 | 缺点 |
|------|------|
| 读放大低 | 写放大高（每层 rewrite） |
| 空间放大低 | Compaction 时 IO 密集 |

### Universal Compaction

```
所有 SST 文件在同一层（不分 Level）
按文件大小排序，相邻文件合并
适合写多读少的场景
写放大比 Leveled 小
```

### FIFO Compaction

```
最简单：SST 文件按时间顺序，最旧的直接删除
适合：日志/时序数据（过期即删）
```

### 对比

| 策略 | 写放大 | 读放大 | 空间放大 | 适用场景 |
|------|--------|--------|----------|---------|
| Leveled | 高（~10x-30x） | 低 | 低 | 通用 ⭐ |
| Universal | 低（~2x-5x） | 高 | 高 | 写密集型 |
| FIFO | 极低 | 极高 | 取决于 TTL | 时序/缓存 |

---

## 五、Column Family（列族）

**类似 MySQL 的「表」**，同一数据库内多个独立 KV 空间：

```cpp
// 创建数据库，含两个列族
Options options;
options.create_if_missing = true;

std::vector<ColumnFamilyDescriptor> cf_descs;
cf_descs.push_back(ColumnFamilyDescriptor("default", ColumnFamilyOptions()));
cf_descs.push_back(ColumnFamilyDescriptor("metadata", ColumnFamilyOptions()));

DB* db;
std::vector<ColumnFamilyHandle*> handles;
DB::Open(options, "/data/rocksdb", cf_descs, &handles, &db);

// 写入不同列族
db->Put(WriteOptions(), handles[0], "key1", "value1");  // default
db->Put(WriteOptions(), handles[1], "key2", "value2");  // metadata

// 各自独立 MemTable + SST + Compaction
```

**用途**：
- 热数据 / 冷数据分离（不同 Compaction 策略）
- 不同 TTL 的数据
- 不同业务的数据隔离

---

## 六、关键特性

### 1. Merge Operator（合并操作）

不是简单的 Put/Get，支持自定义合并逻辑：

```cpp
// 计数器累加，无需 Get→+1→Put
db->Merge(WriteOptions(), "page_views", "1");
db->Merge(WriteOptions(), "page_views", "1");
// 最终 Get 时返回 "2"，Merge 在 Compaction 或 Get 时执行
```

**应用**：计数器、追加列表、统计聚合

### 2. Snapshot（快照）

```cpp
Snapshot* snapshot = db->GetSnapshot();

// 基于该快照的读，不随后续写而变化
db->Get(ReadOptions(), "key");           // 读最新
db->Get(ReadOptions(), snapshot, "key"); // 读快照时刻的值

db->ReleaseSnapshot(snapshot);
```

**实现**：基于 sequence number（全局递增），快照记录当时的 seqno。

### 3. Transaction（事务）

```cpp
TransactionDB* txn_db;
Transaction* txn = txn_db->BeginTransaction(WriteOptions());

txn->Put("account:A", "900");
txn->Put("account:B", "200");
txn->Commit();  // 或 Rollback()
```

- **PessimisticTransaction**：锁冲突多场景（加锁）
- **OptimisticTransaction**：冲突少场景（提交时检测）

### 4. DeleteRange

```cpp
// 删除 [a, z) 范围的所有 key（只写一个墓碑标记）
db->DeleteRange(WriteOptions(), db->DefaultColumnFamily(), "a", "z");
```

比逐条 Delete 高效得多。

---

## 七、调优参数（面试重点）

### 写入优化

```ini
# 增大 MemTable → 减少 flush，但占内存
write_buffer_size=256MB

# MemTable 数量 → 写峰值缓冲
max_write_buffer_number=4

# 后台 compaction 线程
max_background_compactions=4
max_background_flushes=2

# 限制 L0 文件数（超过会阻塞写入）
level0_slowdown_writes_trigger=20
level0_stop_writes_trigger=36
```

### 读取优化

```ini
# Bloom Filter 精度（10 bits ≈ 1% 误判）
optimize_filters_for_hits=true
bloom_locality=1

# Block Cache（热数据缓存）
block_cache_size=2GB  # LRU 缓存未压缩数据

# Pin L0 索引和 filter 在内存中
pin_l0_filter_and_index_blocks_in_cache=true
```

### Compaction 优化

```ini
compaction_style=LEVEL  # 或 UNIVERSAL, FIFO

# Leveled Compaction 参数
target_file_size_base=64MB   # L1 单个 SST 大小
target_file_size_multiplier=1
max_bytes_for_level_base=512MB  # L1 总容量
max_bytes_for_level_multiplier=10

# 并行读取 Compaction 输入
max_subcompactions=4
```

### 通用

```ini
# ZSTD 压缩（平衡压缩率和速度）
compression_type=ZSTD

# 减少空间放大
bottommost_compression_type=ZSTD  # 最底层用更高压缩率
```

---

## 八、RocksDB vs 同类引擎

| 特性 | RocksDB | LevelDB | WiredTiger | InnoDB |
|------|---------|---------|------------|--------|
| 架构 | LSM-Tree | LSM-Tree | B+ Tree / LSM | B+ Tree |
| 语言 | C++ | C++ | C | C |
| 写性能 | ⭐⭐⭐ | ⭐⭐ | ⭐⭐ | ⭐ |
| 读性能 | ⭐⭐ | ⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐ |
| 事务 | 支持 | 不支持 | 支持 | ACID |
| 压缩 | Snappy/LZ4/ZSTD | Snappy | 多种 | — |
| 嵌入式 | ✅ | ✅ | ✅ | ❌（服务层） |
| 适用场景 | 引擎/存储底座 | 嵌入式小数据 | MongoDB 引擎 | MySQL 引擎 |

### RocksDB vs LevelDB

RocksDB 相比 LevelDB 的主要改进：

1. **多线程 Compaction**（LevelDB 单线程）
2. **Column Family**
3. **Merge Operator**
4. **快照 + 备份**
5. **更多 MemTable 实现**
6. **Bloom Filter 配置更灵活**
7. **事务支持**
8. **DeleteRange**
9. **WAL 更灵活**（可关闭、分列族）

---

## 九、典型应用架构

### 1. TiKV（TiDB 的存储引擎）

```
TiDB (SQL Layer)
    ↓
TiKV (Distributed KV)
    ├── Raft 层（多副本共识）
    │   └── RocksDB (存储 Raft 日志)
    └── KV 层
        └── RocksDB (存储用户数据)
```

### 2. MyRocks（Facebook 的 MySQL 引擎）

```
MySQL
  └── MyRocks（替代 InnoDB）
        └── RocksDB

优势：
  - 同等数据量，磁盘占用是 InnoDB 的 50%
  - 写入吞吐高 2-3 倍
  - 压缩率高（Facebook 实测 2-3x 压缩比）
```

### 3. Flink State Backend

```
Flink Job
  └── RocksDBStateBackend
        └── RocksDB（本地磁盘 + 异步 Checkpoint 到远程）

为什么用 RocksDB？
  - 状态超过内存大小时不能纯用 Heap
  - RocksDB 溢出到磁盘但保留增量 Checkpoint
```

---

## 十、面试常见追问

**Q: 为什么 LSM-Tree 写快读慢？**
> 写入是顺序写（WAL + MemTable 内存），无随机 IO。读取要从 MemTable → L0 → L1 → ... 逐层查找，层数多时读放大严重。Bloom Filter 是缓解读放大的关键手段。

**Q: 什么是写放大和读放大？**
> 写放大 = 实际写入磁盘的数据量 / 用户写入的数据量。Leveled Compaction 每层 rewrite，写放大约 10x-30x。读放大 = 每次读取需要访问的磁盘文件数。LSM 读一个 key 可能查多层的多个 SST。

**Q: RocksDB 的 Delete 操作是立刻生效吗？**
> 不是。Delete 写入一个墓碑标记（Tombstone），旧版本在 Compaction 时清理。墓碑本身也会占据空间，直到覆盖它所在的 SST 被 compaction 到最底层并比它更旧的数据也已清理。

**Q: 什么时候选择 Universal Compaction？**
> 写多读少（如日志存储），SSD 空间充足，允许一定的读放大和空间放大。写放大比 Leveled 低很多。

**Q: 为什么不直接用 RocksDB 做数据库？**
> RocksDB 不提供 SQL、网络协议、权限管理、分布式协调。它定位是存储引擎，需要上层系统包装。TiDB（SQL + 分布式 + 事务） + TiKV（Raft 复制 + RocksDB 存储） 是典型的分层设计。

**Q: 如何监控 RocksDB 健康状态？**
> 关键指标：
> - `rocksdb.num-immutable-mem-table` — 等待 flush 的表数量
> - `rocksdb.num-running-compactions` — 正在压缩的任务数
> - `rocksdb.stall-micros` — 写入被阻塞的微秒数
> - `rocksdb.compaction-pending` — 是否积压
> - `rocksdb.size-all-mem-table` — MemTable 总内存
> - `rocksdb.num-files-at-levelN` — 各层 SST 文件数
