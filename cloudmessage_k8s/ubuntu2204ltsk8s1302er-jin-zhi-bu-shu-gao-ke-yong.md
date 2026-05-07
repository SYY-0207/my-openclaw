# Ubuntu 22.04 LTS +  K8S 1.30.2 二进制部署 + 高可用

> 来源: http://cloudmessage.top/archives/ubuntu2204ltsk8s1302er-jin-zhi-bu-shu-gao-ke-yong

# 一、K8S 集群各主机环境准备

## 1、环境准备
主机名IP地址角色划分硬件配置k8s-master0110.0.0.241api-server，control manager，scheduler，etcd2c，4GB，50GB+k8s-master0210.0.0.242api-server，control manager，scheduler，etcd2c，4GB，50GB+k8s-master0310.0.0.243api-server，control manager，scheduler，etcd2c，4GB，50GB+k8s-worker0410.0.0.244kubelet，kube-proxy2c，4GB，50GB+k8s-worker0510.0.0.245kubelet，kube-proxy2c，4GB，50GB+apiserver-lb10.0.0.240apiserver的负载均衡器IP地址2c，4GB，50GB+
## 2、所有节点安装常用的软件包

```
apt updateapt -y install bind9-utils expect rsync jq psmisc net-tools lvm2 vim unzip rename
```

## 3、k8s-master01 节点免密钥登录集群并同步数据

```
# 设置主机名,各节点参考如下命令修改即可hostnamectl set-hostname k8s-master01# 设置相应的主机名及 hosts 文件解析cat >> /etc/hosts <<'EOF'10.0.0.240 apiserver-lb10.0.0.241 k8s-master0110.0.0.242 k8s-master0210.0.0.243 k8s-master0310.0.0.244 k8s-worker0410.0.0.245 k8s-worker05EOF# 配置免密码登录其他节点[root@k8s-master01 ~]# cat > password_free_login.sh <<'EOF'#!/bin/bash# auther: Jason Yin# 创建密钥对ssh-keygen -t rsa -P "" -f /root/.ssh/id_rsa -q# 声明你服务器密码，建议所有节点的密码均一致，否则该脚本需要再次进行优化export mypasswd=wangyang# 定义主机列表k8s_host_list=(k8s-master02 k8s-master03 k8s-worker04 k8s-worker05)# 配置免密登录，利用expect工具免交互输入for i in ${k8s_host_list[@]};doexpect -c "spawn ssh-copy-id -i /root/.ssh/id_rsa.pub root@$i  expect {    \"*yes/no*\" {send \"yes\r\"; exp_continue}    \"*password*\" {send \"$mypasswd\r\"; exp_continue}  }"doneEOF[root@k8s-master01 ~]# bash password_free_login.sh# 编写同步脚本[root@k8s-master01 ~]# cat > /usr/local/sbin/data_rsync.sh <<'EOF'#!/bin/bash# Auther: Jason Yinif  [ $# -lt 1 ];then   echo "Usage: $0 /path/to/file(绝对路径) [mode: m|w]"   exitfi if [ ! -e $1 ];then    echo "[ $1 ] dir or file not find!"    exitfifullpath=`dirname $1`basename=`basename $1`cd $fullpathcase $2 in    WORKER_NODE|w)      K8S_NODE=(k8s-master02 k8s-master03 k8s-worker04 k8s-worker05)      ;;    MASTER_NODE|m)      K8S_NODE=(k8s-master02 k8s-master03)      ;;    *)      K8S_NODE=(k8s-master02 k8s-master03 k8s-worker04 k8s-worker05)     ;;esacfor host in ${K8S_NODE[@]};do  tput setaf 2    echo ===== rsyncing ${host}: $basename =====    tput setaf 7    rsync -az $basename  `whoami`@${host}:$fullpath    if [ $? -eq 0 ];then      echo "命令执行成功!"    fidoneEOF[root@k8s-master01 ~]# [root@k8s-master01 ~]# chmod +x /usr/local/sbin/data_rsync.sh# 同步"/etc/hosts"文件到集群[root@k8s-master01 ~]# data_rsync.sh /etc/hosts 
```

## 4、所有节点 Linux 基础环境优化

```
# 所有节点关闭NetworkManager，ufwsystemctl disable --now NetworkManager ufw # 所有节点关闭swap分区，fstab注释swapswapoff -a && sysctl -w vm.swappiness=0sed -ri '/^[^#]*swap/s@^@#@' /etc/fstabfree -h# 手动同步时区和时间ln -svf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime# 所有节点配置limitcat >> /etc/security/limits.conf <<'EOF'* soft nofile 655360* hard nofile 131072* soft nproc 655350* hard nproc 655350* soft memlock unlimited* hard memlock unlimitedEOF # 所有节点优化 sshd 服务sed -i 's@#UseDNS yes@UseDNS no@g' /etc/ssh/sshd_configsed -i 's@^GSSAPIAuthentication yes@GSSAPIAuthentication no@g' /etc/ssh/sshd_config- UseDNS选项:打开状态下，当客户端试图登录SSH服务器时，服务器端先根据客户端的IP地址进行DNS PTR反向查询出客户端的主机名，然后根据查询出的客户端主机名进行DNS正向A记录查询，验证与其原始IP地址是否一致，这是防止客户端欺骗的一种措施，但一般我们的是动态IP不会有PTR记录，打开这个选项不过是在白白浪费时间而已，不如将其关闭。- GSSAPIAuthentication:当这个参数开启（ GSSAPIAuthentication  yes ）的时候，通过SSH登陆服务器时候会有些会很慢！这是由于服务器端启用了GSSAPI。登陆的时候客户端需要对服务器端的IP地址进行反解析，如果服务器的IP地址没有配置PTR记录，那么就容易在这里卡住了。# Linux内核调优cat > /etc/sysctl.d/k8s.conf <<'EOF'# 以下3个参数是containerd所依赖的内核参数net.ipv4.ip_forward = 1net.bridge.bridge-nf-call-iptables = 1net.bridge.bridge-nf-call-ip6tables = 1net.ipv6.conf.all.disable_ipv6 = 1fs.may_detach_mounts = 1vm.overcommit_memory=1vm.panic_on_oom=0fs.inotify.max_user_watches=89100fs.file-max=52706963fs.nr_open=52706963net.netfilter.nf_conntrack_max=2310720net.ipv4.tcp_keepalive_time = 600net.ipv4.tcp_keepalive_probes = 3net.ipv4.tcp_keepalive_intvl =15net.ipv4.tcp_max_tw_buckets = 36000net.ipv4.tcp_tw_reuse = 1net.ipv4.tcp_max_orphans = 327680net.ipv4.tcp_orphan_retries = 3net.ipv4.tcp_syncookies = 1net.ipv4.tcp_max_syn_backlog = 16384net.ipv4.ip_conntrack_max = 65536net.ipv4.tcp_max_syn_backlog = 16384net.ipv4.tcp_timestamps = 0net.core.somaxconn = 16384EOFsysctl --system
```

## 5、所有节点安装 ipvsadm 以实现 kube-proxy 的负载均衡

```
# 安装 ipvsadm 等相关工具apt -y install ipvsadm ipset sysstat conntrack libseccomp # 所有节点创建要开机自动加载的模块配置文件cat > /etc/modules-load.d/ipvs.conf << 'EOF'ip_vsip_vs_lcip_vs_wlcip_vs_rrip_vs_wrrip_vs_lblcip_vs_lblcrip_vs_dhip_vs_ship_vs_foip_vs_nqip_vs_sedip_vs_ftpip_vs_shnf_conntrackip_tablesip_setxt_setipt_setipt_rpfilteript_REJECTipipEOF# 修改ens33网卡名称为eth0【选做，建议修改】## 修改配置文件vim /etc/default/grub...GRUB_CMDLINE_LINUX="... net.ifnames=0 biosdevname=0"## 用 grub2-mkconfig 重新生成配置 grub-mkconfig -o /boot/grub/grub.cfg ## 修改网卡配置[root@k8s-master01 ~]# cat /etc/netplan/00-installer-config.yamlnetwork:  ethernets:    eth0:      dhcp4: false      addresses:        - 10.0.0.241/24      routes:        - to: default          via: 10.0.0.254      nameservers:        addresses:            # 114 DNS          - 114.114.114.114          - 114.114.115.115            # 阿里云DNS          - 223.5.5.5          - 223.6.6.6            # 腾讯云DNS          - 119.29.29.29          - 119.28.28.28            # 百度DNS          - 180.76.76.76            # Google DNS          - 8.8.8.8          - 4.4.4.4  version: 2  ## 重启操作系统即可reboot ## 验证加载的模块lsmod | grep --color=auto -e ip_vs -e nf_conntrackuname -rifconfig温馨提示:Linux kernel 4.19+版本已经将之前的"nf_conntrack_ipv4"模块更名为"nf_conntrack"模块哟~
```

# 二.安装 containerd 组件

## 1、安装必要的一些系统工具

```
apt-get -y install apt-transport-https ca-certificates curl software-properties-common
```

## 2、安装 GPG 证书

```
curl -fsSL https://mirrors.aliyun.com/docker-ce/linux/ubuntu/gpg | apt-key add -
```

## 3、写入软件源信息

```
add-apt-repository "deb [arch=amd64] https://mirrors.aliyun.com/docker-ce/linux/ubuntu $(lsb_release -cs) stable"
```

## 4、更新软件源

```
apt-get update
```

## 5、安装 containerd 组件

```
apt-get -y install containerd.io
```

## 6、配置 containerd 需要的模块

```
# 临时手动加载模块modprobe -- overlaymodprobe -- br_netfilter# 开机自动加载所需的内核模块cat > /etc/modules-load.d/containerd.conf <<EOFoverlaybr_netfilterEOF
```

## 7、修改 containerd 的配置文件

```
# 重新初始化 containerd 的配置文件containerd config default | tee /etc/containerd/config.toml # 修改 Cgroup 的管理者为 systemd 组件sed -ri 's#(SystemdCgroup = )false#\1true#' /etc/containerd/config.toml grep SystemdCgroup /etc/containerd/config.toml# 修改 pause 的基础镜像名称sed -i 's#registry.k8s.io/pause:3.6#registry.cn-hangzhou.aliyuncs.com/google_containers/pause:3.7#' /etc/containerd/config.tomlgrep sandbox_image /etc/containerd/config.toml
```

## 8、所有节点启动 containerd

```
# 启动 containerd 服务systemctl daemon-reloadsystemctl enable --now containerdsystemctl status containerd# 配置 crictl 客户端连接的运行时位置cat > /etc/crictl.yaml <<EOFruntime-endpoint: unix:///run/containerd/containerd.sockimage-endpoint: unix:///run/containerd/containerd.socktimeout: 10debug: falseEOF# 查看 containerd 的版本[root@k8s-worker05 ~]# ctr versionClient:  Version:  1.6.33  Revision: d2d58213f83a351ca8f528a95fbd145f5654e957  Go version: go1.21.11Server:  Version:  1.6.33  Revision: d2d58213f83a351ca8f528a95fbd145f5654e957  UUID: 2edf9793-a435-4cd2-86fa-f15cc9572ad1[root@k8s-worker05 ~]# 
```

# 三、containerd 的名称空间，镜像和容器，任务管理快速入门

## 1、名称空间管理

```
# 查看现有的名称空间[root@k8s-master01 ~]# ctr ns lsNAME LABELS # 创建名称空间[root@k8s-master01 ~]# ctr ns c wangyangroc-linux[root@k8s-master01 ~]# ctr ns ls    NAME            LABELS     wangyangroc-linux    # 删除名称空间[root@k8s-master01 ~]# ctr ns rm wangyangroc-linuxwangyangroc-linux[root@k8s-master01 ~]# ctr ns lsNAME LABELS 温馨提示:  删除的名称空间必须为空，否则无法删除！
```

## 2、镜像管理

