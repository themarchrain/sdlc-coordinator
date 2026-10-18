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

Then start from `docker-compose.yml`. Agent Runner image uses `alpine:3.21` (git/bash/python 3.12 via apk, go 1.25 reused from the builder). Runtime packages live under `deploy/agent-runner-runtime/` (not committed; Codex is required, claude/node/jdk/maven are optional).

Set `RESULT_CHANNEL_SECRET` to a stable 64-character hex string and `AGENT_RUNNER_INTERNAL_TOKEN` to the same nonempty token on the coordinator and Runner. These deployment credentials are required and are not stored in runtime configuration snapshots. Multiple coordinators must share the result-channel secret.

The coordinator prepares, delivers, and cleans up workspaces through the Runner HTTP API, including on a single host with SQLite. Only the Runner mounts the workspace directory. Its `AGENT_RUNNER_WORKSPACE_ROOT` environment variable selects the root; changing it requires updating the deployment and recreating the Runner container. The admin configuration API and runtime snapshots contain no workspace-root settings.

Admin UI: `http://<host>:<port>/admin`

The first super admin is seeded from `CONTROL_PLANE_SUPERADMIN_USERNAME` / `CONTROL_PLANE_SUPERADMIN_PASSWORD` environment variables at startup; startup refuses to run without `CONTROL_PLANE_DSN` and these credentials. Do not expose this service to the public internet.

## Tests

```bash
go test ./...
cd web && npm ci && npm test -- --run && npm run typecheck && npm run build
```

GitHub Actions runs the same checks on `main` and pull requests.

## License

Apache License 2.0. See [LICENSE](./LICENSE).
