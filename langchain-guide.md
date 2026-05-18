# LangChain 全解 + 学习路线

> LLM 应用开发框架——把大模型、工具、记忆、链式调用组装成可用的应用

---

## 一、LangChain 是什么

把大语言模型（LLM）和各种外部资源（数据库、API、文件、搜索引擎）串起来的 Python/JavaScript 框架。

```
传统：prompt → LLM → 回答（一次性）

LangChain：
  用户问题 → Chain
              ├── 查知识库（RAG）      → 检索相关文档
              ├── 调用工具（Agent）     → 搜索/查数据库/调 API
              ├── 记忆（Memory）       → 记住对话上下文
              └── LLM 推理/生成        → 最终回答
```

| 问题 | LangChain 怎么解决 |
|------|-------------------|
| LLM 不知道私有数据 | **RAG**：检索 + 增强生成 |
| LLM 知识有截止日期 | **Tools**：让 LLM 调用搜索引擎/数据库 |
| 单次调用不够 | **Chains**：多步骤流水线 |
| 没有记忆 | **Memory**：对话历史/摘要 |
| 输出不稳定 | **Output Parsers**：结构化输出 |

---

## 二、核心概念

### 1. Model（模型）

```python
from langchain_openai import ChatOpenAI

llm = ChatOpenAI(model="gpt-4o", temperature=0.7)
response = llm.invoke("什么是数据库索引？")
print(response.content)
```

支持：OpenAI / Claude / Gemini / 通义千问 / DeepSeek / 本地 Ollama

### 2. Prompt Template（提示模板）

```python
from langchain_core.prompts import ChatPromptTemplate

prompt = ChatPromptTemplate.from_messages([
    ("system", "你是一个{role}，擅长{skill}。用{lang}回答问题。"),
    ("human", "{question}"),
])

messages = prompt.format_messages(
    role="DBA专家", skill="MySQL优化", lang="中文",
    question="慢查询怎么排查？"
)
```

### 3. Chain（链）⭐ 核心

用 `|` 管道符（LCEL）串联步骤：

```python
from langchain_core.output_parsers import StrOutputParser

chain = prompt | llm | StrOutputParser()
result = chain.invoke({
    "role": "DBA专家", "skill": "MySQL优化",
    "lang": "中文", "question": "EXPLAIN 怎么看？"
})
```

### 4. Memory（记忆）

```python
from langchain.memory import ConversationBufferMemory
from langchain.chains import ConversationChain

memory = ConversationBufferMemory(return_messages=True)
conversation = ConversationChain(llm=llm, memory=memory)

conversation.predict(input="我叫张三，是DBA")
conversation.predict(input="我叫什么？")   # "张三"
conversation.predict(input="我是做什么的？") # "DBA"
```

| 记忆类型 | 原理 | 适用 |
|---------|------|------|
| BufferMemory | 存全部对话 | 短对话 |
| BufferWindowMemory | 只存最近 K 轮 | 中等 |
| SummaryMemory | LLM 摘要历史 | 长对话 |
| TokenBufferMemory | 按 token 截断 | 控制成本 |
| VectorStoreMemory | 向量检索历史 | 海量对话 |

### 5. RAG（检索增强生成）🔥 最重要

```
用户提问 → 向量化 → 检索 Top-K 相关文档 → 拼入 Prompt → LLM 回答
```

```python
from langchain_community.document_loaders import TextLoader
from langchain_text_splitters import RecursiveCharacterTextSplitter
from langchain_openai import OpenAIEmbeddings
from langchain_community.vectorstores import Chroma

# 1. 加载文档
loader = TextLoader("mysql_manual.txt")
documents = loader.load()

# 2. 切割文本
text_splitter = RecursiveCharacterTextSplitter(
    chunk_size=500, chunk_overlap=50
)
chunks = text_splitter.split_documents(documents)

# 3. 向量化 + 存向量数据库
embeddings = OpenAIEmbeddings()
vectorstore = Chroma.from_documents(chunks, embeddings)
retriever = vectorstore.as_retriever(search_kwargs={"k": 3})

# 4. RAG Chain
template = """根据以下上下文回答问题。不知道就说不知道。

上下文：{context}
问题：{question}
"""
prompt = ChatPromptTemplate.from_template(template)

rag_chain = (
    {"context": retriever, "question": RunnablePassthrough()}
    | prompt | llm | StrOutputParser()
)

answer = rag_chain.invoke("InnoDB 缓冲池怎么调优？")
print(answer)
```

### 6. Agent（智能体）🔥🔥

LLM 自主规划 + 调用工具，循环直到完成：

```python
from langchain.agents import create_tool_calling_agent, AgentExecutor
from langchain_core.tools import tool

@tool
def read_file(filepath: str) -> str:
    """读取服务器上的文件内容"""
    with open(filepath) as f:
        return f.read()

@tool
def analyze_slow_log(content: str) -> str:
    """分析慢查询日志"""
    return f"发现 5 个慢查询"

@tool
def suggest_index(sql: str) -> str:
    """为 SQL 推荐索引"""
    return "建议添加联合索引 idx_col1_col2"

tools = [read_file, analyze_slow_log, suggest_index]

agent = create_tool_calling_agent(llm, tools, prompt)
agent_executor = AgentExecutor(
    agent=agent, tools=tools, verbose=True, max_iterations=5
)

result = agent_executor.invoke({
    "input": "分析 /var/log/mysql/slow.log，给出优化建议"
})
```