```
# 拉取镜像到指定的名称空间[root@k8s-master01 ~]# ctr ns c wangyangroc-linux# default 空间中下载[root@k8s-master01 ~]# ctr image pull wangyanglinux/myapp:v1.0# 指定 wangyangroc-linux 空间中下载[root@k8s-master01 ~]# ctr -n wangyangroc-linux image pull wangyanglinux/myapp:v2.0# 指定 wangyangroc-linux  空间查看镜像[root@k8s-master01 ~]# ctr  -n wangyangroc-linux i ls# 删除镜像[root@k8s-master01 ~]# ctr -n wangyangroc-linux  i rm wangyanglinux/myapp:v2.0
```

## 3、容器管理

```
# 运行一个容器[root@k8s-master01 ~]# ctr -n wangyangroc-linux  run wangyanglinux/myapp:v2.0 haha# 查看容器列表[root@k8s-master01 ~]# ctr -n wangyangroc-linux c ls    CONTAINER    IMAGE                                                        RUNTIME             haha         wangyanglinux/myapp:v2.0    io.containerd.runc.v2     # 查看正在运行的容器 ID[root@k8s-master01 ~]# ctr -n wangyangroc-linux t ls    TASK    PID     STATUS        haha    4842    RUNNING# 连接正在运行的容器[root@k8s-master01 ~]# ctr -n wangyangroc-linux t exec -t --exec-id 2024 haha sh/ # / # ifconfig lo        Link encap:Local Loopback            inet addr:127.0.0.1  Mask:255.0.0.0          inet6 addr: ::1/128 Scope:Host          UP LOOPBACK RUNNING  MTU:65536  Metric:1          RX packets:0 errors:0 dropped:0 overruns:0 frame:0          TX packets:0 errors:0 dropped:0 overruns:0 carrier:0          collisions:0 txqueuelen:1000           RX bytes:0 (0.0 B)  TX bytes:0 (0.0 B)温馨提示:containerd本身并不提供网络，只负责容器的生命周期。将来网络部分交给专门的 CNI 插件提供。# 杀死一个正在运行的容器[root@k8s-master01 ~]# ctr -n wangyangroc-linux t ls    TASK    PID     STATUS        haha    4842    RUNNING[root@k8s-master01 ~]# ctr -n wangyangroc-linux  t kill haha[root@k8s-master01 ~]# ctr -n wangyangroc-linux t ls    TASK    PID    STATUS    # 删除容器 [root@k8s-master01 ~]# ctr -n wangyangroc-linux c lsCONTAINER    IMAGE                                                        RUNTIME         haha         wangyanglinux/myapp:v2.0 [root@k8s-master01 ~]# ctr -n wangyangroc-linux c rm haha[root@k8s-master01 ~]# ctr -n wangyangroc-linux c lsCONTAINER    IMAGE                                                        RUNTIME         
```

# 四、安装 etcd 程序

## 1、下载 etcd 的软件包

```
[root@k8s-master01 ~]# wget https://github.com/etcd-io/etcd/releases/download/v3.5.14/etcd-v3.5.14-linux-amd64.tar.gz
```

## 2、解压 etcd 的二进制程序包到 PATH 环境变量路径

```
# 解压软件包[root@k8s-master01 ~]# tar -xf etcd-v3.5.14-linux-amd64.tar.gz --strip-components=1 -C /usr/local/bin etcd-v3.5.14-linux-amd64/etcd{,ctl}[root@k8s-master01 ~]# ll /usr/local/bin/    total 39760    drwxr-xr-x  2 root        root            4096 Jun 24 14:39 ./    drwxr-xr-x 10 root        root            4096 Aug 10  2023 ../    -rwxr-xr-x  1 root root 23162880 May 30 02:35 etcd*    -rwxr-xr-x  1 root root 17543168 May 30 02:35 etcdctl*# 查看 etcd 版本[root@k8s-master01 ~]# etcdctl versionetcdctl version: 3.5.14API version: 3.5[root@k8s-master01 ~]# 
```

## 3、将软件包下发到所有节点

```
[root@k8s-master01 ~]# data_rsync.sh /usr/local/bin/etcd m===== rsyncing k8s-master02: etcd =====命令执行成功!===== rsyncing k8s-master03: etcd =====命令执行成功![root@k8s-master01 ~]# data_rsync.sh /usr/local/bin/etcdctl m===== rsyncing k8s-master02: etcdctl =====命令执行成功!===== rsyncing k8s-master03: etcdctl =====命令执行成功!
```

# 五、安装 K8S 程序

## 1、下载软件包

```
[root@k8s-master01 ~]# wget https://dl.k8s.io/v1.30.2/kubernetes-server-linux-amd64.tar.gz
```

## 2、解压 K8S 的二进制程序包到 PATH 环境变量路径

```
# 解压软件包[root@k8s-master01 ~]# tar -xf kubernetes-server-linux-amd64.tar.gz  --strip-components=3 -C /usr/local/bin kubernetes/server/bin/kube{let,ctl,-apiserver,-controller-manager,-scheduler,-proxy}# 查看 kubelet 的版本[root@k8s-master01 ~]# kubelet --versionKubernetes v1.30.2[root@k8s-master01 ~]# # 分发软件包[root@k8s-master01 ~]# data_rsync.sh /usr/local/bin/kube-apiserver m[root@k8s-master01 ~]# data_rsync.sh /usr/local/bin/kube-scheduler m[root@k8s-master01 ~]# data_rsync.sh /usr/local/bin/kube-controller-manager m[root@k8s-master01 ~]# data_rsync.sh /usr/local/bin/kubectl m[root@k8s-master01 ~]# data_rsync.sh /usr/local/bin/kubelet w[root@k8s-master01 ~]# data_rsync.sh /usr/local/bin/kube-proxy w
```

# 六、生成 etcd 证书文件

## 1、安装 cfssl 证书管理工具

```
github下载地址:  https://github.com/cloudflare/cfssl温馨提示:生成K8S和etcd证书这一步骤很关键，我建议各位在做实验前先对K8S集群的所有节点拍一下快照，以避免你实验做失败了方便回滚。关于cfssl证书可以自行在github下载即可，当然也可以使用我课堂上给大家下载好的软件包哟。# 下载软件包wget https://github.com/cloudflare/cfssl/releases/download/v1.6.5/cfssl-certinfo_1.6.5_linux_amd64wget https://github.com/cloudflare/cfssl/releases/download/v1.6.5/cfssljson_1.6.5_linux_amd64wget https://github.com/cloudflare/cfssl/releases/download/v1.6.5/cfssl_1.6.5_linux_amd64# 解压后拷贝至 /usr/local/bin 即可[root@k8s-master01 ~]# ll /usr/local/bin/cfssl*-rwxr-xr-x 1 root root 11890840 Jun 15 11:56 /usr/local/bin/cfssl*-rwxr-xr-x 1 root root  8413336 Jun 15 11:56 /usr/local/bin/cfssl-certinfo*-rwxr-xr-x 1 root root  6205592 Jun 15 11:56 /usr/local/bin/cfssljson*[root@k8s-master01 ~]# 
```

## 2、k8s-master01 节点创建 etcd 证书存储目录

```
[root@k8s-master01 ~]# mkdir -pv /wangyangroc/certs/{etcd,pki}/ && cd /wangyangroc/certs/pki/
```

## 3、k8s-master01 节点生成 etcd 证书的自建 ca 证书

```
# 生成证书的 CSR 文件： 证书签发请求文件，配置了一些域名，公司，单位[root@k8s-master01 pki]# cat > etcd-ca-csr.json <<EOF{  "CN": "etcd",  "key": {    "algo": "rsa",    "size": 2048  },  "names": [    {      "C": "CN",      "ST": "Beijing",      "L": "Beijing",      "O": "etcd",      "OU": "Etcd Security"    }  ],  "ca": {    "expiry": "876000h"  }}EOF # 生成 etcd C A证书和 CA 证书的 key[root@k8s-master01 pki]# cfssl gencert -initca etcd-ca-csr.json | cfssljson -bare /wangyangroc/certs/etcd/etcd-ca2024/06/24 15:14:28 [INFO] generating a new CA key and certificate from CSR2024/06/24 15:14:28 [INFO] generate received request2024/06/24 15:14:28 [INFO] received CSR2024/06/24 15:14:28 [INFO] generating key: rsa-20482024/06/24 15:14:29 [INFO] encoded CSR2024/06/24 15:14:29 [INFO] signed certificate with serial number 45472152341303655022232330737492953237610051483[root@k8s-master01 pki]# pwd[root@k8s-master01 pki]# ll /wangyangroc/certs/etcd/    total 20    drwxr-xr-x 2 root root 4096 Jun 24 15:14 ./    drwxr-xr-x 4 root root 4096 Jun 24 15:12 ../    -rw-r--r-- 1 root root 1050 Jun 24 15:14 etcd-ca.csr    -rw------- 1 root root 1679 Jun 24 15:14 etcd-ca-key.pem    -rw-r--r-- 1 root root 1318 Jun 24 15:14 etcd-ca.pem
```

## 4、k8s-master01 节点基于自建 ca 证书颁发 etcd 证书

```
# 生成 etcd 证书的有效期为 100 年[root@k8s-master01 pki]# cat > ca-config.json <<EOF{  "signing": {    "default": {      "expiry": "876000h"    },    "profiles": {      "kubernetes": {        "usages": [            "signing",            "key encipherment",            "server auth",            "client auth"        ],        "expiry": "876000h"      }    }  }}EOF# 生成证书的 CSR 文件： 证书签发请求文件，配置了一些域名，公司，单位[root@k8s-master01 pki]# cat > etcd-csr.json <<EOF{  "CN": "etcd",  "key": {    "algo": "rsa",    "size": 2048  },  "names": [    {      "C": "CN",      "ST": "Beijing",      "L": "Beijing",      "O": "etcd",      "OU": "Etcd Security"    }  ]}EOF# 基于自建的 ectd ca 证书生成 etcd 的证书[root@k8s-master01 pki]# cfssl gencert \  -ca=/wangyangroc/certs/etcd/etcd-ca.pem \  -ca-key=/wangyangroc/certs/etcd/etcd-ca-key.pem \  -config=ca-config.json \  --hostname=127.0.0.1,k8s-master01,k8s-master02,k8s-master03,10.0.0.241,10.0.0.242,10.0.0.243 \  --profile=kubernetes \  etcd-csr.json  | cfssljson -bare /wangyangroc/certs/etcd/etcd-server[root@k8s-master01 pki]# ll /wangyangroc/certs/etcd/etcd-server*    -rw-r--r-- 1 root root 1131 Jun 24 15:18 /wangyangroc/certs/etcd/etcd-server.csr    -rw------- 1 root root 1679 Jun 24 15:18 /wangyangroc/certs/etcd/etcd-server-key.pem    -rw-r--r-- 1 root root 1464 Jun 24 15:18 /wangyangroc/certs/etcd/etcd-server.pem
```

## 5、k8s-master01 节点将 etcd 证书拷贝到其他两个 master 节点

```
[root@k8s-master01 pki]# MasterNodes='k8s-master02 k8s-master03'[root@k8s-master01 pki]# for NODE in $MasterNodes; do ssh $NODE "mkdir -pv /wangyangroc/certs/etcd/" done[root@k8s-master01 pki]# data_rsync.sh /wangyangroc/certs/etcd/etcd-ca-key.pem m===== rsyncing k8s-master02: etcd-ca-key.pem =====命令执行成功!===== rsyncing k8s-master03: etcd-ca-key.pem =====命令执行成功![root@k8s-master01 pki]# data_rsync.sh /wangyangroc/certs/etcd/etcd-ca.pem m===== rsyncing k8s-master02: etcd-ca.pem =====命令执行成功!===== rsyncing k8s-master03: etcd-ca.pem =====命令执行成功![root@k8s-master01 pki]# data_rsync.sh /wangyangroc/certs/etcd/etcd-server-key.pem m===== rsyncing k8s-master02: etcd-server-key.pem =====命令执行成功!===== rsyncing k8s-master03: etcd-server-key.pem =====命令执行成功![root@k8s-master01 pki]# data_rsync.sh /wangyangroc/certs/etcd/etcd-server.pem m===== rsyncing k8s-master02: etcd-server.pem =====命令执行成功!===== rsyncing k8s-master03: etcd-server.pem =====命令执行成功!
```

