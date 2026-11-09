# Gitea 仓库接入指南（部署后）

[English version](gitea-repository-integration.en.md)

本文说明如何把一个 Gitea 仓库接入已部署的 SDLC Coordinator。它不是完整部署手册，也不介绍创建组织、配置 AI Provider 或安装 Runner。

## 接入前准备

- 目标 Gitea 仓库已经创建，并且你有权管理它。
- 你能登录目标组织在 Coordinator 中的租户管理员账号。
- Coordinator 的 Webhook 入口地址对 **Gitea 实例**可达，而不是只对你浏览器可达。
- 准备一个随机生成的 Webhook Secret。它是 webhook 签名密钥，与访问令牌是两项不同的凭据。

仓库路径统一使用 `owner/repository`，例如 `acme/service-api`。

### 关于“平台地址”

登记仓库时填写的平台地址，是 Coordinator 与 Runner **在运行时**访问 Gitea 的地址，会被同时用于两类操作：调用 `/api/v1/...` 接口，以及拼接 Git 克隆地址 `平台地址/owner/repository.git`。

因此：

- 分体部署时填 Gitea 的公网或内网可达地址，例如 `https://gitea.example.com`。
- Gitea 与 Coordinator 在同一容器网络时，填网络内的服务名与端口，例如 `http://gitea:3000`。
- 不要填浏览器地址栏里的 `localhost:端口`：容器里的 `localhost` 指向容器自己，克隆会必然失败。

## 1. 准备只覆盖目标仓库的访问令牌

Gitea 的个人访问令牌是**账号级 + 权限范围**，没有 GitHub 那种“只授权单个仓库”的令牌类型。要让实际可达范围收敛到一个仓库，做法是给机器人账号只授予该仓库的协作者权限，再用这个账号签发令牌。

在 Gitea **设置 → 应用 → 生成令牌**，按下面选择权限范围：

| 权限范围 | 等级 | 用途 |
|---|---|---|
| **repository** | **读 / 写** | Runner 读取仓库、推送开发分支；Coordinator 读取 PR 状态并执行合并。 |
| **issue** | **读 / 写** | 读取 Issue、写入 Issue/PR 评论、管理标签。 |
| **user** | **读**（可选） | 解析令牌所属账号身份，用于提交作者与审计。 |

创建后立即安全复制令牌。不要把令牌放进 Issue、聊天、文档或 Git 仓库。

> 登记页有一个“确认这是项目级（repository 级）令牌”的勾选框，它是必填项。由于 Gitea 没有仓库级令牌，请用“专用账号 + 仅该仓库协作者”满足其真实意图，而不是拿实例管理员令牌直接勾选。

## 2. 在 Coordinator 登记并启用仓库

使用组织的租户管理员账号打开 **仓库管理 → 新增仓库**，填写：

| 表单项 | 填写方式 |
|---|---|
| **显示名称** | 便于在 Coordinator 中识别的名称。 |
| **平台** | `Gitea`。 |
| **平台地址** | 见上文“关于平台地址”，必须是 Coordinator 与 Runner 可达的基地址。 |
| **仓库路径** | `owner/repository`；不要粘贴带 `/settings` 的网页地址或追加 `.git`。 |
| **项目级 Token** | 粘贴上一步创建的令牌。 |
| **项目级 Token 确认框** | 确认令牌可达范围只覆盖这个仓库后勾选。 |
| **Webhook Secret** | 填入准备好的随机密钥，并留存一份用于下一步。不要填令牌。 |
| **验证命令** | 可选，填写 Runner 中确实可执行的测试或构建命令，例如 Go 项目的 `go test ./...`。该命令会在自动验证中执行。 |
| **提交作者姓名 / 邮箱** | 可选；如填写，两项需成对配置，而且会公开出现在 Git 提交历史。留空则使用内置身份。 |

保存时 Coordinator 会立即用令牌请求 `/api/v1/repos/owner/repository` 探测仓库身份与默认分支。探测失败会直接报错且不落库。成功后确认仓库状态为 **enabled**、最近配置错误为空。

## 3. 配置 Gitea Webhook

打开目标仓库的 **设置 → Web 钩子 → 添加 Web 钩子 → Gitea**，填写：

| 字段 | 填写方式 |
|---|---|
| **目标 URL** | `<Coordinator 地址>/api/events/gitea`。必须是 Gitea 实例可达的地址；不要填 MCP 回调地址或仓库网页地址。 |
| **HTTP 方法** | `POST`。 |
| **POST 内容类型** | `application/json`。 |
| **Webhook name** | 便于识别的名称，例如 `sdlc-coordinator`。 |
| **激活** | 勾选。 |
| **授权标头** | 留空。签名走独立请求头，不使用 Authorization。 |
| **密钥** | 与 Coordinator 仓库登记表中的 **Webhook Secret** 完全一致，且不要带任何前后空白。 |
| **分支过滤** | 保持 `*`。 |

