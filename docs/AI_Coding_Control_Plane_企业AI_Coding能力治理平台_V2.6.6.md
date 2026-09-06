# AI Coding Control Plane｜企业 AI Coding 能力治理平台 V2.6.6 立项冻结版

---

# 一、产品定位

## 1.1 统一品类名称

英文统一定位：

> **AI Coding Control Plane**

中文统一定位：

> **企业 AI Coding 能力治理平台**

对外完整描述：

> **Open-source AI Coding Control Plane for engineering teams.**

中文描述：

> **面向研发团队，统一治理和分发模型、工具、知识、Skill、权限、用量与成本的企业 AI Coding 控制面。**

`Control Plane` 表示平台不取代 Codex、Claude Code 等客户端，而是作为它们之上的企业统一控制与能力分发层。

---

## 1.2 产品职责

平台统一治理和分发四类企业 AI 能力：

```text
1. Model
   Claude / GPT / Gemini / DeepSeek 等官方模型

2. Tool
   企业统一提供的 Managed MCP 工具，例如 Gemini 生图、搜索等

3. Knowledge
   企业内部研发文档、规范、业务说明等知识

4. Skill
   企业内部可复用的工作方法、规范、流程和任务模板
```

同时统一管理：

```text
Principal / Group
Access Key
AI Resource
Permission
Usage
Cost
Health
Operation Log
```

---

## 1.3 用户工作方式

员工继续使用自己熟悉的客户端：

```text
Codex
Claude Code
其他支持的 AI Coding Client
```

平台不要求员工迁移到新的聊天界面。

整体关系：

```text
Codex / Claude Code
        ↓
AI Coding Control Plane
        ↓
模型 + 工具 + 知识 + Skill
```

管理员通过 Control Plane 完成企业能力配置。员工以 MEMBER Principal 通过 Virtual Key 使用授权能力；企业内部应用以 APPLICATION Principal 通过 App Key 使用同一套标准 Gateway API。两类入口共享底层治理能力。

---

## 1.4 Governance + Enablement

平台同时承担两类价值：

### Governance

```text
权限
Resource
安全
Usage
Cost
Health
Operation Log
```

### Enablement

```text
模型分发
Managed MCP
企业 Knowledge
企业 Skill
```

因此产品不仅负责“管住 AI Coding”，也负责“把企业允许使用的 AI 能力交付给员工”。

---

## 1.5 国内与国际市场统一定位

中文市场统一使用：

> **企业 AI Coding 能力治理平台**

英文市场统一使用：

> **AI Coding Control Plane**

GitHub / 官网英文描述：

> **Open-source AI Coding Control Plane for engineering teams. Govern and distribute models, tools, skills and knowledge across Codex, Claude Code and other AI coding clients.**

中文描述：

> **面向研发团队的开源 AI Coding Control Plane，统一治理并分发模型、工具、Skill、企业知识、权限、用量与成本。**

---

## 1.6 市场定位与商业路径

### 市场定位

模型 Gateway 已经是成熟且竞争充分的基础设施能力。AI Coding Control Plane 不以“重新实现一个通用 AI Gateway”为核心差异，而是聚焦：

> **企业研发人员如何在 Codex、Claude Code 等 AI Coding Client 中，统一、安全、可控地获得公司的模型、工具、知识和 Skill。**

产品核心关系：

```text
企业 AI Resource
+
Managed MCP
+
Enterprise Knowledge
+
Enterprise Skill
        ↓
AI Coding Control Plane
        ↓
Codex / Claude Code / Other Coding Clients
        ↓
Principal / Group / Permission / Usage / Cost / Operation Log
```

### 产品差异化

通用 AI Gateway 的主要关注点是：

```text
AI App / Agent
→ Model
```

AI Coding Control Plane 的主要关注点是：

```text
Enterprise
→ Developer
→ AI Coding Client
→ Model / Tool / Knowledge / Skill
```

核心差异集中在：

```text
Client-native AI Coding 适配
员工与 Group 治理
企业 AI Resource 分发
企业 Knowledge 进入 Coding Workflow
企业 Skill 的统一发布与同步
Managed MCP Tool 的 Credential / Permission / Cost 治理
研发人员维度的 Usage / Cost
```

Gateway 是基础设施层，研发人员 AI 能力治理与分发是产品层。

### 竞争定位

| 维度 | 通用 AI Gateway / AI Infrastructure | AI Coding Control Plane |
|---|---|---|
| 核心对象 | AI Application / Agent / API Traffic | 企业研发人员与 AI Coding Client |
| 主要入口 | API / SDK / Agent Framework | Codex / Claude Code 等 Coding Client |
| 模型治理 | 支持 | 支持 |
| Principal / Group | 通用 Team / Org 治理 | 面向企业员工与内部应用的统一治理主体设计 |
| MCP | 通用 MCP Gateway / Tool Governance | 面向 Coding Client 分发受管工具 |
| Knowledge | 通用 Context / RAG 能力 | 企业研发知识直接进入 Coding Workflow |
| Skill | Agent / Workflow 能力 | 企业 Coding Skill Registry、版本与分发 |
| 成本 | Key / Team / Org Spend | Principal / Group / Resource / Model / Tool 成本归属 |
| 核心目标 | 统一 AI 基础设施 | 统一企业研发人员的 AI Coding 能力 |

产品不依赖某一个单一功能形成差异，而依靠：

> **AI Coding Client + 企业人员治理 + Model / Tool / Knowledge / Skill 统一分发**

形成整体产品定位。

### 目标客户

优先面向已经出现以下特征的研发组织：

```text
已经实际使用 Codex / Claude Code 等 AI Coding Client
存在两个或以上 AI Provider / Model 资源
企业 API Key 分散在不同成员或项目中
开始关注 AI Coding Usage / Cost
存在统一 Knowledge / Skill / MCP Tool 的需求
希望保持员工现有 Coding Client，不重新建设一套 AI 工作界面
```

相比团队规模，更重要的判断标准是：

> **AI Coding 已经从个人工具使用进入团队级资源配置与治理阶段。**

### 开源与商业路径

采用：

> **Open Core + Enterprise Service**

Community Edition 提供可独立使用的核心能力：

```text
Model Gateway
Model Catalog / Provider / Provider Model / Resource
Access Key
Principal（MEMBER / APPLICATION）/ Group 基础治理
Usage / Cost
基础 Resource Router
基础 Knowledge Search
Skill Registry 基础能力
```

Community Edition 的基础原则是：

> **不付费也必须能够让真实研发团队完整、流畅地使用核心治理能力。**

为降低首次部署和接入门槛，Community Edition 内置主流 Provider 与 Model Catalog 模板，包括但不限于 OpenAI、Anthropic、Google、DeepSeek、Qwen 等常见模型体系。预置内容只提供 Provider 元数据、默认协议、默认 Base URL、逻辑 Model 与 Provider Model 映射等初始化信息，**不预置任何真实 Credential / API Key**。

管理员安装后原则上只需要补充企业自己的 Resource Credential、测试连接并启用，即可开始使用。预置模板不构成能力限制，管理员仍可动态新增、修改、停用 Provider / Model / Provider Model。

Enterprise 面向组织级生产环境增加：

```text
SSO / OIDC / LDAP / SCIM
KMS / Vault
高级 Audit / Compliance Export
复杂组织与权限治理
高可用 / 多节点 / 多组织
企业 Knowledge Connector
高级 Skill / MCP Governance
企业部署与升级保障
SLA / Dedicated Support
```

SSO / OIDC 等企业准入能力根据实际客户采购要求进入对应 Enterprise Release，不与 Community 功能开发阶段强绑定。

### 企业预算治理

Control Plane 核心治理能力不依赖充值、支付或余额体系。P0 / Community 核心路径以企业自有 Resource（BYOK）和内部成本治理为主，不实现平台资金账户。

未来当平台作为正规 AI Provider 代理商、合作渠道或自营 Provider 向企业提供模型调用能力时，可独立增加 Provider 商业计费模块，包括 Organization Account、Prepaid Balance、Rate Card、Billing Ledger 与 Balance Ledger。该模块属于商业扩展，不改变 Model / Provider / Resource / Principal / Usage / Operation Log 的核心治理关系。

企业内部预算治理采用：

```text
Budget
    ↓
Usage
    ↓
Cost Attribution
    ↓
Review / Adjustment
```

管理员可以为：

```text
Principal
Group
Project / Cost Center（扩展）
```

设置预算或使用上限。

平台解决的是：

> **企业内部 AI 资源预算如何被分配、使用、统计和复盘。**

### 商业收入

主要收入来源：

```text
Enterprise License
私有化部署与实施
企业系统集成
年度技术保障与 SLA
Managed Cloud
Knowledge / Skill / MCP 企业治理增强
企业 Connector / Custom Provider
正规 AI Provider 渠道合作 / 自营 Provider 服务（可选商业扩展）
```

### 一句话定位

> **AI Coding Control Plane 是企业研发团队的 AI Coding 控制面：在不改变员工 Codex、Claude Code 等工作习惯的前提下，统一治理并分发模型、工具、知识、Skill、权限、用量与成本。**

## 1.7 文档定位与开发基线

V2.6.6 明确区分两层内容：

```text
Target Architecture / Product Architecture
→ 描述平台长期稳定的产品边界、核心数据模型和技术接缝

Current Development Baseline
→ 描述当前阶段真正进入开发 Backlog 的最小范围
```

因此，本方案中关于 Knowledge、Skill、Managed MCP、Enterprise、Billing、多 Provider、复杂 Router、Kubernetes 等内容属于**目标架构与后续阶段设计**，不表示需要在首个可运行版本中同步实现。

开发统一遵循：

> **范围做 MVP，地基保持生产级。**

具体含义：

1. 优先砍未验证的业务功能，不为了 MVP 牺牲 Credential 安全、Access Key 安全、数据库 Migration、日志、Request ID、Graceful Shutdown 等低成本高返工价值的基础能力。
2. 保留 Model / Provider / Resource / Principal 等核心架构接缝，但 P0-MVP 只实现当前验证链路实际需要的最小行为。
3. DDD 只用于控制模块边界，不引入与当前业务无关的复杂 Aggregate、Specification、Event Bus 或过度 Interface 抽象。
4. 国际化保持结构可扩展，但 P0-MVP 不以完整中英文翻译、国际官网或完整开源文档作为验收条件。
5. 当前最大不确定性是“真实研发团队是否持续需要 Principal / Group 级 AI Coding 治理”，因此首要目标是尽快进入真实使用，而不是继续扩展未验证功能。
6. V2.6.6 之后，新的实现决策优先由真实代码、真实 Claude Code / Codex 行为、内部团队使用和首批用户反馈驱动。

---

# 二、核心设计原则

## 2.1 Client-native First

优先利用 Codex、Claude Code 等客户端自身已有能力。

客户端负责：

```text
模型选择
会话管理
上下文管理
Tool Call
MCP Client
Skill 执行
用户交互
```

平台负责：

```text
Access Key 鉴权
Principal / Resource 权限
Model → Provider
Resource 选择
必要的协议转换
Streaming 转发
Usage / Cost
企业 MCP Tool
企业 Skill 分发
企业 Knowledge Retrieval
```

原则：

1. 不重新实现客户端已经具备的能力。
2. 不通过修改用户 Prompt 实现常规模型路由。
3. Gateway 只在协议不兼容时做最小必要转换。
4. 客户端模型选择最终统一解析为 `TargetModel`。
5. `@claude / @gemini` 等消息正文前缀不作为正式主路径。

---

## 2.2 Thin Gateway

Gateway 保持尽量薄：

```text
请求进入
  ↓
鉴权 / 权限
  ↓
模型解析
  ↓
Resource 路由
  ↓
协议适配
  ↓
官方 Provider
  ↓
Streaming 返回
```

模型选择、MCP 调用、Skill 触发等尽量复用客户端原生机制。

---

## 2.3 Gateway-first 实施原则

产品能力按完整治理平台设计，但开发从最小 Client-native 治理切片开始，不把完整 Gateway 能力本身当作首个市场验证目标。

第一开发阶段调整为：

```text
P0-MVP：Claude Code Governance Slice

Claude Code
    ↓
Anthropic Compatible Native Path
    ↓
Virtual Key → MEMBER Principal → Group → Model Permission
    ↓
Single Provider Model → Single Resource
    ↓
Anthropic Official API
    ↓
Usage Attribution
```

P0-MVP 直接验证产品真正的差异化问题：

> **企业是否愿意按员工 / Group 统一分配、治理并统计 AI Coding 模型使用。**

P0-MVP 验证成立后，再逐步扩展为正式 P0：

```text
P0-MVP Claude Code Governance Slice
        ↓
P0-A Community Claude Native Path
        ↓
P0-B Codex / OpenAI Native Path
        ↓
P0-C Cross-Protocol Adapter
```

P0-MVP 与正式 P0 的验收均不依赖：

```text
Knowledge Base
Skill Registry
Managed MCP
pgvector
Enterprise Identity
Billing
```

技术底座可以保留必要扩展接口，但不得为了未来能力提前实现未进入当前阶段的业务功能。

---

## 2.4 Principal 统一治理原则

平台内部统一以：

> **Principal（治理主体）**

作为企业 AI 能力访问与治理的根对象。

当前只定义两种类型：

```text
MEMBER
→ 企业员工 / 开发人员

APPLICATION
→ 企业内部应用、产品或服务
```

两类 Principal 在系统底层没有治理能力差异，统一复用：

```text
Access Key
Group
Permission
Model
Resource
Budget
Rate Limit
Concurrency
Usage
Cost
Operation Log
```

产品界面按业务语义区分：

```text
成员管理
→ principal_type = MEMBER

应用管理
→ principal_type = APPLICATION
```

第一版 Principal 只保存最小通用信息：

```text
id
organization_id
principal_type
name
remark
status
created_at
updated_at
```

其中：

```text
MEMBER
→ name 表示员工姓名

APPLICATION
→ name 表示应用名称
```

P0 不保存手机号、邮箱、部门、员工号、应用负责人、环境等扩展资料，也不预留通用 JSONB Profile。此类信息只有在后续企业身份集成或真实治理需求出现时再扩展。

员工与应用底层共用 `access_key`。管理界面可分别显示为：

```text
MEMBER      → Virtual Key
APPLICATION → App Key
```

企业内部应用接入不新建专用模型协议或独立 Gateway，直接复用现有 OpenAI Compatible / Anthropic Compatible 等标准入口。应用只需要获得管理员分配的 App Key 和模型权限即可调用。

该能力不改变产品定位：AI Coding 仍然是首要场景和市场切入点；Application Access 只是复用同一治理底座，为企业内部产品和服务提供统一 AI 能力入口。