# 七、启动 etcd 集群

## 1、创建 etcd 集群各节点配置文件

```
# k8s-master01 节点的配置文件[root@k8s-master01 ~]# mkdir -pv /wangyangroc/softwares/etcd[root@k8s-master01 ~]# cat > /wangyangroc/softwares/etcd/etcd.config.yml <<'EOF'name: 'k8s-master01'data-dir: /var/lib/etcdwal-dir: /var/lib/etcd/walsnapshot-count: 5000heartbeat-interval: 100election-timeout: 1000quota-backend-bytes: 0listen-peer-urls: 'https://10.0.0.241:2380'listen-client-urls: 'https://10.0.0.241:2379,http://127.0.0.1:2379'max-snapshots: 3max-wals: 5cors:initial-advertise-peer-urls: 'https://10.0.0.241:2380'advertise-client-urls: 'https://10.0.0.241:2379'discovery:discovery-fallback: 'proxy'discovery-proxy:discovery-srv:initial-cluster: 'k8s-master01=https://10.0.0.241:2380,k8s-master02=https://10.0.0.242:2380,k8s-master03=https://10.0.0.243:2380'initial-cluster-token: 'etcd-k8s-cluster'initial-cluster-state: 'new'strict-reconfig-check: falseenable-v2: trueenable-pprof: trueproxy: 'off'proxy-failure-wait: 5000proxy-refresh-interval: 30000proxy-dial-timeout: 1000proxy-write-timeout: 5000proxy-read-timeout: 0client-transport-security:cert-file: '/wangyangroc/certs/etcd/etcd-server.pem'key-file: '/wangyangroc/certs/etcd/etcd-server-key.pem'client-cert-auth: truetrusted-ca-file: '/wangyangroc/certs/etcd/etcd-ca.pem'auto-tls: truepeer-transport-security:cert-file: '/wangyangroc/certs/etcd/etcd-server.pem'key-file: '/wangyangroc/certs/etcd/etcd-server-key.pem'peer-client-cert-auth: truetrusted-ca-file: '/wangyangroc/certs/etcd/etcd-ca.pem'auto-tls: truedebug: falselog-package-levels:log-outputs: [default]force-new-cluster: falseEOF# k8s-master02 节点的配置文件[root@k8s-master02 ~]# mkdir -pv /wangyangroc/softwares/etcd[root@k8s-master02 ~]# cat > /wangyangroc/softwares/etcd/etcd.config.yml << 'EOF'name: 'k8s-master02'data-dir: /var/lib/etcdwal-dir: /var/lib/etcd/walsnapshot-count: 5000heartbeat-interval: 100election-timeout: 1000quota-backend-bytes: 0listen-peer-urls: 'https://10.0.0.242:2380'listen-client-urls: 'https://10.0.0.242:2379,http://127.0.0.1:2379'max-snapshots: 3max-wals: 5cors:initial-advertise-peer-urls: 'https://10.0.0.242:2380'advertise-client-urls: 'https://10.0.0.242:2379'discovery:discovery-fallback: 'proxy'discovery-proxy:discovery-srv:initial-cluster: 'k8s-master01=https://10.0.0.241:2380,k8s-master02=https://10.0.0.242:2380,k8s-master03=https://10.0.0.243:2380'initial-cluster-token: 'etcd-k8s-cluster'initial-cluster-state: 'new'strict-reconfig-check: falseenable-v2: trueenable-pprof: trueproxy: 'off'proxy-failure-wait: 5000proxy-refresh-interval: 30000proxy-dial-timeout: 1000proxy-write-timeout: 5000proxy-read-timeout: 0client-transport-security:  cert-file: '/wangyangroc/certs/etcd/etcd-server.pem'  key-file: '/wangyangroc/certs/etcd/etcd-server-key.pem'  client-cert-auth: true  trusted-ca-file: '/wangyangroc/certs/etcd/etcd-ca.pem'  auto-tls: truepeer-transport-security:  cert-file: '/wangyangroc/certs/etcd/etcd-server.pem'  key-file: '/wangyangroc/certs/etcd/etcd-server-key.pem'  peer-client-cert-auth: true  trusted-ca-file: '/wangyangroc/certs/etcd/etcd-ca.pem'  auto-tls: truedebug: falselog-package-levels:log-outputs: [default]force-new-cluster: falseEOF# k8s-master03 节点的配置文件[root@k8s-master03 ~]# mkdir -pv /wangyangroc/softwares/etcd[root@k8s-master03 ~]# cat > /wangyangroc/softwares/etcd/etcd.config.yml << 'EOF'name: 'k8s-master03'data-dir: /var/lib/etcdwal-dir: /var/lib/etcd/walsnapshot-count: 5000heartbeat-interval: 100election-timeout: 1000quota-backend-bytes: 0listen-peer-urls: 'https://10.0.0.243:2380'listen-client-urls: 'https://10.0.0.243:2379,http://127.0.0.1:2379'max-snapshots: 3max-wals: 5cors:initial-advertise-peer-urls: 'https://10.0.0.243:2380'advertise-client-urls: 'https://10.0.0.243:2379'discovery:discovery-fallback: 'proxy'discovery-proxy:discovery-srv:initial-cluster: 'k8s-master01=https://10.0.0.241:2380,k8s-master02=https://10.0.0.242:2380,k8s-master03=https://10.0.0.243:2380'initial-cluster-token: 'etcd-k8s-cluster'initial-cluster-state: 'new'strict-reconfig-check: falseenable-v2: trueenable-pprof: trueproxy: 'off'proxy-failure-wait: 5000proxy-refresh-interval: 30000proxy-dial-timeout: 1000proxy-write-timeout: 5000proxy-read-timeout: 0client-transport-security:  cert-file: '/wangyangroc/certs/etcd/etcd-server.pem'  key-file: '/wangyangroc/certs/etcd/etcd-server-key.pem'  client-cert-auth: true  trusted-ca-file: '/wangyangroc/certs/etcd/etcd-ca.pem'  auto-tls: truepeer-transport-security:  cert-file: '/wangyangroc/certs/etcd/etcd-server.pem'  key-file: '/wangyangroc/certs/etcd/etcd-server-key.pem'  peer-client-cert-auth: true  trusted-ca-file: '/wangyangroc/certs/etcd/etcd-ca.pem'  auto-tls: truedebug: falselog-package-levels:log-outputs: [default]force-new-cluster: falseEOF
```

## 2、k8s-master[01-03] 编写 etcd 启动脚本

```
cat > /usr/lib/systemd/system/etcd.service <<'EOF'[Unit]Description=Jason Yin's Etcd ServiceDocumentation=https://coreos.com/etcd/docs/latest/After=network.target[Service]Type=notifyExecStart=/usr/local/bin/etcd --config-file=/wangyangroc/softwares/etcd/etcd.config.ymlRestart=on-failureRestartSec=10LimitNOFILE=65536[Install]WantedBy=multi-user.targetAlias=etcd3.serviceEOF
```

## 3、启动 etcd 集群

```
# 启动服务systemctl daemon-reload && systemctl enable --now etcdsystemctl status etcd# 查看 etcd 集群状态etcdctl --endpoints="10.0.0.241:2379,10.0.0.242:2379,10.0.0.243:2379" --cacert=/wangyangroc/certs/etcd/etcd-ca.pem --cert=/wangyangroc/certs/etcd/etcd-server.pem --key=/wangyangroc/certs/etcd/etcd-server-key.pem  endpoint status --write-out=table# 验证 etcd 高可用停用任意节点的 ectd 观察集群是否正常工作。
```

# 八、生成 k8s 组件相关证书

## 1、所有节点创建 k8s 证书存储目录

```
mkdir -pv /wangyangroc/certs/kubernetes/
```

## 2、k8s-master01 节点生成 kubernetes 自建 ca 证书

```
# 生成证书的 CSR 文件： 证书签发请求文件，配置了一些域名，公司，单位[root@k8s-master01 pki]# pwd/wangyangroc/certs/pki[root@k8s-master01 pki]# cat > k8s-ca-csr.json  <<EOF{"CN": "kubernetes","key": {  "algo": "rsa",  "size": 2048},"names": [  {    "C": "CN",    "ST": "Beijing",    "L": "Beijing",    "O": "Kubernetes",    "OU": "Kubernetes-manual"  }],"ca": {  "expiry": "876000h"}}EOF# 生成 kubernetes 证书[root@k8s-master01 pki]# cfssl gencert -initca k8s-ca-csr.json | cfssljson -bare /wangyangroc/certs/kubernetes/k8s-ca[root@k8s-master01 pki]# ll /wangyangroc/certs/kubernetes/    total 20    drwxr-xr-x 2 root root 4096 Jun 24 15:51 ./    drwxr-xr-x 5 root root 4096 Jun 24 15:49 ../    -rw-r--r-- 1 root root 1070 Jun 24 15:51 k8s-ca.csr    -rw------- 1 root root 1679 Jun 24 15:51 k8s-ca-key.pem    -rw-r--r-- 1 root root 1363 Jun 24 15:51 k8s-ca.pem
```

## 3、k8s-master01 节点基于自建 ca 证书颁发 apiserver 相关证书

```
# 生成 k8s 证书的有效期为 100 年[root@k8s-master01 pki]# cat > k8s-ca-config.json <<EOF{  "signing": {    "default": {      "expiry": "876000h"    },    "profiles": {      "kubernetes": {        "usages": [            "signing",            "key encipherment",            "server auth",            "client auth"        ],        "expiry": "876000h"      }    }  }}EOF# 生成 apiserver 证书的 CSR 文件： 证书签发请求文件，配置了一些域名，公司，单位[root@k8s-master01 pki]# cat > apiserver-csr.json <<EOF{  "CN": "kube-apiserver",  "key": {    "algo": "rsa",    "size": 2048  },  "names": [    {      "C": "CN",      "ST": "Beijing",      "L": "Beijing",      "O": "Kubernetes",      "OU": "Kubernetes-manual"    }  ]}EOF# 基于自建 ca 证书生成 apiServer 的证书文件[root@k8s-master01 pki]# cfssl gencert \  -ca=/wangyangroc/certs/kubernetes/k8s-ca.pem \  -ca-key=/wangyangroc/certs/kubernetes/k8s-ca-key.pem \  -config=k8s-ca-config.json \  --hostname=10.200.0.1,10.0.0.240,kubernetes,kubernetes.default,kubernetes.default.svc,kubernetes.default.svc.wangyangroc,kubernetes.default.svc.wangyangroc.com,10.0.0.241,10.0.0.242,10.0.0.243,10.0.0.244,10.0.0.245 \  --profile=kubernetes \   apiserver-csr.json  | cfssljson -bare /wangyangroc/certs/kubernetes/apiserver[root@k8s-master01 pki]# ll /wangyangroc/certs/kubernetes/apiserver*-rw-r--r-- 1 root root 1310 Jun 24 15:55 /wangyangroc/certs/kubernetes/apiserver.csr-rw------- 1 root root 1675 Jun 24 15:55 /wangyangroc/certs/kubernetes/apiserver-key.pem-rw-r--r-- 1 root root 1704 Jun 24 15:55 /wangyangroc/certs/kubernetes/apiserver.pem[root@k8s-master01 pki]# 温馨提示:"10.200.0.1"为咱们的svc网段的第一个地址，您需要根据自己的场景稍作修改。"10.0.0.240"是负载均衡器的VIP地址。"kubernetes,...,kubernetes.default.svc.wangyangroc.com"对应的是apiServer解析的A记录。"10.0.0.241,...,10.0.0.245"对应的是K8S集群的地址。 
```

