#!/bin/sh
set -eu

if [ "${1:-}" = agent ]; then
    test "$(id -u):$(id -g)" = 10002:10002
    test "$AGENT_RUNNER_CACHE_ROOT" = "$EXPECTED_CACHE_ROOT"
    for directory in "$GOCACHE" "$GOMODCACHE" "$npm_config_cache" "$pnpm_config_store_dir" "$pnpm_config_cache_dir" "$PIP_CACHE_DIR"; do
        mkdir -p "$directory"
        printf check > "$directory/write-check"
    done
    exit 0
fi

if [ "${1:-}" = runner ]; then
    printf 'initialized cache root: <%s>\n' "$AGENT_RUNNER_CACHE_ROOT"
    test "$AGENT_RUNNER_CACHE_ROOT" = "$EXPECTED_CACHE_ROOT"
    mkdir -p /app/workspaces/runs/cache-root
    exec /usr/local/bin/agent-runner-cli-launcher \
        --workspace-root /app/workspaces --workspace /app/workspaces/runs/cache-root \
        -- /opt/codex/bin/codex agent
fi

image=${1:-sdlc-agent-runner:latest}
directory=$(CDPATH='' cd -- "$(dirname "$0")" && pwd)
failed=0
for scenario in normal padded blank empty; do
    expected=/app/test-cache
    case "$scenario" in
        normal) configured=$expected ;;
        padded) configured=$(printf '\t %s \t' "$expected") ;;
        blank) configured=$(printf ' \t '); expected=/app/dependency-cache ;;
        empty) configured=; expected=/app/dependency-cache ;;
    esac
    if docker run --rm --network none \
        -e "AGENT_RUNNER_CACHE_ROOT=$configured" -e "EXPECTED_CACHE_ROOT=$expected" \
        --mount "type=bind,src=$directory/../agent-runner-entrypoint.sh,dst=/usr/local/bin/agent-runner-entrypoint,readonly" \
        --mount "type=bind,src=$directory/runner-cache-root.sh,dst=/opt/codex/bin/codex,readonly" \
        "$image" sh /opt/codex/bin/codex runner; then
        printf '%s\n' "$scenario: PASS"
    else
        printf '%s\n' "$scenario: FAIL"
        failed=1
    fi
done
exit "$failed"
