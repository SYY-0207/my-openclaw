# Docker 面试题集

---

## 🔹 基础概念

**Q1: 什么是 Docker？Docker 和虚拟机有什么区别？**

Docker 是容器化平台，将应用及其依赖打包成轻量级容器，在任何 Docker 环境中运行。

| | Docker 容器 | 虚拟机 |
|---|---|---|
| 虚拟化层级 | OS 层（共享宿主机内核） | 硬件层 |
| 启动速度 | 秒级 | 分钟级 |
| 资源占用 | MB 级 | GB 级 |
| 隔离性 | 进程级（namespace/cgroup） | 完全隔离 |
| 性能损耗 | 接近原生 | 有 hypervisor 开销 |
| Guest OS | 不需要 | 每个 VM 有完整 OS |
| 迁移性 | 镜像小，分发快 | 镜像大 |

**一图胜千言：** 虚拟机每个都跑一个完整 OS，容器共享宿主机内核，只打包应用 + 依赖。

---

**Q2: Docker 的核心组件有哪些？各自的作用是什么？**

| 组件 | 作用 |
|------|------|
| **Docker Daemon (dockerd)** | 后台守护进程，管理容器、镜像、网络、存储 |
| **Docker Client (docker CLI)** | 用户命令行工具，通过 REST API 与 daemon 通信 |
| **Docker Image** | 只读模板，容器运行的基础 |
| **Docker Container** | 镜像的运行实例，可读写层叠加在镜像层之上 |
| **Docker Registry** | 镜像仓库（Docker Hub 是默认公共仓库） |
| **Docker Network** | 容器间网络通信 |
| **Docker Volume** | 持久化存储，数据独立于容器生命周期 |

**通信方式：** Client 通过 `/var/run/docker.sock` (Unix Socket) 或 TCP 与 daemon 通信。

---

**Q3: Docker 镜像的分层结构是怎么回事？有什么好处？**

Docker 镜像由多层只读层叠加组成，Dockerfile 中每个指令生成一层。

```
容器层 (可读写 R/W)
───────────────────
Layer 4: CMD / ENTRYPOINT   ← 只读
Layer 3: COPY app.jar        ← 只读
Layer 2: RUN apt-get install  ← 只读
Layer 1: FROM ubuntu:22.04    ← 只读
```

**好处：**
- **共享层**：不同镜像共用相同基础层（如 ubuntu:22.04），节省磁盘和传输带宽
- **增量构建**：只重建变更层，缓存未变的层
- **快速分发**：拉取镜像时只下载本地没有的层

**查看镜像层：** `docker image history <image>`

---

**Q4: Dockerfile 中 COPY 和 ADD 的区别？**

| | COPY | ADD |
|---|---|---|
| 功能 | 只做文件复制 | 复制 + 额外功能 |
| 解压 tar | ❌ | ✅ 自动解压 tar.gz/tar.bz2 |
| 远程 URL | ❌ | ✅ 支持 URL 下载（但不推荐） |
| 推荐度 | ✅ 首选 | 仅在需要自动解压时使用 |

**最佳实践：** 默认用 COPY。需要解压用 ADD。需要下载文件用 `RUN curl/wget`，不要用 ADD 下载远程文件（会产生不必要的一层，且无法清理缓存）。

---

**Q5: CMD 和 ENTRYPOINT 的区别？怎么配合使用？**

| | CMD | ENTRYPOINT |
|---|---|---|
| 是否可被覆盖 | ✅ `docker run` 后跟的命令会覆盖 CMD | ❌ `docker run` 后的参数作为追加参数 |
| 典型用途 | 默认参数 | 指定可执行程序 |
| 形式 | `CMD ["executable","param1","param2"]` | `ENTRYPOINT ["executable","param1"]` |

**三者配合（CMD + ENTRYPOINT + docker run）：**

```dockerfile
ENTRYPOINT ["python"]
CMD ["app.py"]
```

| 运行命令 | 实际执行 |
|----------|----------|
| `docker run img` | `python app.py` |
| `docker run img test.py --debug` | `python test.py --debug` |
| `docker run img -m http.server 8080` | `python -m http.server 8080` |

**最佳实践：** ENTRYPOINT 定程序，CMD 定默认参数。

---

**Q6: Docker 的网络模式有哪些？各自有什么特点？**

| 模式 | 命令 | 特点 |
|------|------|------|
| **bridge（默认）** | `--network bridge` | 容器在隔离的桥接网络，通过宿主机 NAT 访问外网 |
| **host** | `--network host` | 与宿主机共享网络栈，无隔离，性能最好 |
| **none** | `--network none` | 无网络，只有 lo 接口 |
| **container:<id>** | `--network container:xxx` | 共享另一个容器的网络栈（共享 localhost） |
| **overlay** | Swarm 模式 | 跨宿主机容器通信 |
| **macvlan/ipvlan** | 高级模式 | 给容器分配物理网络 IP，直接接入物理网络 |