## 4、生成第三方组件与 apiServer 通信的聚合证书

```
聚合证书的作用就是让第三方组件(比如 metrics-server 等)能够拿这个证书文件和 apiServer 进行通信。# 生成聚合证书的用于自建 ca 的 CSR 文件[root@k8s-master01 pki]# cat > front-proxy-ca-csr.json <<EOF{  "CN": "kubernetes",  "key": {     "algo": "rsa",     "size": 2048  }}EOF# 生成聚合证书的自建 ca 证书[root@k8s-master01 pki]# cfssl gencert -initca front-proxy-ca-csr.json | cfssljson -bare /wangyangroc/certs/kubernetes/front-proxy-ca[root@k8s-master01 pki]# ll /wangyangroc/certs/kubernetes/front-proxy-ca*-rw-r--r-- 1 root root  891 Jun 24 15:59 /wangyangroc/certs/kubernetes/front-proxy-ca.csr-rw------- 1 root root 1675 Jun 24 15:59 /wangyangroc/certs/kubernetes/front-proxy-ca-key.pem-rw-r--r-- 1 root root 1094 Jun 24 15:59 /wangyangroc/certs/kubernetes/front-proxy-ca.pem[root@k8s-master01 pki]# 3.生成聚合证书的用于客户端的CSR文件[root@k8s-master01 pki]# cat > front-proxy-client-csr.json <<EOF{  "CN": "front-proxy-client",  "key": {     "algo": "rsa",     "size": 2048  }}EOF# 基于聚合证书的自建 ca 证书签发聚合证书的客户端证书[root@k8s-master01 pki]# cfssl gencert \  -ca=/wangyangroc/certs/kubernetes/front-proxy-ca.pem \  -ca-key=/wangyangroc/certs/kubernetes/front-proxy-ca-key.pem \  -config=k8s-ca-config.json \  -profile=kubernetes \  front-proxy-client-csr.json | cfssljson -bare /wangyangroc/certs/kubernetes/front-proxy-client[root@k8s-master01 pki]# ll /wangyangroc/certs/kubernetes/front-proxy-client*-rw-r--r-- 1 root root  903 Jun 24 16:00 /wangyangroc/certs/kubernetes/front-proxy-client.csr-rw------- 1 root root 1679 Jun 24 16:00 /wangyangroc/certs/kubernetes/front-proxy-client-key.pem-rw-r--r-- 1 root root 1188 Jun 24 16:00 /wangyangroc/certs/kubernetes/front-proxy-client.pem
```

## 5、生成 controller-manager 证书及 kubeconfig 文件

```
# 生成 controller-manager 的 CSR 文件[root@k8s-master01 pki]# cat > controller-manager-csr.json <<EOF{  "CN": "system:kube-controller-manager",  "key": {    "algo": "rsa",    "size": 2048  },  "names": [    {      "C": "CN",      "ST": "Beijing",      "L": "Beijing",      "O": "system:kube-controller-manager",      "OU": "Kubernetes-manual"    }  ]}EOF# 生成 controller-manager 证书文件[root@k8s-master01 pki]# cfssl gencert \  -ca=/wangyangroc/certs/kubernetes/k8s-ca.pem \  -ca-key=/wangyangroc/certs/kubernetes/k8s-ca-key.pem \  -config=k8s-ca-config.json \  -profile=kubernetes \  controller-manager-csr.json | cfssljson -bare /wangyangroc/certs/kubernetes/controller-manager[root@k8s-master01 pki]# ll /wangyangroc/certs/kubernetes/controller-manager*-rw-r--r-- 1 root root 1082 Jun 24 16:02 /wangyangroc/certs/kubernetes/controller-manager.csr-rw------- 1 root root 1675 Jun 24 16:02 /wangyangroc/certs/kubernetes/controller-manager-key.pem-rw-r--r-- 1 root root 1501 Jun 24 16:02 /wangyangroc/certs/kubernetes/controller-manager.pem# 创建一个 kubeconfig 目录[root@k8s-master01 pki]# mkdir -pv /wangyangroc/certs/kubeconfig# 设置一个集群[root@k8s-master01 pki]# kubectl config set-cluster wangyangroc-k8s \  --certificate-authority=/wangyangroc/certs/kubernetes/k8s-ca.pem \  --embed-certs=true \  --server=https://10.0.0.240:8443 \  --kubeconfig=/wangyangroc/certs/kubeconfig/kube-controller-manager.kubeconfig# 设置一个用户项[root@k8s-master01 pki]# kubectl config set-credentials system:kube-controller-manager \  --client-certificate=/wangyangroc/certs/kubernetes/controller-manager.pem \  --client-key=/wangyangroc/certs/kubernetes/controller-manager-key.pem \  --embed-certs=true \  --kubeconfig=/wangyangroc/certs/kubeconfig/kube-controller-manager.kubeconfig  # 设置一个上下文环境[root@k8s-master01 pki]# kubectl config set-context system:kube-controller-manager@kubernetes \  --cluster=wangyangroc-k8s \  --user=system:kube-controller-manager \  --kubeconfig=/wangyangroc/certs/kubeconfig/kube-controller-manager.kubeconfig  # 使用默认的上下文[root@k8s-master01 pki]# kubectl config use-context system:kube-controller-manager@kubernetes \  --kubeconfig=/wangyangroc/certs/kubeconfig/kube-controller-manager.kubeconfig
```

## 6、生成 scheduler 证书及 kubeconfig 文件

```
# 生成 scheduler 的 CSR 文件[root@k8s-master01 pki]# cat > scheduler-csr.json <<EOF{  "CN": "system:kube-scheduler",  "key": {    "algo": "rsa",    "size": 2048  },  "names": [    {      "C": "CN",      "ST": "Beijing",      "L": "Beijing",      "O": "system:kube-scheduler",      "OU": "Kubernetes-manual"    }  ]}EOF# 生成 scheduler 证书文件[root@k8s-master01 pki]# cfssl gencert \  -ca=/wangyangroc/certs/kubernetes/k8s-ca.pem \  -ca-key=/wangyangroc/certs/kubernetes/k8s-ca-key.pem \  -config=k8s-ca-config.json \  -profile=kubernetes \  scheduler-csr.json | cfssljson -bare /wangyangroc/certs/kubernetes/scheduler[root@k8s-master01 pki]# ll /wangyangroc/certs/kubernetes/scheduler*-rw-r--r-- 1 root root 1058 Jun 24 16:06 /wangyangroc/certs/kubernetes/scheduler.csr-rw------- 1 root root 1679 Jun 24 16:06 /wangyangroc/certs/kubernetes/scheduler-key.pem-rw-r--r-- 1 root root 1476 Jun 24 16:06 /wangyangroc/certs/kubernetes/scheduler.pem# 设置一个集群[root@k8s-master01 pki]# kubectl config set-cluster wangyangroc-k8s \  --certificate-authority=/wangyangroc/certs/kubernetes/k8s-ca.pem \  --embed-certs=true \  --server=https://10.0.0.240:8443 \  --kubeconfig=/wangyangroc/certs/kubeconfig/kube-scheduler.kubeconfig  # 设置一个用户项[root@k8s-master01 pki]# kubectl config set-credentials system:kube-scheduler \  --client-certificate=/wangyangroc/certs/kubernetes/scheduler.pem \  --client-key=/wangyangroc/certs/kubernetes/scheduler-key.pem \  --embed-certs=true \  --kubeconfig=/wangyangroc/certs/kubeconfig/kube-scheduler.kubeconfig# 设置一个上下文环境[root@k8s-master01 pki]# kubectl config set-context system:kube-scheduler@kubernetes \  --cluster=wangyangroc-k8s \  --user=system:kube-scheduler \  --kubeconfig=/wangyangroc/certs/kubeconfig/kube-scheduler.kubeconfig  # 使用默认的上下文[root@k8s-master01 pki]# kubectl config use-context system:kube-scheduler@kubernetes \  --kubeconfig=/wangyangroc/certs/kubeconfig/kube-scheduler.kubeconfig
```

## 7、配置 k8s 集群管理员证书及 kubeconfig 文件

```
# 生成管理员的 CSR 文件[root@k8s-master01 pki]# cat > admin-csr.json <<EOF{  "CN": "admin",  "key": {    "algo": "rsa",    "size": 2048  },  "names": [    {      "C": "CN",      "ST": "Beijing",      "L": "Beijing",      "O": "system:masters",      "OU": "Kubernetes-manual"    }  ]}EOF# 生成 k8s 集群管理员证书[root@k8s-master01 pki]# cfssl gencert \  -ca=/wangyangroc/certs/kubernetes/k8s-ca.pem \  -ca-key=/wangyangroc/certs/kubernetes/k8s-ca-key.pem \  -config=k8s-ca-config.json \  -profile=kubernetes \  admin-csr.json | cfssljson -bare /wangyangroc/certs/kubernetes/admin[root@k8s-master01 pki]# ll /wangyangroc/certs/kubernetes/admin*-rw-r--r-- 1 root root 1025 Jun 24 16:09 /wangyangroc/certs/kubernetes/admin.csr-rw------- 1 root root 1679 Jun 24 16:09 /wangyangroc/certs/kubernetes/admin-key.pem-rw-r--r-- 1 root root 1444 Jun 24 16:09 /wangyangroc/certs/kubernetes/admin.pem[root@k8s-master01 pki]# # 设置一个集群[root@k8s-master01 pki]# kubectl config set-cluster wangyangroc-k8s \  --certificate-authority=/wangyangroc/certs/kubernetes/k8s-ca.pem \  --embed-certs=true \  --server=https://10.0.0.240:8443 \  --kubeconfig=/wangyangroc/certs/kubeconfig/kube-admin.kubeconfig  # 设置一个用户项[root@k8s-master01 pki]# kubectl config set-credentials kube-admin \  --client-certificate=/wangyangroc/certs/kubernetes/admin.pem \  --client-key=/wangyangroc/certs/kubernetes/admin-key.pem \  --embed-certs=true \  --kubeconfig=/wangyangroc/certs/kubeconfig/kube-admin.kubeconfig# 设置一个上下文环境[root@k8s-master01 pki]# kubectl config set-context kube-admin@kubernetes \  --cluster=wangyangroc-k8s \  --user=kube-admin \  --kubeconfig=/wangyangroc/certs/kubeconfig/kube-admin.kubeconfig  # 使用默认的上下文[root@k8s-master01 pki]# kubectl config use-context kube-admin@kubernetes \  --kubeconfig=/wangyangroc/certs/kubeconfig/kube-admin.kubeconfig
```

## 8、创建ServiceAccount

```
# ServiceAccount 是 k8s 一种认证方式，创建 ServiceAccount 的时候会创建一个与之绑定的 secret，这个secret 会生成一个 token[root@k8s-master01 pki]# openssl genrsa -out /wangyangroc/certs/kubernetes/sa.key 2048# 基于 sa.key 创建 sa.pub[root@k8s-master01 pki]# openssl rsa -in /wangyangroc/certs/kubernetes/sa.key -pubout -out /wangyangroc/certs/kubernetes/sa.pub[root@k8s-master01 pki]# ll /wangyangroc/certs/kubernetes/sa*-rw------- 1 root root 1704 Jun 24 16:11 /wangyangroc/certs/kubernetes/sa.key-rw-r--r-- 1 root root  451 Jun 24 16:11 /wangyangroc/certs/kubernetes/sa.pub
```

## 9、k8s-master01 节点 K8S 组件证书拷贝到其他两个 master 节点

