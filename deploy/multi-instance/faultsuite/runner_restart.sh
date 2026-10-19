#!/bin/sh
# A11 Runner HTTP + persistent-volume restart check.
#
# Runs one uniquely named container at a time against a uniquely named volume.
# The volume is deliberately retained after the script exits. No host port is
# published; Python inside the Runner container calls its own localhost API.
# A temporary container-local wrapper delays only the slow run, then executes
# the image's fake Codex. It stays at the launcher's approved Codex path.
#
# Usage:
#   faultsuite/runner_restart.sh sdlc-agent-runner:<tag>
#   RUNNER_IMAGE=sdlc-agent-runner:<tag> faultsuite/runner_restart.sh
#
# The image must contain the test fake Codex at /opt/codex/bin/codex. This
# deployment check covers Runner HTTP and volume recovery. Coordinator projection
# convergence remains covered by TestRestartRecoveredInterruptedExecutionConvergesThroughCoordinator.

set -eu

IMAGE=${1:-${RUNNER_IMAGE:-}}
if [ -z "$IMAGE" ]; then
    echo "Runner image tag is required" >&2
    exit 2
fi
WORKSPACE_ROOT=${RUNNER_WORKSPACE_ROOT:-/app/workspaces}
SUFFIX="$(date -u +%Y%m%d%H%M%S)-$$"
CONTAINER="a11-runner-restart-${SUFFIX}"
VOLUME="a11-runner-data-${SUFFIX}"
RUN_PREFIX="a11-${SUFFIX}"
CONTAINER_CREATED=0
REPORTED=0

RUNNER_BOOTSTRAP=$(cat <<'SH'
set -eu
mkdir -p "$AGENT_RUNNER_WORKSPACE_ROOT"
chown runner:runner "$AGENT_RUNNER_WORKSPACE_ROOT"
if [ ! -f /opt/codex/bin/codex.a11-original ]; then
    cp /opt/codex/bin/codex /opt/codex/bin/codex.a11-original
fi
cat >/tmp/a11-codex-wrapper <<'WRAPPER'
#!/bin/sh
set -eu
workspace=
previous=
for argument do
    if [ "$previous" = "-C" ]; then
        workspace=$argument
    fi
    previous=$argument
done
case "$workspace" in
    *-slow)
        : >"$workspace/a11-slow-started"
        sleep 180
        ;;
esac
exec /opt/codex/bin/codex.a11-original "$@"
WRAPPER
chmod 0755 /tmp/a11-codex-wrapper
mv /tmp/a11-codex-wrapper /opt/codex/bin/codex
exec /usr/local/bin/agent-runner-entrypoint /app/agent-runner
SH
)

report_and_cleanup() {

    exit_code=$?
    trap - EXIT HUP INT TERM
    if [ "$REPORTED" -eq 0 ] && [ "$CONTAINER_CREATED" -eq 1 ]; then
        docker exec "$CONTAINER" python3 -c \
            'from pathlib import Path; p=Path("/app/runner-data/faultsuite-a11-results.jsonl"); print(p.read_text(), end="") if p.exists() else None' \
            2>/dev/null || true
    fi
    if [ "$exit_code" -ne 0 ] && [ "$CONTAINER_CREATED" -eq 1 ]; then
        docker logs --tail 30 "$CONTAINER" >&2 || true
    fi
    if [ "$CONTAINER_CREATED" -eq 1 ]; then
        docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
    fi
    echo "preserved_volume=$VOLUME" >&2
    exit "$exit_code"
}
trap report_and_cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

if docker container inspect "$CONTAINER" >/dev/null 2>&1; then
    echo "refusing to use existing container name: $CONTAINER" >&2
    exit 2
fi
if docker volume inspect "$VOLUME" >/dev/null 2>&1; then
    echo "refusing to use existing volume name: $VOLUME" >&2
    exit 2
fi
docker image inspect "$IMAGE" >/dev/null
docker volume create "$VOLUME" >/dev/null

start_runner() {
    docker run -d --name "$CONTAINER" \
        --mount "type=volume,source=${VOLUME},target=/app/runner-data" \
        -e "AGENT_RUNNER_WORKSPACE_ROOT=${WORKSPACE_ROOT}" \
        --entrypoint /bin/sh \
        "$IMAGE" -c "$RUNNER_BOOTSTRAP" >/dev/null
    CONTAINER_CREATED=1
    docker exec "$CONTAINER" test -x /opt/codex/bin/codex
    wait_runner_ready
}