**自定义 bridge 网络 vs 默认 bridge：**
- 自定义网络支持 **DNS 解析**（容器名 → IP），默认 bridge 只能用 IP
- 自定义网络更好隔离

---

**Q7: Docker 的数据持久化方式有哪些？各自的使用场景？**

| 方式 | 管理方式 | 适用场景 |
|------|----------|----------|
| **Volume** | Docker 管理（`/var/lib/docker/volumes/`） | **首选**，生产环境用 |
| **Bind Mount** | 宿主机任意路径 | 开发环境，需要实时同步代码 |
| **tmpfs Mount** | 只存内存 | 临时敏感数据，容器停止即消失 |

**命令对比：**
```bash
# Volume（推荐）
docker run -v my_volume:/data nginx

# Bind Mount
docker run -v /host/path:/container/path nginx

# tmpfs
docker run --tmpfs /tmp nginx
```

**为什么推荐 Volume：**
- Docker 管理，跨平台一致性好
- 可以通过 `docker volume` 命令管理（备份、迁移、清理）
- 不依赖宿主机目录结构
- 卷驱动可扩展（NFS、Ceph 等）

---

## 🔹 Dockerfile 最佳实践

**Q8: Dockerfile 优化技巧有哪些？**

**1. 减小镜像体积：**
```dockerfile
# ❌ 差
FROM ubuntu
RUN apt-get update && apt-get install -y python3
COPY . .

# ✅ 好：多阶段构建
FROM golang:1.21 AS builder
WORKDIR /app
COPY . .
RUN go build -o myapp

FROM alpine:3.19
COPY --from=builder /app/myapp /usr/local/bin/myapp
ENTRYPOINT ["myapp"]
```

**2. 利用缓存：**
```dockerfile
# 先复制依赖文件，再复制源码
COPY requirements.txt .
RUN pip install -r requirements.txt   # 这层在改动代码时被缓存
COPY . .
```

**3. 减少层数：**
```dockerfile
# ❌ 差
RUN apt-get update
RUN apt-get install -y curl
RUN apt-get install -y vim

# ✅ 好：合并并清理
RUN apt-get update && apt-get install -y curl vim \
    && rm -rf /var/lib/apt/lists/*
```

**4. 使用 .dockerignore：**
```
node_modules
.git
*.md
Dockerfile
```

**5. 用非 root 用户运行：**
```dockerfile
RUN addgroup -S app && adduser -S app -G app
USER app
```

---

**Q9: 多阶段构建 (Multi-stage Build) 是什么？解决了什么问题？**

**问题：** 编译需要整套 SDK（几百 MB），运行时只需要编译产物（几 MB）。传统做法要么一个镜像又大又脏，要么用脚本在外面编译。

**解决方案：**
```dockerfile
# Stage 1: 编译
FROM maven:3.9 AS builder
COPY . /app
WORKDIR /app
RUN mvn clean package -DskipTests

# Stage 2: 运行（用 JRE 不用 JDK）
FROM eclipse-temurin:17-jre-alpine
COPY --from=builder /app/target/app.jar /app/app.jar
ENTRYPOINT ["java", "-jar", "/app/app.jar"]
```

**好处：** 最终镜像只有运行时依赖 + 编译产物，极度瘦身。构建阶段和运行阶段环境可以完全不同。

---

## 🔹 高级专题

**Q10: Docker 的资源限制怎么做？cgroup 的原理是什么？**

Docker 通过 Linux **cgroups (Control Groups)** 限制资源。

**常用限制参数：**
```bash
docker run \
  --cpus="1.5"              # CPU 核数上限
  --memory="512m"           # 内存上限
  --memory-swap="1g"        # 内存+Swap 上限
  --blkio-weight=500        # IO 权重 (10-1000)
  --pids-limit=100          # PID 数量上限
  nginx
```

**cgroup 原理：**
- CPU：通过 `cpu.cfs_period_us` / `cpu.cfs_quota_us` 限制 CPU 时间片
- Memory：`memory.limit_in_bytes` 限制内存，超限触发 OOM Killer
- 在 `/sys/fs/cgroup/` 下可以看到每个容器的 cgroup 配置

**查看容器资源使用：** `docker stats`

---

**Q11: Docker Compose 是什么？和 docker run 相比有什么优势？swarm 和 k8s 怎么选？**

**Docker Compose：** 通过 YAML 定义多容器应用，一键启动/停止。