---

## 2.5 Protocol Capability Matrix

跨协议转换不以“字段全部映射”为目标，而以**客户端可感知能力是否保持正确**为判断标准。

每个协议转换方向维护独立 Capability Matrix：

```text
Capability × Source Protocol × Target Protocol
```

状态统一为：

```text
LOSSLESS
DEGRADED
UNSUPPORTED
```

定义：

```text
LOSSLESS
→ 客户端行为与目标 Provider 原生能力等价

DEGRADED
→ 请求可以执行，但部分能力降级
→ 必须显式开启对应降级能力，禁止静默降级

UNSUPPORTED
→ 当前方向无法可靠保证正确性
→ 必须在请求发送到上游之前返回 CAPABILITY_UNSUPPORTED
```

Capability Matrix 按客户端可感知能力定义，包括：

```text
Text / Streaming
Tool Call
Thinking / Reasoning
Prompt Caching
Multimodal
Structured Output
Usage
Stop Reason
```

Capability Matrix 作为代码资产版本化管理，并由 Admin 只读展示。

---

# 三、总体架构

平台整体作为企业 **AI Coding Control Plane**，Model Gateway、Managed MCP、Knowledge 和 Skill 均属于 Control Plane 下的能力模块。

```text
                         Codex / Claude Code
                                 │
              ┌──────────────────┴──────────────────┐
              │                                     │
        Model Request                          MCP / Skill
              │                                     │
              ▼                                     ▼
       Model Gateway                       Enterprise MCP
              │                                     │
    ┌─────────┼─────────┐                ┌──────────┼──────────┐
    │         │         │                │          │          │
 Anthropic  OpenAI    Google         Knowledge   Image      Future Tool
    │         │         │                │          │
    └─────────┼─────────┘                │      Gemini Image
              │                          │
      AI Resource / API Key              │
              │                          │
              └──────────────┬───────────┘
                             │
                       Usage / Cost
                             │
                        Principal / Permission
```

平台内部能力分为：

```text
Model Governance
Tool Governance
Knowledge Governance
Skill Governance
```

---

# 四、核心数据模型

## 4.1 整体关系

Model、Provider、Resource 与 Principal 正式解耦：

```text
                         ai_model
                            │
                            ▼
                     provider_model
                    /              \
                   /                \
                  ▼                  ▼
           ai_provider        provider_model_price
                  │
                  ▼
             ai_resource
                  │
                  ▼
          principal_resource
                  │
                  ▼
              principal
              /       \
             /         \
        MEMBER       APPLICATION
             \         /
              \       /
               ▼     ▼
              access_key
                  │
                  ▼
            principal_group
                  │
                  ▼
                group

knowledge_base
      │
      ├── embedding_model_id → ai_model(model_type=EMBEDDING)
      │
      ▼
knowledge_document
      │
      ▼
knowledge_chunk(embedding)

skill
  │
  ▼
skill_version
  │
  ▼
principal_skill / group_skill
```

核心语义：

```text
Principal
= 企业 AI 能力治理主体，当前包括 MEMBER 与 APPLICATION

Access Key
= Principal 的调用凭证；成员界面称 Virtual Key，应用界面称 App Key

Model
= 平台统一定义“是什么模型 / 什么能力”

Provider
= 谁向 Control Plane 提供该模型的调用服务

Provider Model
= 某个 Provider 具体提供哪些 Model，以及实际上游模型标识

Resource
= 调用某个 Provider 时实际使用的 Credential / Account / Project / Endpoint 资源
```

普通开发人员只感知并选择 `Model`。`Provider`、`Provider Model` 与 `Resource` 属于企业治理与路由层，由管理员配置，最终由 Control Plane 选择。企业内部 Application 同样只使用管理员授权的逻辑 Model，不直接感知 Provider / Resource。

### 核心表字段约定

除 `operation_log` 等明确的 Append Only 事实表外，可变业务表默认包含以下通用字段：

```text
id
is_deleted
created_by
updated_by
created_at
updated_at
```

本章各数据模型的字段清单以业务字段为主；即使未重复列出上述通用字段，建表时仍必须按第十五章数据库规范统一创建。

---

## 4.2 ai_provider

`ai_provider` 表示**向 Control Plane 提供模型调用服务的供应方 / 接入方**，不再限定为模型原厂。

例如：

```text
OpenAI Official
Anthropic Official
DeepSeek Official
YourCompany AI
Partner-A AI
Enterprise Custom Provider
```

因此未来同一个 Model 可以由多个 Provider 提供服务。

Provider 类型可预留：

```text
OFFICIAL
PLATFORM
PARTNER
CUSTOM
```

主要保存：

```text
provider_code
provider_name
provider_type
official_website
openai_base_url
anthropic_base_url
status
```

Provider 不直接保存：

```text
具体 API Credential
企业成员权限
统一 Model 定义
客户余额
```

Credential 仍由 `ai_resource` 保存。Provider 能提供哪些 Model，由 `provider_model` 表达。

### 4.2.1 Provider 轻量接入原则

Provider 模块的定位是：

> **为 Control Plane 提供一个标准化的模型服务接入口，而不是建设独立的渠道管理平台。**

对于支持平台既有标准协议的 Provider，例如：

```text
OpenAI Compatible
Anthropic Compatible
```

应优先做到**纯配置接入**，典型管理员操作流程为：

```text
新增 Provider
    ↓
填写 Provider 名称 / 类型
    ↓
配置 Base URL / 协议类型
    ↓
创建 ai_resource 并填写 Credential
    ↓
通过 provider_model 绑定可提供的 Model
    ↓
配置 upstream_model_code
    ↓
连接测试
    ↓
启用
```

核心约束：

1. **新增一个标准兼容 Provider，原则上不应要求修改 Gateway 核心代码。**
2. 不为每个合作 Provider 单独创建专属数据库表、专属路由流程或专属业务模块。
3. Provider 只描述“谁提供模型服务以及如何接入”；Credential 继续归 `ai_resource` 管理，Model 映射继续归 `provider_model` 管理。
4. 合作 Provider 与官方 Provider 在治理层使用同一套 Model / Resource / Permission / Usage / Cost / Health / Operation Log 机制。
5. Provider 接入不默认引入合同、渠道层级、分润、充值、结算、审批等商业逻辑；这些能力如未来确有需要，由独立 Billing / Commercial 模块扩展。
6. 普通 Principal 不感知 Provider 接入方式，也不得直接指定 Provider；MEMBER 与 APPLICATION 均由管理员策略决定 Provider / Resource。

对于无法通过现有兼容协议接入的特殊 Provider，才允许增加：

```text
ProviderAdapter
```

其职责仅限于处理必要的协议、认证、请求 / 响应和错误语义差异。Adapter 必须保持轻量，不允许把供应商特有商业逻辑侵入 Gateway 核心。

因此 Provider 接入分为两级：

```text
标准 Provider
→ 配置接入

特殊 Provider
→ ProviderAdapter 扩展
```

目标是让标准合作 Provider 的接入成本接近“新增配置”，而不是“新增一个项目”。

### 4.2.2 Community Provider / Model 预置与动态配置原则

Community Edition 默认提供一组可直接使用的 Provider / Model 模板，目标是让新部署环境尽可能做到：

```text
安装 Community Edition
    ↓
主流 Provider / Model 已存在
    ↓
管理员填写 Resource Credential
    ↓
连接测试
    ↓
启用
    ↓
开始使用
```

预置内容分为三类：

```text
1. Provider Template
2. Model Catalog
3. Provider ↔ Model 默认映射
```

例如：

```text
Provider
→ DeepSeek Official

Protocol
→ OpenAI Compatible

Default Base URL
→ DeepSeek 官方接口地址

Model
→ DeepSeek Chat / DeepSeek Reasoner
```

预置模板只解决“开箱即用”，不得把 Provider / Model 写死在代码或配置文件中。

#### 运行时真实配置源

以下运行时业务配置统一以 PostgreSQL 为 Source of Truth：

```text
ai_provider
ai_model
provider_model
ai_resource
provider_model_price
Permission
Routing Policy
状态 / 启停配置
```

管理员通过 Web Admin 或 Admin API 修改后应动态生效，不要求修改 YAML、重新构建镜像或重启 Gateway。

配置文件 / 环境变量仅用于系统级启动配置，例如：

```text
HTTP Port
PostgreSQL DSN
Redis DSN
Logging
Tracing
Encryption Master Key / KMS 配置
默认系统参数
```

启动配置统一采用以下优先级：

```text
System Environment Variable
        ↓ 覆盖
.env.{dev|test|prod}
        ↓ 覆盖
.env
        ↓ 覆盖
Program Default
```

规则：

1. `.env` 主要用于本地开发，也可作为简单 Self-hosted 部署的默认配置来源。
2. 生产环境允许通过系统环境变量、Docker Compose Environment、Docker Secret、Kubernetes ConfigMap / Secret 等覆盖 `.env`。
3. `.env` 不作为企业运行时业务配置中心，不存储 Provider、Model、Permission、Routing Policy 等动态业务配置。
4. `.env` 必须默认加入 `.gitignore`，不得把生产 Secret 提交到 Git。
5. P0 / Community 不强依赖 Nacos、Consul、etcd、Apollo 等独立注册中心或配置中心；服务发现与动态业务配置分别由部署环境、PostgreSQL 与 Redis 协调完成。
6. `APP_ENV` 从系统环境变量或通用 `.env` 中选择，默认 `dev`；仅加载对应环境文件，缺失时跳过。旧值 `development / production` 兼容归一化为 `dev / prod`。
7. 环境文件只定义新增项或覆盖项，不得再次定义 `APP_ENV`。显式空值覆盖低优先级值，并由启动校验判断是否允许；环境文件同样忽略提交，仓库仅保存不含真实凭证的示例。

其中加密 Master Key 不得保存到 PostgreSQL；其本地文件、外部 Secret 或 KMS 属于独立的根密钥来源。

#### Bootstrap 初始化

平台允许提供可选 Bootstrap 数据，用于首次初始化主流 Provider / Model 模板。

Bootstrap 可以来自：

```text
内置 Migration / Seed
或
可选 Bootstrap YAML
```

但必须遵循：

> **Bootstrap 只负责首次初始化，数据库初始化完成后，PostgreSQL 才是运行时唯一真实配置源。**

后续应用启动不得使用 Bootstrap 文件覆盖管理员已经在数据库中修改的 Provider、Model、价格、权限或 Resource 配置。

#### 预置模板恢复

后续可提供“恢复官方模板”能力，用于恢复某个 Provider 的标准元数据，例如：

```text
protocol_type
default_base_url
默认 provider_model 映射
```

恢复模板不得覆盖：

```text
Credential
企业权限
企业自定义价格
自定义路由策略
客户业务数据
```

#### 自定义能力

Community Edition 必须允许管理员自行：

```text
新增 Provider
新增 Model
修改 Model Display Name
修改 upstream_model_code
修改 Base URL
新增 / 删除 provider_model 映射
停用旧 Model / Provider
```

因此 Community Edition 的预置 Provider / Model 是**默认模板**，不是白名单，也不是商业版限制。

---

## 4.3 ai_model

`ai_model` 是平台统一 Model Catalog，表示开发人员和管理员共同识别的**逻辑模型 / 能力定义**，不直接归属于某一个 Provider。

字段建议：

```text
id
vendor_code
display_name
model_code
model_type
embedding_dimension
max_input_tokens
status
```

其中：

```text
display_name
```

用于后台和客户端展示。

```text
model_code
```

表示 Control Plane 内部稳定的逻辑模型标识，例如：

```text
deepseek-v3
claude-sonnet
gpt-5
```

它不再要求与某个 Provider 的实际请求模型代码完全一致。

实际发送给上游 Provider 的模型标识由：

```text
provider_model.upstream_model_code
```

保存。

`model_type` 支持：

```text
CHAT
EMBEDDING
IMAGE
RERANK
```

这样同一个逻辑 Model 可以由多个 Provider 提供，同时开发人员的模型选择保持稳定，不因企业更换供应商而修改客户端配置。

---

## 4.4 provider_model

`provider_model` 建立：

```text
ai_provider N:M ai_model
```

关系，表示某个 Provider 可以提供哪些 Model。

建议字段：

```text
id
provider_id
model_id
upstream_model_code
protocol_type
status
capability_profile / metadata
created_at
updated_at
```

例如：

```text
ai_model
DeepSeek V3

provider_model
DeepSeek Official → upstream_model_code = deepseek-chat
YourCompany AI   → upstream_model_code = deepseek-chat
Partner-A AI     → upstream_model_code = ds-v3
```

开发人员仍然只看到：

```text
DeepSeek V3
```

Provider 的选择由管理员配置。

管理侧未来可以形成：

```text
DeepSeek V3
├── Primary Provider  → YourCompany AI
└── Fallback Provider → DeepSeek Official
```

但 P0 不实现复杂多 Provider 路由、权重和自动价格优化。P0 可以限制一个 Model 仅启用一个有效 Provider Model；数据模型从第一版支持未来扩展。

---

## 4.5 ai_resource

`ai_resource` 表示平台实际用于调用某个 Provider 的一份上游资源。

例如：

```text
OpenAI Project-A Key
Anthropic Public Key
YourCompany DeepSeek Pool-01
Partner-A Main Key
备用 Key
```

关系：

```text
ai_provider 1:N ai_resource
```

一个 Provider 可以拥有多个 Resource，用于项目隔离、容量、容灾、不同账号、不同合同或不同网络出口。

Resource 主要保存：

```text
provider_id
resource_name
resource_source
credential
status
health_score
health_status
balance_amount
balance_currency
last_balance_check_at
last_health_check_at
last_active_at
```

`resource_source` 预留：

```text
CUSTOMER_OWNED
PLATFORM_MANAGED
PARTNER_MANAGED
```

含义：

```text
CUSTOMER_OWNED
→ 企业自有 Provider Account / API Key（BYOK）

PLATFORM_MANAGED
→ 平台作为正规 Provider / 代理渠道统一持有的上游资源

PARTNER_MANAGED
→ 合作 Provider 提供并由平台接入治理的资源
```

Credential 必须加密保存。普通 Principal 及其客户端 / 应用永远不能取得真实 Provider Credential。

企业如果使用平台提供的模型服务，调用方配置的仍然是 Control Plane 的 Access Key（成员为 Virtual Key，应用为 App Key），而不是平台持有的 DeepSeek / OpenAI / Anthropic 等真实上游 Key。

---

## 4.6 principal

`principal` 是平台统一的企业 AI 能力治理主体。

第一版字段保持最小化：

