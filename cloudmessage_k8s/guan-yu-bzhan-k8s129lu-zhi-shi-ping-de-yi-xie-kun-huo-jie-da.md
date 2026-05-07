# 关于 B 站 k8s 1.29 录制视屏的一些困惑解答

> 来源: http://cloudmessage.top/archives/guan-yu-bzhan-k8s129lu-zhi-shi-ping-de-yi-xie-kun-huo-jie-da

目前，我们整个团队专注于云计算技术培养已经超过 8 年的时间，在整个技术链路中，最后一个环节的教学，是相对比较麻烦的。原因无他，主要是网络问题。这一次的录制同样如此，现今与录制之时网络环境出现了较大差异。所以，出一个帖子，给大家把重复的问题进行统一归口解答，请认真观看！

## 一、关于 Docker YUM 源的问题

因为 Docker-CE yum 源在国内的大部分地区无法访问，所以教学视屏中使用的是基于中科大的 YUM 源进行 Docker-CE 的安装，由于一些原因中科大目前已经停止了对其的支持。所以，国内环境可以替换为阿里巴巴的 Docker-CE 源进行安装即可。

```
$ yum install -y yum-utils device-mapper-persistent-data lvm2$ yum-config-manager --add-repo https://mirrors.aliyun.com/docker-ce/linux/centos/docker-ce.repo$ sed -i 's+download.docker.com+mirrors.aliyun.com/docker-ce+' /etc/yum.repos.d/docker-ce.repo$ yum -y install docker-ce
```

## 二、关于实验镜像下载错误或失败的问题

### 1、方法一，更换加速器为 Daocloud 加速器（tips：不清楚何时会停止）

```
# 修改加速器地址为 Daocloud 加速地址$ cat /etc/docker/daemon.json {  "registry-mirrors": ["https://docker.m.daocloud.io"]}
```

### 2、方法二，使用国内站点服务替换镜像名

在 [https://docker.aityp.com/](https://docker.aityp.com/) 搜索你需要下载的镜像，得到国内镜像名称后，替换至 yaml 文件中即可。比如需要下载 wangyanglinux/myapp:v1，搜索后将国内地址 swr.cn-north-4.myhuaweicloud.com/ddn-k8s/docker.io/wangyanglinux/myapp:v1 替换即可

![](https://img.cloudmessage.top/i/2024/10/29/672046e79ce41.png)

![](https://img.cloudmessage.top/i/2024/10/29/672046fe4aa65.png)

![](https://img.cloudmessage.top/i/2024/10/29/6720471d5dbd7.png)

### 3、方法三，进行提交后不定时回复(请注意看评论，很多镜像都已经在评论中回复地址了！！！！）

在 [https://cloudmessage.top/archives/bang-zhu-da-jia-xia-zai-guo-nei-wu-fa-xia-zai-de-jing-xiang](https://cloudmessage.top/archives/bang-zhu-da-jia-xia-zai-guo-nei-wu-fa-xia-zai-de-jing-xiang) 评论区进行需要镜像的提交，本作者看到后，进行下载替换国内地址后回复，根据评论回复地址使用即可

![](https://img.cloudmessage.top/i/2024/10/29/672047b0e046d.png)

## 三、关于 Ghost 博客部署   connect ECONNREFUSED 127.0.0.1:3306 的问题

ghost 实验构建的时候处于 4 的大版本，现在已经更新至 5 的大版本，之间的差异在于 4 下 mysql 数据库连接不是必备条件，可以降级为 SQLite，所在大家做 Ghost 实验的时候请注意版本

```
docker run --name pause -p 8080:80 -d registry.aliyuncs.com/google_containers/pause:3.8docker run --name nginx -v /root/nginx.conf:/etc/nginx/nginx.conf --net=container:pause --ipc=container:pause --pid=container:pause -d nginxdocker run --name ghost --net=container:pause --ipc=container:pause --pid=container:pause -d ghost:4.48.2
```

```
# 较新版本中的 Docker 未默认开始 IPC 共享，将 pause 容器启动命令修改如下docker run --name pause --ipc=shareable -p 8080:80 -d registry.aliyuncs.com/google_containers/pause:3.8
```

## 四、关于 helm 超时的问题

helm.sh 的站点，很多国内访问不了，需要科学上网。对于刚入行的同学来说，是个问题。

![](https://img.cloudmessage.top/i/2025/08/11/6899aa44e04cb.png)

[https://helm-charts.itboon.top/docs/](https://helm-charts.itboon.top/docs/)  是一个套用 CDN，可以有效解决国内无法访问的站点。

例如视频中的 apache chart 包，可以使用网站中的命令完成下载

```
helm repo add bitnami "https://helm-charts.itboon.top/bitnami" --force-updatehelm update bitnamihelm install my-release bitnami/apache
```