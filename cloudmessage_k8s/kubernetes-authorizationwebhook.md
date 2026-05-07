# Kubernetes - Authorization Webhook

> 来源: http://cloudmessage.top/archives/kubernetes-authorizationwebhook

继上文 [Kubernetes - Authentication Webhook](https://cloudmessage.top/archives/kubernetes-authenticationwebhook) ，下面我将实现一个 Authorization Webhook 案例，用于控制 Kubernetes API 请求的权限。这个 Webhook 会根据用户所属的组（Groups）决定是否允许特定操作（例如：只有 admin 组可以删除 Pod，developer 组只能查看 Pod）。

## 一、Authorization Webhook 工作原理

Kubernetes API Server 在认证通过后，会将请求的详细信息（用户、操作类型、资源等）发送给 Authorization Webhook。Webhook 根据预设的权限规则返回 “允许” 或 “拒绝” 的决策，API Server 根据结果决定是否执行请求。

## 二、案例实现

### 1、Webhook 服务代码

```
package mainimport (        "encoding/json"        "log"        "net/http"        authorizationv1 "k8s.io/api/authorization/v1"        metav1 "k8s.io/apimachinery/pkg/apis/meta/v1")// 权限判断逻辑（使用 Groups 字段）func allowAccess(userGroups []string, resource, verb string) bool {        // admin 组允许所有操作        for _, g := range userGroups {                if g == "system:masters" || g == "masters" || g == "admin" {                        return true                }        }        // developer 组仅允许查看 Pod        for _, g := range userGroups {                if g == "system:developers" || g == "developers" {                        return resource == "pods" && (verb == "get" || verb == "list" || verb == "watch")                }        }        return false // 默认拒绝}// 处理授权请求（使用实际存在的 User 和 Groups 字段）func authorizeHandler(w http.ResponseWriter, r *http.Request) {        var sar authorizationv1.SubjectAccessReview        if err := json.NewDecoder(r.Body).Decode(&sar); err != nil {                log.Printf("解析请求失败: %v", err)                http.Error(w, "无效请求", http.StatusBadRequest)                return        }        // 从 Spec 中提取实际存在的字段（根据你的 types.go 定义）        spec := sar.Spec        user := spec.User         // 用户名（对应 types.go 中的 User 字段）        userGroups := spec.Groups // 用户组（对应 types.go 中的 Groups 字段）                // 提取资源和操作信息        resource := ""        verb := ""        if spec.ResourceAttributes != nil {                resource = spec.ResourceAttributes.Resource                verb = spec.ResourceAttributes.Verb        }        // 判断权限        allowed := allowAccess(userGroups, resource, verb)        log.Printf(                "授权检查: 用户=%s, 组=%v, 资源=%s, 操作=%s, 允许=%v",                user,       // 使用 spec.User                userGroups, // 使用 spec.Groups                resource,                verb,                allowed,        )        // 构建响应        response := authorizationv1.SubjectAccessReview{                TypeMeta: metav1.TypeMeta{                        APIVersion: "authorization.k8s.io/v1",                        Kind:       "SubjectAccessReview",                },                Status: authorizationv1.SubjectAccessReviewStatus{                        Allowed: allowed,                        Denied:  !allowed,                        Reason:  "自定义授权规则决策",                },        }        // 返回响应        w.Header().Set("Content-Type", "application/json")        if err := json.NewEncoder(w).Encode(response); err != nil {                log.Printf("响应编码失败: %v", err)                http.Error(w, "服务器错误", http.StatusInternalServerError)        }}func main() {        http.HandleFunc("/authorize", authorizeHandler)        log.Println("授权 Webhook 服务启动，监听 :443")        // 启动 HTTPS 服务（需提前准备 tls.crt 和 tls.key）        if err := http.ListenAndServeTLS(":443", "/etc/tls/tls.crt", "/etc/tls/tls.key", nil); err != nil {                log.Fatalf("服务启动失败: %v", err)        }}
```

```
go 1.24.4require (        k8s.io/api v0.27.0        k8s.io/apimachinery v0.27.0)require (        github.com/go-logr/logr v1.2.3 // indirect        github.com/gogo/protobuf v1.3.2 // indirect        github.com/google/gofuzz v1.1.0 // indirect        github.com/json-iterator/go v1.1.12 // indirect        github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect        github.com/modern-go/reflect2 v1.0.2 // indirect        golang.org/x/net v0.8.0 // indirect        golang.org/x/text v0.8.0 // indirect        gopkg.in/inf.v0 v0.9.1 // indirect        gopkg.in/yaml.v2 v2.4.0 // indirect        k8s.io/klog/v2 v2.90.1 // indirect        k8s.io/utils v0.0.0-20230209194617-a36077c30491 // indirect        sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect        sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect)
```

### 2、容器化 webhook

```
FROM docker.cnb.cool/wangyanglinux/docker-images-chrom/alpine:3.17WORKDIR /root/# 从构建阶段复制二进制文件COPY ./authorization-webhook /rootRUN mkdir  /etc/tls# 暴露端口EXPOSE 443# 运行应用CMD ["./authorization-webhook"]
```

> 
本项目代码案例，已经上传至阿里云镜像仓库，地址为：registry.cn-hangzhou.aliyuncs.com/wangyangshare/share:authorizationnew

### 3、配置生成 TLS 证书（与 Authentication Webhook 类似）

创建 openssl.cnf 配置文件

```
[req]default_bits = 2048prompt = nodefault_md = sha256distinguished_name = dnreq_extensions = req_ext[dn]CN = authz-webhook-svc.default.svc  # 服务域名[req_ext]subjectAltName = DNS:authz-webhook-svc.default.svc
```

生成证书

```
# 生成 CA 和服务器证书openssl genrsa -out ca.key 2048openssl req -x509 -new -nodes -key ca.key -days 3650 -out ca.crt -subj "/CN=authz-ca"openssl genrsa -out tls.key 2048openssl req -new -key tls.key -out tls.csr -config openssl.cnfopenssl x509 -req -in tls.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out tls.crt -days 3650 -extensions req_ext -extfile openssl.cnf# 创建 Secret 存储证书kubectl create secret tls authz-webhook-tls --cert=tls.crt --key=tls.key
```

### 4、部署 Webhook 服务（Deployment + Service）

```
apiVersion: apps/v1kind: Deploymentmetadata:  name: authz-webhook  namespace: defaultspec:  replicas: 1  selector:    matchLabels:      app: authz-webhook  template:    metadata:      labels:        app: authz-webhook    spec:      containers:      - name: authz-webhook        image: registry.cn-hangzhou.aliyuncs.com/wangyangshare/share:authorizationnew        ports:        - containerPort: 443        volumeMounts:        - name: tls-cert          mountPath: /etc/tls          readOnly: true      volumes:      - name: tls-cert        secret:          secretName: authz-webhook-tls---apiVersion: v1kind: Servicemetadata:  name: authz-webhook-svc  namespace: defaultspec:  selector:    app: authz-webhook  ports:  - port: 443    targetPort: 443
```

### 5、添加解析记录至 /etc/hosts 文件

<对应 svc clusterip>authz-webhook-svc.default.svc

### 6、配置 API Server 使用 Authorization Webhook

拷贝文件至对应路径

```
cp ca.crt /etc/kubernetes/pki/authz-ca.crtcp authz-config.yaml /etc/kubernetes/authz-config.yaml
```

创建 Webhook 配置文件（authz-config.yaml）

```
apiVersion: v1kind: Configpreferences: {}clusters:- name: authz-webhook-cluster  cluster:    certificate-authority: /etc/kubernetes/pki/authz-ca.crt  # 挂载的 CA 证书    server: https://authz-webhook-svc.default.svc:443/authorize  # Webhook 服务地址users:- name: apiserver  user:    client-certificate: /etc/kubernetes/pki/apiserver.crt  # 可选，双向认证    client-key: /etc/kubernetes/pki/apiserver.keycontexts:- name: authz-webhook-context  context:    cluster: authz-webhook-cluster    user: apiservercurrent-context: authz-webhook-context
```

配置 API Server 启动参数：修改 API Server 静态 Pod 配置（/etc/kubernetes/manifests/kube-apiserver.yaml），添加授权 Webhook 参数

```
spec:  containers:  - command:    - kube-apiserver    # 保留原有参数...    # 添加以下授权参数    - --authorization-mode=Node,RBAC,Webhook  # 新增 Webhook 模式    - --authorization-webhook-config-file=/etc/kubernetes/authz-config.yaml    - --authorization-webhook-version=v1    # 挂载配置文件和 CA 证书    volumeMounts:    - name: authz-config      mountPath: /etc/kubernetes/authz-config.yaml    - name: authz-ca      mountPath: /etc/kubernetes/pki/authz-ca.crt  volumes:  - name: authz-config    hostPath:      path: /etc/kubernetes/authz-config.yaml      type: File  - name: authz-ca    hostPath:      path: /etc/kubernetes/pki/authz-ca.crt      type: File
```

### 7、测试授权效果

#### 1）准备测试用户（基于之前的 Authentication Webhook）

- admin 用户（属于 admin 组）：令牌 valid-token-123
- developer 用户（属于 developer 组）：令牌 valid-token-456

#### 2）测试 developer 组权限

```
# 使用 developer 令牌配置 kubectlkubectl config set-credentials developer --token=valid-token-456kubectl config set-context developer-context --cluster=kubernetes --user=developerkubectl config use-context developer-context# 允许操作：查看 Podkubectl get pods  # 应成功# 拒绝操作：创建 Podkubectl run test-pod --image=nginx  # 应返回 "Error from server (Forbidden)"
```

#### 3）测试 admin 组权限

```
# 切换到 admin 用户kubectl config set-credentials admin --token=valid-token-123kubectl config set-context admin-context --cluster=kubernetes --user=adminkubectl config use-context admin-context# 允许操作：创建和删除 Podkubectl run test-pod --image=nginx  # 成功kubectl delete pod test-pod  # 成功
```

#### 4）查看 Webhook 服务日志验证决策

```
kubectl logs -n default <authz-webhook-pod> -f
```

预期日志

```
Authorization check: user=developer, groups=[system:developers], resource=pods, verb=get, allowed=trueAuthorization check: user=developer, groups=[system:developers], resource=pods, verb=create, allowed=falseAuthorization check: user=admin, groups=[system:masters], resource=pods, verb=create, allowed=true
```

### 8、核心说明

- 权限规则：案例中 admin 组拥有所有权限，developer 组仅能查看 Pod。实际生产中可扩展为更复杂的规则（如基于命名空间、资源标签的权限控制）。
- 与 RBAC 结合：--authorization-mode=Node,RBAC,Webhook 表示先执行 RBAC 授权，再执行 Webhook 授权（两者都允许才通过）。
- 安全性：必须使用 TLS 加密通信，避免权限决策被篡改。
- 性能考虑：Webhook 服务需低延迟，可通过缓存频繁请求的权限决策优化性能。

## 三、关于多个鉴权模式同时使用的问题

Kubernetes API Server 的 --authorization-mode 参数指定的是授权器的执行顺序，但判定逻辑是：只要有一个授权器允许请求，就会通过授权；只有当所有授权器都拒绝时，才会最终拒绝。

具体到 --authorization-mode=Node,RBAC,Webhook 的场景，执行流程是

### 1、按顺序依次调用授权器

API Server 会按照配置的顺序（Node → RBAC → Webhook）依次调用每个授权器对请求进行判断

- 首先由 Node 授权器 检查请求（专门处理节点相关的权限，如 kubelet 对 Pod、ConfigMap 的操作）
- 若 Node 授权器允许，则直接通过授权，后续的 RBAC 和 Webhook 不再执行
- 若 Node 授权器拒绝，则调用 RBAC 授权器 继续检查
- 若 RBAC 授权器允许，则通过授权，Webhook 不再执行
- 若 RBAC 授权器拒绝，则调用 Webhook 授权器 检查
- 若 Webhook 授权器允许，则通过授权；若 Webhook 也拒绝，则最终拒绝请求

### 2、关键原则：“一允许即通过”

授权流程不是 “前面的授权器失败才轮到下一个”，而是 “依次检查，只要有一个授权器允许，就立即通过”。例如：

- 节点（system:node 用户）的请求会被 Node 授权器 直接允许（无需经过 RBAC 或 Webhook）
- 普通用户通过 RBAC 角色绑定获得的权限，会被 RBAC 授权器 允许（无需经过 Webhook）
- 只有 RBAC 未覆盖的自定义权限场景（如你的 developer 组规则），才会触发 Webhook 授权器 进行判断

### 3、为什么这样配置？

Node,RBAC,Webhook 是推荐的组合模式，原因是：

- Node 授权器：专门处理节点（kubelet）的核心权限，优先级最高，确保节点基础功能不受其他授权器影响
- RBAC 授权器：处理集群内大部分常规权限（如角色、集群角色），覆盖绝大多数场景

- Webhook 授权器：作为补充，处理 RBAC 难以覆盖的自定义权限逻辑（如你的基于用户组的精细化控制）。

三者分工明确，既保留了 Kubernetes 原生的授权能力，又支持自定义扩展。

#### 举例说明

##### 1）kubelet 访问 ConfigMap：请求用户是 system:node:k8s-master01，属于 system:nodes 组。

- Node 授权器 识别到这是节点请求，直接允许 → 授权通过（不经过 RBAC 和 Webhook）

##### 2）admin 用户删除 Pod：用户属于 system:masters 组，RBAC 中 cluster-admin 角色允许删除 Pod

- Node 授权器拒绝（非节点请求）→ RBAC 授权器允许 → 授权通过（不经过 Webhook）

##### 3）developer 用户查看 Pod：用户属于 developer 组，RBAC 未配置相关权限。

- Node 授权器拒绝 → RBAC 授权器拒绝 → Webhook 授权器检查并允许 → 授权通过

##### 4）anonymous 用户创建 Pod：无任何权限

- Node 拒绝 → RBAC 拒绝 → Webhook 拒绝 → 最终授权失败。通过这种顺序，既能保证集群原生组件（如 kubelet）的正常运行，又能通过 RBAC 和 Webhook 实现灵活的权限控制，是生产环境的最佳实践。