# Agent Runner 投放目录

构建 `sdlc-agent-runner` **不会**从网上下载任何 agent 工具。codex 官方包放到本目录后，Dockerfile 只 `COPY` 再展开。目录相对仓库根：`deploy/agent-runner-runtime/`。大包不要提交 Git（已 gitignore），只提交本 README、`materialize.sh` 和各子目录的 `.gitkeep`。

## 镜像运行时来源

| 工具 | 来源 | 说明 |
|---|---|---|
| codex | **本目录投放** `codex-package/` | **必投**；musl 静态二进制，alpine 原生兼容 |
| claude / node / jdk / maven | **本目录投放**（全部可选） | **有则构建、无则跳过**；目录为空或缺席仅告警，不影响构建 |
| go 1.25 | 镜像内复制自 builder 阶段 | 不投放；`GOPROXY` 默认 `proxy.golang.org` |
| python 3.12 / git / bash / su-exec / gcompat | alpine:3.21 apk | gcompat 供 glibc 形态 claude 二进制在 musl 上尝试运行 |

可选投放子目录形态与 codex 相同：**已解压的树**（含 `bin/`）或 **一个** tar（`.tar` / `.tar.gz` / `.tar.xz`）。node/jdk/maven 为 glibc 目标时同样受 gcompat 兜底约束（不保证全部可用）。Go 项目联调通常全部留空。

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
