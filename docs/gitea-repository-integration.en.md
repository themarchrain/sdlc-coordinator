# Connect a Gitea Repository (After Deployment)

[中文版](gitea-repository-integration.zh-CN.md)

This guide explains how to connect one Gitea repository to an already deployed SDLC Coordinator. It is not a full deployment guide and does not cover organization creation, AI provider setup, or Runner installation.

## Before you start

- The target Gitea repository exists, and you have permission to manage it.
- You can sign in to the tenant administrator account for the target organization in Coordinator.
- The Coordinator webhook endpoint is reachable from the **Gitea instance**, not merely from your browser.
- Generate a random Webhook Secret. It is a signing secret and is separate from the access token.

Use `OWNER/REPOSITORY` for the repository path throughout this guide, for example `acme/service-api`.

### About the platform URL

The platform URL you enter when registering the repository is the address that Coordinator and the Runner use **at runtime**. It serves two purposes: calling `/api/v1/...` endpoints, and building the clone URL `PLATFORM_URL/OWNER/REPOSITORY.git`.

Therefore:

- With a split deployment, enter a publicly or internally reachable Gitea address such as `https://gitea.example.com`.
- When Gitea and Coordinator share a container network, use the in-network service name and port, for example `http://gitea:3000`.
- Do not enter `localhost:PORT` from your browser address bar: inside a container `localhost` refers to the container itself, so cloning will always fail.

## 1. Prepare an access token limited to the target repository

Gitea personal access tokens are **account-scoped with permission scopes**; there is no token type that is limited to a single repository as in GitHub. To keep the effective reach down to one repository, grant a dedicated bot account collaborator access to that repository only, then issue the token from that account.

In Gitea, open **Settings → Applications → Generate Token** and select:

| Scope | Access | Why it is needed |
|---|---|---|
| **repository** | **Read / Write** | The Runner reads the repository and pushes development branches; Coordinator reads pull-request state and performs the merge. |
| **issue** | **Read / Write** | Read Issues, write Issue/PR comments, and manage labels. |
| **user** | **Read** (optional) | Resolve the token owner for commit authorship and audit. |

Copy the token securely as soon as it is displayed. Never put a token in an Issue, chat, document, or Git repository.

Gitea does not provide project-level (repository-level) tokens, only account-scoped PATs, so the registration form does not ask for a project-level declaration; narrow the reach with a dedicated account instead.

## 2. Register and enable the repository in Coordinator

Sign in as the organization's tenant administrator and open **Repository Management → Add Repository**. Fill in:

| Field | Value |
|---|---|
| **Display name** | A recognizable name for Coordinator. |
| **Platform** | `Gitea`. |
| **Platform URL** | See "About the platform URL"; it must be reachable from both Coordinator and the Runner. |
| **Repository path** | `owner/repository`; do not paste a browser URL containing `/settings` or append `.git`. |
| **Access token** | Paste the token created above. |
| **Webhook Secret** | Enter the generated random secret and keep a copy for the next step. Do not enter the token here. |
| **Verification command** | Optional. Enter a test or build command the Runner can actually execute, such as `go test ./...` for a Go project. It runs during automated verification. |
| **Commit author name / email** | Optional; if provided, both must be set and they will appear in public Git history. Leave both empty to use the built-in identity. |

On save, Coordinator immediately calls `/api/v1/repos/owner/repository` with the token to probe the repository identity and default branch. A failed probe does not roll back: the repository row is kept in **configuring** status and the failure reason is recorded in the *recent configuration error* column. Whether the probe succeeds or not, a newly added repository stays in **configuring** and requires an administrator to explicitly click enable in the list; enabling probes the repository once more, and only a successful enable moves it to **enabled** with an empty recent configuration error.

## 3. Configure the Gitea webhook

Open the target repository's **Settings → Webhooks → Add Webhook → Gitea** and fill in:

| Field | Value |
|---|---|
| **Target URL** | `<Coordinator address>/api/events/gitea`. It must be reachable from the Gitea instance; do not enter the MCP callback address or the repository web URL. |
| **HTTP method** | `POST`. |
| **POST content type** | `application/json`. |
| **Webhook name** | A recognizable name, for example `sdlc-coordinator`. |
| **Active** | Checked. |
| **Authorization header** | Leave empty. Signing uses a separate header, not Authorization. |
| **Secret** | Exactly the **Webhook Secret** stored for this repository in Coordinator, with no leading or trailing whitespace. |
| **Branch filter** | Keep `*`. |

