# CUDA 使用指南 — 从入门到能跑

---

## 一、CUDA 是什么

**CUDA (Compute Unified Device Architecture)** 是 NVIDIA 的 GPU 并行计算平台。

```
CPU（主机端）                      GPU（设备端）
┌─────────────────┐           ┌───────────────────────┐
│  串行代码        │           │  SM  SM  SM  SM       │
│  ↓               │   launch  │  ├── 32个CUDA核心     │
│  kernel<<<>>>    │ ───────▶  │  ├── 32个CUDA核心     │
│  ↓               │           │  ├── ...              │
│  拷贝结果回来     │  ◀─────── │  └── 成千上万个线程   │
└─────────────────┘           └───────────────────────┘
```

**核心思想：** CPU 负责逻辑 + 调度，GPU 负责数据并行的大量计算。

---

## 二、CUDA 程序结构

### 一个最简单的例子：向量加法

```cpp
// vector_add.cu
#include <stdio.h>

// __global__ 标记：这是 GPU 上运行的函数（kernel）
__global__ void vector_add(float *a, float *b, float *c, int n) {
    // 计算当前线程应该处理哪个元素
    int idx = blockIdx.x * blockDim.x + threadIdx.x;
    if (idx < n) {
        c[idx] = a[idx] + b[idx];
    }
}

int main() {
    int n = 1 << 20;  // 1M 个元素

    // 1. 在 CPU 上分配内存
    float *a = (float*)malloc(n * sizeof(float));
    float *b = (float*)malloc(n * sizeof(float));
    float *c = (float*)malloc(n * sizeof(float));

    // 2. 初始化数据
    for (int i = 0; i < n; i++) {
        a[i] = 1.0f;
        b[i] = 2.0f;
    }

    // 3. 在 GPU 上分配显存
    float *d_a, *d_b, *d_c;
    cudaMalloc(&d_a, n * sizeof(float));
    cudaMalloc(&d_b, n * sizeof(float));
    cudaMalloc(&d_c, n * sizeof(float));

    // 4. 数据从 CPU → GPU
    cudaMemcpy(d_a, a, n * sizeof(float), cudaMemcpyHostToDevice);
    cudaMemcpy(d_b, b, n * sizeof(float), cudaMemcpyHostToDevice);

    // 5. 启动 kernel（GPU 执行！）
    int blockSize = 256;
    int gridSize  = (n + blockSize - 1) / blockSize;
    vector_add<<<gridSize, blockSize>>>(d_a, d_b, d_c, n);

    // 6. 结果从 GPU → CPU
    cudaMemcpy(c, d_c, n * sizeof(float), cudaMemcpyDeviceToHost);

    // 7. 验证
    printf("c[0] = %f, c[%d] = %f\n", c[0], n-1, c[n-1]);

    // 8. 释放
    cudaFree(d_a); cudaFree(d_b); cudaFree(d_c);
    free(a); free(b); free(c);
}
```

### 编译运行

```bash
# 用 nvcc 编译（NVIDIA CUDA Compiler）
nvcc -o vector_add vector_add.cu
./vector_add

# 输出: c[0] = 3.000000, c[1048575] = 3.000000
```

---

## 三、线程组织模型（核心概念）

```
Grid（网格）
├── Block 0
│   ├── Thread 0
│   ├── Thread 1
│   ├── Thread 2
│   └── ...
├── Block 1
│   ├── Thread 0
│   └── ...
└── Block 2
    └── ...
```

**关键：**

| 概念 | 含义 | 访问方式 | 典型大小 |
|------|------|----------|----------|
| Thread | 单个执行单元 | `threadIdx.x/y/z` | 1 |
| Block | 一组线程（在同一 SM 上） | `blockIdx.x/y/z`、`blockDim.x/y/z` | 128/256/512 |
| Grid | 所有 Block | `gridDim.x/y/z` | 成千上万个 |

**怎么算全局索引：**
```cpp
int idx = blockIdx.x * blockDim.x + threadIdx.x;
//        ↓ 第几个 block    ↓ 每个 block 多少线程  ↓ 当前 block 的第几个线程
```

**Warp（32 个线程为最小调度单元）：**
```
一个 Block 里的线程以 32 个为一组（Warp）执行，
同一个 Warp 里的所有线程执行同一条指令（SIMT）。
如果出现 if/else 分支，两个分支都执行，不走的线程被屏蔽。
这就是著名的 Warp Divergence 问题。
```

---

## 四、内存层次结构

