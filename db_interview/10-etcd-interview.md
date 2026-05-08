# etcd 面试题集

> 共 20 题，覆盖 Raft 协议、集群架构、数据模型、Kubernetes 集成、运维排错等核心面试场景

---

## 1. etcd 是什么？核心特性有哪些？

**定义**：etcd 是一个分布式、高可用的键值存储系统，由 CoreOS 开发，使用 Go 语言编写，基于 Raft 共识算法。

**核心特性：**

| 特性 | 说明 |
|------|------|
| **强一致性** | Raft 协议保证，所有节点数据一致 |
| **高可用** | 奇数节点集群（3/5/7），容忍 (N-1)/2 节点故障 |
| **Watch 机制** | 客户端可监听 key 变化，实现发布订阅 |
| **MVCC** | 多版本并发控制，支持按 revision 查询历史数据 |
| **Lease 租约** | TTL 自动过期，实现服务发现和心跳检测 |
| **事务** | 支持 CAS（Compare-And-Swap）原子操作 |
| **gRPC API** | v3 API 基于 gRPC，性能优于 v2 HTTP API |

**典型应用场景**：
- **服务发现**：微服务注册与健康检查
- **配置中心**：分布式配置管理
- **分布式锁**：基于 Lease + 事务实现
- **Leader 选举**：基于 Lease 和 Txn
- **Kubernetes**：存储所有集群状态数据

---

## 2. Raft 协议核心原理是什么？

**Raft 将共识问题分解为三个子问题：**

### Leader 选举（Leader Election）
```
Follower → Candidate → Leader

1. 节点启动时是 Follower
2. 选举超时(timeout 150-300ms 随机)未收到 Leader 心跳
3. 变为 Candidate，term++，给自己投票
4. 向其他节点 RequestVote
5. 获得多数票 → 成为 Leader
6. Leader 定期发心跳(heartbeat)维持统治
```

**关键机制**：
- **随机超时**：防止多个节点同时竞选导致脑裂
- **Term（任期）**：逻辑时钟，每次选举 term+1
- **多数派原则**：获得 N/2+1 票才能当选

### 日志复制（Log Replication）
```
客户端写请求 → Leader → 追加日志 → 并行发 AppendEntries
→ 多数节点确认 → Leader 提交 → 通知 Follower 提交
→ 应用到状态机 → 返回客户端
```

**关键点**：
- 只有 Leader 处理写请求
- 读请求可以走 Follower（线性一致性读除外）
- 日志顺序一致，不允许空洞

### 安全性（Safety）

| 约束 | 说明 |
|------|------|
| **选举限制** | Candidate 的日志不能旧于大多数节点 |
| **提交限制** | Leader 只能提交当前 term 的日志 |
| **日志匹配** | 如果两个日志有相同的 index 和 term，则之前的全部相同 |

---

## 3. etcd 数据模型是怎样的？

**层级结构：**
```
etcd 是一个有序的键值存储
Key 是扁平的字节数组（没有目录概念，但支持前缀）
Value 是任意字节
```

**版本体系（MVCC）：**
```
每次修改产生新的 revision（全局递增的 64 位整数）
每个 key 有 create_revision 和 mod_revision
通过 revision 可以实现历史版本查询
```

**核心 API（v3）：**

```go
// Range - 范围查询（支持前缀）
Range(key, end_key, limit, revision)

// Put - 写入
Put(key, value, lease_id)

// Delete - 删除（支持范围）
Delete(key, end_key)

// Txn - 事务
If(Compare) Then(Op) Else(Op)

// Watch - 监听
Watch(key, end_key, start_revision)

// Lease - 租约
Grant(TTL) → lease_id
KeepAlive(lease_id)  // 续约
```

---

## 4. etcd v2 和 v3 API 有什么区别？

| 特性 | v2 API | v3 API |
|------|--------|--------|
| **协议** | HTTP/1.1 + JSON | gRPC + Protobuf |
| **数据模型** | 树形目录结构 | 扁平 key-value |
| **Watch** | 长轮询，一次性 | 双向流，支持重连 |
| **事务** | 不支持 | 强事务支持 |
| **Lease** | TTL key（性能差） | Lease 对象（可复用） |
| **性能** | 万级 QPS | 十万级 QPS |
| **MVCC** | 无 | 有 |

