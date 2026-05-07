# Kubernetes 1.33 版本的 Pod 资源动态调整功能

> 来源: http://cloudmessage.top/archives/kubernetes133ban-ben-de-podzi-yuan-dong-tai-diao-zheng-gong-neng

在 Kubernetes 集群管理中，精确配置 Pod 的 CPU 和内存资源一直是一个挑战。当应用程序的资源需求超出预期配置时，传统的解决方案只能通过完全重启 Pod 来调整资源限制。这种方式不仅会造成服务中断，还会影响用户体验，特别是对于有状态应用和数据库服务而言。

Kubernetes 1.33 版本引入了一个重要的新功能：**Pod 原地垂直扩缩容**[1]。这项功能允许管理员在不重启 Pod 的情况下动态调整运行中容器的 CPU 和内存资源。

该功能在 Kubernetes 1.33 中已升级为 beta 版本并默认启用，无需手动开启功能门控即可在生产环境中使用（Kubernetes 官方文档[2]）。

对于一直关注垂直 Pod 自动扩缩器（VPA）[3]发展的用户来说，这个功能具有特殊意义。VPA 虽然在理论上功能强大，但其当前的"重新创建"模式在实际应用中存在较大局限性。Pod 原地调整功能提供了一种更优雅的资源调整方式，为未来 VPA 的改进奠定了基础。

## 功能价值与应用意义

Pod 原地垂直扩缩容功能对 Kubernetes 生态系统具有重要意义。在传统架构中，当应用程序遇到流量激增时，管理员面临两个选择：要么预先过度配置资源（增加成本），要么通过 VPA 触发 Pod 重启来调整资源（造成服务中断）。新功能允许管理员动态增加 CPU 和内存资源，在保持应用程序正常运行的同时满足用户需求。

这项功能对有状态应用、数据库系统以及需要持续可用性的服务特别有价值。通过原地调整，可以显著减少停机时间，提供更加流畅的扩缩容体验。具体优势包括：

消除 Pod 重启风险：传统的资源调整方式需要终止并重新创建 Pod，这种方式类似于在系统运行时进行高风险操作。垂直 Pod 自动扩缩器（VPA）[4]会强制终止 Pod 来应用新的资源配置。新功能则可以在不中断服务的情况下平滑调整资源。

优化成本结构：减少了"预防性过度配置"的需求。正如Sysdig 团队分析[5]所指出的，这种功能实现了真正的按需付费云计算模式。

提升有状态工作负载可用性：数据库和其他有状态服务不再需要在性能和可用性之间做出妥协。虽然操作仍有一定风险，但技术上已经可行。

## 实际应用场景

以下场景展示了 Pod 原地调整功能的实际价值：

数据库工作负载：当 PostgreSQL 实例需要处理来自业务部门的大型分析查询而需要更多内存时，可以在不中断现有事务或断开连接池的情况下扩展资源。这避免了向用户显示"服务暂时不可用"的错误信息。

Node.js API 服务：Node.js 应用程序可以在流量高峰期间动态利用额外的 CPU 和内存资源，而无需重启服务。这使得此类应用成为原地调整的理想候选者。

机器学习推理服务：当 TensorFlow Serving Pod 需要处理更大批次的数据或更复杂的模型时，可以在不中断正在进行的推理请求的情况下分配额外资源。

服务网格边车容器：Istio 等服务网格中的 Envoy 代理可以根据流量模式动态调整资源，而不会影响主应用容器的运行。

Java 应用程序的特殊考虑：对于基于 JVM 的应用程序，需要注意一个重要限制。简单调整 Pod 的内存资源并不会自动调整 JVM 的堆大小，因为堆大小通常在启动时通过-Xmx等参数设置（参考资料[6]）。虽然原地调整可以帮助处理非堆内存和 CPU 资源，但要充分利用增加的内存限制，Java 应用程序通常需要配置调整和重启。因此，这类应用程序不太适合进行无重启的内存调整。

## 技术实现原理

Pod 原地调整功能的实现涉及 Kubernetes 系统的多个组件协同工作。以下详细说明了该功能的技术实现机制：

可变资源字段机制：通过KEP-1287[7]提案的实现，Pod 规范中的resources.requests和resources.limits字段现在支持运行时修改。这解决了之前 Pod 规范不可变的限制。

