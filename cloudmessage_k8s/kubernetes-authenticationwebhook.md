# Kubernetes - Authentication Webhook

> 来源: http://cloudmessage.top/archives/kubernetes-authenticationwebhook

在 Kubernetes 生态中，除了最常见的 Validating Webhook（验证型准入 Webhook）和 Mutating Webhook（变异型准入 Webhook），还有其他几种基于 Webhook 机制的扩展方式，主要用于 Kubernetes 的认证、授权或特定资源的生命周期管理。以下是主要相关类型：

1、Authentication Webhooks（认证 Webhook）

作用：用于验证用户身份（替代传统的客户端证书、密码等认证方式）。当 Kubernetes API Server 收到请求时，会调用外部 Webhook 服务验证请求的身份合法性（例如检查令牌是否有效）。

场景：集成外部身份系统（如 LDAP、OAuth2、企业单点登录 SSO 等），让 Kubernetes 支持自定义身份认证逻辑。

流程：请求到达 API Server 后，首先进入认证阶段，认证 Webhook 返回用户信息（如用户名、UID、组），API Server 根据返回结果判断是否通过认证。

2、Authorization Webhooks（授权 Webhook）

作用：用于判断已认证用户是否有权限执行某个操作（例如 “用户 A 是否能删除 namespace B 中的 Pod”）。

场景：当 Kubernetes 内置的授权策略（如 RBAC、Node 授权）不足以满足需求时，通过外部 Webhook 实现复杂的权限逻辑（如基于属性的访问控制 ABAC、多租户权限隔离等）。

流程：认证通过后，API Server 进入授权阶段，调用授权 Webhook 检查操作权限，Webhook 返回 “允许” 或 “拒绝” 的决策。

3、Conversion Webhooks（转换 Webhook）

作用：用于自定义资源（CRD）的版本转换。当 CRD 存在多个 API 版本（如 v1alpha1 升级到 v1beta1）时，转换 Webhook 可以定义不同版本之间的字段映射逻辑，确保资源在版本切换时正确转换。

场景：CRD 多版本管理，避免手动处理版本兼容问题。例如，当用户提交 v1alpha1 版本的 CR 时，Webhook 自动转换为 v1beta1 版本供后端处理。

4、Dynamic Admission Control 中的其他细分场景**虽然核心是 Mutating 和 Validating Webhook，但它们可以针对不同资源类型（如 Pod、Deployment、自定义 CR 等）或操作（创建、更新、删除）进行细分，例如：

- 针对 Pod 调度前验证 的 Webhook（检查资源是否满足调度条件）；
- 针对 Secret 变更审计 的 Webhook（记录 Secret 修改操作）；
- 针对 命名空间生命周期 的 Webhook（限制特定命名空间的创建）。

总结：不同 Webhook 的阶段与用途**

Kubernetes 对请求的处理流程分为三个核心阶段，对应不同的 Webhook 类型：

- 认证（Authentication）：确认 “谁在请求”→ 由 Authentication Webhook 处理；
- 授权（Authorization）：确认 “是否允许操作”→ 由 Authorization Webhook 处理；
- 准入控制（Admission）：确认 “操作的资源是否合法 / 是否需要修改”→ 由 Mutating Webhook（修改资源）和 Validating Webhook（验证资源）处理；
- CRD 版本转换：处理自定义资源的版本兼容→ 由 Conversion Webhook 处理。

这些 Webhook 共同构成了 Kubernetes 的扩展机制，允许用户通过外部服务自定义集群的安全策略、资源管理逻辑和身份体系。

下面我将实现一个 Authentication Webhook 案例，用于验证 Kubernetes API 请求中的 Bearer Token（令牌）是否有效。这个 Webhook 会检查令牌是否在预设的 “允许列表” 中，若有效则返回对应的用户信息（用户名、组），否则拒绝认证。

## 一、案例实现

### 1、webhook 服务代码