```
# k8s-master01 节点将 etcd 证书拷贝到其他两个 master 节点[root@k8s-master01 pki]# data_rsync.sh /wangyangroc/certs/kubernetes m[root@k8s-master01 pki]# data_rsync.sh /wangyangroc/certs/kubeconfig m# 其他两个节点验证文件数量是否正确[root@k8s-master02 ~]# ls /wangyangroc/certs/kubernetes  | wc -l23[root@k8s-master02 ~]# ls /wangyangroc/certs/kubeconfig | wc -l3[root@k8s-master03 ~]# ls /wangyangroc/certs/kubernetes  | wc -l23[root@k8s-master03 ~]# ls /wangyangroc/certs/kubeconfig | wc -l3
```

# 九、高可用组件 haproxy+keepalived 安装及验证

## 1、所有master【k8s-master[01-03]】节点安装高可用组件

```
温馨提示:    - 对于高可用组件，其实我们也可以单独找两台虚拟机来部署，但我为了节省2台机器，就直接在master节点复用了。    - 如果在云上安装 K8S 则无安装高可用组件了，毕竟公有云大部分都是不支持 keepalived 的，可以直接使用云产品，比如阿里的"SLB"，腾讯的"ELB"等 SAAS 产品;    - 推荐使用 ELB，SLB 有回环的问题，也就是 SLB 代理的服务器不能反向访问 SLB，但是腾讯云修复了这个问题;具体实操:apt-get -y install keepalived haproxy 
```

## 2、所有 master 节点配置 haproxy

```
温馨提示:- haproxy 的负载均衡器监听地址我配置是 8443，你可以修改为其他端口，haproxy 会用来反向代理各个master 组件的地址;- 如果你真的修改晴一定注意上面的证书配置的 kubeconfig 文件，也要一起修改，否则就会出现链接集群失败的问题;具体实操:# 备份配置文件cp /etc/haproxy/haproxy.cfg{,`date +%F`}# 所有节点的配置文件内容相同cat > /etc/haproxy/haproxy.cfg <<'EOF'global  maxconn  2000  ulimit-n  16384  log  127.0.0.1 local0 err  stats timeout 30sdefaults  log global  mode  http  option  httplog  timeout connect 5000  timeout client  50000  timeout server  50000  timeout http-request 15s  timeout http-keep-alive 15sfrontend monitor-haproxy  bind *:9999  mode http  option httplog  monitor-uri /ruokfrontend wangyangroc-k8s  bind 0.0.0.0:8443  bind 127.0.0.1:8443  mode tcp  option tcplog  tcp-request inspect-delay 5s  default_backend wangyangroc-k8sbackend wangyangroc-k8s  mode tcp  option tcplog  option tcp-check  balance roundrobin  default-server inter 10s downinter 5s rise 2 fall 2 slowstart 60s maxconn 250 maxqueue 256 weight 100  server k8s-master01   10.0.0.241:6443  check  server k8s-master02   10.0.0.242:6443  check  server k8s-master03   10.0.0.243:6443  checkEOF
```

## 3、所有 master 节点配置 keepalived

```
温馨提示:- 注意"interface"字段为你的物理网卡的名称，如果你的网卡是ens33，请将"eth0"修改为"ens33"哟;- 注意"mcast_src_ip"各master节点的配置均不相同，修改根据实际环境进行修改哟;- 注意"virtual_ipaddress"指定的是负载均衡器的VIP地址，这个地址也要和kubeconfig文件的Apiserver地址要一致哟;- 注意"script"字段的脚本用于检测后端的apiServer是否健康;- 注意"router_id"字段为节点ip，master每个节点配置自己的IP具体实操:# "k8s-master01" 节点创建配置文件[root@k8s-master01 ~]# ifconfig eth0: flags=4163<UP,BROADCAST,RUNNING,MULTICAST>  mtu 1500        inet 10.0.0.241  netmask 255.255.255.0  broadcast 10.0.0.255        ether 00:0c:29:32:73:ac  txqueuelen 1000  (Ethernet)        RX packets 324292  bytes 234183010 (223.3 MiB)        RX errors 0  dropped 0  overruns 0  frame 0        TX packets 242256  bytes 31242156 (29.7 MiB)        TX errors 0  dropped 0 overruns 0  carrier 0  collisions 0...[root@k8s-master01 ~]#  cat > /etc/keepalived/keepalived.conf <<'EOF'! Configuration File for keepalivedglobal_defs {   router_id 10.0.0.241}vrrp_script chk_nginx {    script "/etc/keepalived/check_port.sh 8443"    interval 2    weight -20}vrrp_instance VI_1 {    state MASTER    interface eth0    virtual_router_id 251    priority 100    advert_int 1    mcast_src_ip 10.0.0.241    nopreempt    authentication {        auth_type PASS        auth_pass wangyangroc_k8s    }    track_script {         chk_nginx    }    virtual_ipaddress {        10.0.0.240    }}EOF# "k8s-master02" 节点创建配置文件[root@k8s-master02 ~]# ifconfig eth0: flags=4163<UP,BROADCAST,RUNNING,MULTICAST>  mtu 1500        inet 10.0.0.242  netmask 255.255.255.0  broadcast 10.0.0.255        ether 00:0c:29:cf:ad:0a  txqueuelen 1000  (Ethernet)        RX packets 256743  bytes 42628265 (40.6 MiB)        RX errors 0  dropped 0  overruns 0  frame 0        TX packets 252589  bytes 34277384 (32.6 MiB)        TX errors 0  dropped 0 overruns 0  carrier 0  collisions 0...[root@k8s-master02 ~]# cat > /etc/keepalived/keepalived.conf <<EOF! Configuration File for keepalivedglobal_defs {   router_id 10.0.0.242}vrrp_script chk_nginx {    script "/etc/keepalived/check_port.sh 8443"    interval 2    weight -20}vrrp_instance VI_1 {    state MASTER    interface eth0    virtual_router_id 251    priority 100    advert_int 1    mcast_src_ip 10.0.0.242    nopreempt    authentication {        auth_type PASS        auth_pass wangyangroc_k8s    }    track_script {         chk_nginx    }    virtual_ipaddress {        10.0.0.240    }}EOF# "k8s-master03" 节点创建配置文件[root@k8s-master03 ~]# ifconfig eth0: flags=4163<UP,BROADCAST,RUNNING,MULTICAST>  mtu 1500        inet 10.0.0.243  netmask 255.255.255.0  broadcast 10.0.0.255        ether 00:0c:29:5f:f7:4f  txqueuelen 1000  (Ethernet)        RX packets 178577  bytes 34808750 (33.1 MiB)        RX errors 0  dropped 0  overruns 0  frame 0        TX packets 171025  bytes 26471309 (25.2 MiB)        TX errors 0  dropped 0 overruns 0  carrier 0  collisions 0...[root@k8s-master03 ~]# [root@k8s-master03 ~]# cat > /etc/keepalived/keepalived.conf <<EOF! Configuration File for keepalivedglobal_defs {   router_id 10.0.0.243}vrrp_script chk_nginx {    script "/etc/keepalived/check_port.sh 8443"    interval 2    weight -20}vrrp_instance VI_1 {    state MASTER    interface eth0    virtual_router_id 251    priority 100    advert_int 1    mcast_src_ip 10.0.0.243    nopreempt    authentication {        auth_type PASS        auth_pass wangyangroc_k8s    }    track_script {         chk_nginx    }    virtual_ipaddress {        10.0.0.240    }}EOF# 所有 keepalived 节点均需要创建健康检查脚本cat > /etc/keepalived/check_port.sh <<'EOF'#!/bin/bashCHK_PORT=$1if [ -n "$CHK_PORT" ];then    PORT_PROCESS=`ss -lt|grep $CHK_PORT|wc -l`    if [ $PORT_PROCESS -eq 0 ];then        echo "Port $CHK_PORT Is Not Used,End."        systemctl stop keepalived    fielse    echo "Check Port Cant Be Empty!"fiEOFchmod +x /etc/keepalived/check_port.sh 
```

## 4、启动 keepalived 服务并验证

```
# 启动 keepalived 服务systemctl daemon-reloadsystemctl enable --now keepalivedsystemctl status keepalived# 验证服务是否正常[root@k8s-master03 ~]# ip a    1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UNKNOWN group default qlen 1000        link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00        inet 127.0.0.1/8 scope host lo           valid_lft forever preferred_lft forever    2: eth0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc pfifo_fast state UP group default qlen 1000        link/ether 00:0c:29:5f:f7:4f brd ff:ff:ff:ff:ff:ff        inet 10.0.0.243/24 brd 10.0.0.255 scope global eth0           valid_lft forever preferred_lft forever        inet 10.0.0.240/32 scope global eth0           valid_lft forever preferred_lft forever    3: tunl0@NONE: <NOARP> mtu 1480 qdisc noop state DOWN group default qlen 1000        link/ipip 0.0.0.0 brd 0.0.0.0[root@k8s-master03 ~]# ping 10.0.0.240PING 10.0.0.240 (10.0.0.240) 56(84) bytes of data.64 bytes from 10.0.0.240: icmp_seq=1 ttl=64 time=0.019 ms64 bytes from 10.0.0.240: icmp_seq=2 ttl=64 time=0.027 ms64 bytes from 10.0.0.240: icmp_seq=3 ttl=64 time=0.023 ms...# 单独开一个终端尝试停止 keepalived 服务[root@k8s-master03 ~]# systemctl stop keepalived# 再次观察终端输出[root@k8s-master03 ~]# ping 10.0.0.240PING 10.0.0.240 (10.0.0.240) 56(84) bytes of data.64 bytes from 10.0.0.240: icmp_seq=1 ttl=64 time=0.019 ms64 bytes from 10.0.0.240: icmp_seq=2 ttl=64 time=0.027 ms64 bytes from 10.0.0.240: icmp_seq=3 ttl=64 time=0.023 ms...64 bytes from 10.0.0.240: icmp_seq=36 ttl=64 time=0.037 ms64 bytes from 10.0.0.240: icmp_seq=37 ttl=64 time=0.023 msFrom 10.0.0.242: icmp_seq=38 Redirect Host(New nexthop: 10.0.0.240)From 10.0.0.242: icmp_seq=39 Redirect Host(New nexthop: 10.0.0.240)64 bytes from 10.0.0.240: icmp_seq=40 ttl=64 time=1.81 ms64 bytes from 10.0.0.240: icmp_seq=41 ttl=64 time=0.680 ms64 bytes from 10.0.0.240: icmp_seq=42 ttl=64 time=0.751 ms# 验证 vip 是否漂移到其他节点，果不其然，真的飘逸到其他 master 节点啦！[root@k8s-master02 ~]# ip a...2: eth0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc pfifo_fast state UP group default qlen 1000    link/ether 00:0c:29:cf:ad:0a brd ff:ff:ff:ff:ff:ff    inet 10.0.0.242/24 brd 10.0.0.255 scope global eth0       valid_lft forever preferred_lft forever    inet 10.0.0.240/32 scope global eth0       valid_lft forever preferred_lft forever...
```

## 5、验证 haproxy 服务并验证

```
# 所有节点启动 haproxy 服务systemctl enable --now haproxy systemctl restart haproxy systemctl status haproxy # 所有节点启动 keepalived systemctl start keepalived# 基于 telnet 验证 haporxy 是否正常[root@k8s-master02 ~]# telnet 10.0.0.240 8443    Trying 10.0.0.240...    Connected to 10.0.0.240.    Escape character is '^]'.    Connection closed by foreign host.# 基于 webUI 进行验证[root@k8s-worker05 ~]# curl http://10.0.0.240:9999/ruok<html><body><h1>200 OK</h1>Service ready.</body></html>
```

# 十、部署 ApiServer 组件

## 1、k8s-master01 节点启动 ApiServer

