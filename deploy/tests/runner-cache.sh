#!/bin/sh
set -eu

image=${1:-sdlc-agent-runner:latest}
cache_root=${TEST_CACHE_ROOT:-/app/dependency-cache}
go_proxy=${TEST_GOPROXY:-https://proxy.golang.org,direct}
directory=$(CDPATH='' cd -- "$(dirname "$0")" && pwd)
prefix=runner-cache-check-$(date +%s)-$$
first=$prefix-a
second=$prefix-b
volume_a=$prefix-cache-a
volume_b=$prefix-cache-b
evidence=${RUNNER_CACHE_TEST_OUTPUT:-$(mktemp -d /tmp/runner-cache-evidence.XXXXXX)}
mkdir -p "$evidence"

for name in "$first" "$second"; do
    if docker container inspect "$name" >/dev/null 2>&1; then printf 'container already exists: %s\n' "$name" >&2; exit 1; fi
done
for volume in "$volume_a" "$volume_b"; do
    if docker volume inspect "$volume" >/dev/null 2>&1; then printf 'volume already exists: %s\n' "$volume" >&2; exit 1; fi
done

cleanup() {
    docker rm -f "$first" "$second" >/dev/null 2>&1 || true
    docker volume rm "$volume_a" "$volume_b" >/dev/null 2>&1 || true
}
trap cleanup EXIT
docker volume create "$volume_a" >/dev/null
docker volume create "$volume_b" >/dev/null

start() {
    name=$1
    volume=$2
    network=$3
    docker run -d --name "$name" --init --network "$network" \
        -e "GOPROXY=$go_proxy" -e "AGENT_RUNNER_CACHE_ROOT=$cache_root" \
        -e AGENT_RUNNER_CLAUDE_BINARY=/opt/claude/bin/claude \
        -e AGENT_RUNNER_INTERNAL_TOKEN=must-not-reach-agent \
        -e CONTROL_PLANE_DSN=must-not-reach-agent -e GITLAB_TOKEN=must-not-reach-agent \
        --mount "type=volume,src=$volume,dst=$cache_root" \
        --mount "type=bind,src=$directory/runner-provider-cli.sh,dst=/opt/codex/bin/codex,readonly" \
        --mount "type=bind,src=$directory/runner-provider-cli.sh,dst=/opt/claude/bin/claude,readonly" \
        --mount "type=bind,src=$directory/runner-cache-probe.sh,dst=/runner-cache-probe.sh,readonly" \
        "$image" >/dev/null
    tries=0
    until docker exec "$name" curl -fsS http://127.0.0.1:8081/healthz >/dev/null 2>&1; do
        tries=$((tries + 1))
        if [ "$tries" -ge 30 ]; then docker logs "$name"; return 1; fi
        sleep 1
    done
}

probe() {
    phase=$1
    provider=$2
    execution=$phase-$provider
    docker exec -u runner "$first" sh -c '
        mkdir -p /app/workspaces/runs/probe
        chmod 0700 /app/workspaces/runs/probe
        cd /app/workspaces/runs/probe
        printf "%s" "$1" > prompt.md
        printf "%s" "$2" > expected-cache-root
        printf "%s" "$3" > expected-goproxy
        printf "%s" '\''{"type":"object","properties":{"verdict":{"type":"string"}},"required":["verdict"]}'\'' > schema.json
    ' cache-probe "$phase" "$cache_root" "$go_proxy"
    printf '{"execution_id":"%s","task_id":"cache-probe","role":"ReviewerWorker","workspace_path":"/app/workspaces/runs/probe","prompt_file":"prompt.md","schema_file":"schema.json","timeout_seconds":1200,"provider":{"name":"%s"}}' "$execution" "$provider" |
        docker exec -i "$first" curl -fsS -H 'Content-Type: application/json' -d @- http://127.0.0.1:8081/api/v1/executions > "$evidence/$execution-accepted.json"
    tries=0
    while :; do
        docker exec "$first" curl -fsS "http://127.0.0.1:8081/api/v1/executions/$execution" > "$evidence/$execution.json"
        status=$(docker exec -i "$first" jq -r .status < "$evidence/$execution.json")
        case "$status" in succeeded|failed|cancelled|interrupted) break ;; esac
        tries=$((tries + 1))
        if [ "$tries" -ge 1200 ]; then docker logs "$first"; return 1; fi
        sleep 1
    done
    docker cp "$first:/app/workspaces/runs/probe" "$evidence/$execution" >/dev/null
    if [ "$status" != succeeded ]; then cat "$evidence/$execution.json"; tail -n 40 "$evidence/$execution/agent-probe.log"; return 1; fi
    docker exec "$first" curl -fsS "http://127.0.0.1:8081/api/v1/executions/$execution/result" > "$evidence/$execution-result.json"
    home=$(docker exec -u runner "$first" cat /app/workspaces/runs/probe/home.path)
    docker exec -u runner "$first" test ! -e "$home"
    test "$(docker exec -u runner "$first" stat -c '%u:%g %a' /app/workspaces/runs/probe)" = '10001:10001 700'
    docker exec -u runner "$first" sh -c 'rm -rf /app/workspaces/runs/probe'
    printf '%s\n' "$execution: PASS"
}

start "$first" "$volume_a" bridge
probe seed codex-cli
probe warm claude-cli
docker rm -f "$first" >/dev/null
start "$first" "$volume_a" none
probe replacement-offline codex-cli
probe replacement-offline claude-cli
start "$second" "$volume_b" none
docker exec -u agent "$second" test ! -e "$cache_root/runner-probe"
printf '%s\n' 'independent runner cache: PASS' "evidence: $evidence"