```
package mainimport (        "encoding/json"        "log"        "net/http"        authenticationv1 "k8s.io/api/authentication/v1"        metav1 "k8s.io/apimachinery/pkg/apis/meta/v1")// 预设的有效令牌列表（key: token, value: 对应的用户信息）var validTokens = map[string]struct {        Username string        Groups   []string}{        "valid-token-123": {                Username: "admin",                Groups:   []string{"system:masters"}, // 管理员组（拥有最高权限）        },        "valid-token-456": {                Username: "developer",                Groups:   []string{"system:developers"}, // 开发者组        },}func main() {        // 处理认证请求的端点        http.HandleFunc("/authenticate", authenticateHandler)        log.Println("Starting authentication webhook server on :443")        // 生产环境需使用合法 TLS 证书        if err := http.ListenAndServeTLS(":443", "/etc/tls/tls.crt", "/etc/tls/tls.key", nil); err != nil {                log.Fatalf("Failed to start server: %v", err)        }}// 处理认证请求func authenticateHandler(w http.ResponseWriter, r *http.Request) {        var review authenticationv1.TokenReview        if err := json.NewDecoder(r.Body).Decode(&review); err != nil {                log.Printf("Error decoding request: %v", err)                http.Error(w, "Invalid request body", http.StatusBadRequest)                return        }        // 解析请求中的 Bearer Token        token := review.Spec.Token        if token == "" {                sendAuthResponse(w, review.Spec, false, "", nil)                return        }        // 验证令牌是否在有效列表中        userInfo, isValid := validTokens[token]        if !isValid {                log.Printf("Invalid token: %s", token)                sendAuthResponse(w, review.Spec, false, "", nil)                return        }        // 令牌有效，返回用户信息        log.Printf("Valid token for user: %s", userInfo.Username)        sendAuthResponse(w, review.Spec, true, userInfo.Username, userInfo.Groups)}// 发送认证响应func sendAuthResponse(        w http.ResponseWriter,        spec authenticationv1.TokenReviewSpec,        authenticated bool,        username string,        groups []string,) {        response := authenticationv1.TokenReview{                TypeMeta: metav1.TypeMeta{                        APIVersion: "authentication.k8s.io/v1",                        Kind:       "TokenReview",                },                Spec: spec,                Status: authenticationv1.TokenReviewStatus{                        Authenticated: authenticated,                        User: authenticationv1.UserInfo{                                Username: username,                                Groups:   groups,                        },                },        }        w.Header().Set("Content-Type", "application/json")        if err := json.NewEncoder(w).Encode(response); err != nil {                log.Printf("Error encoding response: %v", err)                http.Error(w, "Error sending response", http.StatusInternalServerError)        }}
```

### 2、webhook 容器化

```
FROM docker.cnb.cool/wangyanglinux/docker-images-chrom/alpine:3.17WORKDIR /root/# 从构建阶段复制二进制文件COPY ./authen /rootRUN mkdir  /etc/tls# 暴露端口EXPOSE 443# 运行应用CMD ["./authen"]
```

> 
本项目镜像已经保存至阿里云镜像平台：registry.cn-hangzhou.aliyuncs.com/wangyangshare/share:authwebhook

### 3、生成 API Server 与 Webhook 通信需要的 HTTPS 证书

创建 openssl.cnf 配置文件（确保包含 SAN）

```
[req]default_bits = 2048prompt = nodefault_md = sha256distinguished_name = dnreq_extensions = req_ext[dn]CN = auth-webhook-svc.default.svc[req_ext]subjectAltName = DNS:auth-webhook-svc.default.svc
```

生成证书

```
# 生成 CA 和服务器证书openssl genrsa -out ca.key 2048openssl req -x509 -new -nodes -key ca.key -days 3650 -out ca.crt -subj "/CN=auth-ca"openssl genrsa -out tls.key 2048openssl req -new -key tls.key -out tls.csr -config openssl.cnfopenssl x509 -req -in tls.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out tls.crt -days 3650 -extensions req_ext -extfile openssl.cnf
```

创建 Secret 存储证书

```
kubectl create secret tls auth-webhook-tls --cert=tls.crt --key=tls.key
```

### 4、部署服务至集群