```text
id
organization_id
principal_type
name
remark
status
is_deleted
created_by
updated_by
created_at
updated_at
```

`principal_type` 当前只支持：

```text
MEMBER
APPLICATION
```

业务含义：

```text
MEMBER
→ name = 员工姓名
→ remark = 员工备注

APPLICATION
→ name = 应用名称
→ remark = 应用备注
```

P0 不增加：

```text
mobile
email
department_id
external_ref
attributes JSONB
application_owner
environment
```

Principal 只描述“谁在使用企业 AI 能力”；密钥、分组、权限、预算、限流、Usage 与 Cost 均由独立治理关系承载；针对 Principal 的管理变更由 `operation_log` 记录。

---

## 4.7 access_key

`access_key` 是 Principal 访问 Control Plane 的统一凭证。

关系：

```text
principal 1:N access_key
```

建议字段：

```text
id
principal_id
key_prefix
key_hash
name
status
expires_at
last_used_at
revoked_at
created_at
updated_at
```

安全原则：

1. 完整 Access Key 只在创建时展示一次。
2. Access Key 使用密码学安全随机数生成；数据库只保存 `key_hash + key_prefix`，不保存可还原的完整 Key。
3. P0 使用 SHA-256 对 Access Key 做不可逆摘要并建立索引；Access Key 属于高熵随机凭证，不使用 bcrypt / Argon2 这类面向低熵用户密码的慢 Hash 作为网关热路径校验算法。
4. 支持 Revoke / Expire / Rotate / Last Used。
5. Access Key 只负责认证并定位 Principal，不直接承载 Model / Resource 权限。
6. Key 轮换不得改变 Principal 的权限、Group、Budget 或 Usage 归属。
7. Access Key 不能访问 Admin API；Admin JWT 也不能作为 Gateway / MCP 的模型调用凭证。

管理界面按 Principal Type 使用不同名称：

```text
MEMBER      → Virtual Key
APPLICATION → App Key
```

底层均使用同一张 `access_key` 表和同一套鉴权逻辑。

---

## 4.8 Group、关系表与 Resource 授权

P0 使用显式关系表表达 Principal 与 Group、Resource 的治理关系，不建设万能 Permission JSONB / Policy Expression。

Group 主表建议使用：

```text
ai_group

organization_id
group_code
group_name
remark
status
```

Principal 与 Group 的关系：

```text
principal_group

organization_id
principal_id
group_id
```

Resource 授权分别使用：

```text
principal_resource

organization_id
principal_id
resource_id
status
```

```text
group_resource

organization_id
group_id
resource_id
status
```

Resource 权限必须绑定 Principal / Group，而不是 Access Key。Access Key 轮换不得改变 Resource 权限。

P0 Resource 权限只表达 Grant，不实现 Deny / Principal 收窄：

```text
Final Resource Permission
= Group Resource Grant ∪ Principal Resource Grant
```

因此 Principal 同时属于多个 Group 时，最终 Resource 权限取所有 Group Grant 与 Principal Grant 的并集。P0 不引入 Deny Priority、Group Priority、Policy Expression 或 Resource Override Mode。

---

## 4.9 Group 与 Principal Policy

治理采用：

> **Organization Default + Group Policy + Principal Override**

Group 用于承载团队、岗位、项目组、应用组等通用 AI 使用策略；MEMBER 与 APPLICATION 都可以加入 Group，一个 Principal 允许属于多个 Group。

例如：

```text
张三（MEMBER）
├── Backend
└── Project-A

ERP（APPLICATION）
├── Business-Apps
└── Production
```

Group 之间平级，不区分主组 / 副组。

建议使用：

```text
group_policy

group_id
model_restricted
monthly_budget
rate_limit_per_minute
max_concurrency
```

```text
principal_policy

principal_id
model_restricted
monthly_budget
rate_limit_per_minute
max_concurrency
```

Model Allowlist 使用显式关系表：

```text
group_model

group_id
model_id
```

```text
principal_model

principal_id
model_id
```

`model_restricted = false` 表示不限制 Model；`model_restricted = true` 时，对应 `group_model / principal_model` 为 Allowlist。这样可以明确区分“没有限制”和“限制已开启但列表为空”。

Group 用于 AI 使用治理；系统 Role 只用于后台管理权限。P0 不建设复杂 RBAC / ABAC。

---

## 4.10 稀疏限制模型

除 Resource 必须显式授权外，大部分限制采用：

> **默认不限制，显式配置才限制。**

```text
Model Limit
model_restricted = false → 默认全部开放
model_restricted = true  → 使用 Allowlist

Monthly Budget
NULL → 不限制
500  → 月度预算 500

Rate Limit
NULL → 不限制
60   → 每分钟最多 60 次

Concurrency
NULL → 不限制
3    → 最大并发 3
```

Resource 是例外：

> **Resource 必须通过 Group / Principal 显式 Grant。**

因为 Resource 代表企业真实 API Key / 项目 Key，涉及项目隔离、成本归属和访问边界。

---

## 4.11 多 Group 与 Principal Override 合并规则

P0 采用简单、可解释的规则。

### Resource / Knowledge / Skill / MCP 等授权型配置

Group Policy 只表达 Grant，不表达 Deny。P0 最终权限取并集：

```text
Group Grants ∪ Principal Grants
```

Resource 在 P0 不支持 Principal Override 收窄。Knowledge / Skill / MCP 如后续需要更严格的收窄语义，再增加专用 Override 规则，不在 P0 引入通用 Deny Policy。

### Model

```text
Principal model_restricted = true
→ 直接使用 Principal Model Allowlist

Principal 未显式限制
→ 继承并合并 Group Model Policy

所有 Group 均未限制
→ Model 默认全部开放
```

### Budget / Rate Limit / Concurrency

```text
Principal 显式配置
→ Principal 优先

Principal 未配置
→ 使用 Group Policy

多个 Group 同时配置
→ 取最严格的非 NULL 值

Group 也未配置
→ 使用 Organization Default / 不限制
```

P0 不引入 Allow / Deny 冲突规则、Deny Priority、Group Priority 或 Policy Expression。

Group / Principal 权限与限制策略变化必须写入 `operation_log`，至少覆盖：

```text
PRINCIPAL_GROUP_ADD
PRINCIPAL_GROUP_REMOVE
GROUP_RESOURCE_GRANT
GROUP_RESOURCE_REVOKE
GROUP_MODEL_POLICY_CHANGE
GROUP_BUDGET_CHANGE
```

操作日志必须能追溯操作人、目标 Group / Principal、变更前后值、request_id 与时间。

---

## 4.12 Principal Profile 与企业身份扩展边界

P0 不把 Principal 设计成企业通讯录或 CMDB。

以下信息不进入第一版 Principal 核心模型：

```text
手机号
邮箱
部门
员工号
OIDC / LDAP / SCIM 外部身份
应用负责人
应用环境
任意 Profile JSONB
```

后续 Enterprise Identity / Directory Integration 出现真实需求时，可增加专用字段或独立身份映射模型。扩展不得改变 `principal.id` 作为 Permission / Usage / Cost / Operation Log 统一归属主键的原则。
---

## 4.13 organization

P0 从第一天保留 `organization` 数据模型，但产品行为仅支持 Single Organization。

建议业务字段：

```text
organization_code
organization_name
status
remark
```

首次初始化时创建默认 Organization。Principal、Group、Access Key、Resource、Usage、Operation Log 等组织级数据从第一版保留 `organization_id`，避免未来启用 Multi-Organization 时大规模补字段。

P0 不提供创建多个 Organization 的产品能力。

---

## 4.14 operation_log

P0 不建设完整企业合规 Audit System，先提供“操作日志”，用于记录管理员和系统对治理对象执行的管理操作。

表名：

```text
operation_log
```

业务字段建议：

```text
organization_id
operator_type
operator_id
operator_name
module
operation_type
target_type
target_id
target_name
request_id
request_method
request_path
ip_address
user_agent
result
error_code
before_data JSONB
after_data JSONB
remark
created_at
```

`operation_log` 是 Append Only 表，只允许 INSERT，不提供普通 UPDATE / DELETE / 逻辑删除能力，因此不包含：

```text
is_deleted
created_by
updated_by
updated_at
```

`operator_type` 当前支持：

```text
ADMIN=管理员
SYSTEM=系统
```

`operation_type` 使用稳定的大写英文编码，例如：

```text
CREATE=创建
UPDATE=修改
DELETE=删除
ENABLE=启用
DISABLE=停用
RESET=重置
ROTATE=轮换
GRANT=授权
REVOKE=撤销授权
LOGIN=登录
LOGOUT=退出
```

`before_data / after_data` 用于保存必要的变更快照，但 Credential、Master Key、密码、Password Hash、JWT、完整 Access Key 等敏感信息不得进入操作日志。

操作日志与 Usage Ledger 分离：管理员 / 系统管理操作进入 `operation_log`；Principal 的模型调用事实进入 `ai_request / usage_record`。

---

# 五、模型价格、成本与未来计费边界

## 5.1 provider_model_price

模型的上游成本价格绑定到 `provider_model`，而不是只绑定逻辑 `ai_model`。

原因：同一个 Model 可以由不同 Provider 提供，而不同 Provider 的采购成本、峰谷规则、缓存价格和合同价格可能不同。

例如：

```text
DeepSeek V3
├── DeepSeek Official → Cost A
├── YourCompany AI   → Cost B
└── Partner-A AI     → Cost C
```

`provider_model_price` 用于：

```text
历史价格
峰谷价格
缓存命中 / 未命中价格
上游成本修正
```

核心规则：

> 每条价格只有 `effective_from`，不维护 `effective_to`。

查询某次请求适用价格：

```text
provider_model_id = ?
effective_from <= request_at
ORDER BY effective_from DESC
```

取时间点适用的最新价格版本。

---

## 5.2 峰谷价

价格规则支持：

```text
price_type
timezone
weekdays
start_time
end_time
priority
effective_from
```

普通模型：

```text
DEFAULT
```

即可。

DeepSeek 一类峰谷模型可以配置：

```text
PEAK + weekday + time window
DEFAULT 兜底
```

查询时：

```text
request_at
  ↓
转换价格规则 timezone
  ↓
匹配 weekday + time
  ↓
优先具体时段规则
  ↓
无匹配则 DEFAULT
```

P0 Runtime 可以只实现 `DEFAULT`，Schema 保留后续峰谷扩展。

---

## 5.3 Usage 成本快照 + 局部重算

请求结束：

```text
request_at
  ↓
找到实际 provider_model
  ↓
找到当时有效的上游成本价格
  ↓
计算 cost_amount
  ↓
写 usage_record
```

`usage_record.cost_amount` 在核心治理体系中表示：

> **该次调用的上游成本 / 企业内部成本快照。**

管理员后补历史价格时，只重算受影响时间区间。

例如：

```text
9月1日 1.5
9月2日新增 1.3
9月5日已有 1.6
```

新增 9 月 2 日价格后只重算：

```text
request_at >= 9月2日
AND request_at < 9月5日
```

如果不存在下一价格版本，则从 `effective_from` 重算到当前时间。

---

## 5.4 Cost 与 Billing 必须分离

未来平台作为正规 Provider 代理商、自营 Provider 或合作渠道向企业提供模型调用服务时，必须把：

```text
Upstream Cost
```

与：

```text
Customer Charge / Billing
```

分开。

例如：

```text
上游实际成本：¥6.00
企业销售计费：¥7.10
毛利：        ¥1.10
```

核心 Usage Ledger 继续记录真实使用和上游成本；客户销售价格由独立 Billing 模块计算，禁止把客户售价直接覆盖 `usage_record.cost_amount`。

---

## 5.5 Provider 商业计费扩展（Future）

Provider 收费属于未来可选商业模块，不是 P0 / Community 核心依赖。

未来可增加：

```text
organization_account
billing_rate_card
billing_rate_item
billing_record
balance_transaction
```

典型预充值链路：

```text
企业对公付款
    ↓
运营人员确认到账
    ↓
Organization Account 充值
    ↓
Principal 使用 Access Key 调用模型
    ↓
Usage Record
    ↓
按企业 Rate Card 计算 Customer Charge
    ↓
Billing Record
    ↓
Balance Transaction 扣减企业余额
    ↓
账单 / 对账 / 发票
```

账本逻辑至少分为：

```text
Usage Ledger
→ 使用了多少、用了哪个 Model / Provider / Resource

Billing Ledger
→ 按客户合同价格应收多少

Balance Ledger
→ 充值、消费、退款、人工调整后的资金变化
```

`Balance` 与企业内部 `Budget` 必须独立：

```text
Balance
= 企业真实可消费余额

Budget
= Principal / Group 的内部治理额度
```

第一版商业计费可以采用 B2B 最小模式：对公收款后由平台运营人员在后台人工确认充值，不要求立即建设在线支付。

---

# 六、Usage

Usage 从第一版区分“客户端的一次请求”和“真实上游调用 Attempt”，避免 Retry / Failover 后无法准确对账。

## 6.1 ai_request

`ai_request` 表示 Principal 视角的一次 AI 调用。

建议业务字段：

```text
organization_id
request_id
principal_id
model_id
usage_scene
client_protocol
request_at
completed_at
status
error_type
latency_ms
```

`request_id` 是一次客户端调用的稳定请求标识，用于关联 Gateway Log、Trace、Usage Attempt 和错误排查。

## 6.2 usage_record

`usage_record` 表示一次真实 Provider / Resource Attempt。一个 `ai_request` 可以对应一条或多条 `usage_record`。

建议业务字段：

```text
organization_id
request_id
attempt_no
principal_id
resource_id
provider_id
provider_model_id
model_id
usage_scene

input_tokens
output_tokens
cached_input_tokens

started_at
completed_at
latency_ms
status
error_type

cost_amount        // 上游成本 / 内部成本快照
cost_currency

billing_unit
billing_quantity

tool_code           // MCP Tool 场景可为空
```

同一 `request_id` 下 `attempt_no` 从 1 递增。Retry / Resource Failover 必须创建新的 Attempt，不覆盖上一条真实调用事实。

例如：

```text
req_001
├── attempt 1 → Resource A → RATE_LIMIT
└── attempt 2 → Resource B → SUCCESS
```

即使失败 Attempt 已经产生 Token / 上游成本，也必须按 Provider 能够确认的真实 Usage 记录，不把失败等同于“成本为 0”。Provider 未返回可靠 Usage 时不得自行猜测 Token。

`usage_scene`：

```text
MODEL_GATEWAY
EMBEDDING
MCP_TOOL
KNOWLEDGE
```

`billing_unit` 支持：

```text
TOKEN
CALL
IMAGE
SECOND
```