**Kubernetes 中的使用：**
- 早期 K8s 用 etcd v2，性能瓶颈明显
- K8s 1.13+ 默认 v3
- v3 支持 key 分桶存储，减少热点

---

## 5. etcd 集群架构和节点数选择？

**节点数选择：**

| 节点数 | 容忍故障 | 推荐场景 |
|--------|---------|---------|
| 1 | 0 | 开发测试 |
| 3 | 1 | 小规模生产 ⭐ |
| 5 | 2 | 中大型集群 |
| 7 | 3 | 超大规模 |

**为什么是奇数？**
偶数节点不能增加容错能力：4 节点只能容忍 1 个故障（需要 3 票），和 3 节点一样，却多一台成本。

**为什么不用代理/负载均衡？**
- etcd client 需要直连每个节点
- Leader 变更后 client 自动切换到新 Leader
- 加代理会增加延迟和故障点

---

## 6. etcd 如何保证数据一致性？

**写一致性（提交过程）：**
```
1. Leader 接收写请求
2. 写入本地 WAL(write-ahead log)
3. 并行发送 AppendEntries 给所有 Follower
4. 等待多数节点（quorum）确认
5. Leader commit（更新状态机）
6. 返回成功给客户端
```

**读一致性：**

| 读类型 | 一致性 | 实现 |
|--------|--------|------|
| **Serializable** | 弱一致（可能读到旧数据） | 直接从本地读 |
| **Linearizable** ⭐ | 强一致（读最新） | Leader 确认自己仍是 Leader 后才返回 |
| **Safe** | 同 Linearizable | v3 默认 |

**Linearizable Read 流程：**
```
1. 收到读请求
2. Leader 发心跳确认自己仍是 Leader
3. 读取 committed index 位置的数据
4. 返回结果
```

---

## 7. etcd 的 WAL 和 Snapshot 机制？

### WAL（Write-Ahead Log）
```
作用：持久化每一条操作日志，保证数据不丢失
位置：data-dir/member/wal/
格式：分段文件（默认 64MB 一个段）
流程：先写 WAL → 再更新内存 → 定期刷盘
```

### Snapshot（快照）
```
为什么需要？
- WAL 无限增长，会耗尽磁盘
- 启动时重放所有 WAL 太慢

机制：
1. 定期创建快照（默认 10000 条日志后）
2. 快照 = 当前状态机的完整状态
3. 快照后旧的 WAL 可以删除
4. 新节点加入先恢复快照，再重放后续 WAL

参数：
--snapshot-count=100000  # 多少条日志后做快照
--auto-compaction-mode=periodic
--auto-compaction-retention=1h  # 保留1小时历史版本
```

---

## 8. etcd 的 Watch 机制原理？

**v3 Watch 架构：**
```
Client → gRPC Stream → etcd Server → Watcher Group
                            ↓
                    WatchableStore（MVCC 存储层）
```

**特性：**
- **双向流**：一个 gRPC 连接支持多个 Watch
- **断线重连**：记录最后接收的 revision，重连后从断点继续
- **批量推送**：多个事件合并到一个推送，减少网络开销
- **支持前缀监听**：`Watch("/registry/pods/", WithPrefix())`

```go
// 从指定 revision 开始监听
watchChan := client.Watch(ctx, "/foo", client.WithRev(rev))
for resp := range watchChan {
    for _, ev := range resp.Events {
        fmt.Printf("Type: %s Key: %s Value: %s\n", ev.Type, ev.Kv.Key, ev.Kv.Value)
    }
}
```

---

## 9. etcd 的 Lease（租约）机制？

**用途**：给 key 绑定 TTL，到期自动删除。

```go
// 创建 10 秒租约
lease := clientv3.NewLease(client)
grantResp, _ := lease.Grant(ctx, 10)

// 绑定 key 到租约
client.Put(ctx, "/service/instance1", "10.0.0.1:8080", clientv3.WithLease(grantResp.ID))

// 续约（后台自动）
keepAliveChan, _ := lease.KeepAlive(ctx, grantResp.ID)
```

**实现原理：**
```
1. Lease 过期时间最小堆
2. 定时检查堆顶是否过期
3. 过期 → 删除绑定 key → 生成事件 → Watch 推送到客户端
4. 续约只改 Leader 内 expire time，不写 WAL
```