```
Grid 全局
┌─────────────────────────────────────────────────────┐
│  Global Memory (HBM) — 最大（GB 级）但最慢（~500 周期） │
│                                                      │
│  ┌─────────────────────────────────────────────────┐│
│  │ L2 Cache — 所有 SM 共享                          ││
│  │  ┌──────────────┐ ┌──────────────┐              ││
│  │  │ SM 0          │ │ SM 1          │             ││
│  │  │ ┌───────────┐ │ │               │             ││
│  │  │ │ Shared Mem │ │ │  共享内存     │             ││
│  │  │ │ Block 内   │ │ │  Block 内    │             ││
│  │  │ │ 共享       │ │ │  线程共享    │             ││
│  │  │ └───────────┘ │ │               │             ││
│  │  │ Registers     │ │ Registers ── 最快！        ││
│  │  │ L1 Cache     │ │ L1 Cache                    ││
│  │  └──────────────┘ └──────────────┘              ││
│  └─────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────┘
```

| 内存类型 | 速度 | 大小 | 作用域 | 用途 |
|----------|------|------|--------|------|
| **Register** | 最快（1 周期） | 每线程 255 个 | 单线程 | 局部变量 |
| **Shared Memory** | 快（~20 周期） | 每 Block 48-96KB | Block 内 | Block 内线程协作 |
| **Constant Memory** | 快（缓存命中） | 64KB | Grid | 只读常量 |
| **Global Memory** | 慢（~500 周期） | GB 级 | 所有线程 | 大数据集 |
| **Local Memory** | 慢（溢出到全局内存） | - | 单线程 | 寄存器溢出时用 |

---

## 五、完整实战例子

### 例子 1：矩阵乘法（深度学习基石）

```cpp
// matmul.cu
#include <stdio.h>
#define TILE_SIZE 16  // 分块大小

__global__ void matmul(float *A, float *B, float *C, int N) {
    // 使用 Shared Memory 做分块优化
    __shared__ float As[TILE_SIZE][TILE_SIZE];
    __shared__ float Bs[TILE_SIZE][TILE_SIZE];

    int row = blockIdx.y * TILE_SIZE + threadIdx.y;
    int col = blockIdx.x * TILE_SIZE + threadIdx.x;
    float sum = 0.0f;

    // 分块遍历
    for (int t = 0; t < N / TILE_SIZE; t++) {
        // 每个线程从 Global Memory 加载一个元素到 Shared Memory
        As[threadIdx.y][threadIdx.x] = A[row * N + t * TILE_SIZE + threadIdx.x];
        Bs[threadIdx.y][threadIdx.x] = B[(t * TILE_SIZE + threadIdx.y) * N + col];
        __syncthreads();  // 等所有线程加载完

        // 计算当前块的部分积
        for (int k = 0; k < TILE_SIZE; k++) {
            sum += As[threadIdx.y][k] * Bs[k][threadIdx.x];
        }
        __syncthreads();  // 等所有线程用完再加载下一块
    }
    C[row * N + col] = sum;
}
```

**为什么用 Shared Memory？**
- Global Memory 访问一次 ~500 周期
- Shared Memory 访问一次 ~20 周期
- 把常用的数据块从 Global 搬到 Shared → 快 25 倍

---

### 例子 2：Softmax（深度学习算子）

```cpp
__global__ void softmax(float *input, float *output, int n) {
    int idx = blockIdx.x * blockDim.x + threadIdx.x;

    if (idx < n) {
        // 1. 找最大值（为了数值稳定）
        float max_val = input[0];
        for (int i = 1; i < n; i++) {
            if (input[i] > max_val) max_val = input[i];
        }

        // 2. exp(xi - max) + sum
        float sum = 0.0f;
        for (int i = 0; i < n; i++) {
            sum += expf(input[i] - max_val);
        }

        // 3. exp(xi - max) / sum
        output[idx] = expf(input[idx] - max_val) / sum;
    }
}
```

---

## 六、错误处理（一定要写！）

```cpp
#define CUDA_CHECK(err) \
    if (err != cudaSuccess) { \
        fprintf(stderr, "CUDA error: %s at %s:%d\n", \
                cudaGetErrorString(err), __FILE__, __LINE__); \
        exit(1); \
    }

cudaError_t err = cudaMalloc(&d_a, n * sizeof(float));
CUDA_CHECK(err);

// 或者直接包一层
cudaMalloc(&d_a, n * sizeof(float));
cudaDeviceSynchronize();  // 等 GPU 跑完
CUDA_CHECK(cudaGetLastError());
```

**kernel 启动后必须检查错误：**
```cpp
kernel<<<grid, block>>>(args);
CUDA_CHECK(cudaGetLastError());        // 检查启动错误
CUDA_CHECK(cudaDeviceSynchronize());   // 等执行完 + 检查运行时错误
```

---

## 七、CUDA 开发环境搭建

