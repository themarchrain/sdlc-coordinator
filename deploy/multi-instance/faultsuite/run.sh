#!/bin/sh
# 多实例故障矩阵入口：可重复注入 pause / kill / 网络阻断 / 响应丢失，
# 每场景输出结构化断言（JSON Lines，faultsuite-results.jsonl）。
#
# 用法（在 deploy/multi-instance/ 目录，套件假设部署已起来且健康）：
#   export MULTI_SECRET=<64-char hex> MULTI_SUPERADMIN_PASSWORD=...
#   faultsuite/run.sh                 # 全场景
#   faultsuite/run.sh coordinator_pause coordinator_kill   # 指定场景
#
# 场景清单：本 shell suite 只断言部署/容器行为和租户控制面探针。
# 在途交付、租约栅栏、恢复语义及 MySQL 故障注入目前不由这些场景覆盖。

set -u
cd "$(dirname "$0")/.." || exit 1

COMPOSE_FILE=docker-compose.multi.yml
export COMPOSE_FILE

# 环境面与部署同源：宿主环境优先，其次 compose 同款 .env（密钥不入库）。
if [ -z "${MULTI_SUPERADMIN_PASSWORD:-}" ] && [ -f .env ]; then
    MULTI_SUPERADMIN_PASSWORD=$(sed -n 's/^MULTI_SUPERADMIN_PASSWORD=//p' .env | head -n 1)
    export MULTI_SUPERADMIN_PASSWORD
fi

. faultsuite/lib.sh

SCENARIOS_ALL="coordinator_pause coordinator_kill runner_kill net_block lb_failover"
SCENARIOS="${*:-$SCENARIOS_ALL}"
SUITE_START=$(date +%s)

# 业务验收缺口显式落入结果文件，避免健康检查被误读为业务通过。
record "coverage" "A06_original_execution_reuse_and_single_side_effect" not_covered "This shell suite does not create an in-flight delivery; TestA06TakeoverReusesExecutionStartedBeforeCoordinatorExit covers orchestration with a fake Runner, outside this deployment suite."
record "coverage" "A07_stale_owner_generation_fence" not_covered "This shell suite does not create an in-flight run or assert stale-write rejection; TestRemoteDeliveryTakeoverFencesStaleCoordinatorOnMySQL covers takeover in the Go+MySQL integration suite."
record "coverage" "A11_runner_restart_result_recovery" not_covered "This shell suite checks container health and volume marker only; TestRestartRecoveredInterruptedExecutionConvergesThroughCoordinator and runner service restart tests cover recovery outside this deployment suite."
record "coverage" "A12_runner_uncertainty_retry_boundary" not_covered "This shell suite does not assert the unknown-outcome boundary; TestAcceptedExecutionMissingAfterRunnerVolumeLossRemainsUnknown and TestAmbiguousCreateAndRunner404DoNotFailOrRotate cover orchestration behavior outside this deployment suite."
record "coverage" "A18_mysql_disconnect_deadlock_lock_timeout" not_covered "This shell suite blocks Runner connectivity, not MySQL; TestA18MySQLDeadlockErrorIsRetryable, TestA18MySQLDispatchLockTimeoutDoesNotConsumeAttemptAndRetriesNextDispatch, and TestA18MySQLDisconnectDoesNotDispatch run in the Go+MySQL integration suite."

# ---- 前置自检：部署健康才跑矩阵（避免把部署问题误报成故障场景失败）----
preflight() {
    scenario="preflight"
    wait_until "coordinator-a healthy" 60 2 sh -c \
        'curl -fsS http://127.0.0.1:8090/healthz >/dev/null 2>&1'
    assert_true "$scenario" "smoke_lb_reaches_coordinators" $?
    jar=$(mktemp)
    portal_login superadmin "${MULTI_SUPERADMIN_PASSWORD:?set MULTI_SUPERADMIN_PASSWORD}" "$jar"
    assert_true "$scenario" "smoke_superadmin_login_via_lb" $?
    session_jar="$jar"
}

# Create a tenant through the current LB pool and verify the shared control-plane
# row and its provisioned database in MySQL.
assert_tenant_provisioned() {
    scenario="$1"; slug="$2"
    tenant_count=$(mysql_scalar "SELECT COUNT(*) FROM sdlc_control_plane.tenants WHERE slug='${slug}' AND status='active' AND db_locator LIKE 'mysql://%'")
    assert_eq "$scenario" "tenant_row_provisioned_${slug}" "1" "${tenant_count:-unknown}"

    tenant_db=$(mysql_scalar "SELECT SUBSTRING_INDEX(SUBSTRING_INDEX(db_locator, '/', -1), '?', 1) FROM sdlc_control_plane.tenants WHERE slug='${slug}'")
    case "$tenant_db" in
        sdlc_tenant_*) tenant_id=${tenant_db#sdlc_tenant_} ;;
        *) record "$scenario" "tenant_database_locator_${slug}" fail "unexpected database locator: ${tenant_db:-empty}"; return ;;
    esac
    case "$tenant_id" in
        ''|*[!0-9]*) record "$scenario" "tenant_database_locator_${slug}" fail "unexpected database locator: ${tenant_db}"; return ;;
    esac
    db_count=$(mysql_scalar "SELECT COUNT(*) FROM information_schema.SCHEMATA WHERE SCHEMA_NAME='${tenant_db}'")
    assert_eq "$scenario" "tenant_database_exists_${slug}" "1" "${db_count:-unknown}"
}

