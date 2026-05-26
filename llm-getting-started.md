# Getting Started with LLM (大模型入门指南)

> 从零到大模型应用开发——分阶段路线图

---

## 一、先画地图：LLM 领域全景

```
                    LLM 生态
                       │
        ┌──────────────┼──────────────┐
        │              │              │
    理解原理        使用/调用        训练/微调
        │              │              │
  Transformer     API 调用       Fine-tuning
  Attention       Prompt 工程     LoRA/QLoRA
  Tokenization    RAG            RLHF/DPO
  Decoding        Agent          Pretraining
                                 (门槛极高)
```

**你的入口应该在哪？** 取决于目标：

| 目标 | 入口 | 时间 |
|------|------|------|
| 快速做应用 | **API 调用 + Prompt** | 1 天上手 |
| 搭建智能应用 | + RAG + Agent | 2-4 周 |
| 理解原理 | + Transformer 论文 | 1-2 月 |
| 微调模型 | + Fine-tuning | 1-3 月 |
| 从头训练 | + Pretraining（不推荐） | 数年 + 千万级预算 |

---

## 二、Day 1：用起来（1 天）

### 第一步：注册 API

```
国内：
  通义千问（阿里）→ dashscope.aliyun.com
  文心一言（百度）→ yiyan.baidu.com
  DeepSeek          → platform.deepseek.com  ← 性价比之王
  Moonshot (Kimi)   → platform.moonshot.cn

国外：
  OpenAI            → platform.openai.com
  Anthropic Claude  → console.anthropic.com
  Google Gemini     → ai.google.dev
```

### 第二步：第一行代码

```python
# pip install openai

from openai import OpenAI

# DeepSeek 示例（兼容 OpenAI SDK）
client = OpenAI(
    api_key="sk-xxx",  # 换成你的 key
    base_url="https://api.deepseek.com"
)

response = client.chat.completions.create(
    model="deepseek-chat",
    messages=[
        {"role": "system", "content": "你是一个MySQL DBA专家"},
        {"role": "user", "content": "什么是覆盖索引？"}
    ]
)

print(response.choices[0].message.content)
```

### 第三步：理解三个核心概念

```
1. Token → LLM 的计价单位，不是字符
   "hello world" ≈ 2 tokens
   "你好世界" ≈ 4 tokens（中文 token 更多）
   一个 token ≈ 0.75 个英文单词 ≈ 1.5 个中文字

2. Context Window（上下文窗口）
   GPT-4o: 128K tokens ≈ 一本书的厚度
   DeepSeek: 128K tokens
   上下文 = 系统提示 + 对话历史 + 你的输入

3. Temperature（温度）
   0.0 → 确定、保守（翻译/抽取）
   0.7 → 通用对话
   1.0 → 创造、发散（写作/头脑风暴）
```

---

## 三、Week 1-2：Prompt Engineering（提示工程）

**这是最重要的基础技能，比你想象的深得多。**

### 基础技巧

```python
# ❌ 差的 Prompt
"解释一下 MySQL 慢查询"

# ✅ 好的 Prompt
"""
你是一个有10年经验的MySQL DBA。用中文回答，面向初中级开发者。

请解释 MySQL 慢查询的以下方面：
1. 什么是慢查询（一句话定义）
2. 如何开启慢查询日志
3. 3 个最常见的原因
4. 每种原因对应 1 个优化方案

要求：每点不超过 3 行，给出实际可运行的 SQL 示例。
"""
```

### 进阶技巧

```python
# 1. Few-Shot（给示例）
prompt = """
任务：把非标准 SQL 改写成规范格式。

示例 1:
输入: select * from users where id=1
输出: SELECT * FROM users WHERE id = 1;

示例 2:
输入: SELECT name,age FROM u WHERE a>10
输出: SELECT name, age FROM u WHERE a > 10;

现在请改写：
输入: select id,name,email from orders where status='已发货'
输出:
"""

# 2. Chain of Thought（思维链）
prompt = """
问题：一个 MySQL 表有 500 万行数据，执行 COUNT(*) 需要 3 秒。
请逐步思考如何优化，每一步说明原因。

第 1 步：分析当前情况...
第 2 步：考虑优化方案...
第 3 步：实施建议...
"""

# 3. 角色扮演
system_prompt = """你是全球顶级 DBA，曾在阿里巴巴负责双11数据库保障。

回答风格：
- 先给结论，再解释原因
- 用实际生产案例说话
- 不确定的地方明确标注
- 涉及危险操作给出警告
"""
```

