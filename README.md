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

Then start from `docker-compose.yml`. Agent Runner runtime packages live under `deploy/agent-runner-runtime/` (not committed).

Admin UI: `http://<host>:<port>/admin`

Default first-boot login is `admin` / `admin123`. Change it before any shared deployment. Do not expose this service to the public internet.

## Tests

```bash
go test ./...
cd web && npm ci && npm test -- --run && npm run typecheck && npm run build
```

## License

Apache License 2.0. See [LICENSE](./LICENSE).