```
温馨提示:  - "--advertise-address"是对应的 master 节点的 IP 地址  - "--service-cluster-ip-range" 对应的是 svc 的网段  - "--service-node-port-range" 对应的是 svc 的 NodePort 端口范围  - "--etcd-servers" 指定的是 etcd 集群地址配置文件参考链接:https://kubernetes.io/zh-cn/docs/reference/command-line-tools-reference/kube-apiserver/# 具体实操# 创建 k8s-master01 节点的配置文件cat > /usr/lib/systemd/system/kube-apiserver.service << 'EOF'[Unit]Description=Jason Yin's Kubernetes API ServerDocumentation=https://github.com/kubernetes/kubernetesAfter=network.target[Service]ExecStart=/usr/local/bin/kube-apiserver \      --v=2  \      --bind-address=0.0.0.0  \      --secure-port=6443  \      --allow_privileged=true \      --advertise-address=10.0.0.241 \      --service-cluster-ip-range=10.200.0.0/16  \      --service-node-port-range=3000-50000  \      --etcd-servers=https://10.0.0.241:2379,https://10.0.0.242:2379,https://10.0.0.243:2379 \      --etcd-cafile=/wangyangroc/certs/etcd/etcd-ca.pem  \      --etcd-certfile=/wangyangroc/certs/etcd/etcd-server.pem  \      --etcd-keyfile=/wangyangroc/certs/etcd/etcd-server-key.pem  \      --client-ca-file=/wangyangroc/certs/kubernetes/k8s-ca.pem  \      --tls-cert-file=/wangyangroc/certs/kubernetes/apiserver.pem  \      --tls-private-key-file=/wangyangroc/certs/kubernetes/apiserver-key.pem  \      --kubelet-client-certificate=/wangyangroc/certs/kubernetes/apiserver.pem  \      --kubelet-client-key=/wangyangroc/certs/kubernetes/apiserver-key.pem  \      --service-account-key-file=/wangyangroc/certs/kubernetes/sa.pub  \      --service-account-signing-key-file=/wangyangroc/certs/kubernetes/sa.key \      --service-account-issuer=https://kubernetes.default.svc.wangyangroc.com \      --kubelet-preferred-address-types=InternalIP,ExternalIP,Hostname  \      --enable-admission-plugins=NamespaceLifecycle,LimitRanger,ServiceAccount,DefaultStorageClass,DefaultTolerationSeconds,NodeRestriction,ResourceQuota  \      --authorization-mode=Node,RBAC  \      --enable-bootstrap-token-auth=true  \      --requestheader-client-ca-file=/wangyangroc/certs/kubernetes/front-proxy-ca.pem  \      --proxy-client-cert-file=/wangyangroc/certs/kubernetes/front-proxy-client.pem  \      --proxy-client-key-file=/wangyangroc/certs/kubernetes/front-proxy-client-key.pem  \      --requestheader-allowed-names=aggregator  \      --requestheader-group-headers=X-Remote-Group  \      --requestheader-extra-headers-prefix=X-Remote-Extra-  \      --requestheader-username-headers=X-Remote-UserRestart=on-failureRestartSec=10sLimitNOFILE=65535[Install]WantedBy=multi-user.targetEOF# 启动服务systemctl daemon-reload && systemctl enable --now kube-apiserversystemctl status kube-apiserverss -ntl | grep 6443
```

## 2、k8s-master02 节点启动 ApiServer

```
# 温馨提示:- "--advertise-address"是对应的 master 节点的 IP 地址- "--service-cluster-ip-range" 对应的是 svc 的网段- "--service-node-port-range" 对应的是 svc 的 NodePort 端口范围- "--etcd-servers" 指定的是 etcd 集群地址# 具体实操# 创建 k8s-master02 节点的配置文件cat > /usr/lib/systemd/system/kube-apiserver.service << 'EOF'[Unit]Description=Jason Yin's Kubernetes API ServerDocumentation=https://github.com/kubernetes/kubernetesAfter=network.target[Service]ExecStart=/usr/local/bin/kube-apiserver \      --v=2  \      --bind-address=0.0.0.0  \      --secure-port=6443  \      --advertise-address=10.0.0.242 \      --service-cluster-ip-range=10.200.0.0/16  \      --service-node-port-range=3000-50000  \      --etcd-servers=https://10.0.0.241:2379,https://10.0.0.242:2379,https://10.0.0.243:2379 \      --etcd-cafile=/wangyangroc/certs/etcd/etcd-ca.pem  \      --etcd-certfile=/wangyangroc/certs/etcd/etcd-server.pem  \      --etcd-keyfile=/wangyangroc/certs/etcd/etcd-server-key.pem  \      --client-ca-file=/wangyangroc/certs/kubernetes/k8s-ca.pem  \      --tls-cert-file=/wangyangroc/certs/kubernetes/apiserver.pem  \      --tls-private-key-file=/wangyangroc/certs/kubernetes/apiserver-key.pem  \      --kubelet-client-certificate=/wangyangroc/certs/kubernetes/apiserver.pem  \      --kubelet-client-key=/wangyangroc/certs/kubernetes/apiserver-key.pem  \      --service-account-key-file=/wangyangroc/certs/kubernetes/sa.pub  \      --service-account-signing-key-file=/wangyangroc/certs/kubernetes/sa.key \      --service-account-issuer=https://kubernetes.default.svc.wangyangroc.com \      --kubelet-preferred-address-types=InternalIP,ExternalIP,Hostname  \      --enable-admission-plugins=NamespaceLifecycle,LimitRanger,ServiceAccount,DefaultStorageClass,DefaultTolerationSeconds,NodeRestriction,ResourceQuota  \      --authorization-mode=Node,RBAC  \      --enable-bootstrap-token-auth=true  \      --requestheader-client-ca-file=/wangyangroc/certs/kubernetes/front-proxy-ca.pem  \      --proxy-client-cert-file=/wangyangroc/certs/kubernetes/front-proxy-client.pem  \      --proxy-client-key-file=/wangyangroc/certs/kubernetes/front-proxy-client-key.pem  \      --requestheader-allowed-names=aggregator  \      --requestheader-group-headers=X-Remote-Group  \      --requestheader-extra-headers-prefix=X-Remote-Extra-  \      --requestheader-username-headers=X-Remote-UserRestart=on-failureRestartSec=10sLimitNOFILE=65535[Install]WantedBy=multi-user.targetEOF# 启动服务systemctl daemon-reload && systemctl enable --now kube-apiserversystemctl status kube-apiserverss -ntl | grep 6443
```

## 3、k8s-master03 节点启动 ApiServer

```
# 温馨提示 - "--advertise-address" 是对应的 master 节点的 IP 地址;- "--service-cluster-ip-range" 对应的是 svc 的网段- "--service-node-port-range" 对应的是 svc 的 NodePort 端口范围;- "--etcd-servers" 指定的是 etcd 集群地址# 具体实操# 创建 k8s-master03 节点的配置文件cat > /usr/lib/systemd/system/kube-apiserver.service << 'EOF'[Unit]Description=Jason Yin's Kubernetes API ServerDocumentation=https://github.com/kubernetes/kubernetesAfter=network.target[Service]ExecStart=/usr/local/bin/kube-apiserver \      --v=2  \      --bind-address=0.0.0.0  \      --secure-port=6443  \      --advertise-address=10.0.0.243 \      --service-cluster-ip-range=10.200.0.0/16  \      --service-node-port-range=3000-50000  \      --etcd-servers=https://10.0.0.241:2379,https://10.0.0.242:2379,https://10.0.0.243:2379 \      --etcd-cafile=/wangyangroc/certs/etcd/etcd-ca.pem  \      --etcd-certfile=/wangyangroc/certs/etcd/etcd-server.pem  \      --etcd-keyfile=/wangyangroc/certs/etcd/etcd-server-key.pem  \      --client-ca-file=/wangyangroc/certs/kubernetes/k8s-ca.pem  \      --tls-cert-file=/wangyangroc/certs/kubernetes/apiserver.pem  \      --tls-private-key-file=/wangyangroc/certs/kubernetes/apiserver-key.pem  \      --kubelet-client-certificate=/wangyangroc/certs/kubernetes/apiserver.pem  \      --kubelet-client-key=/wangyangroc/certs/kubernetes/apiserver-key.pem  \      --service-account-key-file=/wangyangroc/certs/kubernetes/sa.pub  \      --service-account-signing-key-file=/wangyangroc/certs/kubernetes/sa.key \      --service-account-issuer=https://kubernetes.default.svc.wangyangroc.com \      --kubelet-preferred-address-types=InternalIP,ExternalIP,Hostname  \      --enable-admission-plugins=NamespaceLifecycle,LimitRanger,ServiceAccount,DefaultStorageClass,DefaultTolerationSeconds,NodeRestriction,ResourceQuota  \      --authorization-mode=Node,RBAC  \      --enable-bootstrap-token-auth=true  \      --requestheader-client-ca-file=/wangyangroc/certs/kubernetes/front-proxy-ca.pem  \      --proxy-client-cert-file=/wangyangroc/certs/kubernetes/front-proxy-client.pem  \      --proxy-client-key-file=/wangyangroc/certs/kubernetes/front-proxy-client-key.pem  \      --requestheader-allowed-names=aggregator  \      --requestheader-group-headers=X-Remote-Group  \      --requestheader-extra-headers-prefix=X-Remote-Extra-  \      --requestheader-username-headers=X-Remote-UserRestart=on-failureRestartSec=10sLimitNOFILE=65535[Install]WantedBy=multi-user.targetEOF# 启动服务systemctl daemon-reload && systemctl enable --now kube-apiserversystemctl status kube-apiserverss -ntl | grep 6443
```

# 十一、部署 ControlerManager 组件

## 1、所有节点创建配置文件

```
# 温馨提示- "--cluster-cidr" 是 Pod 的网段地址，我们可以自行修改。# 配置文件参考链接https://kubernetes.io/zh-cn/docs/reference/command-line-tools-reference/kube-controller-manager/ # 所有节点的 controller-manager 组件配置文件相同: (前提是证书文件存放的位置也要相同哟!)cat > /usr/lib/systemd/system/kube-controller-manager.service << 'EOF'[Unit]Description=Jason Yin's Kubernetes Controller ManagerDocumentation=https://github.com/kubernetes/kubernetesAfter=network.target[Service]ExecStart=/usr/local/bin/kube-controller-manager \      --v=2 \      --root-ca-file=/wangyangroc/certs/kubernetes/k8s-ca.pem \      --cluster-signing-cert-file=/wangyangroc/certs/kubernetes/k8s-ca.pem \      --cluster-signing-key-file=/wangyangroc/certs/kubernetes/k8s-ca-key.pem \      --service-account-private-key-file=/wangyangroc/certs/kubernetes/sa.key \      --kubeconfig=/wangyangroc/certs/kubeconfig/kube-controller-manager.kubeconfig \      --leader-elect=true \      --use-service-account-credentials=true \      --node-monitor-grace-period=40s \      --node-monitor-period=5s \      --controllers=*,bootstrapsigner,tokencleaner \      --allocate-node-cidrs=true \      --cluster-cidr=10.100.0.0/16 \      --requestheader-client-ca-file=/wangyangroc/certs/kubernetes/front-proxy-ca.pem \      --node-cidr-mask-size=24      Restart=alwaysRestartSec=10s[Install]WantedBy=multi-user.targetEOF
```

## 2、启动 controller-manager 服务

```
systemctl daemon-reloadsystemctl enable --now kube-controller-managersystemctl  status kube-controller-managerss -ntl | grep 10257
```

# 十二、部署 Scheduler 组件

## 1、所有节点创建配置文件

```
# 配置文件参考链接https://kubernetes.io/zh-cn/docs/reference/command-line-tools-reference/kube-scheduler/# 所有节点的 controller-manager 组件配置文件相同: (前提是证书文件存放的位置也要相同哟!)cat > /usr/lib/systemd/system/kube-scheduler.service <<'EOF'[Unit]Description=Jason Yin's Kubernetes SchedulerDocumentation=https://github.com/kubernetes/kubernetesAfter=network.target[Service]ExecStart=/usr/local/bin/kube-scheduler \      --v=2 \      --leader-elect=true \      --kubeconfig=/wangyangroc/certs/kubeconfig/kube-scheduler.kubeconfigRestart=alwaysRestartSec=10s[Install]WantedBy=multi-user.targetEOF
```

