# Kubernetes - pod 多网卡配置

> 来源: http://cloudmessage.top/archives/kubernetes-pod多网卡配置

## 背景

一个容器启动后，在默认情况下一般都会只存在两个虚拟网络接口（loopback 和 eth0），而 loopback 的流量始终都会在本容器内或本机循环，真正对业务起到支撑作用的只有 eth0，当然这对大部分业务场景而言已经能够满足。

但是如果一个应用或服务既需要对外提供 API 调用服务，也需要满足自身基于分布式特性产生的数据同步，那么这时候一张网卡的性能显然很难达到生产级别的要求，网络流量延时、阻塞便成为此应用的一项瓶颈。

基于上述痛点和需求，容器多网络方案不断涌现。k8s 有一个多网卡规范：[K8sNetworkPlumbingWG/multi-net-spec](https://github.com/K8sNetworkPlumbingWG/multi-net-spec)。目前已知的多网卡方案：

- 华为开发的多网络 CNI 插件：[huawei-cloudnative/CNI-Genie](https://github.com/huawei-cloudnative/CNI-Genie)
- Intel 开发的多网络 CNI 插件： [k8snetworkplumbingwg/multus-cni](https://github.com/k8snetworkplumbingwg/multus-cni)

而根据开源社区活跃度、是否实现 CNI 规范以及稳定性，建议采用 multus-cni 作为在 K8s 环境下的容器多网络方案。

![](https://img.cloudmessage.top/i/2025/08/21/68a6bdf17e9e4.png)

Multus CNI 简单来说是一种符合 CNI（Container Network Interface）规范的开源插件，旨在为实现 K8s（Kubernetes） 环境下容器多网卡而提出的解决方案。它作为一种“元插件”，可以与其他 CNI 插件搭配使用。

## Multus CNI

> 
Multus CNI enables attaching multiple network interfaces to pods in Kubernetes.

以上是 Multus CNI 项目官方对其存在意义的精简描述，它的存在就是帮助 K8s 的 Pod（可简单理解为一组容器的集合，是 K8s 可管理的最小“容器”单位）建立多网络接口。

Multus CNI 本身不提供网络配置功能，它是通过用其他满足 CNI 规范的插件进行容器的网络配置。

如下图所示，在此场景下我们可以把 Pod 抽象为单个容器。原本容器里应仅存在 eth0 接口（loopback 忽略不计），是由主插件产生创建并配置的；而当集群环境存在 Multus CNI 插件，并添加额外配置后，将会发现此容器内不再仅有 eth0 接口，你可以利用这些新增的接口去契合实际业务需求。

![](https://img.cloudmessage.top/i/2025/08/21/68a6be3ddb2b4.png)

![](https://img.cloudmessage.top/i/2025/08/21/68a6be47c051e.png)

![](https://img.cloudmessage.top/i/2025/08/21/68a6be4dd758c.png)

## CNI 规范

CNI (Container Network Interface), a Cloud Native Computing Foundation project, consists of a specification and libraries for writing plugins to configure network interfaces in Linux containers, along with a number of supported plugins. CNI concerns itself only with network connectivity of containers and removing allocated resources when the container is deleted. Because of this focus, CNI has a wide range of support and the specification is simple to implement.

以上引用了官方对其的描述，简短来讲 CNI 是一组限于容器网络面的规范，定义了容器网络资源创建、管理的规则。

但它并不仅仅是规范，它也包含了库和实现，CNI 自身实现并提供了内置且通用的网络插件，同时为第三方实现其规范预留了扩展。

CNI 将插件分成三种类型：

![](https://img.cloudmessage.top/i/2025/08/21/68a6bebea1f05.png)

到这里，我们已经明白，Multus CNI 属于 Meta 类, 它可以与其他第三方插件适配（也就是主插件），主插件来作为 Pod 的主网络并且被 K8s 所感知，它们可以搭配使用且不冲突。

## 适用场景

这边具体讲下 multus-cni 的使用场景，一般可以在需要网络隔离的情况下使用额外网络，包括分离数据平面流量与控制平面流量。隔离网络流量对以下性能和安全性是很有用的：

安全性：用户可以将敏感的流量发送到专为安全考虑而管理的网络平面，也可隔离不能在租户或客户间共享的私密数据。

性能：用户可以在两个不同的平面上发送流量，以管理每个平面上流量的多少。

## Multus CNI 部署

集群已选用 Calico 作为网络插件并配置为 IPIP 模式

```
$ cat /etc/cni/net.d/10-calico.conflist | jq{  "name": "k8s-pod-network",  "cniVersion": "0.3.1",  "plugins": [    {      "type": "calico",      "log_level": "info",      "log_file_path": "/var/log/calico/cni/cni.log",      "datastore_type": "kubernetes",      "nodename": "multuscni-test0",      "mtu": 0,      "ipam": {        "type": "calico-ipam"      },      "policy": {        "type": "k8s"      },      "kubernetes": {        "kubeconfig": "/etc/cni/net.d/calico-kubeconfig"      }    },    {      "type": "portmap",      "snat": true,      "capabilities": {        "portMappings": true      }    },    {      "type": "bandwidth",      "capabilities": {        "bandwidth": true      }    }  ]}
```

Multus 的快速入门方法是使用 Daemonset（一种在集群中的每个节点上运行 pod 的方法）进行部署，这会启动安装 Multus 二进制文件并配置 Multus 以供使用的 pod。

### 1、安装 Multus daemonset

首先，克隆这个 GitHub 存储库

```
git clone https://github.com/k8snetworkplumbingwg/multus-cni.git && cd multus-cni
```

使用kubectl从这个 repo 应用一个 YAML 文件。

```
cat ./deployments/multus-daemonset-thick.yml | kubectl apply -f -
```

- multus-daemonset.yml这是 Multus 的默认基础部署清单，适用于大多数使用 Docker 或 Containerd 作为容器运行时的 Kubernetes 集群。
- 特点：仅包含 Multus 核心组件（multus-cni 二进制文件和基础配置），依赖集群中已存在的 CNI 插件目录（通常是 /etc/cni/net.d）。
- 适用场景：通用集群环境，默认推荐使用，除非有特殊运行时或功能需求。

- multus-daemonset-crio.yml专门针对CRI-O 容器运行时的部署清单。
- 特点：CRI-O 对 CNI 插件的路径和权限要求与 Docker/Containerd 略有不同（例如 CNI 配置目录可能为 /etc/cni/net.d，但运行时交互逻辑有差异），该文件通过调整挂载路径、权限等配置适配 CRI-O。
- 适用场景：当集群使用 CRI-O 作为容器运行时（而非 Docker/Containerd）时，需使用此文件部署，避免因运行时差异导致 Multus 无法正常工作。

- multus-daemonset-thick.yml“厚部署” 清单，包含更多附加组件的完整部署包。
- 特点：除 Multus 核心组件外，还会内置一些常用的基础 CNI 插件（如 bridge、host-local、loopback 等），并通过 ConfigMap 预配置部分默认网络参数。
- 适用场景：集群中未预先安装基础 CNI 插件，或需要快速搭建包含基础网络功能的 Multus 环境（无需额外手动部署基础 CNI 插件）。

#### Multus daemonset 作用

- 启动一个 Multus 守护程序集，这会在每个节点上运行一个 pod，它在每个节点上放置一个 Multus 二进制文件/opt/cni/bin
- 读取 按字典顺序（按字母顺序）的第一个配置文件/etc/cni/net.d，并在每个节点上为 Multus 创建一个新的配置文件/etc/cni/net.d/00-multus.conf，此配置是自动生成的，并且基于默认网络配置（假定为按字母顺序排列的第一个配置）
- 在每个节点上创建一个/etc/cni/net.d/multus.d目录，其中包含 Multus 访问 Kubernetes API 的身份验证信息。

#### 验证安装

```
kubectl get pods --all-namespaces | grep -i multus
```

你可以通过查看/etc/cni/net.d/目录进一步验证它是否已运行，并确保自动生成的/etc/cni/net.d/00-multus.conf存在对应于按字母顺序排列的第一个配置文件。

因为 Multus CNI 使用的是 DaemonSet 类型，所以默认在所有节点都有一个实例，以下 Whereabouts 同理。

经观察，Pod 运行不久后，将会在各节点上的/opt/cni/bin/下生成 multus 的可执行文件，/etc/cni/下生成网络定义文件以及用于配置集群访问的文件。

```
➜  ~ ls /etc/cni/net.d 00-multus.conf  10-calico.conflist  calico-kubeconfig  multus.d➜  ~ ls /etc/cni/net.d/multus.d multus.kubeconfig➜  multus.d cat multus.kubeconfig # Kubeconfig file for Multus CNI plugin.apiVersion: v1kind: Configclusters:- name: local  cluster:    server: https://[10.233.0.1]:443    certificate-authority-data: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUN5RENDQWJDZ0F3SUJBZ0lCQURBTkJna3Foa2lHOXcwQkFRc0ZBREFWTVJNd0VRWURWUVFERXdwcmRXSmwKY201bGRHVnpNQjRYRFRJeU1EVXdNakV6TkRBek0xb1hEVE15TURReU9URXpOREF6TTFvd0ZURVRNQkVHQTFVRQpBeE1LYTNWaVpYSnVaWFJsY3pDQ0FTSXdEUVlKS29aSWh2Y05BUUVCQlFBRGdnRVBBRENDQVFvQ2dnRUJBTTU3CmZJTmdnZXowa0dpdmVYdy9uVDVFZ1lQVkxJWFlkOGludHVRM2NQdGh0UWFSTzdwSGJUOE5JV3RSN01BbC95eUUKQlVYOUxyME4rbENobVVvR1VldWFZVXhtdUtGM1Z3ZEpFVEZlQnhMdXhoMTRIY1JZcHdvZnY2WVhkTHBpc2FvbQo2UGVab3AwcEFyNkhXUk5Hc0pxanNwbEdiNENSd0UxSG1rRmdPZUVUQXlTQVRqaFFqQVl5NE9NMmsrakx6eHJBCjlFN1lwYzhaa3drdWdSWWtkN3JLbGpIK2laV1dsS3JUNnFxQy8yWU0zVHN2Qm9DWXVGSE9tYW9mL2hmeG9LM2sKT1BpTFBxRlpCZjVrdVNSYU1RMnNodjJzT1g1dE9VMURUSXBaOThDelZ2cEJnSUZWdkZIYTFPTGpVVHBVN25XQQprNUViWjZvWFBGMzhxRmVLRlhNQ0F3RUFBYU1qTUNFd0RnWURWUjBQQVFIL0JBUURBZ0trTUE4R0ExVWRFd0VCCi93UUZNQU1CQWY4d0RRWUpLb1pJaHZjTkFRRUxCUUFEZ2dFQkFGWlBNTDFDeUtqaWx5RjlyMUZzQmpKSjJRRWwKSFhEVXh4SkhTRTc0ak5KWFNXcGlER2M1aFQ3Q2ZaNEF4ZjRKSVczZzhGeW9hY3J1OWFPc3VoQ25XRFRWV0Z1ZgpodmIyZWdUSDVjdGtCZ1Zac2NmTVZOOGVIblZxYUc4Z2dYUlpqM2IxajVoSHI3b0xva0VNakFWN0ZTb3NPY2JUCmlGWFhRdjdiNHBaM0RWRHVlU0JsYUhHQmRINi9IaVBHVTJjSFJJQ3g0YTVITmhWU0t0ZlRoN0wwOHphOVhmcXQKTklFcTYyQURRMHlXTkxES1I3WXVMcUQ0c0lsQ2ROTVlLV1haVm5za0ZPWmoyVTBoWkt3emVCWDQ0MEN1bGNFaQprRWFGdTQvd0Z2US9JTDdXRjYyM2k4SjZzblAwQUR4b2lxUS9ZSmp3ZjJkeEd5MFJ6VVczZXU0MHd4OD0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=users:- name: multus  user:    token: "eyJhbGciOiJSUzI1NiIsImtpZCI6IjhrbTBSdlhBSFhlb3lITDh6eG5qU01nTlgyV3BrdnhqYzR1X29Xd09oRUkifQ.eyJpc3MiOiJrdWJlcm5ldGVzL3NlcnZpY2VhY2NvdW50Iiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9uYW1lc3BhY2UiOiJrdWJlLXN5c3RlbSIsImt1YmVybmV0ZXMuaW8vc2VydmljZWFjY291bnQvc2VjcmV0Lm5hbWUiOiJtdWx0dXMtdG9rZW4tcDQ3NnAiLCJrdWJlcm5ldGVzLmlvL3NlcnZpY2VhY2NvdW50L3NlcnZpY2UtYWNjb3VudC5uYW1lIjoibXVsdHVzIiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9zZXJ2aWNlLWFjY291bnQudWlkIjoiZDgxZWIzZWUtZDFhOS00Y2JiLWE4YjktZGVjMjAyZmVjYmNlIiwic3ViIjoic3lzdGVtOnNlcnZpY2VhY2NvdW50Omt1YmUtc3lzdGVtOm11bHR1cyJ9.WBjNGJhHmdFwnonLKMDo9Pd2yCphRHbY9hRzwlYA4iaH1LT-w-2BM9XXGiafryVxwdMQHnNjl8o204BYEZxwmTnj54DlNUQVi2lLKi3IEGB7ZO11o_8YuGjGCITRTvqCHSFbfKl79H82rnSirOMP0XoHmjRM-s55iVtCu1XVtZY0f44OeMEnTu4hcjHpFjjQwcSRD-egzbCFOyA_DiXzNTwa24LhPEwRNQzi2GhHiS6ThATwUFGOaznPpHhY1vmjRuzAyzStsEVJ5TL_om2s13LcrXfkRgQsMSbynoxrlOmqD2qH9DlSvnAZkPl-Sxvjq8-SJtn5wOyDz3TktjFN4Q"contexts:- name: multus-context  context:    cluster: local    user: multuscurrent-context: multus-context
```

可执行文件的作用是配置 Pod 的网络栈，DaemonSet 的作用是实现网络互通。

一个 Network Namespace 的网络栈包括：网卡（Network interface）、回环设备（Loopback Device）、路由表（Routing Table）和 iptables 规则。

打开 00-multus.conf，查看其内容：

```
➜  net.d cat 00-multus.conf | jq{  "capabilities": {    "bandwidth": true,    "portMappings": true  },  "cniVersion": "0.3.1",  "delegates": [    {      "cniVersion": "0.3.1",      "name": "k8s-pod-network",      "plugins": [        {          "datastore_type": "kubernetes",          "ipam": {            "type": "calico-ipam"          },          "kubernetes": {            "kubeconfig": "/etc/cni/net.d/calico-kubeconfig"          },          "log_file_path": "/var/log/calico/cni/cni.log",          "log_level": "info",          "mtu": 1440,          "nodename": "master",          "policy": {            "type": "k8s"          },          "type": "calico"        },        {          "capabilities": {            "portMappings": true          },          "snat": true,          "type": "portmap"        },        {          "capabilities": {            "bandwidth": true          },          "type": "bandwidth"        }      ]    }  ],  "logLevel": "verbose",  "logToStderr": true,  "kubeconfig": "/etc/cni/net.d/multus.d/multus.kubeconfig",  "name": "multus-cni-network",  "type": "multus"}
```

在 Kubernetes 中，处理容器网络相关的逻辑并不会在 kubelet 主干代码里执行，而是会在具体的 CRI（Container Runtime Interface，容器运行时接口）实现里完成。

CRI 将网络定义文件以 JSON 格式通过 STDIN 方式传递给 Multus CNI 插件可执行文件。文件中 delegates 的意义在于 Multus 会调用其 delegates 指定的插件来执行，这里还有一点需要说明下，如果/etc/cni/net.d/ 目录下有多个网络定义文件，CRI 只会加载按字典顺序排在第一位的文件（即插件），即默认情况下创建 Pod 时使用的是 Calico 插件配置网络。

### 2、创建 NetworkAttachmentDefinition

我们要做的第一件事是为附加到 pod 的每个附加网络接口创建配置（NetworkAttachmentDefinition）。我们将通过创建自定义资源来做到这一点。快速启动安装的一部分会创建一个“CRD”，它是我们保存这些自定义资源的主目录——我们将在其中存储每个接口的配置。

#### CNI 配置

我们将添加的是一个 CNI 配置。如果你不熟悉它们，让我们快速分解它们。这是一个示例 CNI 配置：

```
{  "cniVersion": "0.3.0",  "type": "loopback",  "additional": "information"}
```

CNI 配置是 JSON，我们在这里有一个结构，其中包含一些我们感兴趣的东西：

- cniVersion：告诉每个 CNI 插件正在使用哪个版本，如果它使用的版本太晚（或太早），可以提供插件信息
- type：这告诉 CNI 在磁盘上调用哪个二进制文件。每个 CNI 插件都是一个被调用的二进制文件。通常，这些二进制文件存储在/opt/cni/bin每个节点上，CNI 执行这个二进制文件。在这种情况下，我们指定了loopback二进制文件（它创建了一个环回类型的网络接口）。如果这是你第一次安装 Multus，你可能需要验证“type 字段中的插件实际上是否在/opt/cni/bin目录中的磁盘上
- additional：这里以这个字段为例，每个 CNI 插件都可以在 JSON 中指定他们想要的任何配置参数。这些特定于你在type现场调用的二进制文件。

当 CNI 配置发生变化时，你无需重新加载或刷新 Kubelet。每次创建和删除 pod 时都会读取这些内容。因此，如果你更改配置，它将在下次创建 pod 时应用。如果需要新配置，可能需要重新启动现有的 pod。

#### 将配置存储为自定义资源

所以，我们要创建一个额外的接口。让我们创建一个 macvlan 接口供 pod 使用。我们将创建一个自定义资源来定义接口的 CNI 配置。

请注意，在以下命令中有一个kind: NetworkAttachmentDefinition. 这是我们配置的自定义资源类型——它是 Kubernetes 的自定义扩展，它定义了我们如何将网络连接到我们的 pod。

其次，注意config信息。你会看到这是一个 CNI 配置，就像我们之前解释的那样。

最后但非常重要的是，请注意 metadata 字段下的 name - 这里是我们为这个配置命名的地方，也是我们告诉 pod 使用这个配置的方式。

这是创建此示例配置的命令：

```
cat <<EOF | kubectl create -f -apiVersion: "k8s.cni.cncf.io/v1"kind: NetworkAttachmentDefinitionmetadata:  name: macvlan-confspec:  config: '{      "cniVersion": "0.3.0",      "type": "macvlan",      "master": "eth0",      "mode": "bridge",      "ipam": {        "type": "host-local",        "subnet": "192.168.1.0/24",        "rangeStart": "192.168.1.200",        "rangeEnd": "192.168.1.216",        "routes": [          { "dst": "0.0.0.0/0" }        ],        "gateway": "192.168.1.1"      }    }'EOF
```

注意：此示例master使用的是eth0，应与集群中主机上的网络接口名称匹配。

#### NAD 对象中使用到了 macvaln 和 ipam 里的 host-local 是什么？

containernetworking 团队维护的通用 CNI 网络插件（见[https://www.cni.dev/plugins/current/），网路插件根据用途分成](https://www.cni.dev/plugins/current/），网路插件根据用途分成) Main 插件、Ipam 插件和 Meta 插件：

- Main插件：主要用来创建具体的网络设备的二进制文件
- Ipam插件：主要用来负责分配 IP 地址
- Meta插件：由 CNI 社区维护的内部插件

macvlan 就是属于 Main 插件，host-local 属于 Ipam 插件，在这里我们也可以了解到，cni 插件中至少要包含 Main 插件和 Ipam 插件。

**Main 插件是用于在集群中创建额外的网络：**

- bridge：创建基于网桥的额外网络可让同一主机中的 Pod 相互通信，并与主机通信。
- host-device：创建 host-device 额外网络可让 Pod 访问主机系统上的物理以太网网络设备。
- macvlan：创建基于 macvlan 的额外网络可让主机上的 Pod 通过使用物理网络接口与其他主机和那些主机上的 Pod 通信。附加到基于 macvlan 的额外网络的每个 Pod 都会获得一个唯一的 MAC 地址。
- ipvlan：创建基于 ipvlan 的额外网络可让主机上的 Pod 与其他主机和那些主机上的 Pod 通信，这类似于基于 macvlan 的额外网络。与基于 macvlan 的额外网络不同，每个 Pod 共享与父级物理网络接口相同的 MAC 地址。
- SR-IOV：创建基于 SR-IOV 的额外网络可让 Pod 附加到主机系统上支持 SR-IOV 的硬件的虚拟功能 (VF) 接口。

**Ipam（IP Address Management）插件主要用来负责分配 IP 地址：**

- dhcp插件，节点上需要有 DHCP server；
- host-local插件 ，是给定子网范围，在单个几点上基于这个子网进行 ip 地址分配；
- static插件，静态地址管理，直接指定 ip 地址使用的。
- calico-ipam ，是 clalico cni 自己的 ip 地址分配插件，是一种集中式 ip 分配插件；
- whereabouts ，也是一个集中式 ip 分配插件，用的比较少，使用了 sriov 设备才用到的，这个是 k8snetworkplumbingwg 社区开源的。

**Meta 插件：由 CNI 社区维护的内部插件**

- flannel，这就是专门为 Flannel 项目提供的 CNI 插件；
- tunning，是一个通过 sysctl 调整网络设备参数的二进制文件；
- portmap ，是一个通过 iptables 配置端口映射的二进制文件；
- bandwidth ，是一个使用 Token Bucket Filter（TBF）来进行限流的二进制文件；
- calico，是专门为 Calico 项目提供的 CNI 插件。

你可以使用以下方法查看你创建kubectl的配置：

```
kubectl get network-attachment-definitions
```

你可以通过描述它们来获得更多详细信息：

```
kubectl describe network-attachment-definitions macvlan-conf
```

### 3、Pod 配置附加网络接口

我们将创建一个 pod。这看起来就像你之前创建的任何 pod 一样熟悉，但是，我们将有一个特殊的annotations字段——一个名为k8s.v1.cni.cncf.io/networks的字段，它的值是我们在上面创建的NetworkAttachmentDefinition的名称，多个时候可以使用逗号分隔。

让我们继续使用以下命令创建一个 pod（它只会休眠很长时间）：

```
cat <<EOF | kubectl create -f -apiVersion: v1kind: Podmetadata:  name: samplepod  # 配置附加网络接口  annotations:    k8s.v1.cni.cncf.io/networks: macvlan-confspec:  containers:  - name: samplepod    command: ["/bin/ash", "-c", "trap : TERM INT; sleep infinity & wait"]    image: alpineEOF
```

你现在可以检查 pod 并查看附加了哪些接口，如下所示：

```
kubectl exec -it samplepod -- ip a
```

#### 执行信息

➜  /fly kubectl exec -it samplepod -- ip a
1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UNKNOWN qlen 1000
link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
inet 127.0.0.1/8 scope host lo
valid_lft forever preferred_lft forever
2: tunl0@NONE:  mtu 1480 qdisc noop state DOWN qlen 1000
link/ipip 0.0.0.0 brd 0.0.0.0
4: eth0@if48: <BROADCAST,MULTICAST,UP,LOWER_UP,M-DOWN> mtu 1440 qdisc noqueue state UP
link/ether da:1c:f1:4b:7c:55 brd ff:ff:ff:ff:ff:ff
inet 10.233.70.28/32 brd 10.233.70.28 scope global eth0
valid_lft forever preferred_lft forever
5: net1@tunl0: <BROADCAST,MULTICAST,UP,LOWER_UP,M-DOWN> mtu 1500 qdisc noqueue state UP
link/ether 52:4e:d4:62:5e:09 brd ff:ff:ff:ff:ff:ff
inet 192.168.1.204/24 brd 192.168.1.255 scope global net1
valid_lft forever preferred_lft forever

```
你应该注意，有 3 个接口：- lo环回接口- eth0我们的默认网络- net1我们使用 macvlan 配置创建的新界面。#### 如果我想要更多网络接口怎么办？你可以通过创建更多自定义资源然后在 pod 的注解中引用它们来向 pod 添加更多网络接口。你还可以重用配置，例如，要将两个 macvlan 接口附加到一个 pod，你可以像这样创建一个 pod：```yamlcat <<EOF | kubectl create -f -apiVersion: v1kind: Podmetadata:  name: samplepod  annotations:    k8s.v1.cni.cncf.io/networks: macvlan-conf,macvlan-confspec:  containers:  - name: samplepod    command: ["/bin/ash", "-c", "trap : TERM INT; sleep infinity & wait"]    image: alpineEOF
```

请注意，注释现在变为k8s.v1.cni.cncf.io/networks: macvlan-conf,macvlan-conf. 我们有两次使用相同的配置，用逗号分隔。

如果你要创建另一个自定义资源foo，你可以使用诸如:之类的名称k8s.v1.cni.cncf.io/networks: foo,macvlan-conf。

#### 网络联通性测试

在创建两个 pod，一个与刚才创建的 pod 同 worker 主机运行，一个不同 worker 节点运行，不同 pod 之间通过对方地址访问测试

![](https://img.cloudmessage.top/i/2025/08/21/68a6c33138722.png)
非 k8s 的机器同广播域网络访问测试

![](https://img.cloudmessage.top/i/2025/08/21/68a6c3755ad85.png)

## 关于

### 1、直接物理网络插件（适合裸金属 / 固定子网）

**ipvlan（推荐替代 macvlan，支持 IPv4/IPv6）**

特点：与 macvlan 类似，直接绑定物理网卡，但支持更灵活的子网划分（如每个 Pod 独立 IP，无需共享 MAC 地址）。

配置示例：

```
apiVersion: k8s.cni.cncf.io/v1kind: NetworkAttachmentDefinitionmetadata:  name: ipvlan-confspec:  config: '{    "cniVersion": "0.3.1",    "type": "ipvlan",    "master": "eth0",    "mode": "l3",  # 路由模式，Pod 直接使用物理网络 IP    "ipam": {      "type": "host-local",      "subnet": "10.0.0.0/24",      "gateway": "10.0.0.1"    }  }'
```

适用场景：裸金属集群，需 Pod 直接使用物理网络 IP（如数据库节点）。

**host-device（直通物理网卡）**

特点：将物理网卡直接分配给 Pod（需网卡支持 SR-IOV 或 VF），性能接近裸金属。

配置示例：

```
apiVersion: k8s.cni.cncf.io/v1kind: NetworkAttachmentDefinitionmetadata:  name: sriov-confspec:  config: '{    "cniVersion": "0.3.1",    "type": "host-device",    "deviceID": "0000:02:00.0"  # 物理网卡 PCI 地址  }'
```

适用场景：高性能计算、需要极低延迟的场景（如金融、AI）。

### 2、覆盖网络插件（适合跨子网 / 虚拟化环境）

**flannel-vxlan（经典覆盖网络）**

特点：通过 VXLAN 隧道实现跨节点通信，支持与 Multus 绑定多子网。

配置示例（需先部署 Flannel）：

```
apiVersion: k8s.cni.cncf.io/v1kind: NetworkAttachmentDefinitionmetadata:  name: flannel-netspec:  config: '{    "cniVersion": "0.3.1",    "type": "flannel",    "delegate": {      "isDefaultGateway": true    }  }'
```

适用场景：测试集群、跨云环境的多子网隔离。

**calico-ipip（BGP + 覆盖网络混合模式）**

特点：默认使用 BGP 路由，跨子网时自动切换 IPIP 隧道，支持网络策略。

配置示例（需先部署 Calico）：

```
apiVersion: k8s.cni.cncf.io/v1kind: NetworkAttachmentDefinitionmetadata:  name: calico-netspec:  config: '{    "cniVersion": "0.3.1",    "type": "calico",    "ipam": {      "type": "calico-ipam"    },    "policy": {      "type": "k8s"    }  }'
```

适用场景：生产集群，需网络策略和高性能兼顾。

### 3、高级功能插件（安全 / 性能增强）

**cilium-ebpf（基于 eBPF 的高性能方案）**

特点：通过 eBPF 内核编程实现零拷贝，支持多网卡、服务网格、网络加密。

配置示例（需先部署 Cilium）：

```
apiVersion: k8s.cni.cncf.io/v1kind: NetworkAttachmentDefinitionmetadata:  name: cilium-netspec:  config: '{    "cniVersion": "0.3.1",    "type": "cilium-cni",    "enable-srv6": true  # 可选 IPv6 或服务网格功能  }'
```

适用场景：微服务架构、需要服务网格集成的复杂集群。

**canal（Flannel+Calico 组合）**

特点：Flannel 负责跨节点通信，Calico 提供网络策略，开箱即用。

配置示例（Multus 绑定 Canal 子网）：

```
apiVersion: k8s.cni.cncf.io/v1kind: NetworkAttachmentDefinitionmetadata:  name: canal-netspec:  config: '{    "cniVersion": "0.3.1",    "type": "canal",    "flannel_backend_type": "vxlan"  }'
```

适用场景：中小型集群，同时需要简单网络和策略功能。

### 4、插件选择建议

![](https://img.cloudmessage.top/i/2025/08/21/68a6c7307c94d.png)

### 5、操作注意事项

- Multus 必装：所有多网卡方案需先部署 Multus（参考 multus-daemonset.yml）。
- 节点准备：如使用 macvlan/ipvlan，需确保节点内核版本 ≥ 4.8，且物理网卡未被 NetworkManager 管理。
- IP 冲突：多网络插件的子网需确保不重叠（如 192.168.1.0/24 和 10.0.0.0/24）。
- 网络策略：Kubernetes 原生策略仅作用于主网卡（eth0），额外网卡策略需插件自身支持（如 Calico 支持多网卡策略）。