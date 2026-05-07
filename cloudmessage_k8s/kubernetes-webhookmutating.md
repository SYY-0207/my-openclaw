# Kubernetes - Mutating Webhook

> 来源: http://cloudmessage.top/archives/kubernetes-webhookmutating

在 Kubernetes（k8s）中，Mutating Webhook 是一种用于在资源创建 / 更新等操作的准入阶段（Admission Phase） 动态修改资源对象的机制。它可以在资源被 Kubernetes API Server 持久化之前，对资源的配置进行自动调整（例如注入默认值、添加标签、注入 Sidecar 容器等），从而简化用户配置或强制团队规范。

## 一、相关概念

### 核心作用

Mutating Webhook 的核心是 **“修改”**：在资源满足触发条件时，通过 HTTP/HTTPS 请求调用用户自定义的 Web 服务（Webhook 服务），由该服务返回需要修改的内容，最终 API Server 根据修改内容更新资源。

### 工作流程

当用户通过 kubectl或 API 创建 / 更新一个资源（如 Pod、Deployment）时，流程如下：

- 资源请求先经过认证（Authentication） 和授权（Authorization） 阶段
- 进入准入控制（Admission Control） 阶段，先执行所有 Mutating Webhook（修改资源），再执行 Validating Webhook（验证资源，不修改）
- 若所有 Webhook 通过，资源被持久化到 etcd；若失败，请求被拒绝

### 关键时机

Mutating Webhook 运行在资源被持久化之前，且先于 Validating Webhook 执行（确保修改后的资源会被验证）。

### 典型使用场景

- 自动注入 Sidecar 容器：例如 Istio 服务网格中，自动为 Pod 注入代理 Sidecar（istio-proxy），无需用户手动配置
- 设置默认值：为未指定 resources.limits 的 Pod 自动添加默认资源限制，或为 Namespace 自动添加默认标签
- 强制规范：例如强制所有 Pod 添加 app:  标签，或修改镜像拉取策略为 Always
- 敏感信息处理：自动替换明文密码为 Secret 引用

### 如何实现一个 Mutating Webhook？

实现步骤可分为 3 部分：部署 Webhook 服务、配置证书、定义 Webhook 规则

#### 1. 部署 Webhook 服务

Webhook 服务是一个 HTTP/HTTPS 服务（K8s 要求 HTTPS），需部署在 K8s 集群内（通常用 Deployment+Service 暴露），负责接收 API Server 的请求并返回修改指令。

**服务接收的请求格式（简化）**

```
{  "apiVersion": "admission.k8s.io/v1",  "kind": "AdmissionReview",  "request": {    "uid": "xxx",  // 请求唯一ID    "object": { ... },  // 待修改的资源对象（如Pod）    "operation": "CREATE"  // 操作类型：CREATE/UPDATE/DELETE/CONNECT  }}
```

**服务返回的响应格式（简化）**

```
{  "apiVersion": "admission.k8s.io/v1",  "kind": "AdmissionReview",  "response": {    "uid": "xxx",  // 对应请求的uid    "allowed": true,  // 是否允许操作    "patch": "base64编码的JSON Patch",  // 要修改的内容（JSON Patch格式）    "patchType": "JSONPatch"  }}
```

例如：若要给 Pod 添加标签 env: prod，JSON Patch 为[{"op":"add","path":"/metadata/labels/env","value":"prod"}]，base64 编码后放入 patch 字段

#### 2. 配置 HTTPS 证书

K8s API Server 调用 Webhook 时要求 HTTPS（避免中间人攻击），因此需要为 Webhook 服务配置 TLS 证书。通常有两种方式：

- 手动生成证书（自签或 CA 签发），通过 Secret 挂载到 Webhook 服务
- 使用工具自动管理（如cert-manager），自动生成和轮换证书

#### 3. 定义 MutatingWebhookConfiguration

通过 K8s 的 MutatingWebhookConfiguration 资源配置 Webhook 的触发规则，告诉 API Server：“哪些资源、在什么操作下需要调用我的 Webhook 服务”

**示例配置**

