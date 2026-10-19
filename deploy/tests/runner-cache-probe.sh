#!/bin/sh
set -eu

phase=$1
root=$AGENT_RUNNER_CACHE_ROOT
test "$(id -u)" = 10002
test "$(id -g)" = 10002
test ! -r /app/runner-data/operations
test ! -x /usr/local/bin/agent-runner-cli-launcher
printf '%s\n' "$HOME" > home.path
mkdir -p projects
cd projects

if [ "$phase" = seed ]; then
    mkdir -p "$root/test-fixtures" go npm pnpm
    printf 'module cacheprobe\n\ngo 1.22\n\nrequire github.com/google/uuid v1.3.0\n' > go/go.mod
    printf 'package cacheprobe\nimport "github.com/google/uuid"\nfunc ID() string { return uuid.NewString() }\n' > go/probe.go
    (cd go && go mod tidy && go test ./...)
    printf '{"private":true,"dependencies":{"is-number":"7.0.0"}}\n' | tee npm/package.json > pnpm/package.json
    (cd npm && npm install --ignore-scripts --no-audit --no-fund)
    (cd pnpm && pnpm install --ignore-scripts)
    cp go/go.mod go/go.sum go/probe.go "$root/test-fixtures/"
    cp pnpm/pnpm-lock.yaml "$root/test-fixtures/"
    cp npm/package.json npm/package-lock.json "$root/test-fixtures/"
    curl -fsSL https://pypi.org/pypi/idna/3.10/json | jq -r '.urls[] | select(.filename == "idna-3.10-py3-none-any.whl") | .url + "#sha256=" + .digests.sha256' > "$root/test-fixtures/pip-url"
else
    mkdir go npm pnpm
    cp "$root/test-fixtures/go.mod" "$root/test-fixtures/go.sum" "$root/test-fixtures/probe.go" go/
    cp "$root/test-fixtures/package.json" "$root/test-fixtures/package-lock.json" npm/
    cp "$root/test-fixtures/package.json" "$root/test-fixtures/pnpm-lock.yaml" pnpm/
    (cd go && GOPROXY=off GOSUMDB=off go test ./...)
    (cd npm && npm ci --offline --ignore-scripts --no-audit --no-fund)
    (cd pnpm && pnpm install --offline --frozen-lockfile --ignore-scripts --fetch-retries=0 --fetch-timeout=10000)
fi

node -e 'if (!require("./npm/node_modules/is-number")(7)) process.exit(1)'
node -e 'if (!require("./pnpm/node_modules/is-number")(7)) process.exit(1)'
store=$(pnpm store path)
case "$store" in "$root/pnpm"|"$root/pnpm"/*) ;; *) printf 'unexpected pnpm store: %s\n' "$store"; exit 1 ;; esac
pip3 download --no-deps --dest wheels "$(cat "$root/test-fixtures/pip-url")" > pip.log 2>&1
test -f wheels/idna-3.10-py3-none-any.whl
if [ "$phase" != seed ]; then grep 'Using cached' pip.log; fi

printf '<project><modelVersion>4.0.0</modelVersion><groupId>probe</groupId><artifactId>probe</artifactId><version>1</version></project>\n' > pom.xml
offline=
if [ "$phase" != seed ]; then offline=-o; fi
mvn -B $offline org.apache.maven.plugins:maven-dependency-plugin:3.8.1:get -Dartifact=org.apache.commons:commons-lang3:3.17.0 -Dtransitive=false > maven.log 2>&1
test -s "$root/maven/org/apache/commons/commons-lang3/3.17.0/commons-lang3-3.17.0.jar"
if [ "$phase" = seed ]; then
    test ! -e "$root/maven/org/apache/commons/commons-math3/3.6.1/commons-math3-3.6.1.jar"
    mvn -X -B org.apache.maven.plugins:maven-dependency-plugin:3.8.1:get -Dartifact=org.apache.commons:commons-math3:3.6.1 -Dtransitive=false > concurrent-a.log 2>&1 &
    first=$!
    mvn -X -B org.apache.maven.plugins:maven-dependency-plugin:3.8.1:get -Dartifact=org.apache.commons:commons-math3:3.6.1 -Dtransitive=false > concurrent-b.log 2>&1 &
    second=$!
    wait "$first"
    wait "$second"
    test -s "$root/maven/org/apache/commons/commons-math3/3.6.1/commons-math3-3.6.1.jar"
    grep -F "nameMapper 'file-gav' and factory 'file-lock'" concurrent-a.log
    grep -F "nameMapper 'file-gav' and factory 'file-lock'" concurrent-b.log
fi
printf retained > "$root/runner-probe"
printf '%s\n' "runner cache $phase: PASS"
