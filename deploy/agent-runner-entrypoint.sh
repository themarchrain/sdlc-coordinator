#!/bin/sh
set -eu

mkdir -p /app/runner-data/operations /app/workspaces
chown runner:runner /app/runner-data /app/workspaces
chown runner:runner /app/runner-data/operations
chmod 0700 /app/runner-data/operations

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