wait_runner_ready() {
    deadline=$(( $(date +%s) + 60 ))
    while [ "$(date +%s)" -lt "$deadline" ]; do
        if docker exec "$CONTAINER" python3 -c \
            'import urllib.request; urllib.request.urlopen("http://127.0.0.1:8081/healthz", timeout=2)' \
            >/dev/null 2>&1; then
            return 0
        fi
        sleep 1
    done
    echo "Runner did not become healthy in container $CONTAINER" >&2
    return 1
}

run_probe() {
    action=$1
    execution_id=$2
    docker exec -i "$CONTAINER" python3 - "$action" "$execution_id" <<'PY'
import json
import os
import stat
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

base = "http://127.0.0.1:8081"
action, execution_id = sys.argv[1], sys.argv[2]
task_id = execution_id + "-task"
workspace = os.path.abspath(os.environ["AGENT_RUNNER_WORKSPACE_ROOT"]) + "/runs/" + execution_id
state_path = Path("/app/runner-data/faultsuite-a11-state.json")
report_path = Path("/app/runner-data/faultsuite-a11-results.jsonl")


def record(assertion, result, **details):
    entry = {
        "suite": "faultsuite-runner-restart",
        "scenario": "A11",
        "assertion": assertion,
        "result": result,
        "details": details,
    }
    with report_path.open("a", encoding="utf-8") as report:
        report.write(json.dumps(entry, separators=(",", ":"), sort_keys=True) + "\n")


def request(method, path, payload=None):
    data = None if payload is None else json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(
        base + path,
        data=data,
        method=method,
        headers={"Content-Type": "application/json"} if data is not None else {},
    )
    try:
        with urllib.request.urlopen(req, timeout=5) as response:
            raw = response.read()
            return response.status, json.loads(raw) if raw else None
    except urllib.error.HTTPError as error:
        # Do not copy response bodies into logs; keep output free of credentials.
        return error.code, None


def fail(assertion, **details):
    record(assertion, "fail", **details)
    raise SystemExit(1)


if action in ("success-create", "slow-create"):
    Path(workspace).mkdir(parents=True, exist_ok=True)
    os.chown(workspace, 10001, 10001)
    (Path(workspace) / "prompt.md").write_text("A11 fake CLI restart probe\n", encoding="utf-8")
    (Path(workspace) / "schema.json").write_text('{"type":"object"}\n', encoding="utf-8")
    payload = {
        "execution_id": execution_id,
        "task_id": task_id,
        "role": "ReviewerWorker",
        "workspace_path": workspace,
        "prompt_file": "prompt.md",
        "schema_file": "schema.json",
        "provider": {"name": "codex-cli", "revision": 1},
    }
    status, created = request("POST", "/api/v1/executions", payload)
    if status != 202 or not isinstance(created, dict):
        fail(action + "_accepted", http_status=status)
    record(action + "_accepted", "pass", http_status=status, workspace_root=os.environ["AGENT_RUNNER_WORKSPACE_ROOT"])

    wanted = "succeeded" if action == "success-create" else "running"
    deadline = time.monotonic() + (45 if wanted == "succeeded" else 30)
    observed = None
    while time.monotonic() < deadline:
        status, observed = request("GET", "/api/v1/executions/" + execution_id)
        if status == 200 and observed.get("status") == wanted:
            if action == "success-create":
                break
            if Path(workspace, "a11-slow-started").exists():
                break
        if status == 200 and observed.get("status") in ("failed", "cancelled", "interrupted"):
            fail(action + "_reached_expected_state", actual_status=observed.get("status"))
        time.sleep(0.25)
    if status != 200 or not observed or observed.get("status") != wanted:
        fail(action + "_reached_expected_state", http_status=status, actual_status=(observed or {}).get("status"))
    if action == "slow-create":
        if not Path(workspace, "a11-slow-started").exists():
            fail("slow_cli_started_before_kill")
        record("slow_cli_started_before_kill", "pass")
    record(action + "_reached_expected_state", "pass", status=observed["status"])

    if action == "success-create":
        result_status, result = request("GET", "/api/v1/executions/" + execution_id + "/result")
        if result_status != 200 or not isinstance(result, dict):
            fail("success_result_available_before_restart", http_status=result_status)
        if observed.get("result_hash") != result.get("result_hash"):
            fail("success_hash_matches_before_restart")
        state = {
            "execution_id": execution_id,
            "request_hash": observed.get("request_hash"),
            "result_hash": result.get("result_hash"),
            "structured_output": result.get("structured_output"),
        }
        state_path.write_text(json.dumps(state, separators=(",", ":")), encoding="utf-8")
        record("success_result_available_before_restart", "pass", result_hash=result.get("result_hash"))
    else:
        state = json.loads(state_path.read_text(encoding="utf-8"))
        state["slow_execution_id"] = execution_id
        state_path.write_text(json.dumps(state, separators=(",", ":")), encoding="utf-8")
else:
    state = json.loads(state_path.read_text(encoding="utf-8"))
    if action == "success-after-restart":
        execution_id = state["execution_id"]
        status, observed = request("GET", "/api/v1/executions/" + execution_id)
        if status != 200 or observed.get("status") != "succeeded":
            fail("success_status_survives_restart", http_status=status, actual_status=(observed or {}).get("status"))
        same_execution = observed.get("request_hash") == state["request_hash"] and observed.get("result_hash") == state["result_hash"]
        if not same_execution:
            fail("success_execution_hashes_survive_restart")
        result_status, result = request("GET", "/api/v1/executions/" + execution_id + "/result")
        if result_status != 200 or not isinstance(result, dict):
            fail("success_result_available_after_restart", http_status=result_status)
        same_result = (
            result.get("result_hash") == state["result_hash"]
            and result.get("structured_output") == state["structured_output"]
        )
        if not same_result:
            fail("success_result_and_hash_unchanged_after_restart")
        record("success_status_survives_restart", "pass", status=observed["status"])
        record("success_result_and_hash_unchanged_after_restart", "pass", result_hash=result["result_hash"])
    elif action == "slow-after-restart":
        execution_id = state["slow_execution_id"]
        run_root = Path(workspace)
        recovered = all(
            os.lstat(path).st_uid == 10001 and os.lstat(path).st_gid == 10001
            for path in (run_root, run_root / "prompt.md", run_root / "a11-slow-started")
        ) and stat.S_IMODE(os.lstat(run_root).st_mode) == 0o700
        if not recovered:
            fail("slow_workspace_owned_by_runner_after_restart")
        status, observed = request("GET", "/api/v1/executions/" + execution_id)
        if status != 200 or observed.get("status") != "interrupted" or observed.get("error_code") != "runner_restarted":
            fail(
                "slow_execution_interrupted_after_restart",
                http_status=status,
                actual_status=(observed or {}).get("status"),
                error_code=(observed or {}).get("error_code"),
            )
        result_status, _ = request("GET", "/api/v1/executions/" + execution_id + "/result")
        if result_status != 409:
            fail("slow_execution_has_no_result_after_restart", http_status=result_status)
        event_status, events = request("GET", "/api/v1/executions/" + execution_id + "/events?after_seq=0")
        event_types = [item.get("type") for item in (events or {}).get("items", [])]
        if event_status != 200 or "interrupted" not in event_types:
            fail("slow_execution_interrupted_event_persisted", http_status=event_status)
        record("slow_execution_interrupted_after_restart", "pass", status=observed["status"], error_code=observed["error_code"])
        record("slow_execution_has_no_result_after_restart", "pass", http_status=result_status)
        record("slow_execution_interrupted_event_persisted", "pass")
        record("slow_workspace_owned_by_runner_after_restart", "pass")
    else:
        fail("known_probe_action", action=action)
PY
}

