# GitHub 仓库接入指南（部署后）

[English version](github-repository-integration.en.md)

本文说明如何把一个 GitHub 仓库接入已部署的 SDLC Coordinator。它不是完整部署手册，也不介绍创建组织、配置 AI Provider 或安装 Runner。

## 接入前准备

- 目标 GitHub 仓库已经创建，并且你有权管理它。
- 你能登录目标组织在 Coordinator 中的租户管理员账号。
- Coordinator 有一个公网可访问、HTTPS 有效的地址；反向代理会把请求转发到 Coordinator。
- 准备一个随机生成的 Webhook Secret。它是 webhook 签名密钥，与 GitHub PAT 是两项不同的凭据。

如果仓库尚未创建，请先由 GitHub 组织管理员在目标组织下创建；接下来所有仓库路径都使用 `组织名/仓库名`，例如 `acme/service-api`。

## 1. 创建只授权目标仓库的 Fine-grained PAT

在 GitHub 打开 [Fine-grained personal access tokens](https://github.com/settings/personal-access-tokens/new)，然后按下面设置：

1. **Resource owner** 选仓库所属的个人账号或组织。组织可能要求组织管理员审批新令牌；如果状态是 Pending，请先完成审批。
2. **Repository access** 选 **Only select repositories**，并且只选这次要接入的仓库。
3. 在 **Repository permissions** 中设置：

   | 权限 | 等级 | 用途 |
   |---|---|---|
   | **Contents** | **Read and write** | Runner 读取仓库、推送开发分支；Coordinator 需要 Contents 写权限来合并 PR。 |
   | **Issues** | **Read and write** | 读取和更新 Issue、写入 Issue/PR 普通评论、管理 Issue 标签。 |
   | **Pull requests** | **Read and write** | 读取和创建 PR，以及执行 PR 相关操作。 |
   | **Metadata** | **Read-only** | 仓库基本信息；GitHub 通常会自动加入此项。 |

   GitHub 官方权限表分别列出了创建 PR、合并 PR、Issue/评论操作所需的权限；本项目通过仓库设置页面手动创建 Webhook，因此 PAT 不需要 **Webhooks** 或 **Administration** 权限。
4. 选择符合组织政策的到期时间，创建令牌，并在令牌首次显示时安全复制。不要把 PAT 放进 Issue、聊天、文档或 Git 仓库。

参考：[GitHub Fine-grained PAT 权限表](https://docs.github.com/en/rest/authentication/permissions-required-for-fine-grained-personal-access-tokens)、[管理个人访问令牌](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens)。

## 2. 在 Coordinator 登记并启用仓库

使用组织的租户管理员账号打开 **仓库管理 → 新增仓库**，填写：

| 表单项 | 填写方式 |
|---|---|
| **显示名称** | 便于在 Coordinator 中识别的名称。 |
| **平台** | `GitHub`。 |
| **平台地址** | `https://github.com`。 |
| **仓库路径** | `owner/repository`，不要粘贴带 `/settings` 的网页地址或 `.git` 后缀。 |
| **项目级 Token** | 粘贴上一步创建的 Fine-grained PAT。 |
| **项目级 Token 确认框** | 确认该令牌只授权给这个仓库后勾选。 |
| **Webhook Secret** | 填入准备好的随机密钥，并留存一份用于下一步。不要填 PAT。 |
| **验证命令** | 可选，填写 Runner 中确实可执行的测试或构建命令，例如 Go 项目的 `go test ./...`。该命令会在自动验证中执行。 |
| **提交作者姓名 / 邮箱** | 可选；如填写，两项需成对配置，而且会公开出现在 Git 提交历史。使用经授权的机器人身份或 no-reply 邮箱。 |

保存后，Coordinator 会用 PAT 查询仓库身份和默认分支。成功后，在仓库行上启用该仓库，并确认状态为 **enabled**、最近配置错误为空。若身份探测失败，先核对组织审批、仓库选择、PAT 权限和 `owner/repository` 路径。

## 3. 配置 GitHub Repository Webhook

打开目标仓库的 **Settings → Webhooks → Add webhook**，填写：

| 字段 | 填写方式 |
|---|---|
| **Payload URL** | `https://<你的 Coordinator 公网域名>/api/events/github`。把示例域名替换成实际 HTTPS 域名；不要填写 MCP 回调地址或 GitHub 仓库 URL。 |
| **Content type** | `application/json`。 |
| **Secret** | 与 Coordinator 仓库登记表中的 **Webhook Secret** 完全一致。 |
| **SSL verification** | 保持启用。 |
| **Which events would you like to trigger this webhook?** | 选择 **Let me select individual events**，只勾选下表三项。 |
| **Active** | 勾选。 |

只订阅以下事件：

| GitHub 事件 | 本项目使用方式 |
|---|---|
| **Issues** | 新建 Issue，或给 Issue 添加 `developer-agent` 标签。 |
| **Issue comments** | 接收 Issue/PR 普通评论事件；Coordinator 会再检查评论内容和发起人的授权。 |
| **Pull requests** | PR 创建、重新打开、关闭，以及源分支增量更新。增量更新对应 `synchronize`，所以不需要额外勾选 **Pushes**。 |

然后点击 **Add webhook**。GitHub 会自动发送 `ping` 投递。成功到达时，Coordinator 可以把它记为“已忽略”，因为 `ping` 只验证投递和签名，不会创建任务；GitHub Recent Deliveries 应显示成功的 2xx 响应。

参考：[创建仓库 Webhook](https://docs.github.com/en/webhooks/using-webhooks/creating-webhooks)、[Webhook 事件和载荷](https://docs.github.com/en/webhooks/webhook-events-and-payloads)。

## 4. 做一次受控的首次验证

1. 在 GitHub **Settings → Webhooks → Recent Deliveries** 确认首次 `ping` 的响应是 2xx。`ping` 在 Coordinator 中显示 ignored 是正常的。
2. 确认 Coordinator 仓库管理页显示仓库为 **enabled**，且 PAT、Webhook Secret 都显示已配置。页面不会再次显示密钥原文。
3. 首次任务会真实启动 Agent 并可能创建分支或 PR。请先用一个范围很小、允许自动修改的测试 Issue 验证，不要直接用高风险生产需求。
4. 新建 Issue 后添加精确名称为 `developer-agent` 的标签。该标签是当前 GitHub Issue 工作流的启动门槛；仅创建没有该标签的 Issue 不会启动开发任务。
5. 查看 Coordinator 的任务详情以及 GitHub 分支/PR，确认开发、审查和测试结果都对应最新提交。

组织级 **自动合并** 是单独的策略开关，默认关闭。首次试用建议先人工检查 PR 和测试结果；只有在组织明确决定后，才开启自动合并。

## 常见问题

| 现象 | 优先检查 |
|---|---|
| 仓库启用失败或出现身份探测错误 | Fine-grained PAT 是否已获组织审批、是否只选了正确仓库、Contents/Issues/Pull requests 权限是否符合上文、仓库路径是否为 `owner/repository`。 |
| GitHub 投递返回 401 | Webhook Secret 是否与 Coordinator 保存的值完全一致；仓库是否已经登记；Payload URL 的仓库路径是否正确。 |
| 投递失败、超时或返回 5xx | HTTPS 证书、反向代理转发、Coordinator 公网可达性和服务状态。 |
| `ping` 投递成功但显示 ignored | 正常。`ping` 用于确认入口，不能替代 Issue/PR 的实际流程验证。 |
| Issue 没有启动任务 | Issue 是否处于打开状态、是否添加了精确标签 `developer-agent`，以及 Webhook 是否订阅 **Issues**。 |
| PR 增量提交没有触发更新 | Webhook 是否订阅 **Pull requests**，并在 Recent Deliveries 中检查该投递；增量更新是 PR 的 `synchronize` 动作，不是单独的 `push` 事件。 |