create_tenant_probe() {
    scenario="$1"; slug="$2"
    response=$(api_post "/api/super-admin/tenants" "$session_jar" \
        "{\"slug\":\"${slug}\",\"display_name\":\"faultsuite ${scenario}\"}") || return 1
    [ -n "$response" ]
}

# ---- 恢复现场（每场景结束后把拓扑拉回全健康）----
restore_topology() {
    docker compose -f "$COMPOSE_FILE" unpause coordinator-a coordinator-b >/dev/null 2>&1
    docker compose -f "$COMPOSE_FILE" start coordinator-a coordinator-b runner-a runner-b >/dev/null 2>&1
    docker compose -f "$COMPOSE_FILE" up -d --no-deps runner-a runner-b >/dev/null 2>&1
    wait_until "topology healthy" 120 3 sh -c \
        'docker compose -f '"$COMPOSE_FILE"' ps --format json | grep -c "\"Health\":\"healthy\"" >/dev/null 2>&1'
}

scenario_coordinator_pause() {
    scenario="coordinator_pause"
    echo "=== $scenario: A 暂停，经 B 写入共享 MySQL；A 恢复后状态可读 ==="
    restarts_baseline=$(docker inspect multi-instance-coordinator-a-1 --format '{{.RestartCount}}' 2>/dev/null)
    docker compose -f "$COMPOSE_FILE" pause coordinator-a >/dev/null 2>&1
    assert_true "$scenario" "smoke_coordinator_a_paused" $?

    # 暂停窗口内健康面应仍由 B 承接（LB 探活把流量切给 B）。
    wait_until "lb still serves through B" 60 2 sh -c \
        'curl -fsS http://127.0.0.1:8090/healthz >/dev/null 2>&1'
    assert_true "$scenario" "smoke_lb_serves_via_survivor" $?

    # 暂停 20 秒覆盖一个调度 tick 窗口；本场景没有创建在途 run。
    sleep 20
    # 暂停期间经 LB 创建真实租户，验证存活实例写入共享控制面和数据面库。
    tenant_slug="fault-pause-$(date +%s)"
    if create_tenant_probe "$scenario" "$tenant_slug"; then
        assert_tenant_provisioned "$scenario" "$tenant_slug"
    else
        record "$scenario" "tenant_create_while_a_paused" fail "control-plane tenant creation failed"
    fi

    docker compose -f "$COMPOSE_FILE" unpause coordinator-a >/dev/null 2>&1
    assert_true "$scenario" "smoke_coordinator_a_resumed" $?

    # A 恢复后再次读取共享状态，确保恢复过程没有覆盖 B 已落地的租户。
    assert_tenant_provisioned "$scenario" "$tenant_slug"
    restarts_after=$(docker inspect multi-instance-coordinator-a-1 --format '{{.RestartCount}}' 2>/dev/null)
    assert_eq "$scenario" "smoke_coordinator_a_not_restarted" "$restarts_baseline" "${restarts_after:-unknown}"
}

scenario_coordinator_kill() {
    scenario="coordinator_kill"
    echo "=== $scenario: kill -9 协调器 A；存活实例不受影响，A 手动拉起后回池 ==="
    # Docker 语义：docker kill（手动停止）不触发 restart 策略，自动重启只
    # 覆盖进程自身崩溃（exitCode 非 0 且非手动停止）。因此本场景断言分两段：
    # kill 后存活实例继续服务；运维拉起（docker start）后 A 回池。
    docker compose -f "$COMPOSE_FILE" kill -s KILL coordinator-a >/dev/null 2>&1
    assert_true "$scenario" "smoke_coordinator_a_killed" $?
    sleep 3

    # 存活实例全程服务：LB 经 B 继续应答。
    wait_until "lb serves through survivor" 30 2 sh -c \
        'curl -fsS http://127.0.0.1:8090/healthz >/dev/null 2>&1'
    assert_true "$scenario" "smoke_survivor_never_dropped" $?

    # 运维拉起后 A 回池并恢复 ready。
    docker compose -f "$COMPOSE_FILE" start coordinator-a >/dev/null 2>&1
    assert_true "$scenario" "smoke_coordinator_a_started" $?
    wait_until "coordinator-a back to ready" 120 3 sh -c \
        'curl -fsS http://127.0.0.1:8090/healthz >/dev/null 2>&1 && docker logs multi-instance-coordinator-a-1 2>&1 | tail -50 | grep -q "\"mode\": \"ready\""'
    assert_true "$scenario" "smoke_coordinator_a_back_to_ready" $?
}