```yaml
# docker-compose.yml
version: '3.8'
services:
  app:
    build: .
    ports:
      - "8080:8080"
    depends_on:
      - db
    environment:
      - DB_HOST=db
  db:
    image: postgres:16
    volumes:
      - pgdata:/var/lib/postgresql/data
    environment:
      - POSTGRES_PASSWORD=secret

volumes:
  pgdata:
```

| | docker run | Docker Compose | Swarm / K8s |
|---|---|---|---|
| 容器数 | 单个 | 多个 | 大规模集群 |
| 适用 | 开发/测试单容器 | 多容器应用（开发/单机） | 生产集群 |
| 编排 | 手动 | YAML 定义 | 声明式 + 自动调度 |
| 服务发现 | 手动 | 自动（compose 网络） | 内置 |

**Swarm vs K8s：** Swarm 轻量零配置，适合小团队；K8s 功能全，分布式生产标配。

---

**Q12: Docker 的日志管理怎么做？日志驱动有哪些？**

Docker 默认用 `json-file` 驱动，日志存在 `/var/lib/docker/containers/<id>/<id>-json.log`，**不会自动轮转，可能撑爆磁盘。**

**日志驱动：**

| 驱动 | 特点 |
|------|------|
| json-file（默认） | 本地 JSON 格式，需手动配轮转 |
| journald | 写入 systemd journal |
| syslog | 写入 syslog |
| fluentd | 发送到 Fluentd 收集器 |
| awslogs / gcplogs | 直接发云服务 |
| loki | Grafana Loki 收集 |
| splunk | 发送到 Splunk |

**最佳实践——json-file + 轮转：**
```json
// /etc/docker/daemon.json
{
  "log-driver": "json-file",
  "log-opts": {
    "max-size": "10m",
    "max-file": "3"
  }
}
```

**查看日志：**
```bash
docker logs --tail 100 -f container_name
docker logs --since 30m container_name
```

---

**Q13: Docker 安全最佳实践有哪些？**

**1. 镜像安全：**
```bash
# 镜像漏洞扫描
docker scan <image>              # Docker 内置 Snyk
trivy image <image>              # 开源扫描器
```
- 用官方镜像或可信来源
- 固定镜像版本（`nginx:1.25` 不要 `nginx:latest`）
- 最小化镜像（用 alpine 或 distroless）

**2. 运行安全：**
```bash
# ❌ 危险
docker run --privileged --cap-add ALL image

# ✅ 最小权限
docker run --read-only --tmpfs /tmp \
           --cap-drop ALL \
           --security-opt no-new-privileges \
           -u 1000:1000 \
           image
```
- 不要用 `--privileged`
- 只加必需的能力（`--cap-add NET_BIND_SERVICE`）
- 用非 root 用户运行
- 只读根文件系统 `--read-only`

**3. 资源限制防 DoS：** 必须设置 `--cpus`、`--memory`、`--pids-limit`

**4. Docker daemon 安全：**
- `/var/run/docker.sock` 有 660 权限，只有 docker 组成员可访问
- daemon 开启 `user-ns-remap`（用户命名空间映射）
- 用 `seccomp` / `AppArmor` / `SELinux` 限制系统调用

---

**Q14: Docker 的联合文件系统 (UnionFS) 是什么？Overlay2 如何工作？**

**UnionFS** 把多个文件系统叠加成一个，上层覆盖下层，对用户透明。

**Overlay2（Docker 现在默认的存储驱动）：**

```
merged/         ← 容器看到的统一视图
├── upperdir    ← 可写层（容器修改写到这里）
├── lowerdir1   ← 镜像 Layer 3（只读）
├── lowerdir2   ← 镜像 Layer 2（只读）
└── lowerdir3   ← 镜像 Layer 1（只读）
```

**写时复制 (Copy-on-Write)：**
- 修改文件时，先从 lowerdir 复制到 upperdir，再在 upperdir 修改
- 删除文件时，在 upperdir 创建 "whiteout" 文件标记为不可见
- 读取时从上往下查找，先找 upperdir

**为什么用 Overlay2：**
- 内核原生支持（4.0+）
- 性能好，内存占用少
- 对比 aufs：已合入内核主线，不需要外部模块

---

**Q15: 如何进入一个已运行的容器？exec 和 attach 的区别？**

```bash
# exec：在容器内启动新进程（推荐）
docker exec -it container_name /bin/bash

# attach：连接到容器主进程的 stdin/stdout/stderr
docker attach container_name
```

| | exec | attach |
|---|---|---|
| 原理 | 启动新进程 | 连接到 PID 1 的终端 |
| `exit` 后果 | 退出 bash，容器继续运行 | 如果主进程是 bash，容器停止 |
| 用途 | 调试、运维 | 查看主进程输出 |

