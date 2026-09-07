#!/bin/sh
set -eu

mkdir -p /app/runner-data /app/workspaces
chown runner:runner /app/runner-data /app/workspaces

exec /usr/sbin/gosu runner "$@"