非 Token 场景使用 `billing_quantity` 记录实际计费数量，使 Model、Embedding 和 Managed MCP 复用同一套 Usage / Cost 体系。未来 Provider 商业收费产生的客户应收金额进入独立 Billing Ledger，不直接改变 Usage Ledger 的成本语义。

## 6.3 Usage 异步写入

P0 不引入 Redis Stream / Kafka 作为 Usage 队列，使用 Go 原生 goroutine + buffered channel 完成异步写入：

```text
Gateway
  ↓
生成 Usage Event
  ↓
Buffered Channel
  ↓
固定 Usage Worker goroutine
  ↓
Batch Insert PostgreSQL
```

约束：

1. 不允许每个请求直接 `go saveUsage(...)` 创建无上限 goroutine；Usage Writer 使用固定 Worker。
2. Worker 支持按 Batch Size / Flush Interval 批量落库。
3. Queue 满时不得静默丢弃 Usage；P0 可回退为当前请求同步写 PostgreSQL，并记录 Metric / Warning。
4. PostgreSQL 批量写失败允许有限重试；持续失败必须输出错误日志和监控指标，不伪造成功。
5. Graceful Shutdown 时停止接收新的 Usage Event，Flush Channel 中剩余数据后再关闭 PostgreSQL。
6. `kill -9`、进程崩溃或宿主机突然断电仍可能导致尚未落库的内存事件丢失；P0 接受该权衡。未来只有在真实可靠性需求出现时再升级 Redis Stream / Kafka，Gateway 上层不依赖具体队列实现。

## 6.4 幂等与成本语义

`ai_request.request_id` 和 `usage_record(request_id, attempt_no)` 必须具备数据库唯一性保护，避免 Worker / Batch Retry 导致重复落账。

`usage_record.cost_amount` 始终表示真实上游成本 / 企业内部成本快照，不得被未来客户销售价格覆盖。

---

# 七、Resource 健康度

## 7.1 核心因素

健康度由三个主要信号组成：

```text
Balance Score
Success Score
Latency Score
```

系统提供默认权重，例如：

```text
Balance 50
Success 35
Latency 15
```

权重属于可配置策略，不作为协议常量。

当 Provider 不支持某个信号时，不生成虚假分数，而是对剩余有效信号重新归一化。

例如 Balance 不可获取：

```text
原权重：
Balance 50
Success 35
Latency 15

有效权重重新归一：
Success 70%
Latency 30%
```

余额仍然可以作为核心健康信号之一，但不是所有 Provider 的强制输入。

---

## 7.2 Redis 实时滑动窗口

实时健康数据不建数据库明细表，放 Redis。

```text
resource:{id}:health
resource:{id}:health:{yyyyMMddHHmm}
```

每分钟时间桶保存：

```text
total
success
failure
latency_sum
```

计算最近 N 分钟窗口。

错误分类：

```text
SUCCESS
CLIENT_ERROR
AUTH_ERROR
RATE_LIMIT
UPSTREAM_ERROR
TIMEOUT
```

其中：

```text
CLIENT_ERROR
```

不影响 Resource 健康度。

---

## 7.3 Resource Router

Model 与 Provider 解耦后，最终选择流程调整为：

```text
1. Access Key → Principal
2. 请求 model → ai_model（逻辑 Model）
3. 校验 Principal / Group 是否允许使用该 Model
4. 查询管理员启用的 provider_model
5. 按管理员 Provider Policy 确定 Provider
6. 在该 Provider 下结合 principal_resource / Group Resource 权限过滤 Resource
7. 筛选 ACTIVE Resource
8. 过滤 Credential 无效 / 明确无余额 Resource
9. health_score DESC
10. 同分 last_active_at ASC
11. 选择第一条 Resource
```

普通开发人员不参与第 4～11 步，也不能通过客户端指定 Provider 或 Resource。

P0 可以只允许每个逻辑 Model 对应一个有效 Provider Model，以保持 Router 简单。未来增加多个 Provider 时，再由管理员配置 Primary / Fallback 等明确策略。

### Resource Failover

Resource Router 选定 Resource 后，只对可重试错误允许切换 Resource：

```text
CLIENT_ERROR
→ 不重试

AUTH_ERROR
→ 当前 Resource 失效，可尝试同 Provider 的其他 Resource

RATE_LIMIT
→ 可切换同 Provider 的其他 Resource

UPSTREAM_ERROR
→ 可重试 / 切换

TIMEOUT
→ 可重试 / 切换
```

未来如果启用 Provider-level Failover，也必须先满足管理员配置的 Provider Policy 与 Capability Matrix；不能让普通 Principal 自己选择或绕过企业 Provider 策略。

Streaming 边界：

```text
responseCommitted = false
→ 可以 Retry / Switch Resource

responseCommitted = true
→ 不进行同一请求透明 Resource / Provider 切换
→ 当前请求结束
→ 客户端下一次请求重新路由
```

禁止将不同 Resource 或不同 Provider 的输出拼接到同一个 Streaming Response。

P0 不引入：

```text
weight
复杂 Weighted Routing
基于实时价格的自动 Provider 选择
```

---

# 八、双协议 Gateway

协议入口确定为：

```text
https://gateway.company.com/
    → OpenAI Compatible

https://gateway.company.com/anthropic
    → Anthropic Messages Compatible
```

入口表示的是：

```text
客户端协议
```

而不是：

```text
最终 Provider
```

Gateway 仍然根据请求中的目标模型确定实际 Provider。

---

## 8.1 Client-native 模型选择

正式主路径：

```text
客户端原生模型选择
      ↓
请求 model
      ↓
Gateway 解析 TargetModel
      ↓
ai_model
      ↓
Provider / Resource Router
```

Codex、Claude Code 等客户端具体如何选择模型，由 Client Adapter 适配客户端自身能力。

调用方只表达 `TargetModel`。Provider / Resource 不属于普通 Principal 可选参数；同一个 Model 后台更换 Provider 时，成员 Virtual Key、应用 App Key、逻辑模型名和调用方式保持不变。

Gateway 不把：

```text
@claude
@gemini
```

这类消息正文前缀作为核心协议。

只有未来某个客户端确实无法表达 TargetModel 时，才作为兼容性 fallback 单独评估。

---

## 8.2 Protocol Adapter

同协议同 Provider 时可以尽量透传。

跨协议时进入 Protocol Adapter：

```text
Anthropic Messages
        ↕
OpenAI Responses / Chat

未来：
Gemini Native
```

转换范围至少包括：

```text
system
messages
tools
tool_choice
tool_use
tool_result
reasoning / thinking
stop_reason
usage
error
SSE event
```

Protocol Adapter 是 Gateway 核心基础设施，但只在必要时执行转换。

## 8.3 P0-C Capability Matrix

P0-C v1 的基础能力矩阵：

| 能力 | Claude Code → OpenAI | Codex → Anthropic |
|---|---|---|
| Text + Streaming | LOSSLESS | LOSSLESS |
| Usage | LOSSLESS | LOSSLESS |
| Stop Reason | LOSSLESS | LOSSLESS |
| 基础参数子集 | LOSSLESS | LOSSLESS |
| Tool Call / Tool Result | LOSSLESS 或整体 UNSUPPORTED | LOSSLESS 或 UNSUPPORTED |
| Thinking / Reasoning | DEGRADED | DEGRADED |
| Prompt Caching | DEGRADED | DEGRADED |
| Multimodal | UNSUPPORTED | UNSUPPORTED |
| Structured Output 等扩展能力 | UNSUPPORTED | UNSUPPORTED |

Tool Call 不允许静默降级。某方向无法保证完整 Tool Loop 行为正确时，该方向 Tool Capability 标记为 `UNSUPPORTED`。

DEGRADED 能力必须由路由配置显式开启；未开启时按 `UNSUPPORTED` 处理。

## 8.4 Tool Call LOSSLESS 定义

Tool Call 的无损标准是客户端行为等价，而不是 JSON 结构一致。

必须满足：

```text
1. Tool 名称语义保持一致
2. Tool Arguments 语义保持一致
3. Source / Target Tool Call ID 可稳定映射
4. Tool Result 能准确关联原 Tool Call
5. 多轮 Tool Loop 可以持续执行
6. 并行 Tool Call 不发生结果串扰
7. Streaming Arguments 最终形成完整合法参数
```

## 8.5 Tool Call ID Mapping

Tool Call ID 映射属于会话状态。

映射作用域至少包含：

```text
access_key / principal
session_id
source_protocol
target_protocol
```

Redis 保存：

```text
source_tool_call_id
↔
target_tool_call_id
```

要求：

- 跨请求持续存在
- 设置 TTL
- 不同 Session 完全隔离
- 支持历史 Message 中 Tool ID 的重新映射
- 支持多实例 Gateway

Gateway 每次转换请求时，不只转换当前新增 Message，也必须处理请求历史中的 `tool_call / tool_use / tool_result` ID。

## 8.6 Tool Call Go / No-Go 验收

P0-C Tool Call 至少覆盖：

```text
单 Tool Call
连续多个 Tool Call
并行 Tool Call
并行调用顺序保持
复杂嵌套 JSON Arguments
大 Tool Result
Tool Failure
tool_choice
Streaming Tool Arguments
Arguments Streaming 中途断开
≥ 3 轮连续 Tool Loop
客户端主动取消
```

并发验证：

```text
≥ 10 个 Session 并发交错执行 Tool Loop
```

必须保证：

```text
0 Tool ID 跨 Session 串扰
0 Tool Result 错配
0 Arguments JSON 损坏
```

完整 Tool Loop 必须同时验证 Usage / Cost。

多轮 Tool Loop 的 Control Plane 成本与同一对话直接调用 Provider 的账单对账误差目标：

```text
< 1%
```

若目标方向不能达到 Tool Call LOSSLESS，则包含 `tools` 的请求必须在流开始前返回：

```text
CAPABILITY_UNSUPPORTED
```

不允许“先尝试转换”。

## 8.7 Protocol Adapter CI

Tool Call / Streaming 回归套件必须进入 CI，长期运行。

每次修改：

```text
Protocol Adapter
SSE Converter
Tool Mapping
Provider Adapter
```

自动执行。

核心回归要求：

```text
100 次自动回归
0 Tool ID 串扰
0 Tool Result 错配
0 Arguments 损坏
```

---

# 九、企业内置 Managed MCP

平台增加一个轻量 Enterprise MCP Server。

目的不是做通用 MCP Marketplace，而是：

> **让企业统一提供员工共同使用的工具能力，并统一管理 Credential、权限、Usage 和 Cost。**

首批内置场景：

```text
image_generate
  ↓
Gemini Image
```

员工 A、B 都通过公司 MCP 调用，不需要自己购买 Gemini Key。

---

## 9.1 MCP 调用链

```text
Codex / Claude Code
        ↓
客户端原生 MCP Client
        ↓
Enterprise MCP
        ↓
Access Key / Principal
        ↓
Tool Permission
        ↓
ai_model(model_type=IMAGE)
        ↓
ai_resource
        ↓
Gemini Image API
        ↓
Usage / Cost
```

员工不接触真实 Gemini API Key。

---

## 9.2 MCP V1 范围

内置受管 Tool：

```text
knowledge_search
image_generate
```

可扩展：

```text
web_search
image_edit
internal_api
jira
database_read
```

产品范围不包含：

```text
任意 MCP Server 注册
MCP Marketplace
复杂 MCP 聚合平台
```

---

# 十、企业知识库

## 10.1 定位

第一版知识库只解决一个问题：

> **让 Codex / Claude Code 通过内置 MCP 读取公司的内部文档。**

不做完整企业知识管理系统。

员工可以直接问：

```text
公司的数据库建表规范是什么？
退款接口应该遵守哪套规则？
上线前需要检查什么？
```

客户端通过：

```text
knowledge_search
```

获取公司文档片段。

---

## 10.2 最小知识库能力

基础能力：

```text
创建 Knowledge Base
上传 Markdown / PDF / DOCX / TXT
文档解析
Chunk
Embedding
Vector Search
Metadata
权限
knowledge_search MCP
来源引用
```

产品范围不包含：

```text
知识图谱
复杂审核流
飞书 / Notion / Confluence 全量连接器
自动知识整理
复杂 Agentic RAG
```

---

## 10.3 Knowledge 数据模型

最小三张表：

```text
knowledge_base
knowledge_document
knowledge_chunk
```

### knowledge_base

```text
id
name
description
embedding_model_id
status
created_by
created_at
updated_at
```

### knowledge_document

```text
id
knowledge_base_id
file_name
source_type
source_uri
content_hash
status
created_by
created_at
updated_at
```

### knowledge_chunk

```text
id
knowledge_base_id
document_id
chunk_index
content
metadata JSONB
embedding vector(...)
created_at
```

权限第一版可以增加：

```text
principal_knowledge_base
```

Group 权限启用后统一使用：

```text
group_knowledge_base
```

---

# 十一、Embedding 模型管理

## 11.1 与 Chat Model 解耦

Embedding 是独立模型能力。

例如：

```text
OpenAI
├── GPT                 CHAT
└── Embedding Model     EMBEDDING

Anthropic
└── Claude              CHAT
```

聊天模型和知识库 Embedding 模型不要求来自同一家 Provider。

---

## 11.2 用户自己配置 Embedding Model

Embedding Model 复用现有：

```text
ai_model
provider_model
ai_provider
ai_resource
```

体系。管理员选择逻辑 Embedding Model，实际 Provider / Resource 仍由 Control Plane 的管理员配置与路由决定。

`ai_model`：

```text
model_type = EMBEDDING
embedding_dimension = xxx
```

Knowledge Base：

```text
embedding_model_id
```

系统可提供：

```text
default_embedding_model_id
```

作为默认配置。

---

## 11.3 Embedding 切换

一个 Knowledge Base 创建后，不直接随意修改 Embedding Model。

修改流程：

```text
选择新 Embedding Model
        ↓
Re-Embedding
        ↓
全部 Chunk 生成新向量
        ↓
完成后切换
```

避免旧、新向量混用。

系统默认使用统一 Embedding Dimension；更换不同维度模型时通过 Re-Embedding 与必要的 Schema Migration 完成迁移。

---

# 十二、企业 Skill Registry

## 12.1 定位

Skill 表示：

> 企业希望 AI 按什么方式做事。

区别：

```text
Skill     = 怎么做
Knowledge = 公司知道什么
MCP       = 可以调用什么工具
Model     = 用哪个模型
```

企业 Skill 示例：

```text
backend-api-design
sql-review
security-review
release-check
incident-analysis
git-commit
```

---

## 12.2 Skill 不自创私有运行协议

优先兼容 Codex / Claude Code 等客户端原生 Skill 能力。