Subscribe to these events only (under custom events):

| Gitea event | How this project uses it |
|---|---|
| **Issues** | Starts a task when an Issue is created with the trigger label. |
| **Issue label updated** | The common UI flow creates an Issue first and labels it afterwards, which emits `label_updated`; it is treated the same as "created already labeled". |
| **Issue comment** | Receives comments on Issues for rework and follow-up. |
| **Pull request** | The pull-request lifecycle: opened, reopened, closed. |
| **Pull request comment** | Receives comments on pull requests for rework. |
| **Pull request synchronized** | A new commit pushed to the source branch changes the PR head and requires re-review and re-verification. |

Do not select **Push**: incremental commits on a PR are covered by "pull request synchronized", and subscribing to push only produces events the system ignores. Repository, release, status, wiki, package, workflow, assignment and milestone events are not consumed either.

### Signing

Gitea sends `X-Gitea-Signature` as the hexadecimal string of `HMAC-SHA256(secret, request body)`, **without a `sha256=` prefix**, compared case-sensitively.

Coordinator trims leading and trailing whitespace from the Webhook Secret when saving, while Gitea computes the signature over the value exactly as entered. If pasting the secret into Gitea brings along a newline or spaces, signature verification fails permanently with 401. After pasting, visually confirm there are no trailing characters.

## 4. Run one controlled first verification

1. In the Gitea webhook detail page, click **Test Delivery**. The recent delivery should return 2xx. Gitea's test payload is a sample it generates itself and will usually appear as `ignored` in Coordinator; that only proves the endpoint and signature work, not that the workflow works.
2. Confirm the Coordinator repository page shows the repository as **enabled**, with both the token and the Webhook Secret marked as configured. Secrets are never displayed again.
3. The first task really launches the Agent and may create branches or pull requests. Start with a very small test Issue that is allowed to be modified automatically.
4. Add the exact label `developer-agent` to that Issue. It is the entry gate for the current workflow; an Issue without the label starts nothing.
5. Check the Coordinator task detail and the Gitea branch/PR to confirm development, review and test results all correspond to the latest commit.

Organization-level **auto-merge** is a separate policy switch and is off by default. When it is on, a merge happens only if all of the following hold:

- the verification task's `verification_verdict` is passing and its bound head equals the task head;
- the review task's `review_verdict` is approved and it reviewed the same head;
- the repository row exists and is enabled, and the platform exposes a merge capability;
- the live head read back immediately before merging still equals the evidence head.

If any condition fails, the gate falls back to manual handling and writes a merge-gate audit record.

## Differences from GitHub integration

| Aspect | Difference on Gitea |
|---|---|
| Token granularity | Account-scoped with permission scopes; no repository-level token. Use a dedicated account to narrow reach. |
| Signature header | `X-Gitea-Signature`, hexadecimal, no `sha256=` prefix. |
| Event naming | Separate event types such as `issue_label` and `pull_request_sync`, instead of a single `issues`/`pull_request` event with actions. |
| PR comments | At the API level, Gitea treats pull-request comments as issue comments, written through `/issues/{index}/comments`. |
| Merge response | A successful merge returns an empty `200` with no result body, so the PR must be read back to confirm `merged` and the merge commit SHA. |

## Troubleshooting

| Symptom | Check first |
|---|---|
| Repository enable or identity probe fails | Token validity, collaborator access to the repository, repository read scope, the `owner/repository` path, and whether the platform URL is reachable from Coordinator. |
| Webhook delivery returns 401 | Whether the Webhook Secret exactly matches the value stored in Coordinator (pay attention to trailing whitespace and newlines), whether the repository is registered, and whether the path is `owner/repository`. |
| Test delivery returns 2xx but shows `ignored` | Expected. Test payloads usually carry no `developer-agent` label; endpoint verification is not workflow verification. |
| An Issue starts no task | The Issue is open, the label is exactly `developer-agent`, and both **Issues** and **Issue label updated** are subscribed. |
| A new PR commit does not trigger re-review | **Pull request synchronized** is subscribed; that is the PR `synchronized` action, not a separate push. |
| Auto-merge does not happen | Whether organization auto-merge is enabled, whether the scheduler is running (the gate runs each tick), whether the evidence chain is fully green, whether the live head moved, and whether the platform rejected the merge. Inspect the `decision` and reasons in `merge_gate_audits`. |