scenario_runner_kill() {
    scenario="runner_kill"
    echo "=== $scenario: kill -9 Runner A；数据卷标记保留，拉起后 Runner 恢复健康 ==="
    # 先落卷标志：kill 前写入，重启后必须仍在（卷持久性断言）。
    docker exec multi-instance-runner-a-1 sh -c 'echo volume-marker > /app/runner-data/faultsuite-marker'
    docker compose -f "$COMPOSE_FILE" kill -s KILL runner-a >/dev/null 2>&1
    assert_true "$scenario" "smoke_runner_a_killed" $?
    sleep 3

    # 同 docker kill 语义：手动停止不触发 restart 策略，运维拉起。
    docker compose -f "$COMPOSE_FILE" start runner-a >/dev/null 2>&1
    assert_true "$scenario" "smoke_runner_a_started" $?
    wait_until "runner-a back up" 120 3 sh -c \
        "docker exec multi-instance-runner-a-1 wget -qO- http://127.0.0.1:8081/healthz 2>/dev/null | grep -q ok"
    assert_true "$scenario" "smoke_runner_a_back_up" $?

    # 卷保留：kill 前的标志文件重启后仍可读。
    docker exec multi-instance-runner-a-1 sh -c 'cat /app/runner-data/faultsuite-marker' 2>/dev/null | grep -q volume-marker
    assert_true "$scenario" "smoke_runner_volume_preserved_across_kill" $?

    # 双 Runner 池中 B 仍健康：新派发不落死端点。
    wait_until "runner-b still healthy" 30 2 sh -c \
        "docker exec multi-instance-runner-b-1 wget -qO- http://127.0.0.1:8081/healthz 2>/dev/null | grep -q ok"
    assert_true "$scenario" "smoke_runner_b_unaffected" $?
}

scenario_net_block() {
    scenario="net_block"
    echo "=== $scenario: 断 Runner A 网络，验证协调器存活及 Runner 恢复可达 ==="
    docker network disconnect "${MULTI_NET_NAME:-sdlc-multi-net}" multi-instance-runner-a-1 >/dev/null 2>&1
    assert_true "$scenario" "smoke_runner_a_network_removed" $?

    # 阻断期间协调器对 Runner A 的探活/派发会失败——平台不得崩溃。
    sleep 15
    wait_until "coordinators stay healthy during block" 30 2 sh -c \
        'curl -fsS http://127.0.0.1:8090/healthz >/dev/null 2>&1'
    assert_true "$scenario" "smoke_coordinators_survive_block" $?

    docker network connect "${MULTI_NET_NAME:-sdlc-multi-net}" multi-instance-runner-a-1 --alias runner-a >/dev/null 2>&1
    assert_true "$scenario" "smoke_runner_a_network_restored" $?
    wait_until "runner-a reachable again" 60 3 sh -c \
        "docker exec multi-instance-runner-a-1 wget -qO- http://127.0.0.1:8081/healthz 2>/dev/null | grep -q ok"
    assert_true "$scenario" "smoke_runner_a_reachable_after_restore" $?
}

scenario_lb_failover() {
    scenario="lb_failover"
    echo "=== $scenario: LB 后单协调器下线，存活实例继续处理 Portal 与租户请求 ==="
    docker compose -f "$COMPOSE_FILE" stop coordinator-b >/dev/null 2>&1
    assert_true "$scenario" "smoke_coordinator_b_stopped" $?

    # 会话在共享控制面库里：A 单独也能验同一会话 cookie。
    api_get "/api/portal/auth/session" "$session_jar" >/dev/null 2>&1
    assert_true "$scenario" "business_portal_session_survives_failover" $?

    # B 停机期间由 A 完成租户创建；MySQL 必须持有唯一控制面行和数据面库。
    tenant_slug="fault-lb-$(date +%s)"
    if create_tenant_probe "$scenario" "$tenant_slug"; then
        assert_tenant_provisioned "$scenario" "$tenant_slug"
    else
        record "$scenario" "tenant_create_on_survivor" fail "control-plane tenant creation failed"
    fi

    docker compose -f "$COMPOSE_FILE" start coordinator-b >/dev/null 2>&1
    wait_until "coordinator-b back" 120 3 sh -c \
        'docker compose -f '"$COMPOSE_FILE"' ps coordinator-b --format json 2>/dev/null | grep -q "\"State\":\"running\""'
    assert_true "$scenario" "smoke_coordinator_b_restarted" $?
}

# 前置自检始终执行：指定单场景时登录态（session_jar）也要就绪。
preflight
for name in $SCENARIOS; do
    case "$name" in
        coordinator_pause) scenario_coordinator_pause ;;
        coordinator_kill)  scenario_coordinator_kill ;;
        runner_kill)       scenario_runner_kill ;;
        net_block)         scenario_net_block ;;
        lb_failover)       scenario_lb_failover ;;
        *) echo "unknown scenario: $name" >&2; continue ;;
    esac
    restore_topology
done

ELAPSED=$(( $(date +%s) - SUITE_START ))
echo "elapsed=${ELAPSED}s"
finish_suite