平台提供：

```text
Skill Registry
Version
Permission
Distribution / Sync
```

Skill Bundle 建议保持标准目录思路：

```text
skill/
├── SKILL.md
├── references/
├── scripts/
└── templates/
```

---

## 12.3 Skill 数据模型

```text
skill
skill_version
principal_skill
```

### skill

```text
id
skill_code
name
description
status
current_version
created_by
created_at
updated_at
```

### skill_version

```text
id
skill_id
version
bundle_path / content
checksum
created_by
created_at
```

### principal_skill

```text
principal_id
skill_id
status
```

Group 权限启用后统一使用：

```text
group_skill
```

替代大量逐 Principal 配置。

---

## 12.4 Skill + Knowledge + MCP

三个能力可以组合：

```text
backend-api-design Skill
        ↓
要求先查企业 API 规范
        ↓
knowledge_search MCP
        ↓
企业知识库
        ↓
如需外部能力
        ↓
Managed MCP Tool
```

从而实现：

> **让 AI 按公司的方法、使用公司的知识和工具完成任务。**

---

# 十三、PostgreSQL 与 pgvector

主数据库统一采用：

> **PostgreSQL**

P0 阶段仅把 PostgreSQL 作为普通关系数据库使用，承担：

```text
Provider
Model
Provider Model
Provider Model Price
Resource
Principal
Access Key
Group
Permission
Routing Policy
Usage
Operation Log
```

Provider、Model、Provider Model、Resource、Price、Permission 和 Routing Policy 均采用数据库动态配置；PostgreSQL 是这些运行时业务配置的唯一真实数据源。Provider / Model 预置模板只用于首次 Bootstrap，不作为运行时配置文件持续覆盖数据库。

知识库进入开发阶段后，再启用 `pgvector` 扩展，承担：

```text
Knowledge Metadata
Knowledge Chunk
Embedding Vector
Vector Search
```

这样数据库选型从一开始保持统一，但 P0 不引入向量检索业务复杂度。

PostgreSQL 同时保留：

```text
JSONB
Full Text Search
pgvector
```

能力，为 Knowledge 模块提供扩展空间。

Redis 继续承担：

```text
Health
Runtime State
Cache
Rate Limit
临时数据
```

---

## 13.1 PostgreSQL 职责

P0：

```text
Provider
Model
Provider Model
Provider Model Price
Resource
Principal
Access Key
Group
Permission
Usage
Operation Log
```

Knowledge 阶段：

```text
Knowledge Base
Knowledge Document
Knowledge Chunk
pgvector Embedding
```

Skill 阶段：

```text
Skill
Skill Version
Skill Permission
```

---

# 十四、Go + DDD 工程架构

采用：

> **DDD 控制业务边界，Go 控制实现复杂度。**

不照搬 Java 式重型 DDD。

```text
cmd/
└── server/

internal/
├── domain/
│   ├── provider/
│   ├── model/
│   ├── pricing/
│   ├── resource/
│   ├── member/
│   ├── routing/
│   ├── usage/
│   ├── knowledge/
│   ├── skill/
│   └── audit/
│
├── application/
│   ├── gatewayapp/
│   ├── resourceapp/
│   ├── memberapp/
│   ├── pricingapp/
│   ├── knowledgeapp/
│   ├── skillapp/
│   └── usageapp/
│
├── infrastructure/
│   ├── postgres/
│   ├── redis/
│   ├── cache/
│   ├── provider/
│   │   ├── anthropic/
│   │   ├── openai/
│   │   ├── google/
│   │   └── deepseek/
│   ├── protocol/
│   │   ├── anthropic/
│   │   ├── openai/
│   │   └── converter/
│   ├── embedding/
│   └── storage/
│
└── interfaces/
    ├── admin/
    ├── gateway/
    └── mcp/
```

---

## 14.1 分层职责

### Domain

负责纯业务规则：

```text
Resource Health
Principal Permission
Provider Model Price Matching
Cost Recalculation
Resource Selection
Knowledge Permission
Skill Permission
```

Domain 不依赖：

```text
PostgreSQL
Redis
HTTP SDK
具体 Provider Client
```

### Application

负责 Use Case 编排：

```text
处理 Gateway Request
添加 Resource
授权 Principal
重算历史价格
导入文档
执行 Knowledge Search
同步 Skill
```

### Infrastructure

负责：

```text
PostgreSQL
pgvector
Redis
HTTP Client
Provider API
Protocol Converter
文件解析
Embedding 调用
```

### Interfaces

负责：

```text
Admin HTTP API
OpenAI Compatible Gateway
Anthropic Compatible Gateway
Enterprise MCP
```

---

# 十五、Go 技术栈

后端采用标准库优先、轻框架辅助的方案。

核心组合：

```text
Go
+ chi
+ net/http
+ pgx
+ sqlc
+ go-redis
+ slog
+ OpenTelemetry
+ Prometheus
```

---

## 15.1 HTTP Router

采用：

```text
chi
```

主要负责：

```text
Admin HTTP API
Gateway Route
MCP HTTP Endpoint
Middleware
```

Chi 保持标准 `net/http` 体系，不引入额外 HTTP 抽象层。

---

## 15.2 Gateway / Streaming

Gateway 核心直接基于：

```text
net/http
http.Client
http.Transport
httputil.ReverseProxy（适合纯透传场景时使用）
context.Context
```

重点支持：

```text
HTTP
SSE
Streaming
Client Cancellation
Outbound Proxy
Protocol Adapter
```

原则：

> Gateway 核心尽量保持标准 `net/http` 语义，便于处理长连接、流式转发、连接取消和代理 Transport。

---

## 15.3 PostgreSQL

数据库访问采用：

```text
pgx
+
sqlc
```

职责：

```text
pgx
→ PostgreSQL Driver / Connection Pool

sqlc
→ 根据 SQL 生成类型安全的 Go 查询代码
```

数据库访问以明确 SQL 为主，不引入重型 ORM。

DDD 中：

```text
Domain
→ 定义业务接口

Infrastructure/Postgres
→ 使用 pgx + sqlc 实现 Repository
```

### 15.3.1 数据库开发规范

#### 通用字段

除明确声明为 Append Only 的事实 / 日志表外，可变业务表默认包含：

```text
id          BIGINT
is_deleted  BOOLEAN
created_by  VARCHAR(64)
updated_by  VARCHAR(64)
created_at  TIMESTAMPTZ
updated_at  TIMESTAMPTZ
```

规则：

1. `id` 使用 BIGINT，由应用侧生成 64-bit 全局唯一 ID。
2. `is_deleted` 默认 `FALSE`，表示逻辑删除标识。
3. `created_by / updated_by` 使用 `VARCHAR(64)`，可保存 `admin:<id>`、`principal:<id>`、`system` 等统一操作者引用。
4. `created_at / updated_at` 使用 `TIMESTAMPTZ`，数据库统一以 UTC 语义保存，UI 根据用户时区展示。
5. 除非字段业务语义明确允许“未知 / 未发生 / 未配置 / 不适用”，否则字段默认 `NOT NULL`。
6. 必填字段不允许通过空字符串、0 或其他虚假默认值代替缺失数据；只有存在明确业务默认值时才配置 `DEFAULT`。

`operation_log` 是明确例外，仅保留 `id / created_at` 等实际需要字段，不机械继承 `is_deleted / created_by / updated_by / updated_at`。

#### 表名、字段名、状态值与 COMMENT

```text
表名 / 字段名   → 英文 snake_case
状态 / 类型值   → 稳定的大写英文编码
数据库 COMMENT → 中文描述
```

所有表和字段建表时必须写中文 COMMENT。状态 / 类型字段 COMMENT 必须列出当前已知编码与中文含义，例如：

```text
状态：ACTIVE=启用；DISABLED=停用
主体类型：MEMBER=成员；APPLICATION=应用
```

数据库、Go typed constants 与 API 返回值保持同一英文编码；前端通过英文编码进行 i18n。

不使用 PostgreSQL Native ENUM；状态 / 类型字段使用 `VARCHAR` + Go typed constants + Domain Validation。

#### NULL 规范

默认 `NOT NULL`。只有具有真实缺省语义的字段允许 NULL，例如：

```text
expires_at          → NULL=永不过期
last_used_at        → NULL=从未使用
last_health_check_at→ NULL=尚未执行健康检查
```

备注等非必填信息如确实表示“未设置”，允许 NULL；不强制用空字符串替代 NULL。

#### 逻辑删除与唯一索引

逻辑删除默认不可恢复。重新创建同业务标识的对象时创建新记录、新 ID，旧记录保留用于历史追溯。

因此存在业务唯一性的软删除表统一使用 PostgreSQL Partial Unique Index，仅约束未删除数据：

```sql
CREATE UNIQUE INDEX uk_provider_code
ON ai_provider (provider_code)
WHERE is_deleted = false;
```

关系表也遵循同一原则，例如删除 Principal ↔ Group 关系后再次授权时创建新关系记录。

#### 外键与关联完整性

项目不创建 PostgreSQL Foreign Key Constraint，也不使用 Cascade Delete。

```text
数据库
→ 保存关联 ID + 必要普通索引

Application / Domain
→ 校验关联对象存在、未删除、状态满足业务要求
→ 删除主实体前检查依赖关系
```

所有关联字段统一使用 `<entity>_id` 命名；高频关联字段根据查询路径建立普通索引。历史 Usage / Operation Log 只保存当时的 ID / 快照，不因主实体逻辑删除而清除。

#### 数值与金额

```text
Token / Count → BIGINT
金额          → NUMERIC(20,8)
```

金额禁止使用 FLOAT / DOUBLE 保存。

#### 数据库事务边界

数据库事务只覆盖数据库内部必须原子完成的业务操作，不得跨越 OpenAI / Anthropic / 其他 Provider 的网络调用或长时间 SSE Streaming。

---

## 15.4 Redis

采用：

```text
go-redis
```

承担：

```text
Resource Health
Health Sliding Window
Runtime Cache
Rate Limit
Session / Temporary State
Distributed Runtime Data
```

Redis 数据属于运行态，不作为核心业务事实的唯一持久化来源。

---

## 15.5 日志

统一采用 Go 标准库：

```text
log/slog
```

日志以结构化字段为主，不自研一套类似 SLF4J 的 Logger 抽象；通过 `slog.Logger + slog.Handler` 保持输出实现可替换。

推荐字段：

```text
request_id
trace_id
principal_id
provider_id
resource_id
model_id
latency_ms
status
error_type
```

Credential、Proxy Credential、Master Key、Prompt、Response 等敏感内容不得进入普通日志。

### 日志输出模式

`LOG_OUTPUT` 仅允许二选一：

```text
stdout
file
```

不默认同时写 stdout 与 file，避免重复存储和双重滚动。

默认部署策略：

```text
Local Development
→ stdout + text

Docker / Docker Compose
→ stdout + json
→ docker logs / docker compose logs 查看
→ 由 Docker Logging Driver 负责日志文件大小与数量控制

Kubernetes
→ stdout + json
→ kubectl logs / 日志平台查看
→ 由容器运行时、Collector 或日志平台负责采集与留存

Standalone / Traditional Server
→ 默认可使用 stdout + systemd/journald
→ 也可显式选择 file 模式
```

### File 模式

当 `LOG_OUTPUT=file` 时支持自定义日志目录，并采用“按天滚动 + 单文件大小保护 + 留存天数 + 可选压缩”的策略。

建议配置：

```env
LOG_OUTPUT=file
LOG_FORMAT=json
LOG_LEVEL=info

LOG_DIR=/var/log/ai-control-plane
LOG_ROTATE=daily
LOG_MAX_SIZE=500
LOG_RETENTION_DAYS=30
LOG_COMPRESS=true
```

语义：

```text
LOG_DIR
→ 日志目录

LOG_ROTATE=daily
→ 以自然日作为主要滚动边界

LOG_MAX_SIZE
→ 单个日志文件的大小保护阈值，单位 MB

LOG_RETENTION_DAYS
→ 历史日志最多保留天数

LOG_COMPRESS
→ 历史日志是否 gzip 压缩
```

建议文件组织：

```text
/var/log/ai-control-plane/
├── control-plane.log
├── control-plane-2026-09-03.log.gz
├── control-plane-2026-09-02.log.gz
└── ...
```

当天始终写入：

```text
control-plane.log
```

跨日后滚动为日期文件并创建新的 `control-plane.log`；如果某一天日志超过 `LOG_MAX_SIZE`，允许继续生成分片，例如：

```text
control-plane-2026-09-04.1.log.gz
control-plane-2026-09-04.2.log.gz
```

File 模式底层使用独立 Rotating Writer，不把日志滚动逻辑耦合进领域层；如果采用第三方库，应支持 time-based rotation，不能仅以 size-based rotation 伪装成按天滚动。

日志配置属于启动配置，继续遵循：

```text
System ENV > .env > Program Default
```

---

## 15.6 Observability

采用：

```text
OpenTelemetry
+
Prometheus
```

用于：

```text
Tracing
Metrics
HTTP Latency
Provider Latency
Resource Error Rate
SSE Connection
Resource Health
Usage Pipeline
```

可接入：

```text
Grafana
Tempo
Jaeger
其他兼容 OpenTelemetry 的平台
```

---

## 15.7 Dependency Injection

V1 不引入复杂 DI Framework。

采用显式构造：

```text
main
 ↓
Config
 ↓
PostgreSQL / Redis
 ↓
Repository
 ↓
Provider Client
 ↓
Application Service
 ↓
HTTP / MCP Handler
```

通过：

```go
NewXXX(...)
```

显式创建并注入依赖。

原则：

> 依赖关系保持可见、可追踪，避免为了模拟 Spring 容器而增加不必要复杂度。

---

## 15.8 Web Framework 使用边界

不使用重型 Web Framework 承担 Gateway 核心。

最终约定：

```text
chi
→ Route / Middleware

net/http
→ Gateway / Streaming / SSE / Proxy
```

如 Admin API 需要更多辅助能力，也优先在 Chi + 标准库体系内扩展。

---

## 15.9 Go + DDD 原则

技术框架不改变 DDD 边界。

```text
Domain
→ 业务规则

Application
→ Use Case 编排

Infrastructure
→ PostgreSQL / Redis / Provider / Protocol

Interfaces
→ Admin / Gateway / MCP
```

核心原则：

> **DDD 控制业务边界，Go 控制实现复杂度。**

# 十六、请求生命周期

Model Gateway 请求统一流程：

