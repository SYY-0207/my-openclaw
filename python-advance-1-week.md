# Python 高阶学习计划 — 一周突击

## 🗓️ 一周学习计划

### Day 1 · 函数与对象的深层机制

| 主题 | 要点 |
|------|------|
| 闭包与作用域 | `nonlocal`、自由变量、`__closure__` |
| 装饰器进阶 | 带参数装饰器、类装饰器、`functools.wraps`、装饰器栈 |
| 可调用对象 | `__call__`、函数是一等公民 |
| `functools` | `lru_cache`、`partial`、`singledispatch` |

🎯 **今日练习**：写一个 `@retry(max_attempts=3, delay=1)` 装饰器；用一个类装饰器实现单例。

---

### Day 2 · 迭代器 / 生成器 / 协程基础

| 主题 | 要点 |
|------|------|
| 迭代协议 | `__iter__` / `__next__`、`iter()` |
| 生成器 | `yield`、`yield from`、生成器表达式 |
| `itertools` | `chain` `tee` `groupby` `islice` `product` `permutations` |
| `send()` / `throw()` / `close()` | 双向通信、协程雏形 |

🎯 **今日练习**：手写一个惰性分页迭代器；用 `yield from` 实现二叉树中序遍历。

---

### Day 3 · 异步编程全栈

| 主题 | 要点 |
|------|------|
| 事件循环 | `asyncio.get_event_loop()`、`run_until_complete` |
| `async/await` | 协程、`Task`、`Future` |
| `asyncio.gather` vs `asyncio.wait` | 并发执行、超时、取消 |
| 异步上下文管理器 | `__aenter__` / `__aexit__` |
| 生产者-消费者 | `asyncio.Queue` |

🎯 **今日练习**：用 asyncio + aiohttp 写一个并发爬虫（限并发数）；实现异步连接池。

---

### Day 4 · 元类 / 描述符 / 对象模型

| 主题 | 要点 |
|------|------|
| `type` | 动态创建类、`type(name, bases, dict)` |
| 描述符 | `__get__` `__set__` `__delete__`、数据描述符 vs 非数据描述符 |
| `__init_subclass__` | Python 3.6+ 的新钩子 |
| 元类 | `__new__` `__prepare__` |
| 属性查找顺序 | `__getattribute__` → 数据描述符 → 实例 `__dict__` → 非数据描述符 → `__getattr__` |

🎯 **今日练习**：用描述符实现 Django 风格的 ORM Field（类型校验）；写一个 `@validate_types` 装饰器用元类校验方法参数类型。

---

### Day 5 · 并发 · 内存 · 性能

| 主题 | 要点 |
|------|------|
| GIL | 是什么、什么时候是瓶颈、如何绕过 |
| `threading` vs `multiprocessing` | IO 密集用线程、CPU 密集用进程 |
| 内存管理 | 引用计数、gc 模块、循环引用、`weakref`、`__slots__` |
| 性能分析 | `cProfile`、`line_profiler`、`memory_profiler` |
| C 扩展入门 | Cython 写一个纯 Python 函数的加速版 |

🎯 **今日练习**：用 `multiprocessing.Pool` 并行计算大矩阵；用 `__slots__` 减少大对象内存；跑 cProfile 找自己代码的热点。

---

### Day 6 · 类型系统与设计模式

| 主题 | 要点 |
|------|------|
| `typing` 进阶 | `Protocol`、`Literal`、`TypeVar`、`Generic`、`overload` |
| dataclass | 替代样板代码、`field(default_factory=...)` |
| `match/case` | Python 3.10+ 的模式匹配 |
| 设计模式 Python 化 | 策略、观察者、抽象工厂——在 Python 里可以写得很骚 |
| `__new__` vs `__init__` | 单例、不可变类型子类化 |

🎯 **今日练习**：给一个项目加完整类型标注并通过 `mypy --strict`；用 `match/case` 写一个简单解释器。

---

### Day 7 · 综合实战

| 主题 | 要点 |
|------|------|
| 打包发布 | `pyproject.toml`、`setuptools`、发布到 PyPI |
| 测试 | `pytest`、fixture、mock、`pytest-asyncio` |
| 阅读 CPython 源码 | 从 `listobject.c` 或 `dictobject.c` 入手 |
| 社区资源 | **Real Python**、**Fluent Python**、**Python Cookbook** |

🎯 **今日练习**：把本周写的代码打包成一个发布包；给核心模块写完整单元测试。

---

## 📚 推荐资料

| 资料 | 说明 |
|------|------|
| **《Fluent Python》第2版** | Python 进阶圣经，从头讲到尾，必读 |
| **《Python Cookbook》第3版** | David Beazley，实例驱动，进阶必看 |
| **《CPython Internals》** | 想了解解释器底层看这个 |
| **Real Python** (realpython.com) | 免费教程质量极高，逐篇看 |
| **Python.org Asyncio 官方文档** | 异步编程最好的参考资料就是官档 |
| **David Beazley 的 PyCon 演讲** | YouTube 搜 "David Beazley generators"、"David Beazley GIL"，看完原地升天 |
| **Trey Hunner** (treyhunner.com) | 大量关于 Python 语法糖和机制的深度文章 |
| **pymotw.com** | Python Module of the Week，逐个模块精讲 |

---

## ⚡ 建议节奏

- 每天 3-4 小时，**上午学理论 + 看源码，下午写练习**
- 每道练习都**自己敲**，不要复制粘贴
- Day 7 的实战争取覆盖本周所有知识点
- 把代码推 GitHub，学完一周有东西可看