```
apiVersion: admissionregistration.k8s.io/v1kind: MutatingWebhookConfigurationmetadata:  name: example-mutating-webhookwebhooks:- name: example-webhook.example.com  # Webhook名称（需符合域名格式）  clientConfig:    service:      name: example-webhook-service  # Webhook服务的Service名称      namespace: default  # 服务所在命名空间      path: "/mutate"  # 服务接收请求的路径    caBundle: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0t...  # 信任的CA证书（base64编码）  rules:  - operations: ["CREATE"]  # 触发操作：创建资源时    apiGroups: [""]  # 目标API组（空表示core组）    apiVersions: ["v1"]  # 目标API版本    resources: ["pods"]  # 目标资源（这里是Pod）  namespaceSelector:  # 仅匹配特定命名空间的资源（可选）    matchLabels:      env: prod  failurePolicy: Fail  # 若Webhook调用失败，是否拒绝请求（Fail/Ignore）  timeoutSeconds: 5  # 超时时间（默认10s）
```

- rules：定义触发 Webhook 的资源（如pods、deployments）、操作（CREATE/UPDATE等）
- namespaceSelector/objectSelector：进一步过滤目标资源（例如只处理 env=prod 命名空间的 Pod）
- failurePolicy：若 Webhook 服务不可用，Fail表示拒绝请求，Ignore表示跳过 Webhook 继续处理

### 与 Validating Webhook 的区别

- Mutating Webhook：修改资源（返回patch），可添加 / 删除 / 修改字段
- Validating Webhook：仅验证资源（不修改），返回allowed: true/false，用于检查资源是否符合规范（如禁止特权容器）

两者可配合使用：先通过 Mutating 修改资源，再通过 Validating 验证修改后的资源是否合法

### 调试技巧

- 查看 API Server 日志（kubectl logs -n kube-system kube-apiserver-xxx），排查 Webhook 调用失败原因（如证书无效、服务不可达）
- 在 Webhook 服务中打印请求 / 响应日志，确认是否正确处理
- 使用 kubectl describe mutatingwebhookconfiguration  检查 Webhook 配置是否生效

## 二、跟我一步一步开发一个 Mutating Webhook

### 模拟需求

所有在 default 名字空间创建的 pod，都会自动添加一个"created-by":  "pod-label-mutator", "managed-by":  "webhook", "environment": "default" 的标签（来自于同学的面试问题）

### 基于 golang 编写 Kubernetes Mutating Webhook

#### 安装 golang 环境

```
cd /tmpwget https://dl.google.com/go/go1.20.linux-amd64.tar.gzsudo tar -C /usr/local -xzf go1.20.linux-amd64.tar.gzvi ~/.bash_profile    # Golang环境变量    export GOROOT=/usr/local/go    export GOPATH=$HOME/go    export PATH=$PATH:$GOROOT/bin:$GOPATH/binsource ~/.bash_profilego versionmkdir -p $HOME/go/{src,pkg,bin}# 临时配置国内 golang 库代理# 设置代理（以七牛云为例，可替换为其他代理地址）export GOPROXY=https://goproxy.cn,direct# 可选：设置校验和数据库代理（解决 sumdb 访问问题）export GOSUMDB=sum.golang.google.cn
```

#### 程序关键部分

- Webhook 服务：使用标准库的net/http实现 HTTPS 服务器
- 处理/mutate端点的 POST 请求
- 解析 Kubernetes 的 AdmissionReview 请求
- 生成适当的 JSON Patch 来添加标签

- 标签添加逻辑：定义了要添加的默认标签（created-by、managed-by和environment）
- 检查 Pod 是否已有这些标签，如果没有则添加
- 生成 JSON Patch 描述这些修改

- 部署配置：Deployment 定义了如何运行 Webhook 服务
- Service 暴露 Webhook 服务供 Kubernetes API Server 访问
- MutatingWebhookConfiguration 配置了何时触发 Webhook

#### 程序体