```text
Request
  ↓
Access Key
  ↓
Principal
  ↓
TargetModel
  ↓
ai_model
  ↓
Model Permission
  ↓
provider_model
  ↓
Admin Provider Policy
  ↓
ai_provider
  ↓
Principal / Group Resource Permission
  ↓
Resource Filter
  ↓
Redis Health
  ↓
ResourceSelector
  ↓
Protocol Adapter
  ↓
Upstream Provider
  ↓
Streaming
  ↓
Usage Ledger
  ↓
Upstream Cost
  ↓
Health Update
```

两类调用入口仅在 Principal 类型和管理界面上不同：

```text
MEMBER
→ Virtual Key
→ Codex / Claude Code / 其他 AI Coding Client

APPLICATION
→ App Key
→ ERP / CRM / OA / 自研产品 / 内部服务
```

两类入口进入 Access Key 鉴权后走完全相同的 Model / Provider / Resource / Usage / Cost 链路。Application Access 不增加独立 Gateway，也不改变 Provider 路由体系。

如果未来启用平台 Provider 商业计费，则在 Usage Ledger 之后异步或事务性进入：

```text
Usage Record
  ↓
Rate Card
  ↓
Billing Record
  ↓
Balance Ledger
```

该链路不是 P0 核心依赖。

MCP Tool：

```text
MCP Request
  ↓
Access Key / Principal
  ↓
Tool Permission
  ↓
Tool Handler
  ↓
AI Model / Resource or Internal Service
  ↓
Usage / Cost
```

Knowledge：

```text
knowledge_search
  ↓
Principal Permission
  ↓
Knowledge Base
  ↓
Embedding Model
  ↓
Provider / Resource Router
  ↓
Embedding Query
  ↓
pgvector Search
  ↓
Top K Chunks + Source
  ↓
MCP Result
```
---

# 十七、上游网络出口与代理配置

平台默认由服务器直接访问各官方 Provider。

默认链路：

```text
Gateway
   ↓
Provider API
```

当部署环境需要通过代理访问上游 Provider 时，由 Gateway 的出站 HTTP Client 显式配置代理。

代理链路：

```text
Gateway
   ↓
Outbound Proxy
   ↓
Provider API
```

Provider Adapter 不直接处理代理细节，只获取已经配置好的 `http.Client`。

---

## 17.1 代理配置

代理配置采用：

```text
Proxy URL
+
Optional Proxy Headers
```

示例：

```yaml
outbound:
  proxy:
    mode: PROXY
    url: "http://proxy.example.com:10000"

    headers:
      Proxy-Authorization: "Basic xxxxx"
      X-Proxy-Token: "xxxxx"
```

默认直连：

```yaml
outbound:
  proxy:
    mode: DIRECT
```

如果代理通过 URL 携带用户名和密码：

```text
http://username:password@proxy.example.com:10000
```

程序直接解析标准 Proxy URL。

用户名或密码包含特殊字符时必须进行 URL Encode。

---

## 17.2 代理 Header

部分代理服务使用 Header 进行认证或传递线路参数。

平台允许管理员配置自定义 K/V：

```text
Key                    Value
Proxy-Authorization    ********
X-Proxy-Token          ********
X-Region               ********
```

代理 Header 与 Provider Header 必须严格隔离。

例如：

```text
Proxy Header:
Proxy-Authorization
X-Proxy-Token

Provider Header:
Authorization
x-api-key
anthropic-version
```

代理 Header 只发送给代理服务器，不允许透传给 OpenAI、Anthropic、Google、DeepSeek 等上游 Provider。

---

## 17.3 Outbound HTTP Client

基础设施层提供统一的：

```text
OutboundHTTPClientFactory
```

职责：

```text
OutboundHTTPClientFactory
│
├── Direct Client
└── Proxy Client
      ├── proxy_url
      └── proxy_headers
```

Provider Adapter 统一通过：

```text
httpClientFactory.ForProvider(provider)
```

获取出站 Client。

Domain 和 Application 层不感知：

```text
DIRECT
PROXY
```

等网络实现细节。

---

## 17.4 HTTPS Provider 请求

访问 HTTPS Provider 时，HTTP Proxy 通常通过 `CONNECT` 建立隧道：

```text
Gateway
   ↓
Proxy
   ↓ CONNECT provider.example.com:443
   ↓
TLS
   ↓
Provider API
```

Gateway 仍然按 Provider 官方 HTTPS 地址构造请求，业务代码不需要把目标 URL 改成代理地址。

---

## 17.5 Streaming

SSE / Streaming 请求使用与普通 Provider 请求相同的 Outbound Client。

链路：

```text
Provider SSE
   ↓
Proxy
   ↓
Gateway
   ↓
Client
```

代理层只负责网络传输，不改变 Gateway 的 Streaming、Protocol Adapter、Usage 和 Resource Router 逻辑。

---

## 17.6 安全要求

代理认证信息与 Provider Credential 按同级敏感信息处理：

1. Proxy URL 中的用户名和密码不得明文写入日志。
2. Proxy Header Value 不得进入普通日志。
3. 管理后台不回显完整敏感值。
4. 错误信息必须脱敏。
5. 持久化保存时使用加密存储。
6. `Proxy-Authorization` 等代理认证信息不得透传给 Provider。
7. 前端返回代理配置时仅展示脱敏信息。

日志示例：

```text
http://***:***@proxy.example.com:10000
```

---

## 17.7 支持范围

代理能力聚焦标准 Forward Proxy 场景：

```text
Proxy URL
+
Proxy Header K/V
```

可覆盖常见：

```text
无认证代理
用户名密码认证
Proxy-Authorization
自定义 Token Header
IP 白名单
```

动态签名、NTLM、Kerberos、mTLS 等特殊企业认证方式不进入通用代理配置模型；如存在真实需求，通过独立 `ProxyAuthProvider` 或专用网络适配器扩展。

---

# 十八、Credential 与认证安全

平台把凭证分为三类，分别采用不同的安全处理方式。

## 18.1 Principal Access Key

Principal Access Key（MEMBER 的 Virtual Key、APPLICATION 的 App Key）只用于认证 Principal，不需要在服务端恢复原文。

规则：

1. 完整 Key 仅在创建时展示一次。
2. 使用密码学安全随机数生成。
3. 数据库仅保存 `key_prefix + key_hash`。
4. P0 使用 SHA-256 做不可逆摘要；认证时计算摘要后匹配 Access Key 记录。
5. Access Key 支持 Revoke / Expire / Rotate / Last Used。
6. Access Key 不直接保存 Model / Resource / Budget 等业务权限。
7. Access Key 泄露或轮换只影响凭证本身，不改变 Principal 的治理配置。

## 18.2 Provider / Proxy Credential

`ai_resource` 中的 Provider API Key、Proxy Credential 等凭证在访问上游时必须恢复原文，因此采用可逆加密，而不是 Hash。

P0 统一使用标准 AEAD 算法：

```text
AES-256-GCM
```

原则：

1. Credential 以密文保存，数据库不保存明文。
2. 加密 Master Key 不进入 PostgreSQL，不硬编码进源码或镜像。
3. Community Edition 默认支持持久化 Master Key 文件，同时允许系统环境变量 / Docker Secret / Kubernetes Secret 显式提供并覆盖本地文件来源。
4. Enterprise 可扩展 AWS KMS、Azure Key Vault、HashiCorp Vault、阿里云 KMS 等外部密钥系统。
5. Credential 不允许进入普通日志、Tracing Attribute 或 Operation Log Detail。
6. 管理后台不回显完整 Credential，修改时采用覆盖式更新。
7. 前端永远不能取得解密后的真实 Provider / Proxy Credential。
8. Gateway 可在 Resource 配置加载后把已解密 Credential 保存在受控进程内存中复用；Resource Credential 更新、停用或撤销时必须使缓存失效，避免每个模型请求都重复访问数据库和解密。

Credential 的加密算法属于基础安全实现，不自研加密算法。

### Master Key 管理

Master Key 是 Provider / Proxy Credential 加密体系的 Root of Trust，本身不再使用另一个应用内密钥二次加密，避免形成递归密钥依赖。

Community / Self-hosted 默认解析顺序：

```text
1. 显式外部 Secret / System ENV（如 ACP_MASTER_KEY）
        ↓ 未提供
2. 持久化文件 /data/secrets/master.key
        ↓ 不存在
3. crypto/rand 首次生成 32 bytes Master Key
        ↓
4. 原子写入持久化文件，权限 0600
```

Master Key 文件必须位于持久化 Volume，不得只存在于容器临时文件系统；程序不得把完整 Master Key 输出到日志、Operation Log 或管理后台。

Master Key 建议使用 32 bytes 密码学安全随机数生成，可用 Base64 作为文件编码；AES-256-GCM 直接使用解码后的 32 bytes Key。

数据库中的加密 Credential 保存：

```text
credential_ciphertext
credential_nonce
key_id / key_version
```

其中 `key_id` / `key_version` 只用于识别加密密钥版本，可以公开保存，不包含 Master Key 本身。

### Master Key 丢失恢复

如果本地 Master Key 文件被误删，且不存在外部 Secret、备份或 KMS 可恢复副本，则历史 Credential 无法被原密码学密钥解密。

系统处理规则：

1. 生成新的 Master Key 并持久化，但不得尝试用新 Key 解密旧密文。
2. Admin Plane 仍可启动，便于管理员恢复配置。
3. 无法通过 AEAD 完整性校验解密的 Resource 标记为 `CREDENTIAL_UNRECOVERABLE` / 不可用，并从 Gateway 路由候选中排除。
4. 管理员必须重新录入对应 Provider / Proxy Credential，系统使用新 Master Key 重新加密保存。
5. 不提供“绕过 Master Key 找回明文”的后门能力。
6. 生产环境应对 Master Key 文件或外部 Secret 做独立安全备份；Enterprise 优先使用 KMS / Vault 托管根密钥。

Master Key Rotation 后续通过 `key_id / key_version` 支持，不要求 P0 实现复杂 Envelope Encryption。

## 18.3 Admin 登录凭证

Admin 属于管理面身份，不复用 Principal Access Key。

P0：

```text
Admin Username / Password
        ↓
Password Hash Verify
        ↓
JWT
        ↓
Authorization: Bearer <JWT>
        ↓
Admin API
```

规则：

1. Admin 密码只保存密码 Hash，不可逆加密。
2. 密码 Hash 使用 Argon2id 或 bcrypt 等面向用户密码的慢 Hash；不得使用 SHA-256 直接保存用户密码。
3. Admin JWT 通过 `Authorization: Bearer <JWT>` 传递，只允许访问管理面 API，不能用于 Gateway / MCP 模型调用。
4. JWT 签名密钥不硬编码在源码中，由环境变量 / Secret 提供；Enterprise 可接 KMS / Vault。
5. 后续接入 SSO / OIDC / LDAP / SCIM 时，只替换或扩展 Admin / Enterprise Identity 的认证入口，不改变 Principal Access Key 模型。

---

# 十九、访问入口与后台权限

平台区分管理主体与业务调用主体，并严格隔离管理面与 AI 能力调用面：

```text
Admin
↓
Web Admin
↓
管理平台

Principal
├── MEMBER      → Virtual Key → Codex / Claude Code 等客户端
└── APPLICATION → App Key     → ERP / CRM / OA / 自研产品 / 内部服务
```

认证与授权边界：

```text
Management Plane
Admin → Username / Password / SSO → JWT → Authorization Header → Admin Authorization

Capability Plane
Principal → Access Key → Principal Authorization → Gateway / MCP
```

统一原则：

1. Admin JWT 与 Principal Access Key 是两套独立认证凭证，互不通用。
2. 两类认证可以复用统一 HTTP Middleware / Security Context / Organization Boundary / Operation Log 基础设施，但进入不同的授权领域。
3. Admin Authorization 解决“管理员可以执行哪些管理操作”。
4. Principal Authorization 解决“员工或应用可以使用哪些 AI 能力以及可以使用多少”。
5. 不把 Admin 管理权限与 Principal 的 Model / Knowledge / Skill / MCP 权限混入同一个通用 Permission 集合。

## 19.1 认证与授权模型

P0 不建设复杂 RBAC / ABAC / Policy Expression。

### Admin Authorization

P0 只提供最小管理员角色，用于 Provider、Model、Resource、Principal、Group、Usage、Cost、Operation Log、System Config 等管理操作。

后续 Enterprise 再按真实采购需求扩展：

```text
SUPER_ADMIN
ORG_ADMIN
AUDITOR
OPERATOR
```

以及更细的管理权限。

### Principal Authorization

Principal 的 AI 能力授权继续采用：

```text
Organization Default
        ↓
Group Policy
        ↓
Principal Override
```

授权对象包括：

```text
Model
Knowledge
Skill
MCP Tool
Resource Eligibility（仅管理员可配置，Principal 不感知 Resource）
Budget
Rate Limit
Concurrency
```

Access Key 只负责认证并定位 Principal，权限始终绑定 Principal / Group，而不是绑定 Access Key。Provider / Resource 的最终选择由管理员策略和 Resource Router 完成。

## 19.2 管理后台

管理后台仅对管理员开放。

管理员负责：

```text
Provider
Model
Provider Model
Provider Model Price
Resource
成员
应用
Group
Access Key
Usage
Cost
Health
Knowledge
Skill
Managed MCP
Operation Log
System Config
```

底层“成员”和“应用”都来自 `principal`：

```text
成员管理
→ principal_type = MEMBER

应用管理
→ principal_type = APPLICATION
```

第一版成员表单：

```text
姓名 *
备注
状态
```

第一版应用表单：

```text
应用名称 *
备注
状态
```

手机号、邮箱、部门、员工号、应用负责人、环境等扩展信息不进入 P0。

后台认证只需要实现管理员身份认证和管理权限控制，不建设复杂菜单级 / 按钮级 RBAC。

## 19.3 MEMBER Principal

普通员工不登录管理后台。

员工模型使用边界：

```text
可以：
→ 在管理员授权范围内选择 Model

不可以：
→ 选择 Provider
→ 指定 Resource
→ 绕过管理员 Provider Policy
→ 获取真实 Provider Credential
```

员工通过 Virtual Key 识别为对应的 MEMBER Principal，并通过：

```text
Gateway
Enterprise MCP
Skill Sync
Knowledge Search
```

使用企业 AI 能力。

员工日常工作入口始终是：

```text
Codex
Claude Code
其他支持的 AI Coding Client
```

平台尽量不要求员工跳转到额外网页完成日常操作。

## 19.4 APPLICATION Principal

企业内部 ERP、CRM、OA、自研产品或服务可由管理员创建为 APPLICATION Principal。

管理员为应用配置：

```text
App Key
Group
Model Permission
Resource Permission
Budget
Rate Limit
Concurrency
```