### Prompt 工程核心原则

```
1. 角色 + 背景 + 约束 → 效果最好
2. 给示例（Few-Shot）> 只描述
3. 要求逐步思考（CoT）> 直接要答案
4. 明确输出格式（Markdown/JSON/列表）
5. 说不该做什么 > 只说该做什么（负向约束）
```

---

## 四、Week 3-4：RAG 入门（检索增强生成）

**为什么学 RAG？** LLM 不知道你的私有数据，RAG 让它「读」你的文档再回答。

### 最小可用 RAG

```python
# pip install langchain langchain-openai chromadb

from langchain_openai import ChatOpenAI, OpenAIEmbeddings
from langchain_community.vectorstores import Chroma
from langchain_text_splitters import RecursiveCharacterTextSplitter
from langchain_core.prompts import ChatPromptTemplate
from langchain_core.output_parsers import StrOutputParser
from langchain_core.runnables import RunnablePassthrough

# === 配置 ===
llm = ChatOpenAI(
    api_key="sk-xxx", base_url="https://api.deepseek.com",
    model="deepseek-chat"
)
embeddings = OpenAIEmbeddings(
    api_key="sk-xxx", base_url="https://api.deepseek.com",
    model="text-embedding-3-small"
)

# === 1. 准备文档 ===
documents = [
    "MySQL InnoDB 缓冲池是内存中缓存数据和索引的区域，默认128MB",
    "慢查询日志通过 slow_query_log 参数开启，long_query_time 设置阈值",
    "覆盖索引是指查询字段全在索引中，不需要回表查聚簇索引",
    "EXPLAIN 命令用于查看 SQL 执行计划，type=ALL 表示全表扫描",
]

# === 2. 切割 + 向量化 ===
splitter = RecursiveCharacterTextSplitter(chunk_size=200, chunk_overlap=30)
chunks = splitter.create_documents(documents)
vectorstore = Chroma.from_documents(chunks, embeddings)
retriever = vectorstore.as_retriever(search_kwargs={"k": 2})

# === 3. RAG Chain ===
template = """根据以下上下文回答。不知道就说不知道。

上下文：{context}
问题：{question}"""

rag_chain = (
    {"context": retriever, "question": RunnablePassthrough()}
    | ChatPromptTemplate.from_template(template)
    | llm
    | StrOutputParser()
)

# === 4. 使用 ===
print(rag_chain.invoke("怎么优化慢查询？"))
```

### RAG 关键参数

| 参数 | 效果 | 建议 |
|------|------|------|
| chunk_size | 检索粒度 | 中文 300-500，英文 500-1000 |
| chunk_overlap | 语义连续性 | chunk_size 的 10-20% |
| k（检索条数）| 上下文丰富度 | 3-5 条，太多稀释关键信息 |
| embedding 模型 | 语义理解质量 | text-embedding-3-small 性价比最高 |

---

## 五、Month 2：深入原理（选学）

### 必读论文（按顺序）

```
1. Attention Is All You Need (2017)  ← 一切开始的地方
   → 理解 Self-Attention、QKV 机制

2. GPT-1 / GPT-2 / GPT-3 系列
   → 理解 Decoder-only 架构

3. InstructGPT (RLHF) / DPO
   → 理解对齐训练

4. LoRA: Low-Rank Adaptation
   → 理解高效微调
```

### 核心概念理解

```
Tokenization（分词）：
  "MySQL慢查询" → ["My", "SQL", "慢", "查询"]
  不同模型 tokenizer 不同 ← 这是 prompt 跨模型失灵的原因

Embedding（嵌入）：
  把文字映射为向量 → 语义相近的词向量距离近
  "MySQL" 和 "数据库" 的向量距离 << "MySQL" 和 "苹果" 的距离

Attention（注意力机制）：
  让模型知道"这句话里哪个词重要"
  Query: "我想查什么"
  Key:   "我这里有什么"
  Value: "实际内容"
  Query × Key → 哪些 Value 更重要
```

### 优质学习资源

| 资源 | 链接 | 难度 |
|------|------|------|
| Andrej Karpathy "Let's build GPT" | YouTube | ⭐⭐ |
| 李沐《动手学深度学习》 | d2l.ai | ⭐⭐⭐ |
| Jay Alammar 可视化博客 | jalammar.github.io | ⭐ |
| HuggingFace NLP Course | huggingface.co/learn | ⭐⭐ |

