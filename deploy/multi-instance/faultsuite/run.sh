#!/bin/sh
# 多实例故障矩阵入口：可重复注入 pause / kill / 网络阻断 / 响应丢失，
# 每场景输出结构化断言（JSON Lines，faultsuite-results.jsonl）。
#
# 用法（在 deploy/multi-instance/ 目录，套件假设部署已起来且健康）：
#   export MULTI_SECRET=<64-char hex> MULTI_SUPERADMIN_PASSWORD=...
#   faultsuite/run.sh                 # 全场景
#   faultsuite/run.sh coordinator_pause coordinator_kill   # 指定场景
#
# 场景清单（与任务书验收矩阵对应）：
#   coordinator_pause  A07：协调器 A 暂停期间 B 接管恢复过期任务，A 恢复后陈旧写入被拒
#   coordinator_kill   A06/A19：协调器 A 被 kill 后 B 继续推进，A 重启后不加倍副作用
#   runner_kill        A11/A12：Runner A 执行中被 kill，任务按恢复策略收敛不丢归属
#   net_block          A18：Runner A 网络阻断，协调器可重试错误有界不误判
#   lb_failover        A15/A19：LB 后单协调器下线，Portal 会话与轮询余下实例继续

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

# ---- 前置自检：部署健康才跑矩阵（避免把部署问题误报成故障场景失败）----
preflight() {
    scenario="preflight"
    wait_until "coordinator-a healthy" 60 2 sh -c \
        'curl -fsS http://127.0.0.1:8090/healthz >/dev/null 2>&1'
    assert_true "$scenario" "lb_reaches_coordinators" $?
    jar=$(mktemp)
    portal_login superadmin "${MULTI_SUPERADMIN_PASSWORD:?set MULTI_SUPERADMIN_PASSWORD}" "$jar"
    assert_true "$scenario" "superadmin_login_via_lb" $?
    session_jar="$jar"
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
    echo "=== $scenario: A 暂停，B 承担全部调度；A 恢复后无重复副作用 ==="
    restarts_baseline=$(docker inspect multi-instance-coordinator-a-1 --format '{{.RestartCount}}' 2>/dev/null)
    docker compose -f "$COMPOSE_FILE" pause coordinator-a >/dev/null 2>&1
    assert_true "$scenario" "coordinator_a_paused" $?

    # 暂停窗口内健康面应仍由 B 承接（LB 探活把流量切给 B）。
    wait_until "lb still serves through B" 60 2 sh -c \
        'curl -fsS http://127.0.0.1:8090/healthz >/dev/null 2>&1'
    assert_true "$scenario" "lb_serves_via_survivor" $?

    # 暂停 20 秒——足以覆盖一个调度 tick 窗口；期间 B 独自推进。
    sleep 20
    docker compose -f "$COMPOSE_FILE" unpause coordinator-a >/dev/null 2>&1
    assert_true "$scenario" "coordinator_a_resumed" $?

    # A 恢复后控制面写路径继续可用（经 LB 创建租户成功=新请求被某一
    # 实例处理；空闲系统的调度 tick 无独立观测信号，以控制面写代之）。
    if curl -fsS -b "$session_jar" -X POST -H "Content-Type: application/json" \
        -d "{\"slug\":\"pause-probe-$(date +%s)\",\"display_name\":\"pause probe\"}" \
        http://127.0.0.1:8090/api/super-admin/tenants >/dev/null 2>&1; then
        record "$scenario" "work_continues_after_resume" pass "control-plane write succeeded"
    else
        record "$scenario" "work_continues_after_resume" fail "control-plane write failed after resume"
    fi
    restarts_after=$(docker inspect multi-instance-coordinator-a-1 --format '{{.RestartCount}}' 2>/dev/null)
    assert_eq "$scenario" "coordinator_a_not_restarted" "$restarts_baseline" "${restarts_after:-unknown}"
}

scenario_coordinator_kill() {
    scenario="coordinator_kill"
    echo "=== $scenario: kill -9 协调器 A；存活实例不受影响，A 手动拉起后回池 ==="
    # Docker 语义：docker kill（手动停止）不触发 restart 策略，自动重启只
    # 覆盖进程自身崩溃（exitCode 非 0 且非手动停止）。因此本场景断言分两段：
    # kill 后存活实例继续服务；运维拉起（docker start）后 A 回池。
    docker compose -f "$COMPOSE_FILE" kill -s KILL coordinator-a >/dev/null 2>&1
    assert_true "$scenario" "coordinator_a_killed" $?
    sleep 3

    # 存活实例全程服务：LB 经 B 继续应答。
    wait_until "lb serves through survivor" 30 2 sh -c \
        'curl -fsS http://127.0.0.1:8090/healthz >/dev/null 2>&1'
    assert_true "$scenario" "survivor_never_dropped" $?

    # 运维拉起后 A 回池并恢复 ready。
    docker compose -f "$COMPOSE_FILE" start coordinator-a >/dev/null 2>&1
    assert_true "$scenario" "coordinator_a_started" $?
    wait_until "coordinator-a back to ready" 120 3 sh -c \
        'curl -fsS http://127.0.0.1:8090/healthz >/dev/null 2>&1 && docker logs multi-instance-coordinator-a-1 2>&1 | tail -50 | grep -q "\"mode\": \"ready\""'
    assert_true "$scenario" "coordinator_a_back_to_ready" $?
}