start_runner
run_probe success-create "${RUN_PREFIX}-success"
docker stop --time 10 "$CONTAINER" >/dev/null
docker start "$CONTAINER" >/dev/null
wait_runner_ready
run_probe success-after-restart "${RUN_PREFIX}-success"

# Recreate only this script-owned container with the same retained volume for
# the second case. The container-local wrapper delays before invoking fake Codex.
docker stop --time 10 "$CONTAINER" >/dev/null
docker rm "$CONTAINER" >/dev/null
CONTAINER_CREATED=0
start_runner
run_probe slow-create "${RUN_PREFIX}-slow"
docker kill --signal KILL "$CONTAINER" >/dev/null
docker start "$CONTAINER" >/dev/null
wait_runner_ready
run_probe slow-after-restart "${RUN_PREFIX}-slow"

docker exec "$CONTAINER" python3 -c \
    'from pathlib import Path; p=Path("/app/runner-data/faultsuite-a11-results.jsonl"); print(p.read_text(), end="") if p.exists() else None'
printf '{"suite":"faultsuite-runner-restart","scenario":"A11","assertion":"coordinator_projection_convergence","result":"not_covered","details":{"reason":"This isolated Runner-only fixture creates no Coordinator task or MySQL execution projection; use TestRestartRecoveredInterruptedExecutionConvergesThroughCoordinator for that assertion."}}\n'
REPORTED=1
