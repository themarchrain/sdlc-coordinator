#!/bin/sh
# 故障矩阵公共库：结构化断言原语。
# 断言输出为 JSON Lines（每行一个场景断言记录）。
# 用法：套件脚本 source 本文件后调用 assert_* 与 finish_suite。

SUITE_NAME="${SUITE_NAME:-faultsuite}"
RESULTS_FILE="${RESULTS_FILE:-$(pwd)/faultsuite-results.jsonl}"
PASS_COUNT=0
FAIL_COUNT=0

# record <scenario> <assertion> <pass|fail> <detail>
record() {
    scenario="$1"; assertion="$2"; verdict="$3"; detail="$4"
    printf '{"ts":"%s","suite":"%s","scenario":"%s","assertion":"%s","result":"%s","detail":"%s"}\n' \
        "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$SUITE_NAME" "$scenario" "$assertion" "$verdict" \
        "$(printf '%s' "$detail" | tr '\n' ' ' | tr -d '"' | cut -c1-500)" >> "$RESULTS_FILE"
    if [ "$verdict" = "pass" ]; then
        PASS_COUNT=$((PASS_COUNT + 1))
        echo "PASS [$scenario] $assertion"
    else
        FAIL_COUNT=$((FAIL_COUNT + 1))
        echo "FAIL [$scenario] $assertion: $detail"
    fi
}

assert_eq() {
    scenario="$1"; assertion="$2"; expected="$3"; actual="$4"
    if [ "$expected" = "$actual" ]; then
        record "$scenario" "$assertion" pass "value=$actual"
    else
        record "$scenario" "$assertion" fail "expected=$expected actual=$actual"
    fi
}

assert_true() {
    scenario="$1"; assertion="$2"; condition="$3"
    if [ "$condition" = "0" ]; then
        record "$scenario" "$assertion" pass "condition met"
    else
        record "$scenario" "$assertion" fail "condition failed (exit=$condition)"
    fi
}

finish_suite() {
    echo "----"
    echo "suite=$SUITE_NAME pass=$PASS_COUNT fail=$FAIL_COUNT results=$RESULTS_FILE"
    [ "$FAIL_COUNT" -eq 0 ]
}

# wait_until <description> <timeout_secs> <poll_secs> <command...>
# 命令退出 0 即通过；超时返回非 0。
wait_until() {
    description="$1"; timeout_secs="$2"; poll_secs="$3"; shift 3
    deadline=$(( $(date +%s) + timeout_secs ))
    while [ "$(date +%s)" -lt "$deadline" ]; do
        if "$@" >/dev/null 2>&1; then
            return 0
        fi
        sleep "$poll_secs"
    done
    echo "timeout waiting for: $description" >&2
    return 1
}

# mysql_scalar <sql> —— 在部署样例的 MySQL 上执行查询，返回首行首列。
mysql_scalar() {
    docker compose -f "$COMPOSE_FILE" exec -T mysql mysql -uroot -p"${MULTI_MYSQL_PASSWORD:-multi-root-pass}" \
        -N -B -e "$1" 2>/dev/null | head -n 1
}

# api_get <path> <cookie-jar> —— 经 LB 调 Portal/管理 API（会话走 cookie）。
api_get() {
    path="$1"; jar="$2"
    curl -fsS -b "$jar" "http://127.0.0.1:${MULTI_LB_PORT:-8090}$path" 2>/dev/null
}

# api_post <path> <cookie-jar> <body>
api_post() {
    path="$1"; jar="$2"; body="$3"
    curl -fsS -X POST -b "$jar" -H "Content-Type: application/json" \
        -d "$body" "http://127.0.0.1:${MULTI_LB_PORT:-8090}$path" 2>/dev/null
}

# portal_login <username> <password> <cookie-jar-path> —— 登录并保存会话
# cookie（Portal 会话是 http-only cookie，无响应体 token）。退出码 0=成功。
portal_login() {
    username="$1"; password="$2"; jar="$3"
    curl -fsS -c "$jar" -X POST -H "Content-Type: application/json" \
        -d "{\"username\":\"$username\",\"password\":\"$password\"}" \
        "http://127.0.0.1:${MULTI_LB_PORT:-8090}/api/portal/auth/login" >/dev/null 2>&1
}