应用直接复用平台已有标准 Gateway API，不建设新的应用专用模型协议。

典型接入：

```text
企业内部应用
    ↓
Base URL = Control Plane Gateway
App Key
    ↓
OpenAI Compatible / Anthropic Compatible API
    ↓
Model / Provider / Resource
```

应用与员工共享 Usage、Cost、Budget 统计，只通过 `principal_type` 区分来源。管理员对应用和成员的治理变更统一进入操作日志。

## 19.5 自助能力

V1 不建设 Principal Self-service Portal。

如出现真实需求，可为 MEMBER 提供轻量自助能力，例如：

```text
查看可用模型
查看剩余预算
查看已授权 Skill
查看已授权 MCP Tool
查看所属 Group
```

优先通过：

```text
CLI
MCP Tool
Client-native 能力
```

提供，而不是扩展成完整用户后台。
---

# 二十、分阶段实施范围

## 20.1 阶段实施原则

V2.6.6 将“目标架构”和“当前开发 Backlog”正式分离。

开发阶段统一遵循：

> **先验证 Client-native 企业治理价值，再扩展完整 Gateway 与企业能力。**

范围控制原则：

```text
砍实现范围
≠
砍安全底线 / 数据边界 / 架构接缝
```

P0-MVP 只实现真实验证需要的最小闭环；Model / Provider / Resource / Principal 等长期核心模型保持兼容，但不要求所有后台 CRUD、路由策略和商业能力同时上线。

---

## 20.2 P0-MVP：Claude Code Governance Slice

### 目标

> **用约两周级别的开发目标形成内部真实可用版本，验证企业是否需要以 MEMBER / Group 为单位治理 Claude Code 的模型访问和 Usage。**

时间目标仅用于约束范围，不作为产品承诺；如实现过程中发现范围无法维持，应继续缩功能，而不是扩大开发周期去追求完整平台。

### 核心链路

```text
Claude Code
    ↓
Anthropic Compatible Native Path
    ↓
Virtual Key
    ↓
MEMBER Principal
    ↓
Group
    ↓
Model Permission
    ↓
唯一启用 Provider Model
    ↓
唯一 ACTIVE Resource
    ↓
Anthropic Official API
    ↓
Usage Attribution
```

P0-MVP 必须让管理员和开发人员真实感知到 Control Plane 的差异化，而不是只看到一个 API Proxy。

### P0-MVP 最小范围

1. Single Organization，首次初始化默认 Organization。
2. Admin Username / Password 登录 + JWT，使用 `Authorization: Bearer <JWT>`。
3. MEMBER Principal；成员创建、状态管理、Virtual Key 创建 / Revoke。
4. Group；成员加入 Group；Group Model Allowlist。
5. 预置 Anthropic Official Provider / Model / Provider Model，不建设完整 Provider CRUD。
6. Resource 最小配置：Resource Name、Credential、状态；Credential 使用 AES-256-GCM + Master Key 加密。
7. Anthropic Messages Compatible Native Path。
8. SSE 原生透传、客户端主动取消、Anthropic Native Tool Call / Tool Result 原生透传。
9. 简化 Resource Resolver：一个逻辑 Model 只允许一个启用 Provider Model，一个 Provider Model 只允许一个 ACTIVE Resource。
10. `ai_request + usage_record(attempt)` 数据模型；P0-MVP `attempt_no` 正常情况下固定为 `1`。
11. Usage Writer 使用固定 goroutine + buffered channel + Batch Insert PostgreSQL。
12. 最小 `operation_log`：记录 Admin 对 MEMBER、Group、Virtual Key、Resource、Model Permission 的管理操作。
13. PostgreSQL + Migration。
14. `slog`、requestId、基础 OpenTelemetry 接缝、Health Check、Graceful Shutdown。
15. Docker Compose 和单文件运行能力。

P0-MVP 不要求 Redis 作为运行依赖。只有进入实时限流、跨节点缓存、Tool Call 跨协议映射等真实场景后再启用 Redis。

### P0-MVP 最小后台

首个内部版本控制在以下主要页面：

```text
成员
Group
模型
Resource
Usage
```

其中 Provider / Provider Model 采用 Bootstrap 预置数据；Model 页面以查看 / 启停 / Group 授权为主，不要求完成完整 Provider Catalog 管理体验。

### P0-MVP 保留的生产级地基

以下能力实现成本相对低，但安全或返工代价高，因此不因 MVP 而省略：

```text
Access Key 高熵随机生成 + Hash 存储
Provider Credential AES-256-GCM 加密
Master Key 持久化与丢失处理
PostgreSQL Migration
数据库字段 / COMMENT / NOT NULL / 软删除规范
Admin JWT 与 Principal Access Key 隔离
Admin API HTTP Status + Business Code + requestId
slog 日志
Graceful Shutdown
Usage 异步 Flush
基础自动测试
```

DDD 只保留清晰边界与显式依赖，不建设额外抽象层。

### P0-MVP 明确不实现

```text
APPLICATION Principal 的后台与调用闭环
完整 Provider CRUD / Provider Marketplace
Provider Model Price 历史体系
Cost Dashboard / Customer Billing
principal_resource / group_resource 的完整 Resource 权限治理
多 Provider Routing
Resource Health Score
Resource Failover
Budget
Rate Limit
Concurrency
Redis Stream
OpenAI / Codex Native Path
Cross-Protocol Adapter
Tool Call ID Cross-Protocol Mapping
Knowledge
Skill
Managed MCP
SSO / OIDC / LDAP / SCIM
多组织
Kubernetes
Managed Cloud
完整 en-US 翻译与国际发布物料
```

目标架构中的对应数据模型或接口可以保留设计，但不得因此扩大 P0-MVP 实现范围。

### P0-MVP Gate

P0-MVP 的核心验收不是“Gateway 功能有多全”，而是“真实团队是否持续使用治理能力”。

最低验证参考：

```text
真实开发成员 ≥ 5（建议 5～10 人）
连续真实使用 ≥ 14 天
累计真实请求 ≥ 500 次
Claude Code 长 SSE 与 Tool Loop 可稳定完成
Virtual Key Revoke 后立即失效
MEMBER / Group / Model Permission 生效准确
Usage 能准确归属到 MEMBER / Model / Resource
无已知高危 Credential / Permission 问题
```

管理员必须真实完成过：

```text
创建成员
创建 Group
分配成员到 Group
签发 Virtual Key
配置 Group Model Permission
配置 Resource Credential
查看成员 Usage
```

P0-MVP 通过后，再进入正式 Community P0 扩展；未通过时优先分析真实使用反馈，不以增加功能数量作为默认解决方案。

---

## 20.3 P0-A：Community Claude Native Path

目标：

> 在 P0-MVP 已验证员工 / Group 治理价值后，把 Claude Native Path 扩展成可对外发布的 Community 核心闭环。

链路保持：

```text
Claude Code
    ↓
Anthropic Compatible
    ↓
AI Coding Control Plane
    ↓
Anthropic Official / Compatible Provider
```

在 P0-MVP 基础上增加：

1. 完整 Model Catalog / Provider / Provider Model / Provider Model Price 管理。
2. Resource 多记录管理。
3. APPLICATION Principal / App Key 标准调用闭环。
4. principal_resource / group_resource 与正式 Resource Eligibility。
5. 基础 Resource Router。
6. Resource Health 与最小 Failover。
7. Usage / Cost 正式对账能力。
8. Redis 用于进入当前阶段后确有需要的缓存 / 实时治理状态。
9. 更完整 Admin CRUD 与 Operation Log。

P0-A 默认仍只要求每个逻辑 Model 启用一个 Provider Model，不实现平台自营 Provider 收费、预充值账务或复杂多 Provider 调度。

P0-A 不实现：

```text
Platform Provider Billing
Organization Prepaid Balance
复杂 Multi-Provider Routing
Partner Provider Commercial Flow
Cross-Protocol Adapter
OpenAI Responses → Anthropic
Anthropic → OpenAI
Thinking Mapping
Cross-Protocol Tool Call
```

---

## 20.4 P0-B：Codex Native Path

目标：

> 在不引入跨协议转换的情况下完成第二个核心 AI Coding Client 接入。

链路：

```text
Codex
    ↓
OpenAI Responses Compatible
    ↓
AI Coding Control Plane
    ↓
OpenAI Official API
```

重点验证：

```text
Responses API
SSE Event Sequence
Usage 提取
Model Selection
Client Cancellation
Error Semantics
Resource Router
```

P0-B 默认只启用 OpenAI Official 对应的单一 Provider Model，继续验证 Model / Provider / Resource 解耦后的 Native Path；不要求启用任何 Provider 商业计费能力。

APPLICATION Principal 同样可使用 App Key 调用 OpenAI Compatible / Responses 入口，权限、Usage、Cost 与 MEMBER 共用同一治理链路。

P0-B 完成后，平台具备：

```text
Claude Code → Anthropic
Codex       → OpenAI
```

两条独立 Native Path。

---

## 20.5 P0-C：Cross-Protocol Adapter

目标：

> 在 P0-A / P0-B 已稳定的基础上实现跨 Provider 协议适配。

典型链路：

```text
Claude Code
→ Anthropic Messages
→ Control Plane
→ OpenAI Responses
```

```text
Codex
→ OpenAI Responses
→ Control Plane
→ Anthropic Messages
```

范围：

```text
Capability Matrix
Tool Call / Tool Result
Thinking / Reasoning
Prompt Caching
Stop Reason
Usage
Error Mapping
Streaming SSE Conversion
Cross-Protocol Failover Boundary
```

P0-C 的失败不得影响 P0-A / P0-B 已验证的 Native Path。

P0-C 首个转换方向根据内部团队或首批用户的真实使用优先级选择，不在架构层写死。

---

## 20.6 正式 P0 Gate

P0-A / P0-B / P0-C 按实际产品需要逐步进入 Gate，不要求为了“完成文档”一次性全部实现。

正式 P0 量化参考：

```text
真实成员 ≥ 5
连续真实使用 ≥ 14 天
累计请求 ≥ 500 次
Usage / Cost 与官方账单对账误差 < 1%
关键能力自动测试全部通过
无已知高危 Credential / Permission / Routing 错误
```

Native Path 的 Client E2E 至少覆盖：

```text
普通对话
长 SSE
Tool Call
客户端主动取消
401
429
5xx
不存在模型
Resource Failover（进入该能力阶段后）
Usage
Cost（进入正式 Cost 阶段后）
```

Principal E2E 在 APPLICATION 能力进入当前 Release 后增加：

```text
MEMBER Principal → Virtual Key → Coding Client 正常调用
APPLICATION Principal → App Key → 标准 API 正常调用
两类 Principal 的 Permission / Usage / Cost 归属准确
Access Key Revoke 后立即失效
```

不能只使用 curl 或 Provider SDK 作为正式客户端验收。

---

## P1：Enterprise Knowledge 最小闭环

只有 Model Governance 已产生持续真实使用后，再实现最小知识链路：

```text
上传公司文档
    ↓
Parse
    ↓
Chunk
    ↓
管理员配置 Embedding Model
    ↓
Embedding
    ↓
PostgreSQL + pgvector
    ↓
knowledge_search MCP
    ↓
Codex / Claude Code
```

范围：

1. `knowledge_base`
2. `knowledge_document`
3. `knowledge_chunk`
4. Embedding Model 配置
5. pgvector
6. 基础知识库权限
7. `knowledge_search` MCP
8. 检索结果来源引用

不扩展为完整企业知识管理平台。

## P2：Skill 与 Managed MCP

### Skill Registry

范围：

```text
Skill Registry
Version
Permission
Sync
```

充分利用 Codex / Claude Code 原生 Skill 能力，不实现独立 Agent Runtime。

### Managed MCP

只增加实际受管工具。

例如：

```text
image_generate
    ↓
Gemini Image
```

员工和企业内部应用统一使用企业 AI Resource，公司统一统计：

```text
Principal
Usage
Cost
Permission
Operation Log
```

不做通用 MCP Marketplace。

## P3：组织级扩展能力

按企业采购和组织治理需求扩展：

1. Role / Group 高级策略
2. Role → Resource
3. Role → Knowledge
4. Role → Skill
5. 更多 Managed MCP Tool
6. Knowledge Connector
7. 多组织
8. 高可用
9. Kubernetes
10. 复杂 Project / Cost Center
11. 高级企业审计

SSO / OIDC / LDAP / SCIM 属于企业采购准入能力，可根据首批付费客户要求提前进入对应 Enterprise Release，不与 P3 固定绑定。

---

# 二十一、国际化与开源发布规范

目标架构按国际化结构开发，但 P0-MVP 只要求 **i18n-ready**，不把完整双语翻译作为验证 Gate。

正式开源发布目标语言：

```text
en-US
zh-CN
```

P0-MVP 以中文内部使用为优先，同时保持数据库状态值、API Error Code、代码常量等稳定机器语义为英文；前端保留 i18n Key 结构，但 `en-US` 完整翻译、国际官网与完整双语文档可以在准备公开发布时补齐。

正式公开发布后，英文作为 GitHub、国际官网和默认开源文档语言；中文提供完整对应版本。

---

## 前端国际化

Vue 前端所有可见文本使用 i18n Key，不在组件中直接写死中文或英文。

例如：

```text
resource.add
member.group
usage.cost
knowledge.search
```

语言资源：

```text
locales/
├── en-US.json
└── zh-CN.json
```

增加其他语言时只扩展语言资源，不修改业务逻辑。

---

## 后端 API 响应与错误码

平台明确区分 **Admin API** 与 **Protocol API（Gateway / MCP）** 的响应契约。

### Admin API

Admin API 使用统一 JSON 响应结构，但 HTTP Status 与业务 Error Code 分工明确：

```text
HTTP Status
→ 表达标准 HTTP / 协议语义

Business Code
→ 表达稳定、机器可识别的业务语义
```

成功响应：

```json
{
  "code": "OK",
  "data": {},
  "requestId": "req_xxx"
}
```

错误响应：

```json
{
  "code": "MODEL_PERMISSION_DENIED",
  "message": "You do not have permission to use this model.",
  "requestId": "req_xxx"
}
```

可选错误详情：

```json
{
  "code": "INVALID_ARGUMENT",
  "message": "Invalid request parameter.",
  "details": {},
  "requestId": "req_xxx"
}
```

规则：

