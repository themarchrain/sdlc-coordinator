# sdlc-coordinator

Control plane for the software development life cycle (SDLC).

It assigns coding agents to repository work, keeps the official task/run state, and writes results back to your code host. It is not an agent runtime.

## Stack

- Go + Gin + Gorm + Zap
- SQLite (`github.com/glebarez/sqlite`)
- Admin UI: React 18 + TypeScript + Vite + Ant Design (`web/`)

## Layout

- `cmd/coordinator` — control plane
- `cmd/agent-runner` — execution plane
- `web/` — admin console, embedded into the coordinator binary
- `docs/` — public architecture notes (add as needed)
- `.local/` — working notes and local secrets; gitignored

## Run

Build the coordinator image with `build.sh`. Compose expects an external Docker network:

```bash
docker network create sdlc-net
```

Then start from `docker-compose.yml`. Agent Runner image uses `alpine:3.21` (git/bash/python 3.12 via apk, go 1.25 reused from the builder). Runtime packages live under `deploy/agent-runner-runtime/` (not committed; Codex is required, claude/node/jdk/maven are optional). The coordinator image includes git for workspace preparation.

Admin UI: `http://<host>:<port>/admin`

The first super admin is seeded from `CONTROL_PLANE_SUPERADMIN_USERNAME` / `CONTROL_PLANE_SUPERADMIN_PASSWORD` environment variables at startup; startup refuses to run without `CONTROL_PLANE_DSN` and these credentials. Do not expose this service to the public internet.

## Logging

Both binaries write structured logs to stderr and to a daily JSON file (`<prefix>-<YYYY-MM-DD>.log`), keeping 30 days of archives. The coordinator uses `COORDINATOR_LOG_LEVEL` / `COORDINATOR_LOG_DIR` and its daily file follows the same level. The Agent Runner has its own settings, never inherits the coordinator ones, and keeps its daily file at `debug` and above regardless of the terminal level:

- `AGENT_RUNNER_LOG_LEVEL` — minimum level for the terminal stream (default `info`); a quieter terminal loses nothing on disk.
- `AGENT_RUNNER_LOG_DIR` — directory for the daily files (default `logs/agent-runner`, relative to the working directory). The image sets `/app/logs/agent-runner`, backed by the `./runner-logs` host mount in `docker-compose.yml`. Startup fails with a non-zero exit when the log directory cannot be created or written.

Watch the live stream and the persisted file:

```bash
docker compose logs -f agent-runner
tail -f runner-logs/agent-runner-$(date +%F).log
```

## Tests

```bash
go test ./...
cd web && npm ci && npm test -- --run && npm run typecheck && npm run build
```

GitHub Actions runs the same checks on `main` and pull requests.

## License

Apache License 2.0. See [LICENSE](./LICENSE).