**Kubernetes 中的使用：**
- Node 心跳：kubelet 每 10 秒更新 Node 对象的 Lease
- 服务发现：Service 的 Endpoints 绑定 Lease

---

## 10. etcd 事务（Txn）怎么用？

```go
// 分布式锁：CAS 操作
txnResp, err := client.Txn(ctx).
    If(clientv3.Compare(clientv3.CreateRevision("lock"), "=", 0)).  // key 不存在
    Then(clientv3.OpPut("lock", "holder1", clientv3.WithLease(leaseID))).
    Else(clientv3.OpGet("lock")).
    Commit()
```

**常用模式：**
- **CAS 写**：if key == old_value → put new_value
- **分布式锁**：if key 不存在 → put value
- **原子计数器**：if key_ver == N → put N+1
- **条件创建**：if CreateRevision == 0 → put

---

## 11. etcd 在 Kubernetes 中的角色？

**Kubernetes 的所有状态数据都存在 etcd 中：**

```
/registry/
├── pods/
│   └── default/
│       ├── nginx-xxx
│       └── redis-xxx
├── services/
│   └── endpoints/
├── deployments/
├── configmaps/
├── secrets/
├── namespaces/
├── nodes/
├── persistentvolumes/
├── serviceaccounts/
└── ...
```

**K8s 如何使用 etcd：**

| 机制 | K8s 映射 |
|------|---------|
| Watch | Controller 监听资源变化，触发调和循环（Reconcile） |
| Lease | Node 心跳、Leader 选举（controller-manager） |
| Txn | 资源创建、更新时的原子性保证 |
| MVCC | ResourceVersion（乐观并发控制） |

**重要数字：**
- K8s 默认请求 etcd 的 key 大小限制：1.5MB（对象太大需分割）
- ResourceVersion：每次写操作自增，用于冲突检测

---

## 12. etcd 性能优化有哪些要点？

### 磁盘
```
最重要！🔥

- 必须用 SSD/NVMe，IOPS 是关键
- 建议独立的物理磁盘（不要共享）
- WAL 和 Snapshot 放不同磁盘更好
- fdatasync 延迟 < 10ms 为佳
```

### 网络
```
- 节点间延迟 < 10ms（同机房）
- Leader 和 Follower 带宽充足
```

### 配置调优

```bash
# etcd 启动参数
--quota-backend-bytes=8589934592  # 8GB 存储上限
--snapshot-count=100000           # 快照频率
--auto-compaction-mode=periodic
--auto-compaction-retention=1h    # 自动压缩旧版本

# 环境变量
ETCD_HEARTBEAT_INTERVAL=100       # 心跳间隔（ms）
ETCD_ELECTION_TIMEOUT=1000        # 选举超时（ms）
```

### Client 端优化
```go
// 使用批量操作代替单条
// ❌ 逐条 Put
for _, item := range items {
    client.Put(ctx, item.Key, item.Value)
}
// ✅ 批量 Txn
txn := client.Txn(ctx)
for _, item := range items {
    txn = txn.Then(clientv3.OpPut(item.Key, item.Value))
}
txn.Commit()
```

---

## 13. etcd 常见故障排查？

### 问题 1：集群抖动，频繁选举
```
原因：磁盘 IO 慢、网络延迟高、CPU 不足
排查：
  - etcdctl endpoint health / status
  - 检查 fdatasync 延迟：etcd_disk_wal_fsync_duration_seconds
  - 看日志："leader changed" "failed to send heartbeat"
```

### 问题 2：mvcc: database space exceeded
```
原因：存储空间超 quota-backend-bytes
解决：
  - 紧急：etcdctl compact + defrag
  - 长期：开启 auto-compaction
  
  # 压缩
  etcdctl compact <current_revision>
  # 碎片整理
  etcdctl defrag --endpoints=...
```

### 问题 3：新节点加入失败
```
原因：快照太大、网络隔离、时间不同步
排查：
  - 检查 member add 是否成功
  - 新节点启动日志中的 snapshot 同步进度
  - 确认初始集群状态 start-state=existing
```

### 问题 4："request took too long to execute"
```
典型原因：
  - 磁盘慢（最常见）→ 检查 SSD IOPS
  - key 太大 → 检查 value 大小
  - 过度 Watch → 检查 Watch 数量
  - 未压缩 → 检查 DB size
```

