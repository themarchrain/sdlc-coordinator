#!/bin/sh
set -eu

mkdir -p /app/runner-data /app/workspaces
chown runner:runner /app/runner-data /app/workspaces

exec su-exec runner "$@"
