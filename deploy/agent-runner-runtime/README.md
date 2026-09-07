# Agent Runner 投放目录

构建 `sdlc-agent-runner` **不会**再从 GitHub / claude.ai 下载 Codex 或 Claude。把官方包和语言运行时放到这个目录后，Dockerfile 只 `COPY` 再展开。

目录相对仓库根：`deploy/agent-runner-runtime/`。大包不要提交 Git（已 gitignore），只提交本 README、`materialize.sh` 和各子目录的 `.gitkeep`。

架构先按 **linux x86_64 / glibc**（和 `buildpack-deps:bookworm-scm` 一致）。不要下 Alpine/musl 的 Node/JDK/Python。你已下的 Codex musl package 可以整树放进来，Debian 上一般能跑。

## 必备子目录

每个子目录二选一：**已经解压好的树**（含 `bin/`），或 **一个** tar（`.tar` / `.tar.gz` / `.tgz` / `.tar.xz` / `.tar.tar`）。不要只丢单个 `codex` 文件。

| 子目录 | 内容 | 装到镜像 | 构建时检查 |
|---|---|---|---|
| `codex-package/` | 官方 Linux package 整树：`bin/codex`、`bin/codex-code-mode-host`、`codex-package.json`、`codex-path/`、`codex-resources/` | `/opt/codex` | `codex --version` |
| `node/` | Node.js **22.x** 官方 linux-x64 发行包 | `/opt/node` | `node -v`（主版本 ≥ 22）、`npm -v` |
| `jdk/` | **JDK 17**（Temurin or equivalent; JDK, not JRE） | `/opt/jdk` | `java -version`、`javac -version` |
| `python/` | **Python 3.12** 可执行树（含 `bin/python3` 或 `bin/python3.12`，以及 pip） | `/opt/python` | `python3 --version` 为 3.12.x |
| `maven/` | Apache Maven 发行包 | `/opt/maven` | `mvn -version` |

bash、git、curl、tar、ca-certificates 来自基座镜像 `buildpack-deps:bookworm-scm`，不放这个目录。Go 编译 Runner 二进制仍走 Dockerfile 的 `GOPROXY`，与 Agent 工具链无关。

## 可选

| 子目录 | 内容 |
|---|---|
| `claude/` | Claude CLI 已解压树或 tar。没有就不装，构建不失败。 |

## 本机已下载包如何放进仓库

Put each runtime package in its own subdirectory listed above. Do not leave loose tarballs in `deploy/agent-runner-runtime/` root.

## 手动下载（在 WSL、仓库根目录）

先建目录。每个子目录里 **只放一个 tar**，或解压成带 `bin/` 的树；不要混放多个包。

```bash
cd /path/to/sdlc-coordinator
mkdir -p deploy/agent-runner-runtime/{codex-package,node,jdk,python,maven,claude}
```

组织有内部制品库时，换成内部 URL 即可，只要解出来仍是 linux-x64 的 `bin/` 树。

### 1. Node.js 22（官方 linux-x64）

要的文件名形态：`node-v22.*.*-linux-x64.tar.xz`。不要 `arm64`，不要 `musl`。

浏览器打开（挑最新 22.x 的 `linux-x64.tar.xz`）：

- 官方：<https://nodejs.org/dist/latest-v22.x/>
- 国内：<https://npmmirror.com/mirrors/node/latest-v22.x/>

或在 WSL：

```bash
cd deploy/agent-runner-runtime/node
curl -fLO "https://npmmirror.com/mirrors/node/latest-v22.x/$(curl -fsSL https://npmmirror.com/mirrors/node/latest-v22.x/SHASUMS256.txt | awk '/linux-x64\.tar\.xz$/ {print $2; exit}')"
ls -l
tar -tJf node-v22.*-linux-x64.tar.xz | head
# 应看到 node-v22.../bin/node
```