Kubelet 资源验证：当管理员提交资源调整请求时，kubelet 会执行资源可用性检查。计算公式为：(节点可分配容量) - (所有现有容器分配总和) >= (新的资源请求)。如果条件满足，则继续执行调整；否则，系统会设置PodResizePending状态。

容器运行时接口（CRI）交互：kubelet 通过 CRI 与底层容器运行时（如 containerd 或 CRI-O）通信，指示运行时为指定容器分配更多或更少的 CPU/内存资源。运行时通过调整相应的 cgroups 配置来实现资源变更，整个过程无需重启容器。该过程采用异步非阻塞方式执行，确保 kubelet 可以继续处理其他重要任务（参考文档[8]）。

状态跟踪机制：系统提供两个新的 Pod 状态条件来跟踪调整进度：

- PodResizePending：表示节点资源不足，需要等待资源释放
- PodResizeInProgress：表示调整操作正在进行中

## 容器运行时兼容性

不同容器运行时对 Pod 原地调整功能的支持程度有所差异：

- containerd(v1.6+) ：提供完整支持，能够流畅地更新 CPU 和内存的 cgroup 配置。
- CRI-O (v1.24+) ：完全支持所有调整操作。
- Docker ：支持有限，且由于 Docker 正在从 Kubernetes 生态系统中逐步移除，不建议在新部署中使用。

需要注意的是，cgroup v2[9]相比 cgroup v1 在调整操作期间提供了更好的内存管理能力，特别是在减少内存限制时表现更佳（技术文档[10]）。

## 实践演示

以下演示展示了 Pod 原地调整功能的实际操作过程。该演示可以从 Kubernetes API 和 Pod 内部两个角度观察调整效果。演示环境为运行 Kubernetes 1.33。