---

## 六、Month 2-3：Fine-tuning（微调）

```
什么时候需要微调？

✅ 模型输出风格不对（总是太啰嗦）
✅ 特定领域术语模型不会（DBA 说 "redo log" 它答 "重做日志" 但理解不到位）
✅ 需要固定格式输出（JSON Schema）
❌ 想让模型知道新事实 → 用 RAG，别用微调
```

### LoRA 微调（最实用的方式）

```python
# 用 LLaMA-Factory 或 Unsloth 快速微调
# pip install unsloth

from unsloth import FastLanguageModel
import torch

# 1. 加载模型（4bit 量化，12GB 显存就能跑）
model, tokenizer = FastLanguageModel.from_pretrained(
    model_name="unsloth/Qwen2-7B",
    max_seq_length=2048,
    load_in_4bit=True,  # 4bit 量化，省显存
)

# 2. 添加 LoRA 适配器（只训练 1% 参数）
model = FastLanguageModel.get_peft_model(
    model,
    r=16,  # LoRA 秩
    target_modules=["q_proj", "k_proj", "v_proj", "o_proj"],
)

# 3. 准备你的数据（JSONL 格式）
# {"instruction": "...", "input": "...", "output": "..."}

# 4. 训练
from trl import SFTTrainer
trainer = SFTTrainer(
    model=model,
    tokenizer=tokenizer,
    train_dataset=dataset,
    max_seq_length=2048,
)
trainer.train()

# 5. 保存
model.save_pretrained("./my-dba-model")
```

**微调最低门槛**：
- 1 张 RTX 3090/4090（24GB 显存）→ 7B 模型 LoRA
- 100+ 条高质量训练数据
- 1-2 小时训练时间

---

## 七、你的学习路线（DBA 方向）

```
Week 1     → 注册 DeepSeek API，跑通第一个问答
Week 2     → 系统学 Prompt Engineering（角色/CoT/Few-Shot）
Week 3-4   → 搭一个简单的 RAG：用 MySQL 手册做知识库问答
Week 5-6   → 学会 Agent：让 LLM 调用工具（查数据库/读日志）
Week 7-8   → 做一个完整项目：DBA 智能助手
              - 问 "库慢" → 自动查 processlist → 分析 → 给建议
Week 9+    → 深入原理（Transformer / 微调 按需）
```

---

## 八、选模型指南

### 按场景选

| 场景 | 推荐模型 | 原因 |
|------|---------|------|
| 日常开发/学习 | DeepSeek V3 | 极便宜，中文好 |
| 写代码 | Claude 3.5 Sonnet | 代码能力强 |
| 长文档分析 | Gemini 1.5 Pro | 100 万 token 上下文 |
| 多模态（图片） | GPT-4o | 图片理解最稳定 |
| 本地跑（免费） | Qwen2.5-7B | 国产开源最强 |
| 写 SQL 分析 | DeepSeek V3 | 性价比高 |

### 按成本选

```
多花钱（$20/M）：GPT-4o / Claude — 能力最强，最省心
少花钱（$2/M）：DeepSeek V3 — 日常够用，10元/月
不花钱：本地跑 Qwen2.5 + Ollama — 完全免费但需要 GPU
```

---

## 九、第一周行动计划

```
Day 1: 注册 API → 拿到 key → 跑通 "什么是覆盖索引？"
Day 2: 学 system/assistant/user 三种消息角色
Day 3: 写一个好的 Prompt（角色 + 约束 + 格式）
Day 4: 学 Few-Shot（给示例）
Day 5: 学 Chain of Thought（思维链）
Day 6: 做一个 DBA 知识点问答器
Day 7: 尝试 RAG（用 MySQL 官方文档当知识库）
```

---

## 十、一页纸速查

```
核心概念：
  Token      = LLM 计价单位（不是字符）
  Context    = 能塞多少信息（GPT-4o = 128K tokens）
  Embedding  = 文字→向量（RAG 的基础）
  RAG        = 检索 + 生成（让 LLM 读私有文档）
  Agent      = LLM + 工具（自主规划执行）
  LoRA       = 低成本微调（只改 1% 参数）

必会技能：
  1. API 调用（openai SDK）
  2. Prompt Engineering（角色+约束+示例）
  3. RAG（LangChain 最小链）
  4. Agent（Tool 定义 + 调度）

入门成本：
  API 费用：10-50 元/月（DeepSeek）
  不需要 GPU
  推荐用 Python
```