---

## 14. etcd 备份和恢复怎么做？

### 备份（3 种方式）

```bash
# 1. 快照备份（推荐）
etcdctl snapshot save /backup/etcd-$(date +%Y%m%d).db

# 2. 快照状态查看
etcdctl snapshot status /backup/etcd-20240501.db --write-out=table

# 3. 定期自动化备份脚本
#!/bin/bash
BACKUP_DIR=/backup/etcd
RETENTION=7  # 保留7天
etcdctl snapshot save ${BACKUP_DIR}/snapshot-$(date +%Y%m%d%H%M).db
find ${BACKUP_DIR} -name "snapshot-*" -mtime +${RETENTION} -delete
```

### 恢复

```bash
# 从快照恢复
etcdctl snapshot restore snapshot.db \
  --name=etcd-node1 \
  --initial-cluster=etcd-node1=https://10.0.0.1:2380,etcd-node2=https://10.0.0.2:2380,etcd-node3=https://10.0.0.3:2380 \
  --initial-advertise-peer-urls=https://10.0.0.1:2380 \
  --data-dir=/var/lib/etcd-restore
```

**注意事项：**
- 恢复会创建新的 cluster ID 和 member ID
- 所有节点都要从同一个快照恢复
- K8s 场景要同时恢复 API server 配置

---

## 15. etcd v2 升级 v3 的注意事项？

**API 变化：**
- v3 不再有目录层级概念，前缀替代
- Watch 从一次性改为流式
- TTL 从 key 级别改为 Lease 对象

**数据迁移：**
- v2 和 v3 数据存储独立
- `--enable-v2` 参数可同时启用 v2
- 渐进式迁移：v2 读 → 双写 v2+v3 → 切读 v3 → 停写 v2

**Kubernetes 升级：**
- etcd 3.4+ 默认禁用 v2 API
- K8s 1.13 切换到 v3 存储后端
- 检查 K8s 版本兼容性

---

## 16. etcd 的 gRPC Gateway 是什么？

etcd 同时提供 gRPC 和 HTTP/JSON 两种接口：

```
gRPC:  localhost:2379 (性能高)
HTTP:  localhost:2379 (调试方便)

# gRPC 调用
etcdctl --endpoints=localhost:2379 get /foo

# HTTP 调用（等价）
curl -L http://localhost:2379/v3/kv/range \
  -X POST -d '{"key": "L2Zvbw=="}'  # base64(/foo)
```

**内部实现：**
gRPC Gateway 把 HTTP/JSON 请求转换为 gRPC 调用，无需额外部署代理。

---

## 17. etcd 如何实现分布式锁？

```go
func DistributedLock(client *clientv3.Client, lockKey string, ttl int64) error {
    // 1. 创建租约
    lease := clientv3.NewLease(client)
    grantResp, _ := lease.Grant(context.TODO(), ttl)
    
    // 2. 自动续约
    keepAliveCh, _ := lease.KeepAlive(context.TODO(), grantResp.ID)
    
    // 3. 事务 CAS：key 不存在则写入
    ctx := context.TODO()
    txnResp, err := client.Txn(ctx).
        If(clientv3.Compare(clientv3.CreateRevision(lockKey), "=", 0)).
        Then(clientv3.OpPut(lockKey, "locked", clientv3.WithLease(grantResp.ID))).
        Commit()
    
    if !txnResp.Succeeded {
        // 获取锁失败，可以 Watch 等待释放
        return fmt.Errorf("lock held by others")
    }
    
    // 4. 定期检查续约
    go func() {
        for range keepAliveCh {
            // Lease 续约成功
        }
    }()
    
    // 5. 释放锁
    defer lease.Revoke(context.TODO(), grantResp.ID)
    
    // 执行业务逻辑...
    return nil
}
```

**与 Redis 分布式锁对比：**

| 特性 | etcd | Redis (Redlock) |
|------|------|-----------------|
| 一致性 | Raft 强一致 | 最终一致 |
| 安全性 | ⭐⭐⭐ 更安全 | ⭐⭐ 有争议 |
| 性能 | 万级 QPS | 十万级 QPS |
| 自动续约 | Lease KeepAlive | 需手动续约 |

---

## 18. etcd 安全配置（TLS + RBAC）

