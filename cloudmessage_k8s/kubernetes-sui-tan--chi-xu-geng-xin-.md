# Kubernetes - 碎谈（持续更新）

> 来源: http://cloudmessage.top/archives/kubernetes-sui-tan--chi-xu-geng-xin-

## kubelet 如果调用 CNI 插件实现网络创建

Kubernetes 中，节点上的 Kubelet 组件负责调用 CNI 插件为 Pod 配置网络，其核心逻辑正是基于节点本地的 /etc/cni/net.d/ 目录及其中的配置文件。

### 具体逻辑细节：

**1、配置文件位置****每个节点的 /etc/cni/net.d/ 目录是 CNI 插件的配置中心，所有 CNI 插件的配置文件（通常以 .conf 或 .json 结尾）都存放在这里。这些配置文件描述了插件类型（如 flannel、calico、ipvlan 等）、网络参数（子网、网关、模式等）。

2、调用优先级：按文件名排序****Kubelet 会按文件名的字母数字顺序读取 /etc/cni/net.d/ 目录下的配置文件，选择第一个有效配置文件作为默认 CNI 插件的配置。
例如：

- 若目录下有 10-ipvlan.conf、20-calico.conf 两个文件，Kubelet 会优先读取 10-ipvlan.conf（因为 "10" 比 "20" 小），并使用 ipvlan 插件为 Pod 配置网络。
- 文件名的前缀（如 10-、20-）通常是为了显式控制优先级（数字越小，优先级越高），这是行业惯例。

3、执行流程****当 Kubelet 创建 Pod 时，会触发以下步骤：

- 读取 /etc/cni/net.d/ 下的配置文件，按优先级选择默认配置；
- 根据配置文件中的 type 字段（如 type: "ipvlan"），找到对应的 CNI 插件二进制文件（通常放在 /opt/cni/bin/ 目录）；
- 调用该插件，根据配置参数为 Pod 分配 IP、创建网络接口、配置路由等，最终完成 Pod 的网络初始化。

4、多插件场景**
若需要为 Pod 配置多个网络接口（如主网络 + 辅助网络），则需要借助 Multus 等 “元插件”。Multus 会作为 “入口” 插件（配置文件通常以 00-multus.conf 命名，确保优先级最高），再根据其他配置文件调用不同的 CNI 插件（如 ipvlan、macvlan 等）为 Pod 添加多个接口。

综上，Kubernetes 对 CNI 插件的调用逻辑核心是：节点级别的配置文件目录 + 文件名排序优先级，这也是 CNI 规范的约定，确保了插件调用的可预测性。

## --verb 种类

在 Kubernetes 中，--verb 用于指定对资源（--resource）的操作权限，除了 create 之外，常用的动词（verbs）还包括以下几类，涵盖了对资源的完整生命周期操作

### 基础查询类（读取资源）

- get：获取单个资源的详细信息（如 kubectl get deployment nginx）。
- list：列出同一类型的所有资源（如 kubectl get deployments）。
- watch：实时监听资源的变化（如 kubectl get pods --watch），常用于持续跟踪资源状态更新。

### 修改类（更新资源）

- update：全量更新资源（需提供完整的资源定义，覆盖原有配置）。
- patch：部分更新资源（仅需提供要修改的字段，如通过 kubectl patch 修改副本数）。

### 删除类（移除资源）

- delete：删除单个资源（如 kubectl delete deployment nginx）。
- deletecollection：删除多个符合条件的资源（批量删除，通常配合标签选择器使用）。

### 特殊操作类（针对特定场景）

- proxy：为资源创建代理连接（如 kubectl proxy 访问 apiserver 代理的 Pod 服务）。
- connect：建立与资源的直接连接（如通过 kubectl exec 进入 Pod 时使用）。
- create：创建资源（你命令中已使用，如创建 Deployment、StatefulSet 等）。
- edit：编辑资源的配置（等价于 get + update 的组合，如 kubectl edit deployment nginx）。

### 子资源操作类（针对资源的子对象）

部分资源有子资源（如 Pod 的 logs、Deployment 的 scale 等），对应的动词包括：

- logs：获取 Pod 的日志（kubectl logs pod-name）。
- exec：在 Pod 中执行命令（kubectl exec -it pod-name -- /bin/bash）。
- port-forward：端口转发（kubectl port-forward pod-name 8080:80）。
- scale：调整控制器的副本数（kubectl scale deployment nginx --replicas=5）。