### 7. Output Parser（结构化输出）

```python
from langchain_core.output_parsers import PydanticOutputParser
from pydantic import BaseModel, Field

class SlowQueryAnalysis(BaseModel):
    sql: str = Field(description="慢SQL语句")
    time_ms: int = Field(description="执行时间(毫秒)")
    problem: str = Field(description="问题分析")
    suggestion: str = Field(description="优化建议")
    index_ddl: str = Field(description="建议的DDL")

parser = PydanticOutputParser(output_schema=SlowQueryAnalysis)
chain = prompt | llm | parser
result = chain.invoke({"slow_sql": "SELECT * FROM ..."})
print(f"建议索引: {result.index_ddl}")
```

---

## 三、学习路线

### 阶段 1：基础（1-2 周）

```
目标：能写带记忆的对话机器人

Day 1-2：环境 + Hello World
Day 3-4：Prompt 模板（system/human/ai 消息）
Day 5-6：Chain + LCEL 管道符
Day 7-8：Memory 记忆
Day 9-10：小项目——DBA 问答机器人
```

### 阶段 2：RAG（2-3 周）⭐ 最重要

```
Week 1：文档处理
  - 加载：PDF/TXT/CSV/网页
  - 切割：chunk_size 调优
  - 向量化：OpenAI / sentence-transformers
  - 存储：Chroma / FAISS / Milvus

Week 2：检索优化
  - 多种检索器：similarity / MMR / self-query
  - 重排序（Reranker）
  - 多路召回 + 融合

Week 3：生产化
  - 引用来源（show source）
  - 流式输出（Streaming）
  - 评估（RAGAS 评测）
```

### 阶段 3：Agent（2-3 周）

```
1. 定义 Tool（@tool 装饰器）
2. create_tool_calling_agent
3. AgentExecutor 控制执行流程
4. 多 Agent 协作
5. 理解 ReAct 模式（Reasoning + Acting）
```

### 阶段 4：高级

```
- LangGraph：状态机驱动的 Agent 工作流
- LangSmith：调试、测试、监控
- LangServe：部署为 REST API
- 多模态：图片、音频
- 本地模型：Ollama + Llama
```

---

## 四、框架对比

| 框架 | 定位 | 优点 | 缺点 |
|------|------|------|------|
| **LangChain** | 全栈框架 | 生态全、资料多 | 抽象多、版本快 |
| **LlamaIndex** | 数据+RAG | RAG 强 | Agent 弱 |
| **Dify** | 低代码 | 可视化、开箱即用 | 灵活性差 |
| **CrewAI** | 多Agent | 角色分工 | 不稳定 |
| **直接调 API** | 零依赖 | 无学习开销 | 功能需手写 |

---

## 五、实战项目建议

**入门（1周）**：MySQL 慢查询分析助手
- 输入慢 SQL → LLM 分析 → 输出优化建议

**进阶（2-3周）**：公司文档问答（RAG）
- 吃进 MySQL 手册 PDF → 用户提问 → 检索 + 回答 + 引用

**高级（4+周）**：DBA 运维 Agent
- 工具：查数据库状态 / 读慢查询日志 / 执行诊断脚本
- Agent 自主判断：CPU 高 → 查 processlist → 发现慢 SQL → 建议优化

---

## 六、常见坑 + 速查

```python
# 1. 基本调用
llm = ChatOpenAI(model="gpt-4o")
result = llm.invoke("问题")

# 2. Chain
chain = prompt | llm | StrOutputParser()
chain.invoke({"topic": "慢查询"})

# 3. 流式输出
for chunk in chain.stream({"topic": "MySQL索引"}):
    print(chunk, end="", flush=True)

# 4. 批处理
results = chain.batch([{"topic": "B+树"}, {"topic": "LSM树"}])

# 5. 工具
@tool
def my_tool(param: str) -> str:
    """工具描述（LLM 据此判断何时调用）"""
    return f"结果: {param}"

# 6. 并行
from langchain_core.runnables import RunnableParallel
chain = RunnableParallel(analysis=a_chain, summary=s_chain)
```

**避坑**：
1. 用稳定版，别追新
2. Prompt 比代码重要
3. RAG chunk_size 很关键（太小丢上下文，太大检索不准）
4. Agent 不可靠，加兜底逻辑
5. Token 费钱，注意 max_tokens
6. 向量模型推荐 text-embedding-3-small

---

## 资源

- 官方文档：https://python.langchain.com
- LangSmith：https://smith.langchain.com（调试/监控）
- 吴恩达 LangChain 课程（DeepLearning.AI）⭐⭐⭐
- GitHub：langchain-ai/langchain, gkamradt/langchain-tutorials