![](https://img.cloudmessage.top/i/2025/08/11/68996173b8043.png)

### 创建资源监控 Pod

首先创建一个持续监控自身资源分配的 Pod：

```
# root@k8s-master01:~/resize# cat 1.demo.yaml apiVersion: v1kind: Podmetadata:  name: resize-demospec:  containers:  - name: resource-watcher    image: docker.cnb.cool/wangyanglinux/docker-images-chrom/ubuntu:22.04    command:    - "/bin/bash"    - "-c"    - |      apt-get update && apt-get install -y procps bc      echo "=== Pod Started: $(date) ==="      # 读取容器资源限制的函数      get_cpu_limit(){        if [ -f /sys/fs/cgroup/cpu.max ]; then          # cgroup v2          local cpu_data=$(cat /sys/fs/cgroup/cpu.max)          local quota=$(echo $cpu_data | awk '{print $1}')          local period=$(echo $cpu_data | awk '{print $2}')          if [ "$quota" = "max" ]; then            echo "unlimited"          else            echo "$(echo "scale=3; $quota / $period" | bc) cores"          fi        else          # cgroup v1          local quota=$(cat /sys/fs/cgroup/cpu/cpu.cfs_quota_us)          local period=$(cat /sys/fs/cgroup/cpu/cpu.cfs_period_us)          if [ "$quota" = "-1" ]; then            echo "unlimited"          else            echo "$(echo "scale=3; $quota / $period" | bc) cores"          fi        fi      }      get_memory_limit(){        if [ -f /sys/fs/cgroup/memory.max ]; then          # cgroup v2          local mem=$(cat /sys/fs/cgroup/memory.max)          if [ "$mem" = "max" ]; then            echo "unlimited"          else            echo "$((mem / 1048576)) MiB"          fi        else          # cgroup v1          local mem=$(cat /sys/fs/cgroup/memory/memory.limit_in_bytes)          echo "$((mem / 1048576)) MiB"        fi      }      # 每5秒输出资源信息      while true; do        echo "---------- Resource Check: $(date) ----------"        echo "CPU limit: $(get_cpu_limit)"        echo "Memory limit: $(get_memory_limit)"        echo "Available memory: $(free -h | grep Mem | awk '{print $7}')"        sleep 5      done    resizePolicy:    - resourceName: cpu      restartPolicy: NotRequired    - resourceName: memory      restartPolicy: NotRequired    resources:      requests:        memory: "128Mi"        cpu: "100m"      limits:        memory: "128Mi"        cpu: "100m"
```

### 检查 Pod 初始状态

通过 Kubernetes API 查看 Pod 的资源配置：

```
kubectl describe pod resize-demo | grep -A8 Limits:
```

预期输出

```
Limits:      cpu:     100m      memory:  128Mi    Requests:      cpu:     100m      memory:  128Mi
```

查看 Pod 内部的资源视图

```
kubectl logs resize-demo --tail=8
```

### CPU 资源动态调整

执行 CPU 资源翻倍操作

```
kubectl patch pod resize-demo --subresource resize --patch \  '{"spec":{"containers":[{"name":"resource-watcher", "resources":{"requests":{"cpu":"200m"}, "limits":{"cpu":"200m"}}}]}}'
```

检查调整状态

```
kubectl get pod resize-demo -o jsonpath='{.status.conditions[?(@.type=="PodResizeInProgress")]}'
```

验证 API 层面的资源更新

```
kubectl describe pod resize-demo | grep -A8 Limits:
```

确认 Pod 内部观察到的变化

```
kubectl logs resize-demo --tail=8
```

此时可以观察到 CPU 限制从100m增加到200m，且 Pod 没有重启。Pod 日志会显示 cgroup CPU 限制从约10000/100000变为20000/100000（对应 100m 到 200m 的 CPU 配额）。

### 内存资源动态调整

执行内存资源翻倍操作

```
kubectl patch pod resize-demo --subresource resize --patch \  '{"spec":{"containers":[{"name":"resource-watcher", "resources":{"requests":{"memory":"256Mi"}, "limits":{"memory":"256Mi"}}}]}}'
```

通过 API 验证变更

```
kubectl describe pod resize-demo | grep -A8 Limits:
```

通过 Pod 内部验证：

```
kubectl logs resize-demo --tail=8
```

可以观察到内存限制在无容器重启的情况下从 128Mi 增加到 256Mi。

### 验证无重启操作

确认整个调整过程中容器未发生重启：

```
kubectl get pod resize-demo -o jsonpath='{.status.containerStatuses[0].restartCount}'
```

输出应为0，证明实现了无服务中断的资源调整。

### 清理资源

完成演示后清理测试资源

```
kubectl delete pod resize-demo
```

通过以上演示可以看出，Pod 原地调整功能成功实现了 CPU 和内存资源的动态调整，且无需 Pod 重启。该功能适用于任何容器化应用，只需正确配置resizePolicy 即可。

## 云服务商支持情况

在将该功能应用于生产环境之前，需要了解各主要 Kubernetes 服务提供商的支持状态：

Google Kubernetes Engine (GKE) ：功能已在快速通道版本中提供（GKE 官方文档[11]）。

Amazon EKS：Kubernetes 1.33 版本计划于 2025 年 5 月发布。

Azure AKS：Kubernetes 1.33 版本现已提供预览版本（AKS 发布说明[12]）。

自管理集群：只要运行 Kubernetes 1.33+版本并使用 containerd 或 CRI-O 运行时，即可完全支持该功能。

## 功能限制与注意事项

虽然 Pod 原地调整功能具有重要价值，但在实际应用中需要注意以下限制：

### 平台与运行时限制

操作系统限制：该功能目前仅支持 Linux 平台（Kubernetes 官方文档确认[13]）。

节点级别排除：使用静态 CPU 或内存管理器（如static CPU 管理器策略）管理的 Pod 无法使用此功能。

容器运行时要求：需要 containerd v1.6+或 CRI-O v1.24+才能获得完整支持。

### 资源管理约束

服务质量等级不变性：Pod 的原始 QoS 类别（Guaranteed、Burstable、BestEffort）在创建后无法通过调整操作改变。无法仅通过资源调整将BestEffort类别的 Pod 升级为Guaranteed类别。

资源类型限制：当前版本仅支持 CPU 和内存的原地调整。GPU、临时存储等其他资源类型暂不支持动态调整。

内存缩减风险：在不重启容器的情况下减少内存限制存在风险。建议设置restartPolicy: RestartContainer 作为安全措施，避免系统不稳定。这一点在 cgroup v1 系统中尤为重要。

交换内存考虑：如果 Pod 使用交换内存，除非将内存的 resizePolicy 设置为 RestartContainer，否则无法进行原地内存调整。

资源配置不可移除：一旦设置了资源请求或限制，只能通过原地调整修改数值，无法完全移除这些配置。

### 配置与集成限制

容器类型限制：

- Init 容器和临时容器不支持原地调整
- 边车容器支持原地调整功能

调整策略固定性：Pod 创建后无法修改其resizePolicy配置。该配置需要在 Pod 创建时确定，后续无法变更。

应用程序特定限制：如前所述，基于 JVM 的应用程序无法在不进行配置更改和重启的情况下自动利用增加的内存限制（参考资料[14]）。这一限制适用于任何内部管理内存池的应用程序。

资源缩减陷阱：尝试将内存缩减到当前使用量以下可能导致 OOM（内存不足）错误，即使设置了restartPolicy也可能出现问题。

### 性能考虑因素

节点资源余量：节点需要有足够的可用容量才能成功执行调整操作。在资源密集的集群中，可能会频繁出现PodResizePending状态。

调整操作延迟：kubelet 异步处理调整请求，复杂的 cgroup 更新可能需要数秒时间才能完全生效。

调度器感知缺失：调度器在做出调度决策时不会考虑正在进行的调整操作，这可能导致意外的资源压力。

了解这些限制有助于在实际部署中避免潜在问题，确保功能的稳定运行。

## VPA 集成现状与展望

垂直 Pod 自动扩缩器（VPA）长期以来在 Kubernetes 生态系统中扮演着重要但有限的角色。如相关分析[15]所指出的，VPA 在扩缩容功能家族中一直处于相对边缘的位置。

当前状态（Kubernetes 1.33）：VPA 尚未支持原地调整功能，在调整资源时仍然采用重新创建 Pod 的方式。Kubernetes 官方文档[16]明确说明："截至 Kubernetes 1.33 版本，VPA 不支持 Pod 原地调整，但相关集成工作正在进行中。"

目前，kubernetes/autoscaler PR 7673[17]正在积极开发 VPA 与原地调整功能的集成。

**未来集成方案：**
KEP-4951[18]提出的混合方案将使 VPA 真正适用于生产环境中的有状态工作负载。在完全集成实现之前，管理员需要手动管理 Pod 资源调整。

### 当前阶段的 VPA 与手动调整结合方案

在等待完整集成的过程中，可以采用以下方法结合两种功能的优势：

VPA 建议模式：将 VPA 设置为"Off"模式，仅获取资源建议而不自动应用变更。

手动应用调整：基于 VPA 的建议手动执行原地调整操作。

自动化脚本：通过脚本自动化手动操作流程，避免 Pod 重新创建。

```
# VPA建议应用脚本示例#!/bin/bashPOD_NAME="my-important-db"CPU_REC=$(kubectl get vpa db-vpa -o jsonpath='{.status.recommendation.containerRecommendations[0].target.cpu}')MEM_REC=$(kubectl get vpa db-vpa -o jsonpath='{.status.recommendation.containerRecommendations[0].target.memory}')kubectl patch pod $POD_NAME --subresource resize --patch \  "{\"spec\":{\"containers\":[{\"name\":\"database\",\"resources\":{\"requests\":{\"cpu\":\"$CPU_REC\",\"memory\":\"$MEM_REC\"}}}]}}"
```

## 技术发展方向

Kubernetes 1.33 的 Pod 原地调整功能标志着垂直扩缩容技术向无缝、无中断方向迈出的重要一步，但技术发展仍在继续。随着功能的成熟，可以预期以下发展方向：

VPA 完整集成（开发中[19]）：未来的 VPA 将优先尝试原地调整，仅在必要时才回退到重新创建模式，从而消除意外的 Pod 驱逐。

多资源类型支持：除 CPU 和内存外，未来版本可能支持 GPU、临时存储等资源的动态调整。这将使机器学习训练 Pod 等工作负载能够在运行过程中动态调整资源。

调度器集成：当前调整操作不会通知调度器，在节点资源不足时 Pod 仍可能被驱逐。未来的改进可能将调整后的 Pod 视为优先保护对象，预留资源空间并避免意外的重新调度。

集群自动扩缩器协同：与集群自动扩缩器的集成可能实现更智能的资源决策，仅在原地调整无法满足需求时才扩展节点。

基于指标的智能调整：未来可能支持基于应用级指标（如请求延迟、队列深度）的自动调整，而不仅限于 CPU 和内存使用率。

这些技术发展将共同构建一个真正动态、高效且无中断的垂直扩缩容生态系统。建议 Kubernetes 管理员在非生产环境中测试 Pod 原地调整功能，积累经验并为生产环境部署做好准备。在正式应用前，务必仔细阅读功能限制并进行充分测试。

## 参考资料

Pod 原地垂直扩缩容: [https://kubernetes.io/docs/tasks/configure-pod-container/resize-container-resources/](https://kubernetes.io/docs/tasks/configure-pod-container/resize-container-resources/)

[2]
Kubernetes 官方文档: [https://kubernetes.io/docs/tasks/configure-pod-container/resize-container-resources/](https://kubernetes.io/docs/tasks/configure-pod-container/resize-container-resources/)

[3]
垂直 Pod 自动扩缩器（VPA）: [https://www.linkedin.com/pulse/vpa-kubernetes-autoscaler-could-doesnt-yet-alexei-ledenev-dkrbf](https://www.linkedin.com/pulse/vpa-kubernetes-autoscaler-could-doesnt-yet-alexei-ledenev-dkrbf)

[4]
垂直 Pod 自动扩缩器（VPA）: [https://kubernetes.io/docs/concepts/workloads/autoscaling/](https://kubernetes.io/docs/concepts/workloads/autoscaling/)

[5]
Sysdig 团队分析: [https://sysdig.com/blog/kubernetes-1-33-whats-new/](https://sysdig.com/blog/kubernetes-1-33-whats-new/)

[6]
参考资料: [https://stackoverflow.com/questions/77625610/jvm-and-heap-size-in-a-pod-on-kubernetes](https://stackoverflow.com/questions/77625610/jvm-and-heap-size-in-a-pod-on-kubernetes)

[7]
KEP-1287: [https://github.com/kubernetes/enhancements/issues/1287](https://github.com/kubernetes/enhancements/issues/1287)

[8]
参考文档: [https://kubernetes.io/blog/2023/05/12/in-place-pod-resize-alpha/](https://kubernetes.io/blog/2023/05/12/in-place-pod-resize-alpha/)

[9]
cgroup v2: [https://kubernetes.io/docs/concepts/architecture/cgroups/](https://kubernetes.io/docs/concepts/architecture/cgroups/)

[10]
技术文档: [https://kubernetes.io/blog/2023/05/12/in-place-pod-resize-alpha/](https://kubernetes.io/blog/2023/05/12/in-place-pod-resize-alpha/)

[11]
GKE 官方文档: [https://opensource.googleblog.com/2025/05/kubernetes-1.33-available-on-gke.html](https://opensource.googleblog.com/2025/05/kubernetes-1.33-available-on-gke.html)

[12]
AKS 发布说明: [https://github.com/azure/aks/releases](https://github.com/azure/aks/releases)

[13]
Kubernetes 官方文档确认: [https://kubernetes.io/docs/tasks/configure-pod-container/resize-container-resources/](https://kubernetes.io/docs/tasks/configure-pod-container/resize-container-resources/)

[14]
参考资料: [https://xebia.com/blog/guide-kubernetes-jvm-integration/](https://xebia.com/blog/guide-kubernetes-jvm-integration/)

[15]
相关分析: [https://www.linkedin.com/pulse/vpa-kubernetes-autoscaler-could-doesnt-yet-alexei-ledenev-dkrbf](https://www.linkedin.com/pulse/vpa-kubernetes-autoscaler-could-doesnt-yet-alexei-ledenev-dkrbf)

[16]
Kubernetes 官方文档: [https://kubernetes.io/docs/concepts/workloads/autoscaling/](https://kubernetes.io/docs/concepts/workloads/autoscaling/)

[17]
kubernetes/autoscaler PR 7673: [https://github.com/kubernetes/autoscaler/pull/7673](https://github.com/kubernetes/autoscaler/pull/7673)

[18]
KEP-4951: [https://github.com/kubernetes/enhancements/issues/4951](https://github.com/kubernetes/enhancements/issues/4951)

[19]
开发中: [https://github.com/kubernetes/autoscaler/pull/7673](https://github.com/kubernetes/autoscaler/pull/7673)