```
package mainimport (        "encoding/json"        "log"        "net/http"        admissionv1 "k8s.io/api/admission/v1"        corev1 "k8s.io/api/core/v1"        metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"        "k8s.io/apimachinery/pkg/types" // 新增：导入UID类型所在的包)// 要添加的默认标签var defaultLabels = map[string]string{        "created-by":  "pod-label-mutator",        "managed-by":  "webhook",        "environment": "default",}func main() {        http.HandleFunc("/mutate", mutateHandler)        log.Println("Starting webhook server on :443")                // 注意：实际生产环境需要使用合法的TLS证书        if err := http.ListenAndServeTLS(":443", "/etc/tls/tls.crt", "/etc/tls/tls.key", nil); err != nil {                log.Fatalf("Failed to start server: %v", err)        }}func mutateHandler(w http.ResponseWriter, r *http.Request) {        var review admissionv1.AdmissionReview        if err := json.NewDecoder(r.Body).Decode(&review); err != nil {                log.Printf("Error decoding request: %v", err)                http.Error(w, "Invalid request body", http.StatusBadRequest)                return        }        // 提取Pod对象        pod := &corev1.Pod{}        if err := json.Unmarshal(review.Request.Object.Raw, pod); err != nil {                log.Printf("Error unmarshalling pod: %v", err)                // 直接传递types.UID类型，无需转换                sendResponse(w, review.Request.UID, false, nil)                return        }        // 生成需要添加的标签        patchOps := generateLabelPatch(pod)        if len(patchOps) == 0 {                sendResponse(w, review.Request.UID, true, nil)                return        }        // 发送包含补丁的响应        sendResponse(w, review.Request.UID, true, patchOps)}// 生成添加标签的JSON Patch操作func generateLabelPatch(pod *corev1.Pod) []map[string]interface{} {        var patchOps []map[string]interface{}                // 如果Pod没有标签，先创建labels字段        if pod.ObjectMeta.Labels == nil {                patchOps = append(patchOps, map[string]interface{}{                        "op":    "add",                        "path":  "/metadata/labels",                        "value": defaultLabels,                })                return patchOps        }        // 为不存在的标签添加补丁        for key, value := range defaultLabels {                if _, exists := pod.ObjectMeta.Labels[key]; !exists {                        patchOps = append(patchOps, map[string]interface{}{                                "op":    "add",                                "path":  "/metadata/labels/" + key,                                "value": value,                        })                }        }        return patchOps}// 发送 AdmissionReview 响应// 注意：uid参数类型改为types.UID，与Kubernetes API类型一致func sendResponse(w http.ResponseWriter, uid types.UID, allowed bool, patchOps []map[string]interface{}) {        var patchBytes []byte        var err error                if patchOps != nil && len(patchOps) > 0 {                patchBytes, err = json.Marshal(patchOps)                if err != nil {                        log.Printf("Error marshaling patch: %v", err)                        http.Error(w, "Error creating patch", http.StatusInternalServerError)                        return                }        }        response := admissionv1.AdmissionReview{                TypeMeta: metav1.TypeMeta{                        APIVersion: "admission.k8s.io/v1",                        Kind:       "AdmissionReview",                },                Response: &admissionv1.AdmissionResponse{                        UID:     uid, // 直接使用types.UID类型，无需转换                        Allowed: allowed,                },        }        // 如果有补丁，添加到响应中        if len(patchBytes) > 0 {                response.Response.Patch = patchBytes                response.Response.PatchType = &[]admissionv1.PatchType{admissionv1.PatchTypeJSONPatch}[0]        }        w.Header().Set("Content-Type", "application/json")        if err := json.NewEncoder(w).Encode(response); err != nil {                log.Printf("Error encoding response: %v", err)                http.Error(w, "Error sending response", http.StatusInternalServerError)        }}
```

#### 打包为镜像

```
# 使用官方Go镜像作为构建阶段FROM golang:1.20-alpine AS builderWORKDIR /app# 复制go模块文件并下载依赖COPY go.mod go.sum ./RUN go mod download# 复制源代码COPY . .# 构建应用RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o pod-label-mutator .# 使用轻量级Alpine镜像作为运行阶段FROM alpine:3.17WORKDIR /root/# 从构建阶段复制二进制文件COPY --from=builder /app/pod-label-mutator .# 创建存放证书的目录RUN mkdir -p /etc/tls# 暴露端口EXPOSE 443# 运行应用CMD ["./pod-label-mutator"]
```

> 
已推送至阿里云镜像仓库，镜像名称为 registry.cn-hangzhou.aliyuncs.com/wangyangshare/share:mutatetest

#### 编写 deployment 文件，部署 webhook 至集群

