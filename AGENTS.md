# AGENTS.md

## What this repository is

`sdlc-coordinator` is the control plane for multi-repository SDLC automation. It is not a business repository and not an agent runtime.

It owns:

- task discovery, claim, dedupe, lease, retry, recovery
- code-host events and artifacts as structured records
- Agent Runner execution start, context injection, result reconciliation, and recovery

It does not own:

- product/business code
- the core Agent Runner or CLI execution engines
- treating GitLab/GitHub/Gitea labels or comments as the source of truth

## Stack

- Go + Gin + Gorm + Zap
- SQLite (`github.com/glebarez/sqlite`)
- Admin UI: React 18 + TypeScript + Vite + Ant Design + React Router

## Documents

- Public architecture belongs in `docs/`.
- Working notes, specs in progress, and anything with secrets belong in `.local/` (gitignored).
- Do not commit `.local/`.

## Consensus

- The coordinator is the only state source.
- The independent Agent Runner is the execution plane, not a scheduler.
- Git Issue / MR / Label / Comment are external projections.
- MVP workers: TesterWorker, DeveloperWorker, ReviewerWorker.
- Developer/Reviewer results return through the coordinator result ingress. The coordinator advances state and performs external side effects.

## Working rules

- New roles, states, or external systems need a public `docs/` note before the implementation lands.
- Code-host control-plane side effects go through adapters. Agents do not call CodeHost APIs directly.
- Persist state first, then emit external side effects.
- If a design would make the coordinator stop being the only state source, do not do it.
- Except merge commits, commit messages must use this exact shape:

  ```
  feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert: <concise one-line summary>

  - Key point 1
  - Key point 2
  - ...
  ```

  A scope segment is not allowed, e.g. `fix(auth): ...`.