只订阅以下事件（在“自定义事件”下勾选）：

| Gitea 事件 | 本项目使用方式 |
|---|---|
| **工单** | Issue 以带标签的方式创建时启动任务。 |
| **工单增删标签** | UI 常见流程是先建 Issue 再打标签，此时触发的是 `label_updated`，与“创建时已带标签”同等对待。 |
| **工单评论** | 接收 Issue 上的评论，用于返工与追问。 |
| **合并请求** | PR 创建、重新打开、关闭的生命周期。 |
| **合并请求已评论** | 接收 PR 上的评论，用于返工。 |
| **合并请求已同步** | 源分支推入新提交时 PR 头部变化，需要重新审查与验证。 |

不需要勾选 **推送**：PR 的增量提交由“合并请求已同步”覆盖，单独订阅推送只会产生被系统忽略的噪声事件。也不需要勾选工单指派、里程碑、标签事件之外的仓库事件、发布、状态、百科、软件包与工作流事件。

### 签名方式

Gitea 发送的 `X-Gitea-Signature` 是 `HMAC-SHA256(secret, 请求体)` 的十六进制字符串，**没有 `sha256=` 前缀**，比较时区分大小写。

Coordinator 在保存 Webhook Secret 时会去掉首尾空白，而 Gitea 按原样计算签名。因此把密钥粘贴进 Gitea 时若带入了换行或空格，验签会恒定失败并返回 401。建议粘贴后肉眼确认末尾没有多余字符。

## 4. 做一次受控的首次验证

1. 在 Gitea 的 Webhook 详情页点击 **测试推送**，最近投递应返回 2xx。测试推送的载荷是 Gitea 自己造的样例，通常会在 Coordinator 中显示 `ignored`，这只能证明入口与签名通了，不代表流程通了。
2. 确认 Coordinator 仓库管理页显示仓库为 **enabled**，且令牌、Webhook Secret 都显示已配置。页面不会再次显示密钥原文。
3. 首次任务会真实启动 Agent 并可能创建分支或 PR。请先用一个范围很小、允许自动修改的测试 Issue 验证。
4. 给该 Issue 添加精确名称为 `developer-agent` 的标签。这是当前工作流的启动门槛；仅创建没有该标签的 Issue 不会启动任务。
5. 查看 Coordinator 的任务详情以及 Gitea 分支/PR，确认开发、审查和测试结果都对应最新提交。

组织级 **自动合并** 是单独的策略开关，默认关闭。开启后，只有以下条件同时成立才会真正合并：

- 验证任务的 `verification_verdict` 为通过，且其绑定头部与任务头部一致；
- 审查任务的 `review_verdict` 为批准，且审查的头部与验证头部相同；
- 仓库记录存在且为 enabled，平台具备合并能力；
- 合并前读回的实时头部仍等于证据头部。

任一条件不成立即回落人工，并写入合并门禁审计。

## 与 GitHub 接入的差异

| 方面 | Gitea 的差异 |
|---|---|
| 令牌粒度 | 账号级 + 权限范围，没有仓库级令牌；用专用账号收敛可达范围。 |
| 签名头 | `X-Gitea-Signature`，十六进制，无 `sha256=` 前缀。 |
| 事件命名 | 用 `issue_label`、`pull_request_sync` 等独立事件类型，而非统一的 `issues`/`pull_request` 动作。 |
| PR 评论 | Gitea 的 PR 评论在接口层等同 Issue 评论，通过 `/issues/{index}/comments` 写入。 |
| 合并响应 | 合并成功返回空 `200`，不携带结果，必须回读 PR 才能确认 `merged` 与合并提交号。 |

## 常见问题

| 现象 | 优先检查 |
|---|---|
| 仓库启用失败或身份探测错误 | 令牌是否有效、是否被加入该仓库协作者、repository 权限是否为读、`owner/repository` 路径是否正确、平台地址是否从 Coordinator 侧可达。 |
| Webhook 投递返回 401 | Webhook Secret 是否与 Coordinator 保存值完全一致（重点排查首尾空白与换行）；仓库是否已经登记；路径是否为 `owner/repository`。 |
| 测试推送返回 2xx 但显示 `ignored` | 正常。测试载荷通常不带 `developer-agent` 标签，入口验证成功不等于流程验证成功。 |
| Issue 没有启动任务 | Issue 是否打开、标签名是否精确为 `developer-agent`、是否订阅了 **工单** 与 **工单增删标签**。 |
| PR 增量提交没有重新审查 | 是否订阅 **合并请求已同步**；该事件对应 PR 的 `synchronized` 动作，不是独立 push。 |
| 到了合并环节却没有自动合并 | 组织级自动合并是否开启；调度是否在运行（门禁随每轮 tick 执行）；证据链是否全绿；实时头部是否已漂移；平台是否拒绝。可查 `merge_gate_audits` 表中的 decision 与原因。 |