## 2、启动scheduler服务

```
systemctl daemon-reloadsystemctl enable --now kube-schedulersystemctl  status kube-schedulerss -ntl | grep 10259
```

# 十三、创建 Bootstrapping 自动颁发 kubelet 证书配置

## 1、k8s-master01 节点创建 bootstrap-kubelet.kubeconfig 文件

```
# 温馨提示- "--server" 只想的是负载均衡器的 IP 地址，由负载均衡器对 master 节点进行反向代理哟。- "--token" 也可以自定义，但也要同时修改"bootstrap"的Secret的"token-id"和"token-secret"对应值哟;# 设置集群kubectl config set-cluster wangyangroc-k8s \  --certificate-authority=/wangyangroc/certs/kubernetes/k8s-ca.pem \  --embed-certs=true \  --server=https://10.0.0.240:8443 \  --kubeconfig=/wangyangroc/certs/kubeconfig/bootstrap-kubelet.kubeconfig# 创建用户kubectl config set-credentials tls-bootstrap-token-user  \  --token=c8ad9c.2e4d610cf3e7426e \  --kubeconfig=/wangyangroc/certs/kubeconfig/bootstrap-kubelet.kubeconfig  # 将集群和用户进行绑定kubectl config set-context tls-bootstrap-token-user@kubernetes \  --cluster=wangyangroc-k8s \  --user=tls-bootstrap-token-user \  --kubeconfig=/wangyangroc/certs/kubeconfig/bootstrap-kubelet.kubeconfig  # 配置默认的上下文kubectl config use-context tls-bootstrap-token-user@kubernetes \  --kubeconfig=/wangyangroc/certs/kubeconfig/bootstrap-kubelet.kubeconfig
```

## 2、所有 master 节点拷贝管理证书

```
# 温馨提示下面的操作我以 k8s-master01 为案例来操作的，实际上你可以使用所有的 master 节点完成下面的操作哟~# 所有 master 都拷贝管理员的证书文件[root@k8s-master01 ~]#  mkdir -p /root/.kube[root@k8s-master01 ~]#  cp /wangyangroc/certs/kubeconfig/kube-admin.kubeconfig /root/.kube/config# 查看 master 组件，该组件官方在 1.19+ 版本开始弃用，但是在 1.30.2 依旧没有移除哟~[root@k8s-master01 ~]# kubectl get csWarning: v1 ComponentStatus is deprecated in v1.19+NAME                 STATUS    MESSAGE   ERRORscheduler            Healthy   ok        controller-manager   Healthy   ok        etcd-0               Healthy   ok        # 查看集群状态，如果未来 cs 组件移除了也没关系，我们可以使用 "cluster-info" 子命令查看集群状态[root@k8s-master01 ~]# kubectl cluster-info Kubernetes control plane is running at https://10.0.0.240:8443To further debug and diagnose cluster problems, use 'kubectl cluster-info dump'.
```

## 3、创建 bootstrap-secret 授权

```
# 创建配 bootstrap-secret 文件用于授权[root@k8s-master01 ~]# cat > bootstrap-secret.yaml <<EOFapiVersion: v1kind: Secretmetadata:  name: bootstrap-token-wangyangroc  namespace: kube-systemtype: bootstrap.kubernetes.io/tokenstringData:  description: "The default bootstrap token generated by 'kubelet '."  token-id: c8ad9c  token-secret: 2e4d610cf3e7426e  usage-bootstrap-authentication: "true"  usage-bootstrap-signing: "true"  auth-extra-groups:  system:bootstrappers:default-node-token,system:bootstrappers:worker,system:bootstrappers:ingress---apiVersion: rbac.authorization.k8s.io/v1kind: ClusterRoleBindingmetadata:  name: kubelet-bootstraproleRef:  apiGroup: rbac.authorization.k8s.io  kind: ClusterRole  name: system:node-bootstrappersubjects:- apiGroup: rbac.authorization.k8s.io  kind: Group  name: system:bootstrappers:default-node-token---apiVersion: rbac.authorization.k8s.io/v1kind: ClusterRoleBindingmetadata:  name: node-autoapprove-bootstraproleRef:  apiGroup: rbac.authorization.k8s.io  kind: ClusterRole  name: system:certificates.k8s.io:certificatesigningrequests:nodeclientsubjects:- apiGroup: rbac.authorization.k8s.io  kind: Group  name: system:bootstrappers:default-node-token---apiVersion: rbac.authorization.k8s.io/v1kind: ClusterRoleBindingmetadata:  name: node-autoapprove-certificate-rotationroleRef:  apiGroup: rbac.authorization.k8s.io  kind: ClusterRole  name: system:certificates.k8s.io:certificatesigningrequests:selfnodeclientsubjects:- apiGroup: rbac.authorization.k8s.io  kind: Group  name: system:nodes---apiVersion: rbac.authorization.k8s.io/v1kind: ClusterRolemetadata:  annotations:    rbac.authorization.kubernetes.io/autoupdate: "true"  labels:    kubernetes.io/bootstrapping: rbac-defaults  name: system:kube-apiserver-to-kubeletrules:  - apiGroups:    - ""      resources:      - nodes/proxy      - nodes/stats      - nodes/log      - nodes/spec      - nodes/metrics        verbs:      - "*"---apiVersion: rbac.authorization.k8s.io/v1kind: ClusterRoleBindingmetadata:  name: system:kube-apiserver  namespace: ""roleRef:  apiGroup: rbac.authorization.k8s.io  kind: ClusterRole  name: system:kube-apiserver-to-kubeletsubjects:  - apiGroup: rbac.authorization.k8s.io    kind: User    name: kube-apiserverEOF# 应用 bootstrap-secret 配置文件[root@k8s-master01 ~]# kubectl apply -f bootstrap-secret.yaml secret/bootstrap-token-wangyangroc createdclusterrolebinding.rbac.authorization.k8s.io/kubelet-bootstrap createdclusterrolebinding.rbac.authorization.k8s.io/node-autoapprove-bootstrap createdclusterrolebinding.rbac.authorization.k8s.io/node-autoapprove-certificate-rotation createdclusterrole.rbac.authorization.k8s.io/system:kube-apiserver-to-kubelet createdclusterrolebinding.rbac.authorization.k8s.io/system:kube-apiserver created
```

# 十四、部署 worker 节点之 kubelet 启动实战

## 1、复制证书

```
# k8s-master01 节点分发证书到其他节点cd /wangyangroc/certs/for NODE in k8s-master02 k8s-master03 k8s-worker04 k8s-worker05; do   echo $NODE   ssh $NODE "mkdir -p /wangyangroc/certs/kube{config,rnetes}"   for FILE in k8s-ca.pem k8s-ca-key.pem front-proxy-ca.pem; do     scp kubernetes/$FILE $NODE:/wangyangroc/certs/kubernetes/${FILE} done   scp kubeconfig/bootstrap-kubelet.kubeconfig $NODE:/wangyangroc/certs/kubeconfig/done
```

## 2、启动 kubelet 服务

```
# 温馨提示- 在"10-kubelet.con"文件中使用"--kubeconfig"指定的"kubelet.kubeconfig"文件并不存在，这个证书文件后期会自动生成;- 对于"clusterDNS"是NDS地址，我们可以自定义，比如"10.200.0.254";- “clusterDomain”对应的是域名信息，要和我们设计的集群保持一致，比如"wangyangroc.com";- "10-kubelet.conf"文件中的"ExecStart="需要写2次，否则可能无法启动 kubelet;# 具体实操:# 所有节点创建工作目录mkdir -p /var/lib/kubelet /var/log/kubernetes /etc/systemd/system/kubelet.service.d /etc/kubernetes/manifests/# 所有节点创建 kubelet 的配置文件cat > /etc/kubernetes/kubelet-conf.yml <<'EOF'apiVersion: kubelet.config.k8s.io/v1beta1kind: KubeletConfigurationaddress: 0.0.0.0port: 10250readOnlyPort: 10255authentication:  anonymous:    enabled: false  webhook:    cacheTTL: 2m0s    enabled: true  x509:    clientCAFile: /wangyangroc/certs/kubernetes/k8s-ca.pemauthorization:  mode: Webhook  webhook:    cacheAuthorizedTTL: 5m0s    cacheUnauthorizedTTL: 30scgroupDriver: systemdcgroupsPerQOS: trueclusterDNS:- 10.200.0.254clusterDomain: wangyangroc.comcontainerLogMaxFiles: 5containerLogMaxSize: 10MicontentType: application/vnd.kubernetes.protobufcpuCFSQuota: truecpuManagerPolicy: nonecpuManagerReconcilePeriod: 10senableControllerAttachDetach: trueenableDebuggingHandlers: trueenforceNodeAllocatable:- podseventBurst: 10eventRecordQPS: 5evictionHard:  imagefs.available: 15%  memory.available: 100Mi  nodefs.available: 10%  nodefs.inodesFree: 5%evictionPressureTransitionPeriod: 5m0sfailSwapOn: truefileCheckFrequency: 20shairpinMode: promiscuous-bridgehealthzBindAddress: 127.0.0.1healthzPort: 10248httpCheckFrequency: 20simageGCHighThresholdPercent: 85imageGCLowThresholdPercent: 80imageMinimumGCAge: 2m0siptablesDropBit: 15iptablesMasqueradeBit: 14kubeAPIBurst: 10kubeAPIQPS: 5makeIPTablesUtilChains: truemaxOpenFiles: 1000000maxPods: 110nodeStatusUpdateFrequency: 10soomScoreAdj: -999podPidsLimit: -1registryBurst: 10registryPullQPS: 5resolvConf: /etc/resolv.confrotateCertificates: trueruntimeRequestTimeout: 2m0sserializeImagePulls: truestaticPodPath: /etc/kubernetes/manifestsstreamingConnectionIdleTimeout: 4h0m0ssyncFrequency: 1m0svolumeStatsAggPeriod: 1m0sEOF# 所有节点配置 kubelet servicecat >  /usr/lib/systemd/system/kubelet.service <<'EOF'[Unit]Description=JasonYin's Kubernetes KubeletDocumentation=https://github.com/kubernetes/kubernetesAfter=containerd.serviceRequires=containerd.service[Service]ExecStart=/usr/local/bin/kubeletRestart=alwaysStartLimitInterval=0RestartSec=10[Install]WantedBy=multi-user.targetEOF# 所有节点配置 kubelet service 的配置文件cat > /etc/systemd/system/kubelet.service.d/10-kubelet.conf <<'EOF'[Service]Environment="KUBELET_KUBECONFIG_ARGS=--bootstrap-kubeconfig=/wangyangroc/certs/kubeconfig/bootstrap-kubelet.kubeconfig --kubeconfig=/wangyangroc/certs/kubeconfig/kubelet.kubeconfig"Environment="KUBELET_CONFIG_ARGS=--config=/etc/kubernetes/kubelet-conf.yml"Environment="KUBELET_SYSTEM_ARGS=--container-runtime-endpoint=unix:///run/containerd/containerd.sock"Environment="KUBELET_EXTRA_ARGS=--node-labels=node.kubernetes.io/node='' "ExecStart=ExecStart=/usr/local/bin/kubelet $KUBELET_KUBECONFIG_ARGS $KUBELET_CONFIG_ARGS $KUBELET_SYSTEM_ARGS $KUBELET_EXTRA_ARGSEOF# 启动所有节点 kubeletsystemctl daemon-reloadsystemctl enable --now kubeletsystemctl status kubelet# 在所有 master 节点上查看 nodes 信息。[root@k8s-master01 certs]# kubectl get nodes    NAME           STATUS     ROLES    AGE   VERSION    k8s-master01   NotReady   <none>   6s    v1.30.2    k8s-master02   NotReady   <none>   5s    v1.30.2    k8s-master03   NotReady   <none>   6s    v1.30.2    k8s-worker04   NotReady   <none>   6s    v1.30.2    k8s-worker05   NotReady   <none>   6s    v1.30.2# 可以查看到有相应的 csr 用户客户端的证书请求[root@k8s-master01 ~]# kubectl get csr    NAME        AGE    SIGNERNAME                                    REQUESTOR                 REQUESTEDDURATION   CONDITION    csr-5j4xx   110s   kubernetes.io/kube-apiserver-client-kubelet   system:bootstrap:wangyangroc   <none>              Approved,Issued    csr-9cmsh   110s   kubernetes.io/kube-apiserver-client-kubelet   system:bootstrap:wangyangroc   <none>              Approved,Issued    csr-ght4f   110s   kubernetes.io/kube-apiserver-client-kubelet   system:bootstrap:wangyangroc   <none>              Approved,Issued    csr-v6sbq   111s   kubernetes.io/kube-apiserver-client-kubelet   system:bootstrap:wangyangroc   <none>              Approved,Issued    csr-xcq44   110s   kubernetes.io/kube-apiserver-client-kubelet   system:bootstrap:wangyangroc   <none>              Approved,Issued
```

