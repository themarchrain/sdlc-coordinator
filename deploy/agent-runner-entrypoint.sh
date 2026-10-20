#!/bin/sh
set -eu

# 日志目录按实际环境变量准备:bind mount 后宿主机目录的 owner 可能与镜像内
# 不一致,这里只把顶层目录交给 runner 用户,不递归改已有文件。
log_dir="${AGENT_RUNNER_LOG_DIR:-/app/logs/agent-runner}"
log_dir="$(printf '%s' "$log_dir" | sed 's/^[[:space:]]*//; s/[[:space:]]*$//')"
[ -n "$log_dir" ] || log_dir=/app/logs/agent-runner

mkdir -p /app/runner-data/operations /app/workspaces "$log_dir"
chown -h runner:runner /app/runner-data /app/workspaces /app/runner-data/operations "$log_dir"
chmod 0700 /app/runner-data/operations

cache_root="${AGENT_RUNNER_CACHE_ROOT:-/app/dependency-cache}"
cache_root="$(printf '%s' "$cache_root" | sed 's/^[[:space:]]*//; s/[[:space:]]*$//')"
[ -n "$cache_root" ] || cache_root=/app/dependency-cache
mkdir -p "$cache_root"
chown -h agent:agent "$cache_root"
chmod 0700 "$cache_root"
export AGENT_RUNNER_CACHE_ROOT="$cache_root"

runner_cap_eff=$(su-exec runner /bin/sh -c 'sed -n "s/^CapEff:[[:space:]]*//p" /proc/self/status')
runner_cap_prm=$(su-exec runner /bin/sh -c 'sed -n "s/^CapPrm:[[:space:]]*//p" /proc/self/status')
runner_cap_inh=$(su-exec runner /bin/sh -c 'sed -n "s/^CapInh:[[:space:]]*//p" /proc/self/status')
if [ "$runner_cap_eff" != 0000000000000000 ] || [ "$runner_cap_prm" != 0000000000000000 ] || [ "$runner_cap_inh" != 0000000000000000 ]; then
	printf '%s\n' 'Runner process must not retain Linux capabilities' >&2
	exit 1
fi
su-exec runner /usr/local/bin/agent-runner-cli-launcher --check
workspace_root=${AGENT_RUNNER_WORKSPACE_ROOT:-/app/workspaces}
if [ -e "$workspace_root" ] || [ -L "$workspace_root" ]; then
	su-exec runner /usr/local/bin/agent-runner-cli-launcher --recover "$workspace_root"
fi
if su-exec agent /bin/sh -c 'test -x /usr/local/bin/agent-runner-cli-launcher'; then
	printf '%s\n' 'agent user must not be able to execute the privileged CLI launcher' >&2
	exit 1
fi
if su-exec agent test -r /app/runner-data/operations; then
	printf '%s\n' 'agent user must not be able to read Runner operation logs' >&2
	exit 1
fi

exec su-exec runner "$@"