### 2. JDK 17（Temurin linux x64，Hotspot）

要 **jdk** 不要 **jre**、不要 `aarch64`、不要 `alpine`。

- Adoptium 最新 17：<https://api.adoptium.net/v3/binary/latest/17/ga/linux/x64/jdk/hotspot/normal/eclipse?project=jdk>
- 清华目录：<https://mirrors.tuna.tsinghua.edu.cn/Adoptium/17/jdk/x64/linux/>

```bash
cd deploy/agent-runner-runtime/jdk
curl -fL "https://api.adoptium.net/v3/binary/latest/17/ga/linux/x64/jdk/hotspot/normal/eclipse?project=jdk" \
  -o OpenJDK17-jdk_x64_linux_hotspot.tar.gz
file OpenJDK17-jdk_x64_linux_hotspot.tar.gz
tar -tzf OpenJDK17-jdk_x64_linux_hotspot.tar.gz | head
# 应看到 jdk-17.../bin/java 和 bin/javac
```

若组织只发内部 JDK 17 压缩包，直接放进 `jdk/` 即可。

### 3. Python 3.12（现成可执行树，不要源码包）

python.org 的 `Python-3.12.*.tar.xz` 是 **源码**，镜像不会帮你编译。要下 **install_only** 的独立构建（解压后能直接跑 `bin/python3`）：

- GitHub：<https://github.com/astral-sh/python-build-standalone/releases>
- Assets 里找：`cpython-3.12.*-x86_64-unknown-linux-gnu-install_only.tar.gz`
  （必须带 `install_only`，必须是 `x86_64-unknown-linux-gnu`，不要 `unknown-linux-musl`）

放到 `deploy/agent-runner-runtime/python/`。自检：

```bash
tar -tzf cpython-3.12.*-x86_64-unknown-linux-gnu-install_only.tar.gz | head
# 应看到 python/bin/python3 或类似一层目录 + bin/python3
```

GitHub 拉不动就在能访问的机器上下，再拷进 WSL 这个目录。

### 4. Maven（配合 JDK 17）

```bash
cd deploy/agent-runner-runtime/maven
curl -fLO "https://mirrors.tuna.tsinghua.edu.cn/apache/maven/maven-3/3.9.11/binaries/apache-maven-3.9.11-bin.tar.gz"
tar -tzf apache-maven-3.9.11-bin.tar.gz | head
# 应看到 apache-maven-3.9.11/bin/mvn
```

清华若已归档 3.9.11，打开 <https://mirrors.tuna.tsinghua.edu.cn/apache/maven/maven-3/> 换最新 3.9.x 的 `*-bin.tar.gz`。

### 5. Codex（你已经有了）

```bash
cp -a ~/dev/exp/deploy/. deploy/agent-runner-runtime/codex-package/
./deploy/agent-runner-runtime/codex-package/bin/codex --version
```

尽量对准 **0.148.0**。不要只拷 `bin/codex`。

### 放好后的目录形态

```text
deploy/agent-runner-runtime/
  README.md
  materialize.sh
  codex-package/   整树（bin/codex 等）
  node/            node-v22.*-linux-x64.tar.xz
  jdk/             OpenJDK17-*.tar.gz
  python/          cpython-3.12.*-install_only.tar.gz
  maven/           apache-maven-3.9.*-bin.tar.gz
  claude/          可选
```

每个语言目录里 **不要** 同时放 tar 和解压树（脚本会优先用已有的 `bin/`）。确认无误后再 `docker compose build agent-runner`。

## 镜像内 PATH

构建成功后，`runner` 用户能直接调用：`bash`、`git`、`codex`、`node`、`npm`、`java`、`javac`、`python3`、`pip`/`pip3`、`mvn`。

- `JAVA_HOME=/opt/jdk`
- `AGENT_RUNNER_CODEX_BINARY=/opt/codex/bin/codex`
- `SHELL=/bin/bash`