1. 正确使用 `2xx / 4xx / 5xx` HTTP Status，不采用“所有请求固定 HTTP 200，再用业务 code 表达失败”的模式。
2. `code` 使用稳定字符串，不使用难以维护的纯数字业务码。
3. `code` 属于 API 契约，发布后不得随意改名；前端业务分支只依赖 `code`，不得依赖 `message` 文本。
4. `message` 只作为人类可读的调试 / Fallback 文案，可以调整和国际化，不作为稳定程序判断条件。
5. 成功响应默认省略无意义的 `message: success`。
6. `requestId` 从第一版保留，同时写入响应 Header（`X-Request-ID`）并在 Admin API JSON Body 中返回，用于关联 Gateway Log、Usage、Operation Log、Trace 和 Provider Error。
7. 分页数据统一放在 `data` 中，例如 `items / page / pageSize / total`。

推荐 HTTP Status 与业务码关系：

```text
400 → INVALID_ARGUMENT
401 → UNAUTHENTICATED / ACCESS_KEY_INVALID
403 → PERMISSION_DENIED / MODEL_PERMISSION_DENIED
404 → PROVIDER_NOT_FOUND / MODEL_NOT_FOUND / RESOURCE_NOT_FOUND
409 → RESOURCE_CONFLICT
429 → RATE_LIMIT_EXCEEDED / BUDGET_EXCEEDED
500 → INTERNAL_ERROR
```

### 前端国际化

后端不把自然语言错误文案作为稳定协议。统一返回机器可识别的字符串 Error Code，例如：

```text
MODEL_NOT_AVAILABLE
MODEL_PERMISSION_DENIED
RESOURCE_UNAVAILABLE
RESOURCE_BALANCE_INSUFFICIENT
BUDGET_EXCEEDED
RATE_LIMIT_EXCEEDED
PERMISSION_DENIED
```

前端优先通过 Error Code 映射本地化文案：

```text
有本地 i18n(code)
→ 使用本地化文案

没有对应翻译
→ Fallback 到后端 message
```

### Protocol API

OpenAI Compatible、Anthropic Compatible、MCP 与 SSE 不套 Admin API 的 `code / data / message / requestId` 统一包装，必须保持客户端原生协议兼容：

```text
/openai/*    → OpenAI Native Response / Error / Streaming
/anthropic/* → Anthropic Native Response / Error / Streaming
Managed MCP  → MCP / JSON-RPC Native Response
SSE          → 原生事件流格式
```

内部仍生成并传播统一 `requestId / traceId`，但不得通过额外 JSON Wrapper 破坏 OpenAI、Anthropic、MCP 或 SSE 协议结构。需要暴露请求标识时优先使用协议允许的 Header 或原生字段。

---

## README 与文档

开源仓库

```text
README.md
→ English

README.zh-CN.md
→ 简体中文

docs/
├── en/
└── zh-CN/
```

Provider、部署、MCP、Knowledge、Skill 等核心文档保持中英文结构一致。

---

## 内容语言

平台不限制以下内容语言：

```text
Knowledge Document
Skill
MCP Tool Description
Model Display Name
```

企业可以根据自身团队使用中文、英文或其他语言。

Knowledge 的多语言检索能力取决于管理员配置的 Embedding Model。

---

# 二十二、产品边界

Application Access 只作为统一 Principal 治理模型下的标准 API 接入能力，不把产品扩展为通用低代码 AI 平台、业务应用开发平台或 Agent Runtime。

产品范围不包含：

- 个人订阅账号池化
- 非正规 API Key 倒卖或绕过 Provider 服务地区 / 账户规则的转售
- 任意第三方 Provider Marketplace
- 复杂 Weighted Routing
- 复杂 Project / Cost Center
- 自动模型价格同步
- 实时外汇换算
- 完整企业知识管理平台（Knowledge 仅提供 AI Coding 所需的轻量检索能力）
- 知识图谱
- 复杂 Agentic RAG
- MCP Marketplace
- 完整 Agent Runtime
- 自研 Codex / Claude Code 客户端能力

允许作为未来正规商业扩展、但不进入 P0 核心范围的能力：

```text
同一 Model 多 Provider
平台自营 Provider
正规 Provider 代理 / 合作渠道
PLATFORM_MANAGED / PARTNER_MANAGED Resource
Organization Prepaid Balance
Rate Card
Billing Ledger
Balance Ledger
企业账单 / 对账
```

这些能力必须建立在合法、正规的 Provider 商业合作基础上，并始终作为 Control Plane 治理体系的扩展，不把产品转型为单纯模型转售平台。

---

# 二十三、实现约束

本节描述目标架构的长期实现约束。标记为 P0 / P1 / P2 的能力只在进入对应开发阶段后生效；P0-MVP 以第二十章的最小开发范围为唯一当前 Backlog，不要求提前完成目标架构中的全部能力。

P0-MVP 特别约束：

- 只实现 MEMBER Principal 的真实使用闭环，APPLICATION 保留目标模型但不进入首个验证范围。
- Anthropic Provider / Model / Provider Model 允许通过 Bootstrap 预置，不要求完整 Provider CRUD。
- 一个逻辑 Model 只解析到一个启用 Provider Model，一个 Provider Model 只使用一个 ACTIVE Resource，不实现 Health Score、Failover 或多 Provider Routing。
- P0-MVP 不要求 Redis 作为运行依赖；Redis 在进入实时限流、跨节点缓存、Cross-Protocol Tool Call ID Mapping 等真实需求时再启用。
- P0-MVP 只验证 Usage Attribution，不以完整 Cost、Provider Price History、Budget / Rate Limit / Concurrency 为验收条件。
- 保留 Access Key Hash、Credential AES-256-GCM、Master Key、Migration、日志、Request ID、Graceful Shutdown 等基础安全和工程能力。

- 系统核心治理主体统一使用 `Principal`，当前仅支持 `MEMBER` 与 `APPLICATION` 两种 `principal_type`。
- `principal` 第一版业务字段只保留 `organization_id / principal_type / name / remark / status`，并统一包含 `id / is_deleted / created_by / updated_by / created_at / updated_at` 通用字段；不增加手机号、邮箱、部门、external_ref 或通用 JSONB Profile。
- MEMBER 与 APPLICATION 共用 `access_key`、Group、Permission、Budget、Rate Limit、Concurrency、Usage、Cost 等治理能力；后台仅按类型分别展示“成员管理”和“应用管理”，管理变更统一进入 `operation_log`。
- Access Key 只识别 Principal，不承载权限；完整 Key 只展示一次，数据库只保存 hash + prefix；P0 使用高熵随机 Key + SHA-256 摘要，不使用可逆加密存储 Access Key。
- Provider / Proxy Credential 必须使用 AES-256-GCM 等标准 AEAD 算法可逆加密；Master Key 位于程序和数据库之外，禁止自研加密算法或把 Master Key 硬编码进源码 / 镜像。
- 管理面与调用面认证严格隔离：Admin 使用登录会话 / JWT，Principal 使用 Access Key；Admin JWT 不能调用 Gateway / MCP，Principal Access Key 不能访问 Admin API。
- P0 Admin Authorization 保持最小角色模型；Principal Authorization 继续采用 Organization Default → Group Policy → Principal Override，不把两类权限混入同一 Permission 模型。
- 企业 Application 复用现有标准 Gateway API，不建设独立应用协议或独立路由链路。
- `ai_model` 与 `ai_provider` 解耦，必须通过 `provider_model` 表达 Provider 对 Model 的服务关系。
- 普通 Principal 只能使用授权 Model，不得选择 Provider / Resource；Provider Policy 只由管理员配置。
- Provider 变更不得要求 MEMBER 修改 Virtual Key、APPLICATION 修改 App Key，或要求任何 Principal 修改逻辑 Model 名称。
- Resource Credential 永远不下发给普通 Principal。
- Usage Cost 与 Customer Billing 必须保持独立语义；未来 Provider 收费不得覆盖历史 Usage 成本事实。
- P0 只验证官方 / 单一 Provider Native Path；多 Provider、预充值和渠道商业能力仅预留扩展点。
- Provider 必须保持轻量接入：兼容既有 OpenAI / Anthropic 协议的 Provider 原则上通过配置完成接入，不修改 Gateway 核心代码。
- 只有协议、认证或请求语义无法由现有兼容层覆盖时，才新增 `ProviderAdapter`；Adapter 只处理技术适配，不承载渠道合同、充值、结算、分润等商业逻辑。
- 不为单一合作 Provider 建立专属表结构或专属治理链路；所有 Provider 统一复用 `provider_model`、`ai_resource`、Permission、Usage、Cost、Health 与 Operation Log。
- Community Edition 内置主流 Provider / Model 模板以降低首次使用门槛，但预置模板不得形成 Provider / Model 白名单或限制管理员自定义接入。
- Provider、Model、Provider Model、Resource、Price、Permission、Routing Policy 的运行时配置必须数据库化；禁止把这些业务配置长期依赖 YAML / JSON 文件并要求重启后生效。
- 配置文件 / 环境变量只承载系统启动参数与可选 Bootstrap 数据；Bootstrap 只在首次初始化使用，不得覆盖数据库中管理员已经修改的运行时配置。
- Gateway Native Path 与 Cross-Protocol Adapter 独立演进，协议转换失败不得影响 Native Path。
- Capability Matrix 中 `DEGRADED` 必须显式启用，`UNSUPPORTED` 必须在 Streaming 开始前快速失败。
- Tool Call 无损验收以真实客户端完整 Tool Loop 为准，不以 JSON 字段映射完成为准。
- Cross-Protocol Adapter 阶段的 Tool Call ID Mapping 使用 Redis 会话级状态并纳入并发串扰测试；P0-MVP Anthropic Native Path 不需要该映射。
- 进入 Protocol Adapter 阶段后，Tool Call 回归套件纳入 CI；P0-MVP 优先验证 Anthropic Native Tool Loop。
- Resource Failover 仅针对可重试错误，已开始 Streaming 后不做同请求透明切换。
- Usage 同时支持 TOKEN / CALL / IMAGE / SECOND 等计费单位。
- Health 权重可配置；Provider 缺失健康信号时按有效信号重新归一化。
- Group Policy 只表达授予；P0 Resource 权限取 `Group Grant ∪ Principal Grant`，不支持 Principal Resource 收窄；Model / 数值限制按明确的 Principal Override 规则处理。
- Principal / Group / Resource / Model / Budget 等治理变更进入 `operation_log`。
- SSO / OIDC / LDAP / SCIM 根据企业采购准入要求决定 Enterprise Release 优先级。
- Outbound Proxy 默认直连；代理认证信息与 Provider Credential 同级保护，代理 Header 不得透传给 Provider。
- 普通 MEMBER / APPLICATION Principal 不访问 Web 管理后台。
- 限制策略采用稀疏配置：未配置即不限制，Resource 权限除外，Resource 必须显式授权。
- P0 Resource 权限只做 `Group Grant ∪ Principal Grant`，不实现 Deny 或 Principal Resource 收窄。
- 数据库业务表默认使用 BIGINT 应用侧 ID、中文 COMMENT、默认 NOT NULL；状态 / 类型值统一使用大写英文编码。
- 逻辑删除默认不可恢复；重新创建同业务标识时生成新 ID，业务唯一索引仅约束 `is_deleted = false`。
- PostgreSQL 不创建 Foreign Key Constraint，不使用 Cascade Delete；关联完整性由 Application / Domain 层保证。
- Usage 从第一版区分 `ai_request` 与 `usage_record(attempt)`，Retry / Failover 不覆盖历史 Attempt。
- P0 Usage Writer 使用固定 goroutine + buffered channel + Batch Insert PostgreSQL；不引入 Redis Stream，Queue 满不得静默丢数据，Graceful Shutdown 必须 Flush。
- Admin JWT 使用 `Authorization: Bearer <JWT>`，不复用 Principal Access Key。
- `operation_log` 是 Append Only 表，不包含 `is_deleted / created_by / updated_by / updated_at`，普通业务流程不得修改或逻辑删除操作日志。

---

# 二十四、产品能力总结

最终平台形成四个核心能力：

```text
                     AI Coding Governance
                             │
        ┌────────────────────┼────────────────────┐
        │                    │                    │
  Model Governance      Tool Governance      Skill Governance
        │                    │                    │
 Gateway / Resource      Managed MCP          Skill Registry
        │                    │                    │
        └────────────────────┼────────────────────┘
                             │
                    Knowledge Governance
                             │
                     Enterprise Knowledge
```

企业最终获得：

```text
公司统一定义并采购 / 接入的模型能力
+
公司统一提供的 MCP 工具
+
公司内部知识
+
公司内部 Skill
```

员工继续使用：

```text
Codex / Claude Code / 其他 AI Coding Client
```

平台不取代客户端，而是成为：

> **企业 AI Coding 能力的统一治理与分发底座。**

其中开发人员只面向 Model 工作；Provider、Resource、采购渠道与未来 Billing 均属于管理员治理面。

---

# 二十五、方案总结

AI Coding Control Plane 由四个核心能力模块组成：

```text
Model Gateway
+
Enterprise Knowledge
+
Skill Registry
+
Managed MCP
```

统一复用：

```text
Principal
Access Key
Group
Permission
AI Resource
Usage
Cost
Operation Log
Health
```

Principal 治理采用：

```text
Group Default
+
Principal Override
```

管理后台只面向管理员；MEMBER 通过 Virtual Key 在 Codex / Claude Code 等客户端中使用企业 AI 能力，APPLICATION 通过 App Key 调用同一标准 Gateway API。

员工继续使用 Codex、Claude Code 等原生客户端，平台负责把企业采购的模型、公司知识、内部 Skill 和受管工具统一接入员工日常 AI Coding 工作流。

开发顺序调整为：

```text
P0-MVP Claude Code Governance Slice
        ↓
P0-A Community Claude Native Path
        ↓
P0-B Codex Native Path
        ↓
P0-C Cross-Protocol Adapter
        ↓
P1 Enterprise Knowledge
        ↓
P2 Skill + Managed MCP
        ↓
P3 组织级治理
```

其中：

> **P0-MVP 必须先证明 MEMBER / Group 级 AI Coding 治理存在持续真实使用价值；Knowledge / Skill / MCP 不参与这一验证。**

> **正式 P0 仍必须在没有 Knowledge / Skill / MCP 的情况下独立成立。**

数据库从项目开始即采用 PostgreSQL，但：

```text
P0       → 普通关系数据库能力
Knowledge → 启用 pgvector
```

技术路线：

```text
Go + DDD
chi + net/http
pgx + sqlc
PostgreSQL
go-redis（正式 P0 / 实时治理阶段；P0-MVP 可不启用）
slog
OpenTelemetry + Prometheus
Vue 3
Nginx
Docker Compose
Outbound Proxy（可选）

Knowledge 阶段：
PostgreSQL + pgvector
```

核心原则：

> **Client-native First，Gateway-first 实施；AI Coding Control Plane 负责治理与能力分发，不重造客户端。**