```
apiVersion: apps/v1kind: Deploymentmetadata:  name: pod-label-mutator  namespace: defaultspec:  replicas: 1  selector:    matchLabels:      app: pod-label-mutator  template:    metadata:      labels:        app: pod-label-mutator    spec:      containers:      - name: pod-label-mutator        image: registry.cn-hangzhou.aliyuncs.com/wangyangshare/share:mutatetest        ports:        - containerPort: 443        volumeMounts:        - name: tls-certs          mountPath: /etc/tls          readOnly: true        resources:          requests:            memory: "64Mi"            cpu: "250m"          limits:            memory: "128Mi"            cpu: "500m"      volumes:      - name: tls-certs        secret:          secretName: pod-label-mutator-tls---apiVersion: v1kind: Servicemetadata:  name: pod-label-mutator-svc  namespace: defaultspec:  selector:    app: pod-label-mutator  ports:  - port: 443    targetPort: 443
```

#### 配置验证证书

Kubernetes 对 TLS 证书的验证更严格，要求使用 SAN（Subject Alternative Names）扩展字段指定域名，而不是依赖传统的 Common Name (CN) 字段。现代 TLS 标准（如 RFC 6125）已逐渐弃用通过 CN 验证域名的方式，转而要求通过 SAN 字段验证，较新的 Kubernetes 版本会严格执行这一规范。

##### 步骤 1：创建 OpenSSL 配置文件（关键：添加 SAN）

创建一个 openssl.cnf 配置文件，明确指定 SAN 字段（包含 Webhook 服务的 DNS 名称）：

```
[req]default_bits = 2048prompt = nodefault_md = sha256distinguished_name = dnreq_extensions = req_ext[dn]C = CNST = BeijingL = BeijingO = exampleOU = webhookCN = pod-label-mutator-svc.default.svc  # 传统CN（可保留，但需确保与SAN一致）[req_ext]subjectAltName = DNS:pod-label-mutator-svc.default.svc  # 关键：添加SAN，指定服务的DNS名称
```

- SAN 必须包含 Webhook 服务的完整 DNS 名称（格式：service-name.namespace.svc），即 pod-label-mutator-svc.default.svc
- 若 Webhook 服务有其他访问域名，也需添加到 SAN 中（用逗号分隔，如 DNS:xxx,DNS:yyy）

##### 步骤 2：重新生成证书（CA + 服务器证书）

使用上述配置文件生成包含 SAN 的证书

```
# 1. 生成 CA 私钥和 CA 证书（用于签名服务器证书）openssl genrsa -out ca.key 2048openssl req -x509 -new -nodes -key ca.key -days 3650 -out ca.crt -subj "/CN=pod-label-mutator-ca"# 2. 生成 Webhook 服务器私钥openssl genrsa -out tls.key 2048# 3. 生成证书签名请求（CSR），使用上面的配置文件（包含SAN）openssl req -new -key tls.key -out tls.csr -config openssl.cnf# 4. 用 CA 证书签名生成服务器证书（包含SAN）openssl x509 -req -in tls.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out tls.crt -days 3650 -extensions req_ext -extfile openssl.cnf
```

##### 步骤 3：验证证书是否包含 SAN

生成证书后，检查 tls.crt 中是否包含 X509v3 Subject Alternative Name 字段

```
openssl x509 -in tls.crt -noout -text | grep -A 1 "Subject Alternative Name"若输出类似 DNS:pod-label-mutator-svc.default.svc，则表示 SAN 配置成功
```

##### 步骤 4：创建/更新 Secret

更新存储证书的 Secret（确保 Webhook 服务使用新证书）

```
# 先删除旧 Secret（若存在）kubectl delete secret pod-label-mutator-tls -n default# 创建新 Secret（包含新生成的 tls.crt 和 tls.key）kubectl create secret tls pod-label-mutator-tls -n default --cert=tls.crt --key=tls.key
```

##### 步骤 4：部署 deployment

```
kubectl apply -f deployment.yaml
```

### 配置 MutatingWebhookConfiguration

