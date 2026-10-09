#!/bin/sh
set -eu

# 日志目录按实际环境变量准备:bind mount 后宿主机目录的 owner 可能与镜像内
# 不一致,这里只把顶层目录交给 runner 用户,不递归改已有文件。
log_dir="${AGENT_RUNNER_LOG_DIR:-/app/logs/agent-runner}"
log_dir="$(printf '%s' "$log_dir" | sed 's/^[[:space:]]*//; s/[[:space:]]*$//')"
[ -n "$log_dir" ] || log_dir=/app/logs/agent-runner

mkdir -p /app/runner-data /app/workspaces "$log_dir"
chown -h runner:runner /app/runner-data /app/workspaces "$log_dir"

exec su-exec runner "$@"