### TLS 证书配置
```bash
etcd \
  --cert-file=/etc/etcd/etcd-server.crt \
  --key-file=/etc/etcd/etcd-server.key \
  --trusted-ca-file=/etc/etcd/ca.crt \
  --client-cert-auth=true \
  --peer-cert-file=/etc/etcd/etcd-peer.crt \
  --peer-key-file=/etc/etcd/etcd-peer.key \
  --peer-trusted-ca-file=/etc/etcd/ca.crt \
  --peer-client-cert-auth=true
```

### RBAC 权限控制
```bash
# 1. 创建 root 用户
etcdctl user add root

# 2. 开启认证
etcdctl auth enable

# 3. 创建角色
etcdctl role add k8s-reader

# 4. 角色授权（前缀权限）
etcdctl role grant-permission k8s-reader read /registry/ --prefix=true

# 5. 创建用户并绑定角色
etcdctl user add k8s-user
etcdctl user grant-role k8s-user k8s-reader
```

---

## 19. etcd 与其他技术的对比？

### etcd vs ZooKeeper

| 特性 | etcd | ZooKeeper |
|------|------|-----------|
| 协议 | Raft | ZAB |
| 语言 | Go | Java |
| API | gRPC + HTTP | 自定义 TCP 协议 |
| Watch | 流式双向 | 一次性触发 |
| 部署 | 单二进制文件 | 依赖 JVM |
| 性能 | 更高（v3） | 中等 |
| 社区 | K8s 生态核心 | 大数据生态（Kafka/HBase） |

### etcd vs Consul

| 特性 | etcd | Consul |
|------|------|--------|
| 定位 | 通用 KV + 协调 | 服务网格全栈 |
| 服务发现 | 需自建 | 内置 DNS/HTTP |
| 健康检查 | 基于 Lease | 多种检查方式 |
| 多数据中心 | 不支持 | 原生支持 WAN GOSSIP |
| 一致性 | Raft 强一致 | Raft + GOSSIP 弱一致 |

---

## 20. etcd 生产环境部署 Checklist

```
✅ 节点数：3 或 5（奇数）
✅ 独立 SSD/NVMe 磁盘
✅ 节点间延迟 < 10ms
✅ 开启 TLS 证书认证
✅ 定期备份（cron + snapshot）
✅ 开启 auto-compaction
✅ 监控指标告警：
   - etcd_server_has_leader（无 Leader = 集群异常）
   - etcd_disk_wal_fsync_duration_seconds（>0.01s 告警）
   - etcd_mvcc_db_total_size_in_bytes（>80% quota 告警）
   - etcd_network_peer_round_trip_time_seconds（>0.05s 告警）
✅ 禁止与监控/日志等 I/O 密集型服务共用磁盘
✅ etcd 版本统一管理（不要混用版本）
✅ 制定回滚方案（快照恢复演练）
```

---

## 附：面试常见追问

**Q: etcd 的脑裂怎么解决？**
> Raft 的法定人数（quorum）机制天然防止脑裂。网络分区后，少数节点的分区无法提交任何操作（不到多数派），Leader 选举也无法成功。网络恢复后，少数分区自动同步 Leader 的最新日志。

**Q: etcd key 的推荐大小是多少？**
> 官方建议不超过 1.5MB。K8s 对象大小限制也是 1.5MB。太大的 key 会显著影响性能（序列化、网络传输、Raft 日志复制）。

**Q: etcd 节点崩溃后数据丢不丢？**
> 已提交的数据不丢（WAL 保证了持久性）。未 commit 的数据可能丢。多数节点存活时服务不受影响。

**Q: 为什么 etcd 不用 Paxos 而用 Raft？**
> Raft 比 Paxos 更易理解和实现。Paxos 在工程实现上有诸多难点（Multi-Paxos 没有标准实现），Raft 通过分解（选举+日志复制+安全）降低了工程复杂度。

**Q: K8s 中如何查看哪些对象占用了 etcd 空间？**
```bash
# 查看 K8s 资源在 etcd 中的存储大小
ETCDCTL_API=3 etcdctl get /registry/ --prefix --keys-only | wc -l

# 找最大的对象（通常是 ConfigMap/Secret 或带大量 annotation 的对象）
# 排查：太多 Secrets、过大的 ConfigMap、Event 未清理等
```
