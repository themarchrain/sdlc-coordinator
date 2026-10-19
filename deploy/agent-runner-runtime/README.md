# Agent Runner 投放目录

Codex 官方包放到本目录后，Dockerfile 只 `COPY` 再展开，不在构建时下载 Agent CLI。开发工具链通过 Alpine 软件仓库在构建时安装，需要访问软件源。目录相对仓库根为 `deploy/agent-runner-runtime/`。大包不要提交 Git，只提交本 README、`materialize.sh` 和各子目录的 `.gitkeep`。

## 镜像运行时来源

| 工具 | 来源 | 说明 |
|---|---|---|
| codex | **本目录投放** `codex-package/` | **必投**；musl 静态二进制，alpine 原生兼容 |
| claude | **本目录投放**（可选） | 只投 musl 变体；目录为空或缺席时跳过 |
| go 1.25 | 镜像内复制自 builder 阶段 | 不投放；`GOPROXY` 默认 `proxy.golang.org` |
| JDK 8 / 17 / 21 | Alpine 3.24 apk | 默认使用 21，通过 JAVA8_HOME、JAVA17_HOME、JAVA21_HOME 选择版本 |
| Node.js 24 / npm / pnpm 11 / Maven 3.9 | Alpine 3.24 apk | 构建时必装，不再从投放目录覆盖 |
| Python / pip / git / bash / su-exec | Alpine 3.24 apk | Python 支持项目虚拟环境 |

Claude 投放子目录形态与 Codex 相同：已解压的树（含 `bin/`）或一个 tar（`.tar` / `.tar.gz` / `.tar.xz`）。旧 node、jdk、maven 投放目录不再读取。

## 可选子目录：`claude/`（musl 变体为唯一投放形态）

claude 二进制**必须投放 musl 变体**（npm 平台包 `@anthropic-ai/claude-code-linux-x64-musl`，当前钉 **2.1.236** 实测版），**禁止**经 npm 默认安装/下载路径拿 glibc 主包形态。

**获取步骤（部署机）**：

```bash
cd "$(mktemp -d)"
npm pack @anthropic-ai/claude-code-linux-x64-musl@2.1.236   # 注意 -musl 后缀；默认路径给的是 glibc 变体
tar -xzf anthropic-ai-claude-code-linux-x64-musl-2.1.236.tgz
mkdir -p <repo>/deploy/agent-runner-runtime/claude
cp -a package/bin <repo>/deploy/agent-runner-runtime/claude/bin   # 树形投放（含 bin/claude）
<repo>/deploy/agent-runner-runtime/claude/bin/claude --version    # 构建期同款校验，出真实版本号
```

**投放后验证（必做）**：interpreter 检查必须是 musl loader——

```bash
file deploy/agent-runner-runtime/claude/bin/claude
# 正确: interpreter /lib/ld-musl-x86_64.so.1
# 错误: interpreter /lib64/ld-linux-x86-64.so.2  ← glibc 变体，重新按上面步骤获取
```

**易错点**：glibc 变体在 alpine 底座上经 gcompat 运行会在 `__pthread_key_create` 符号处崩（exit 127）——构建期只告警不阻断，运行期必炸。不要用「glibc + gcompat 兜底」的旧结论投放 claude。

## 必备子目录：`codex-package/`

二选一：**已经解压好的树**（含 `bin/`、`codex-package.json`、`codex-path/`、`codex-resources/`），或 **一个** tar（`.tar` / `.tar.gz` / `.tar.xz`）。不要只丢单个 `codex` 文件。

版本基线 **0.148.0 及以上**（协调器的 codex 调用参数对 0.148.0 与 0.150+ 双向兼容，`exec -` 已移到选项末尾）。投放后在部署机记录 `codex --version` 实际值；构建期会跑同一校验，失败即构建失败。

## 放置示例（在部署机、仓库根）

```bash
mkdir -p deploy/agent-runner-runtime/codex-package
cp -a /path/to/codex-all/. deploy/agent-runner-runtime/codex-package/
./deploy/agent-runner-runtime/codex-package/bin/codex --version
```

## 构建后的镜像内形态

`runner` 用户（uid 10001）能直接调用：`codex`（`/opt/codex/bin/codex`）、`git`、`go`、`python3`、`pip3`、`bash`。

- `AGENT_RUNNER_CODEX_BINARY=/opt/codex/bin/codex`
- `GOPROXY=https://proxy.golang.org,direct`（agent 在容器内跑 `go test` 拉依赖用）
- codex 进程由 runner 注入隔离 `CODEX_HOME` 并带 `--ephemeral --ignore-user-config`，镜像层无用户级配置