```
apiVersion: apps/v1kind: Deploymentmetadata:  name: auth-webhook  namespace: defaultspec:  replicas: 1  selector:    matchLabels:      app: auth-webhook  template:    metadata:      labels:        app: auth-webhook    spec:      containers:      - name: auth-webhook        image: k8s-auth-webhook:latest  # 替换为你的镜像        ports:        - containerPort: 443        volumeMounts:        - name: tls-cert          mountPath: /etc/tls          readOnly: true      volumes:      - name: tls-cert        secret:          secretName: auth-webhook-tls---apiVersion: v1kind: Servicemetadata:  name: auth-webhook-svc  namespace: defaultspec:  selector:    app: auth-webhook  ports:  - port: 443    targetPort: 443
```

### 5、配置 API Server 使用 Authentication Webhook

Kubernetes API Server 通过启动参数或配置文件启用 Authentication Webhook。以下是具体步骤

#### 1）建 Webhook 配置文件（auth-config.yaml）

```
apiVersion: v1kind: Configpreferences: {}clusters:- name: auth-webhook-cluster  cluster:    certificate-authority: /etc/kubernetes/pki/auth-ca.crt  # 挂载的 CA 证书路径    server: https://auth-webhook-svc.default.svc:443/authenticate  # Webhook 服务地址users:- name: apiserver  user:    client-certificate: /etc/kubernetes/pki/apiserver.crt  # API Server 证书（可选，用于双向认证）    client-key: /etc/kubernetes/pki/apiserver.keycontexts:- name: auth-webhook-context  context:    cluster: auth-webhook-cluster    user: apiservercurrent-context: auth-webhook-context
```

#### 2）挂载配置到 API Server

- 将 ca.crt 复制到 API Server 节点的证书目录（如 /etc/kubernetes/pki/auth-ca.crt）
- 将 auth-config.yaml 复制到 API Server 配置目录（如 /etc/kubernetes/auth-config.yaml）
- 修改 API Server 的启动参数（通常在 /etc/kubernetes/manifests/kube-apiserver.yaml），添加

```
spec:  containers:  - command:    - kube-apiserver    # 新增以下参数    - --authentication-token-webhook-config-file=/etc/kubernetes/auth-config.yaml    - --authentication-token-webhook-version=v1
```

```
volumeMounts:- name: auth-config  mountPath: /etc/kubernetes/auth-config.yaml  # 直接挂载文件到容器内的路径  readOnly: true  # 建议添加只读权限（配置文件无需写入）- name: auth-ca  mountPath: /etc/kubernetes/pki/auth-ca.crt  # 同理，删除 subPath  readOnly: true# volumes 部分保持不变（确保主机上的文件存在）volumes:- name: auth-config  hostPath:    path: /etc/kubernetes/auth-config.yaml    type: File- name: auth-ca  hostPath:    path: /etc/kubernetes/pki/auth-ca.crt    type: File
```

#### 3）重启 API Server（修改 kube-apiserver.yaml 后会自动重启）

### 6、测试

#### 1）使用有效令牌访问 API

```
# 用有效令牌（如 valid-token-123）发送请求curl -k -H "Authorization: Bearer valid-token-123" https://<API-Server-IP>:6443/api/v1/pods
```

预期结果：请求成功，返回 Pod 列表（因为 admin 属于 system:masters 组，有权限）。

#### 2）使用无效令牌访问 API

```
curl -k -H "Authorization: Bearer invalid-token" https://<API-Server-IP>:6443/api/v1/pods
```

预期结果：返回 401 Unauthorized 错误，认证失败。

#### 3）如果不能显示预期结果，可能原因如下

- 确定是否出现了域名解析不了的情况，有可能是因为 master 节点没有采用集群内的 DNS 实现解析导致的，因为 apiserver 采用的是主机网络实现访问，会使用物理机对应的 DNS 配置 。 可以通过：kubectl logs -n kube-system kube-apiserver-k8s-master01 |  grep -i "auth-webhook-svc" 查看添加 DNS 解析记录至 master 节点的 /etc/hosts 文件中：10.97.77.91 auth-webhook-svc.default.svc（ IP 为对应 svc 的 CLUSTERIP 地址）
- 修改 master DNS 备为集群内 DNS 服务器

#### 4）通过 kubectl 测试

配置 kubectl 使用令牌：

```
创建一个使用有效令牌的 kubeconfigkubectl config set-credentials test-user --token=valid-token-123kubectl config set-context test-context --cluster=kubernetes --user=test-userkubectl config use-context test-context
```

执行命令（应成功）

```
kubectl get pods
```