## 3、出现错误解决方案

```
# 温馨提示# 如果出现报错nodes is forbidden: User \"system:anonymous\" cannot create resource \"nodes\" in API group \"\" at the cluster scope" node="k8s-master01"# 解决方案[root@k8s-master03 ~]# cat test-rbac.yamlapiVersion: rbac.authorization.k8s.io/v1kind: ClusterRoleBindingmetadata:  name: wangyangroc-kubelet-rbacroleRef:  apiGroup: rbac.authorization.k8s.io  kind: ClusterRole  name: cluster-adminsubjects:- apiGroup: rbac.authorization.k8s.io  kind: User  name: system:anonymous  [root@k8s-master03 ~]# kubectl apply -f test-rbac.yamlclusterrolebinding.rbac.authorization.k8s.io/wangyangroc-kubelet-rbac created
```

# 十五、部署 worker 节点之 kube-proxy 服务

## 1、生成 kube-proxy 的 csr 文件

```
[root@k8s-master01 pki]# cat > kube-proxy-csr.json  <<EOF{"CN": "system:kube-proxy","key": {  "algo": "rsa",  "size": 2048},"names": [  {    "C": "CN",    "ST": "Beijing",    "L": "Beijing",    "O": "system:kube-proxy",    "OU": "Kubernetes-manual"  }]}EOF
```

## 2、创建 kube-proxy 需要的证书文件

```
[root@k8s-master01 pki]# cfssl gencert \-ca=/wangyangroc/certs/kubernetes/k8s-ca.pem \-ca-key=/wangyangroc/certs/kubernetes/k8s-ca-key.pem \-config=k8s-ca-config.json \-profile=kubernetes \kube-proxy-csr.json | cfssljson -bare /wangyangroc/certs/kubernetes/kube-proxy[root@k8s-master01 pki]# ll /wangyangroc/certs/kubernetes/kube-proxy*-rw-r--r-- 1 root root 1045 Jun 24 17:53 /wangyangroc/certs/kubernetes/kube-proxy.csr-rw------- 1 root root 1679 Jun 24 17:53 /wangyangroc/certs/kubernetes/kube-proxy-key.pem-rw-r--r-- 1 root root 1464 Jun 24 17:53 /wangyangroc/certs/kubernetes/kube-proxy.pem
```

## 3、设置集群

```
[root@k8s-master01 pki]# kubectl config set-cluster wangyangroc-k8s \  --certificate-authority=/wangyangroc/certs/kubernetes/k8s-ca.pem \  --embed-certs=true \  --server=https://10.0.0.240:8443 \  --kubeconfig=/wangyangroc/certs/kubeconfig/kube-proxy.kubeconfig
```

## 4、设置一个用户项

```
[root@k8s-master01 pki]# kubectl config set-credentials system:kube-proxy \  --client-certificate=/wangyangroc/certs/kubernetes/kube-proxy.pem \  --client-key=/wangyangroc/certs/kubernetes/kube-proxy-key.pem \  --embed-certs=true \  --kubeconfig=/wangyangroc/certs/kubeconfig/kube-proxy.kubeconfig
```

## 5、设置一个上下文环境

```
[root@k8s-master01 pki]# kubectl config set-context kube-proxy@kubernetes \  --cluster=wangyangroc-k8s \  --user=system:kube-proxy \  --kubeconfig=/wangyangroc/certs/kubeconfig/kube-proxy.kubeconfig
```

## 6、使用默认的上下文

```
[root@k8s-master01 pki]# kubectl config use-context kube-proxy@kubernetes \  --kubeconfig=/wangyangroc/certs/kubeconfig/kube-proxy.kubeconfig
```

## 7、将 kube-proxy 的 systemd Service 文件发送到其他节点

```
for NODE in k8s-master02 k8s-master03 k8s-worker04 k8s-worker05; do     echo $NODE     scp /wangyangroc/certs/kubeconfig/kube-proxy.kubeconfig $NODE:/wangyangroc/certs/kubeconfig/done
```

## 8、所有节点创建 kube-proxy.conf 配置文件

```
cat > /etc/kubernetes/kube-proxy.yml << EOFapiVersion: kubeproxy.config.k8s.io/v1alpha1kind: KubeProxyConfigurationbindAddress: 0.0.0.0metricsBindAddress: 127.0.0.1:10249clientConnection:  acceptConnection: ""  burst: 10  contentType: application/vnd.kubernetes.protobuf  kubeconfig: /wangyangroc/certs/kubeconfig/kube-proxy.kubeconfig  qps: 5clusterCIDR: 10.100.0.0/16configSyncPeriod: 15m0sconntrack:  max: null  maxPerCore: 32768  min: 131072  tcpCloseWaitTimeout: 1h0m0s  tcpEstablishedTimeout: 24h0m0senableProfiling: falsehealthzBindAddress: 0.0.0.0:10256hostnameOverride: ""iptables:  masqueradeAll: false  masqueradeBit: 14  minSyncPeriod: 0sipvs:  masqueradeAll: true  minSyncPeriod: 5s  scheduler: "rr"  syncPeriod: 30smode: "ipvs"nodeProtAddress: nulloomScoreAdj: -999portRange: ""udpIdelTimeout: 250msEOF
```

## 9、所有节点使用 systemd 管理 kube-proxy

```
cat > /usr/lib/systemd/system/kube-proxy.service << EOF[Unit]Description=Jason Yin's Kubernetes ProxyAfter=network.target[Service]ExecStart=/usr/local/bin/kube-proxy \  --config=/etc/kubernetes/kube-proxy.yml \  --v=2 Restart=on-failureLimitNOFILE=65536[Install]WantedBy=multi-user.targetEOF
```

## 10、所有节点启动 kube-proxy

```
systemctl daemon-reload && systemctl enable --now kube-proxysystemctl status kube-proxyss -ntl |grep 10249
```

# 十六、网络插件 calico 部署案例

## 1、下载资源清单

```
# 参考链接https://docs.tigera.io/calico/latest/getting-started/kubernetes/quickstart[root@k8s-master02 ~]# wget https://raw.githubusercontent.com/projectcalico/calico/v3.28.0/manifests/tigera-operator.yaml[root@k8s-master02 ~]# wget https://raw.githubusercontent.com/projectcalico/calico/v3.28.0/manifests/custom-resources.yaml
```

## 2、根据自己的 K8S 情况修改 Pod 网段

```
[root@k8s-master02 ~]# grep cidr custom-resources.yaml       cidr: 192.168.0.0/16[root@k8s-master02 ~]# [root@k8s-master02 ~]# sed -i '/cidr/s#192.168#10.100#' custom-resources.yaml [root@k8s-master02 ~]# [root@k8s-master02 ~]# grep cidr custom-resources.yaml       cidr: 10.100.0.0/16
```

## 3、部署 calico

```
[root@k8s-master02 ~]# kubectl create -f tigera-operator.yaml [root@k8s-master02 ~]# kubectl create -f custom-resources.yaml 
```

## 4、查看 calico 是否部署成功

```
[root@k8s-master01 pki]# kubectl get pods -A -o wideNAMESPACE         NAME                               READY   STATUS             RESTARTS   AGE    IP           NODE           NOMINATED NODE   READINESS GATEScalico-system     calico-typha-648d54556d-7g28d      0/1     ImagePullBackOff   0          107s   10.0.0.245   k8s-worker05   <none>           <none>calico-system     calico-typha-648d54556d-b6wpx      0/1     ImagePullBackOff   0          116s   10.0.0.243   k8s-master03   <none>           <none>calico-system     calico-typha-648d54556d-xtmrx      0/1     ImagePullBackOff   0          107s   10.0.0.241   k8s-master01   <none>           <none>tigera-operator   tigera-operator-76ff79f7fd-9rxtr   1/1     Running            0          6m7s   10.0.0.242   k8s-master02   <none>           <none>[root@k8s-master01 pki]# 温馨提示:可能会出现镜像下载失败的情况，因此需要手动拉取镜像！
```

## 5、卸载 calico

```
[root@k8s-master02 ~]# kubectl delete -f custom-resources.yaml  -f  tigera-operator.yaml 
```

# 十七、测试

## 1、配置补全

```
kubectl completion bash > ~/.kube/completion.bash.incecho "source '$HOME/.kube/completion.bash.inc'" >> $HOME/.bashrcsource $HOME/.bashrc
```

## 2、启动 deployment 资源测试

```
cat > deploy-apps.yaml  <<EOFapiVersion: apps/v1kind: Deploymentmetadata:  name: wangyangroc-app01spec:  replicas: 1  selector:    matchLabels:      apps: v1   template:    metadata:      labels:        apps: v1    spec:      nodeName: k8s-worker04      containers:      - name: c1        image: wangyanglinux/myapp:v1.0 ---apiVersion: apps/v1kind: Deploymentmetadata:  name: wangyangroc-app02spec:  replicas: 1  selector:    matchLabels:      apps: v1   template:    metadata:      labels:        apps: v1    spec:      nodeName: k8s-worker05      containers:      - name: c1        image: wangyanglinux/myapp:v2.0EOF
```

## 3、验证 Pod 是否可以正常访问

```
[root@k8s-master01 ~]# kubectl get pods -o wideNAME                               READY   STATUS    RESTARTS   AGE     IP           NODE           NOMINATED NODE   READINESS GATESwangyangroc-app01-5bc547f66f-mmdjj   1/1     Running   0          5m59s   10.100.3.2   k8s-worker04   <none>           <none>wangyangroc-app02-6d44ccd76f-nv4v4   1/1     Running   0          5m58s   10.100.0.2   k8s-worker05   <none>           <none>[root@k8s-master01 ~]# [root@k8s-master01 ~]# curl 10.100.3.2   && curl 10.100.0.2
```

## 4、删除资源

```
[root@k8s-master03 ~]# kubectl delete -f deploy-apps.yaml 
```

## 5、出现错误解决方案

```
# 温馨提示# 如果报错  Warning  FailedCreatePodSandBox  72s                kubelet  Failed to create pod sandbox: rpc error: code = Unknown desc = failed to setup network for sandbox "8f179d59d32bf02a1e63ef23b110ac4bee9813d8652b8625d8e899edbf126e19": plugin type="loopback" failed (add): failed to find plugin "loopback" in path [/opt/cni/bin]  Normal   SandboxChanged          15s (x6 over 71s)  kubelet  Pod sandbox changed, it will be killed and re-created.# 解决方案mkdir -p /opt/cni/bincurl -O -L https://github.com/containernetworking/plugins/releases/download/v1.2.0/cni-plugins-linux-amd64-v1.2.0.tgztar -C /opt/cni/bin -xzf cni-plugins-linux-amd64-v1.2.0.tgz
```