```bash
# 1. 确认有 NVIDIA GPU
lspci | grep -i nvidia

# 2. 装驱动 + CUDA Toolkit
# Ubuntu 上
wget https://developer.download.nvidia.com/compute/cuda/repos/ubuntu2204/x86_64/cuda-keyring_1.1-1_all.deb
sudo dpkg -i cuda-keyring_1.1-1_all.deb
sudo apt update
sudo apt install cuda-toolkit-12

# 3. 验证
nvcc --version
nvidia-smi
# +-----------------------------------------------------------------------------+
# | NVIDIA-SMI 545.23.08    Driver Version: 545.23.08    CUDA Version: 12.3     |
# +-----------------------------------------------------------------------------+

# 4. 编译示例
nvcc -o test test.cu && ./test
```

---

## 八、CUDA 在 Python 中的使用

### CuPy（NumPy 复刻，零学习成本）

```python
import cupy as cp

# 完全跟 NumPy 一样
a = cp.random.randn(1000, 1000)
b = cp.random.randn(1000, 1000)
c = a @ b                    # GPU 矩阵乘法
d = cp.sum(c, axis=1)        # GPU 求和
result = cp.asnumpy(d)       # GPU → CPU（转回 NumPy）
```

### Numba（给 Python 函数加 `@cuda.jit`）

```python
from numba import cuda
import numpy as np

@cuda.jit
def add_kernel(a, b, c):
    idx = cuda.grid(1)
    if idx < a.size:
        c[idx] = a[idx] + b[idx]

n = 1_000_000
a = np.ones(n, dtype=np.float32)
b = np.full(n, 2.0, dtype=np.float32)
c = np.zeros(n, dtype=np.float32)

# 拷贝到 GPU
d_a = cuda.to_device(a)
d_b = cuda.to_device(b)
d_c = cuda.to_device(c)

# 启动 kernel
threads_per_block = 256
blocks_per_grid = (n + threads_per_block - 1) // threads_per_block
add_kernel[blocks_per_grid, threads_per_block](d_a, d_b, d_c)

# 拷贝回来
result = d_c.copy_to_host()
print(result[:5])  # [3. 3. 3. 3. 3.]
```

### PyTorch（研究/深度学习标配）

```python
import torch

# 张量放 GPU
x = torch.randn(1000, 1000, device='cuda')
y = torch.randn(1000, 1000, device='cuda')
z = x @ y                     # GPU 上执行

# 只写 Python，PyTorch 底层调 CUDA
```

---

## 九、常用命令速查

```bash
# 看 GPU 状态
nvidia-smi                     # GPU 占用 / 温度 / 功耗
nvidia-smi -l 1                # 每秒刷新一次

# 看 GPU 详细信息
nvidia-smi -q                  # 完整信息

# 看 CUDA 版本
nvcc --version
cat /usr/local/cuda/version.json

# 编译
nvcc -o app app.cu             # 基础编译
nvcc -arch=sm_80 -o app app.cu # 指定 GPU 架构算力（A100=sm_80）
nvcc -g -G -o app app.cu       # Debug 模式

# 性能分析
nvprof ./app                    # 老工具
nsys profile ./app              # 新工具 (Nsight Systems)
ncu ./app                       # 详细 kernel 分析 (Nsight Compute)
```

---

## 十、常见问题

| 问题 | 原因 | 解决 |
|------|------|------|
| `out of memory` | 申请的显存超过 GPU 容量 | 减小 batch / 用 `cudaMallocManaged` |
| `an illegal memory access` | 数组越界访问 | 加 `if (idx < n)` 边界判断 |
| `invalid device function` | 编译的架构不对 | `nvcc -arch=sm_XX` 匹配你的 GPU |
| kernel 卡死 | 无限循环 / 死锁 | Ctrl+C，检查 `__syncthreads()` 位置 |
| CPU 等 GPU 太久 | Global Memory 访问太频繁 | 用 Shared Memory / 合并访问 |

---

## 十一、学习路线建议

```
第1天: 理解线程模型（Grid/Block/Thread）+ 写一个 Vector Add
第2天: 理解内存层次（Global/Shared/Register）+ Shared Memory 优化
第3天: 矩阵乘法（tiled matmul）—— CUDA 的 Hello World
第4天: Reduction（并行求和）—— 理解 Warp 和同步
第5天: 用 nvprof/ncu 分析性能瓶颈
第6天: 进阶：CUDA Streams（多流并行）、Unified Memory
第7天: 实战：用 CuPy / PyTorch CUDA Extension 写一个自定义算子
```

---

📝 **一句话总结：** CPU 分配内存 → cudaMalloc 分显存 → cudaMemcpy 拷数据 → kernel<<<grid, block>>> 启动 GPU 算 → cudaMemcpy 拷回来 → cudaFree 释放。
