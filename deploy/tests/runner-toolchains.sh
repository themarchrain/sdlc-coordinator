#!/bin/sh
set -eu

test "$(id -u)" = 10002
for tool in go gofmt java javac node npm pnpm mvn python3 pip3; do
    command -v "$tool"
done

scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT
printf 'class Hello { public static void main(String[] args) { System.out.print("ready"); } }\n' > "$scratch/Hello.java"
for version in 8 17 21; do
    case "$version" in
        8) jdk=$JAVA8_HOME; expected='1.8.' ;;
        17) jdk=$JAVA17_HOME; expected='17.' ;;
        21) jdk=$JAVA21_HOME; expected='21.' ;;
    esac
    "$jdk/bin/java" -version 2>&1 | grep "version \"$expected"
    mkdir "$scratch/classes-$version"
    "$jdk/bin/javac" -d "$scratch/classes-$version" "$scratch/Hello.java"
    test "$("$jdk/bin/java" -cp "$scratch/classes-$version" Hello)" = ready
done
test "$JAVA_HOME" = "$JAVA21_HOME"
node -e 'if (process.versions.node.split(".")[0] !== "24") process.exit(1)'
npm --version
pnpm --version
mvn --version | grep 'Java version: 21'
python3 -m venv "$scratch/venv"
"$scratch/venv/bin/python" -c 'import sys; assert sys.prefix != sys.base_prefix'
pip3 --version

mkdir "$scratch/go"
printf 'module smoke\n\ngo 1.22\n' > "$scratch/go/go.mod"
printf 'package smoke\nfunc Ready() bool { return true }\n' > "$scratch/go/smoke.go"
(cd "$scratch/go" && GOPROXY=off GOTOOLCHAIN=local go test ./...)
for shell in /bin/sh /bin/bash; do
    for mode in -c -lc; do
        "$shell" "$mode" 'command -v go; command -v node; command -v pnpm; command -v mvn; java -version 2>&1 | grep "version \"21\."'
    done
    env PATH="/runner-custom-bin:$PATH" EXPECTED_PATH="/runner-custom-bin:$PATH" "$shell" -lc 'test "$PATH" = "$EXPECTED_PATH"'
done

printf '<project><modelVersion>4.0.0</modelVersion><groupId>smoke</groupId><artifactId>smoke</artifactId><version>1</version></project>\n' > "$scratch/pom.xml"
mvn -X -o -B -f "$scratch/pom.xml" validate > "$scratch/maven.log" 2>&1
grep -F "Using local repository at $AGENT_RUNNER_CACHE_ROOT/maven" "$scratch/maven.log"
printf '%s\n' 'runner toolchains: PASS'
