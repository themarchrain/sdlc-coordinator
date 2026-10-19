#!/bin/sh
set -eu

image=${1:-sdlc-agent-runner:latest}
directory=$(CDPATH='' cd -- "$(dirname "$0")" && pwd)
prefix=runner-cache-check-$$
first=$prefix-a
second=$prefix-b
volume_a=$prefix-cache-a
volume_b=$prefix-cache-b
evidence=${RUNNER_CACHE_TEST_OUTPUT:-$(mktemp -d /tmp/runner-cache-evidence.XXXXXX)}
mkdir -p "$evidence"

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
        -e GOPROXY="${TEST_GOPROXY:-https://proxy.golang.org,direct}" \
        --mount "type=volume,src=$volume,dst=/app/dependency-cache" \
        --mount "type=bind,src=$directory/runner-cache-probe.sh,dst=/opt/codex/bin/codex,readonly" \
        "$image" sleep 1200 >/dev/null
    tries=0
    until docker exec "$name" pgrep -u runner -x sleep >/dev/null 2>&1; do
        tries=$((tries + 1))
        if [ "$tries" -ge 30 ]; then docker logs "$name"; return 1; fi
        sleep 1
    done
}

probe() {
    phase=$1
    docker exec -u runner "$first" sh -c 'mkdir -p /app/workspaces/runs/probe; chmod 0700 /app/workspaces/runs/probe'
    status=0
    docker exec -u runner "$first" /usr/local/bin/agent-runner-cli-launcher \
        --workspace-root /app/workspaces --workspace /app/workspaces/runs/probe \
        -- /opt/codex/bin/codex "$phase" > "$evidence/$phase.log" 2>&1 || status=$?
    docker cp "$first:/app/workspaces/runs/probe" "$evidence/$phase" >/dev/null
    if [ "$status" -ne 0 ]; then tail -n 20 "$evidence/$phase.log"; return "$status"; fi
    home=$(docker exec -u runner "$first" cat /app/workspaces/runs/probe/home.path)
    docker exec -u runner "$first" test ! -e "$home"
    test "$(docker exec -u runner "$first" stat -c '%u:%g %a' /app/workspaces/runs/probe)" = '10001:10001 700'
    docker exec -u runner "$first" sh -c 'rm -rf /app/workspaces/runs/probe'
    printf '%s\n' "$phase: PASS"
}

start "$first" "$volume_a" bridge
probe seed
probe warm
docker rm -f "$first" >/dev/null
start "$first" "$volume_a" none
probe replacement-offline
start "$second" "$volume_b" none
docker exec -u agent "$second" test ! -e /app/dependency-cache/runner-probe
printf '%s\n' 'independent runner cache: PASS' "evidence: $evidence"
