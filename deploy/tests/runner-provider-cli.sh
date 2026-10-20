#!/bin/sh
set -eu

if [ "${1:-}" = --version ]; then
    printf '%s\n' 'runner provider test CLI'
    exit 0
fi

phase=$(cat)
final=
while [ "$#" -gt 0 ]; do
    if [ "$1" = --output-last-message ]; then
        shift
        final=$1
    fi
    shift
done

if ! /bin/bash -lc '/bin/sh /runner-cache-probe.sh "$1"' cache-probe "$phase" > agent-probe.log 2>&1; then
    tail -n 40 agent-probe.log >&2
    exit 1
fi

if [ -n "$final" ]; then
    printf '%s\n' '{"verdict":"approved"}' > "$final"
    printf '%s\n' '{"type":"turn.completed"}'
else
    printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"structured_output":{"verdict":"approved"}}'
fi