**exec 高级用法：**
```bash
docker exec -it -u root container_name /bin/sh   # 以 root 进去
docker exec container_name cat /etc/hosts        # 直接执行单条命令
```

---

**Q16: 容器退出后怎么排查？**

```bash
# 1. 查看退出状态
docker ps -a --filter "status=exited"

# 2. 查看退出码和日志
docker inspect container_id | jq '.[0].State'
docker logs container_id

# 3. 常见退出码
# 0   → 正常退出
# 1   → 应用错误
# 137 → 被 OOM Killer 杀掉（SIGKILL，内存超限）
# 143 → 收到 SIGTERM 优雅退出
```

**137 排查：** `docker inspect | grep OOMKilled`，`docker stats` 查看内存使用。

---

**Q17: Docker 的健康检查怎么配？**

```dockerfile
# Dockerfile 中定义
HEALTHCHECK --interval=30s --timeout=3s --retries=3 \
  CMD curl -f http://localhost:8080/health || exit 1
```

```bash
# 或 docker run 中
docker run --health-cmd="curl -f http://localhost/health" \
           --health-interval=30s \
           --health-timeout=3s \
           --health-retries=3 \
           nginx
```

**查看健康状态：**
```bash
docker ps                 # STATUS 列显示 (healthy)/(unhealthy)/(starting)
docker inspect --format='{{json .State.Health}}' container_name | jq
```

**健康检查的作用：** Docker Compose 的 `depends_on` 条件、Swarm 服务更新、负载均衡摘除不健康实例。

---

**Q18: Docker 镜像的构建上下文 (Build Context) 是什么？为什么 `.dockerignore` 重要？**

构建上下文是 `docker build` 时传给 Docker daemon 的目录（通常是 `.`）。

```bash
docker build -t myapp .
#                        ↑ 这就是构建上下文
```

**坑：** 如果项目目录巨大（node_modules、.git、logs），整个打包发给 daemon，极慢。

```bash
docker build -t myapp .
# Sending build context to Docker daemon  1.2GB   ← 浪费！
```

**解决方案——`.dockerignore`：**
```
node_modules
.git
*.log
tmp/
.env
```

**最佳实践：** 减小构建上下文 + 远程构建时用 `.dockerignore` + Dockerfile 不放在根目录就用 `-f` 指定。

---

**Q19（实战）: 线上容器 CPU 100%，你怎么排查？**

```
1. 定位容器
   docker stats                     # 找 CPU 高的容器
   docker top container_name        # 看容器内进程

2. 进容器看进程
   docker exec -it container_name /bin/sh
   top -c                            # 找 CPU 高的进程

3. 更细致的诊
   # 宿主机找到容器的 PID namespace
   docker inspect --format '{{.State.Pid}}' container_name
   # 在宿主机 perf/strace
   strace -p <pid> -c                # 统计系统调用
   perf top -p <pid>                 # 看函数级热点

4. 常见原因
   - 死循环 / 无限递归
   - GC 频繁（Java 容器内存设太小）
   - 锁争用（数据库容器）
   - 外部依赖超时导致线程堆积

5. 止血
   docker restart container_name     # 重启（简单粗暴）
   docker pause && docker unpause    # 暂停再恢复（针对死循环无效）
```

---

**Q20（实战）: 数据库容器（如 MySQL/PostgreSQL）为什么不应该随便重启？应该怎么配？**

**坑点：**

1. **数据丢失风险**：容器停止后，未持久化的数据没了
2. **优雅退出**：数据库需要时间 flushing dirty pages + checkpoint，`docker stop` 默认等 10s 后 SIGKILL，可能损坏数据
3. **Volume 是底线**：没挂 Volume 就重启 = 删库

**正确姿势：**

```yaml
# docker-compose.yml
services:
  db:
    image: mysql:8.0
    restart: unless-stopped
    environment:
      MYSQL_ROOT_PASSWORD: ${MYSQL_PASSWORD}
    volumes:
      - db_data:/var/lib/mysql          # Volume 持久化
      - ./mysql.cnf:/etc/mysql/conf.d/custom.cnf:ro  # 配置文件
    stop_grace_period: 60s              # 给足 flush 时间
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "localhost"]
      interval: 10s
      timeout: 5s
      retries: 5

volumes:
  db_data:
```

**重启规则选择：**
| 规则 | 适用 |
|------|------|
| `no`（默认） | 手动管理 |
| `always` | 即使手动停止，daemon 重启也自动拉起来 |
| `unless-stopped` | ✅ **生产推荐**：手动 stop 后会保持停止 |
| `on-failure[:次数]` | 仅异常退出时重启 |

---

📝 共 20 题，覆盖基础概念、Dockerfile 优化、网络存储、安全、实战排查。大宝贝看看深度如何？