```
apiVersion: admissionregistration.k8s.io/v1kind: MutatingWebhookConfigurationmetadata:  name: pod-label-mutator-webhookwebhooks:- name: pod-label-mutator.default.svc  clientConfig:  # 修正拼写错误：将clientConfigxq改为clientConfig    service:      name: pod-label-mutator-svc      namespace: default      path: "/mutate"    caBundle: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURIekNDQWdlZ0F3SUJBZ0lVQ21zVFVSRmNnL0dlQ2ZOdG81a1hwaXpIbWdnd0RRWUpLb1pJaHZjTkFRRUwKQlFBd0h6RWRNQnNHQTFVRUF3d1VjRzlrTFd4aFltVnNMVzExZEdGMGIzSXRZMkV3SGhjTk1qVXdPREUxTURJMQpOVE16V2hjTk16VXdPREV6TURJMU5UTXpXakFmTVIwd0d3WURWUVFEREJSd2IyUXRiR0ZpWld3dGJYVjBZWFJ2CmNpMWpZVENDQVNJd0RRWUpLb1pJaHZjTkFRRUJCUUFEZ2dFUEFEQ0NBUW9DZ2dFQkFNQzVoci9Cck45U2tsVlcKQVBMVXAyVjZkb3ZBVENYUUZlbzFZSTk3Zk9vTWc1eVlOUEJQRGtQcmVIanhtYkhHcEQ4bG04RkJNdmprN3N5KwpkQUVmaE0wTGFXYTRVZHJueS9kS0xWOStmQXlqZ2QwRlRGWjQ1eEcrVWE4ekxJdE5SdVdnaG9ucXdPZ2hpaGJaCkJiakcrd2tDL2tpUDQ1eitQL2IzSG5kUVlEVFhLaGl5T1JPdUluZG91aXYxVjIvQ1RSckhuYjZYQlE2M0gzdzcKSlRGeHZzeXUreStleEE1MDg0Smp0YXFmUU9iazRvMjZsUTdHbUJuZTVDYU91ak9sSmMvdVZMT2NlRmRvTUt4eApmNkpoazZSL0l3U2NXdHYvV3lqazVPcU90VVZ6UzhjVVVOdUN4OUQ4cW9JTnJOVzV1bkNic1AzdUQ0NmpuczhwCktnMC90SE1DQXdFQUFhTlRNRkV3SFFZRFZSME9CQllFRkRMcWduSFBPVmF5VllPNitBTjNNMlpGODBiVk1COEcKQTFVZEl3UVlNQmFBRkRMcWduSFBPVmF5VllPNitBTjNNMlpGODBiVk1BOEdBMVVkRXdFQi93UUZNQU1CQWY4dwpEUVlKS29aSWh2Y05BUUVMQlFBRGdnRUJBRTUyMkFkZmJzdXJtM2w0c3Flai9vQ0dMMW1pZUhDSGJzd1l2NUd0CnlFa09YcldKdXBVSGg2VDhKWGdnaitGMzBwRlZaSHpRd2gvK2RBOENFRnVlMkJhNzc5K2M0bGZuRDJvMURSZmkKVjBQQmNkUU0rNHNyOU9tZGJxN0s3ZjU2QVB2ZXliVU1Xa2VmSzB0WVZ1UG9hUVZXcklRWmxDaUFRYndzNFFnbQp1UG9iSEVoYTZjcThlZGVBbUc1M2J5THY1UFlsM3QvTG84ZENLZFN5NkpqeGd5VkFtZUMzUitUc3lUMWk2a2x5CkJmVEdUbjBZY3JOU2dIc2c1eWdkTWxiVGNCbmhrZzZzNUZiaDlNbnJCS2FrZFZ6ZytXcXZJcllpc2hpOWsvT3cKVFJXTm9mS2c0UDZtZ3pwMnZubWRLaHZOeXlOYVNjRTdCdWVKNk5xSWdiV2VmL2M9Ci0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K  rules:  - operations: ["CREATE"]    apiGroups: [""]    apiVersions: ["v1"]    resources: ["pods"]  failurePolicy: Fail  sideEffects: None  admissionReviewVersions: ["v1"]  timeoutSeconds: 5
```

### 验证结果

```
# 部署一个 myapp deployment 进行尝试确认kubectl  create deployment myapp --image=docker.cnb.cool/wangyanglinux/docker-images-chrom/myapp:v4.0
```

**查看 pod 标签是否自动添加**

![](https://img.cloudmessage.top/i/2025/08/15/689eaef22d331.png)

---

![](https://img.cloudmessage.top/i/2025/08/15/689eaf28f1e78.png)