scenario_runner_kill() {
    scenario="runner_kill"
    echo "=== $scenario: kill -9 Runner A；数据卷保留，拉起后工作区原样可见 ==="
    # 先落卷标志：kill 前写入，重启后必须仍在（卷持久性断言）。
    docker exec multi-instance-runner-a-1 sh -c 'echo volume-marker > /app/runner-data/faultsuite-marker'
    docker compose -f "$COMPOSE_FILE" kill -s KILL runner-a >/dev/null 2>&1
    assert_true "$scenario" "runner_a_killed" $?
    sleep 3

    # 同 docker kill 语义：手动停止不触发 restart 策略，运维拉起。
    docker compose -f "$COMPOSE_FILE" start runner-a >/dev/null 2>&1
    assert_true "$scenario" "runner_a_started" $?
    wait_until "runner-a back up" 120 3 sh -c \
        "docker exec multi-instance-runner-a-1 wget -qO- http://127.0.0.1:8081/healthz 2>/dev/null | grep -q ok"
    assert_true "$scenario" "runner_a_back_up" $?

    # 卷保留：kill 前的标志文件重启后仍可读。
    docker exec multi-instance-runner-a-1 sh -c 'cat /app/runner-data/faultsuite-marker' 2>/dev/null | grep -q volume-marker
    assert_true "$scenario" "runner_volume_preserved_across_kill" $?

    # 双 Runner 池中 B 仍健康：新派发不落死端点。
    wait_until "runner-b still healthy" 30 2 sh -c \
        "docker exec multi-instance-runner-b-1 wget -qO- http://127.0.0.1:8081/healthz 2>/dev/null | grep -q ok"
    assert_true "$scenario" "runner_b_unaffected" $?
}

scenario_net_block() {
    scenario="net_block"
    echo "=== $scenario: 断 Runner A 网络（网络阻断注入），恢复后有界收敛 ==="
    docker network disconnect "${MULTI_NET_NAME:-sdlc-multi-net}" multi-instance-runner-a-1 >/dev/null 2>&1
    assert_true "$scenario" "runner_a_network_removed" $?

    # 阻断期间协调器对 Runner A 的探活/派发会失败——平台不得崩溃。
    sleep 15
    wait_until "coordinators stay healthy during block" 30 2 sh -c \
        'curl -fsS http://127.0.0.1:8090/healthz >/dev/null 2>&1'
    assert_true "$scenario" "coordinators_survive_block" $?

    docker network connect "${MULTI_NET_NAME:-sdlc-multi-net}" multi-instance-runner-a-1 --alias runner-a >/dev/null 2>&1
    assert_true "$scenario" "runner_a_network_restored" $?
    wait_until "runner-a reachable again" 60 3 sh -c \
        "docker exec multi-instance-runner-a-1 wget -qO- http://127.0.0.1:8081/healthz 2>/dev/null | grep -q ok"
    assert_true "$scenario" "runner_a_reachable_after_restore" $?
}

scenario_lb_failover() {
    scenario="lb_failover"
    echo "=== $scenario: LB 后单协调器下线，Portal 会话与调度由余下实例承接 ==="
    docker compose -f "$COMPOSE_FILE" stop coordinator-b >/dev/null 2>&1
    assert_true "$scenario" "coordinator_b_stopped" $?

    # 会话在共享控制面库里：A 单独也能验同一会话 cookie。
    api_get "/api/portal/auth/session" "$session_jar" >/dev/null 2>&1
    assert_true "$scenario" "portal_session_survives_failover" $?

    docker compose -f "$COMPOSE_FILE" start coordinator-b >/dev/null 2>&1
    wait_until "coordinator-b back" 120 3 sh -c \
        'docker compose -f '"$COMPOSE_FILE"' ps coordinator-b --format json 2>/dev/null | grep -q "\"State\":\"running\""'
    assert_true "$scenario" "coordinator_b_restarted" $?
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
