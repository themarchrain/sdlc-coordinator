# Connect a GitHub Repository (After Deployment)

[中文版](github-repository-integration.zh-CN.md)

This guide explains how to connect one GitHub repository to an already deployed SDLC Coordinator. It is not a full deployment guide and does not cover organization creation, AI provider setup, or Runner installation.

## Before you start

- The target GitHub repository exists, and you have permission to manage it.
- You can sign in to the tenant administrator account for the target organization in Coordinator.
- Coordinator has a publicly reachable HTTPS address, and its reverse proxy forwards requests to Coordinator.
- Generate a random Webhook Secret. It is a signing secret and is separate from the GitHub PAT.

If the repository does not exist yet, have a GitHub organization administrator create it under the target organization first. Use `OWNER/REPOSITORY` for the repository path throughout this guide, for example `acme/service-api`.

## 1. Create a fine-grained PAT for only this repository

Open [Fine-grained personal access tokens](https://github.com/settings/personal-access-tokens/new) and configure the token as follows:

1. Set **Resource owner** to the personal account or organization that owns the repository. The organization may require an administrator to approve new tokens; approve a token that is Pending before continuing.
2. Under **Repository access**, select **Only select repositories** and choose only the repository you are connecting.
3. Under **Repository permissions**, set:

   | Permission | Access | Why it is needed |
   |---|---|---|
   | **Contents** | **Read and write** | The Runner reads the repository and pushes development branches; Coordinator needs Contents write access to merge a PR. |
   | **Issues** | **Read and write** | Read and update Issues, write normal Issue/PR comments, and manage Issue labels. |
   | **Pull requests** | **Read and write** | Read and create PRs and perform pull-request operations. |
   | **Metadata** | **Read-only** | Basic repository information; GitHub usually adds this automatically. |

   GitHub's permission reference lists the permissions required for creating and merging PRs and for Issue/comment operations. This project creates the Webhook manually in repository settings, so the PAT does not need **Webhooks** or **Administration** access.
4. Choose an expiration that follows your organization's policy. Copy the token securely when GitHub displays it. Never put a PAT in an Issue, chat, document, or Git repository.

References: [Fine-grained PAT permission reference](https://docs.github.com/en/rest/authentication/permissions-required-for-fine-grained-personal-access-tokens) and [Managing your personal access tokens](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens).

## 2. Register and enable the repository in Coordinator

Sign in as the organization's tenant administrator and open **Repository Management → Add Repository**. Fill in:

| Field | Value |
|---|---|
| **Display name** | A recognizable name for Coordinator. |
| **Platform** | `GitHub`. |
| **Platform URL** | `https://github.com`. |
| **Repository path** | `owner/repository`; do not paste a browser URL containing `/settings` or append `.git`. |
| **Project Token** | Paste the fine-grained PAT created above. |
| **Project Token confirmation** | Check the box after confirming the token is scoped to this repository. |
| **Webhook Secret** | Enter the generated random secret and keep a copy for the next step. Do not enter the PAT here. |
| **Verification command** | Optional. Enter a test or build command that the Runner can actually execute, such as `go test ./...` for a Go project. This command runs during automated verification. |
| **Git author name / email** | Optional. If set, configure both; they become public Git commit metadata. Use an authorized bot identity or a no-reply address. |

After saving, Coordinator uses the PAT to look up the repository identity and default branch. When that succeeds, enable the repository from its row and confirm its status is **enabled** with no recent configuration error. If the identity probe fails, check organization approval, repository selection, PAT permissions, and the `owner/repository` path.

## 3. Configure the GitHub repository Webhook

Open **Settings → Webhooks → Add webhook** in the target repository and enter:

| Field | Value |
|---|---|
| **Payload URL** | `https://<YOUR_PUBLIC_COORDINATOR_DOMAIN>/api/events/github`. Replace the example host with your HTTPS domain; do not use an MCP callback URL or the GitHub repository URL. |
| **Content type** | `application/json`. |
| **Secret** | Exactly the same value as the repository's **Webhook Secret** saved in Coordinator. |
| **SSL verification** | Keep enabled. |
| **Which events would you like to trigger this webhook?** | Select **Let me select individual events**, then select only the three events below. |
| **Active** | Enable it. |

Subscribe to these events only:

| GitHub event | How Coordinator uses it |
|---|---|
| **Issues** | A new Issue or the addition of the `developer-agent` label to an Issue. |
| **Issue comments** | Normal comments on Issues and PRs; Coordinator also checks comment content and requester authorization. |
| **Pull requests** | PR opened, reopened, closed, and source-branch updates. An incremental update is the `synchronize` action, so you do not need to select **Pushes** separately. |

Click **Add webhook**. GitHub automatically sends a `ping` delivery. Coordinator may record it as ignored because a `ping` verifies delivery and signature but does not create a task. Recent Deliveries in GitHub should show a successful 2xx response.

References: [Creating webhooks](https://docs.github.com/en/webhooks/using-webhooks/creating-webhooks) and [Webhook events and payloads](https://docs.github.com/en/webhooks/webhook-events-and-payloads).

## 4. Run a controlled first check

1. In GitHub, open **Settings → Webhooks → Recent Deliveries** and confirm the initial `ping` received a 2xx response. It is normal for Coordinator to show the `ping` as ignored.
2. In Coordinator's repository management page, confirm the repository is **enabled** and both the PAT and Webhook Secret show as configured. The page does not reveal their values again.
3. A first task starts a real Agent and may create a branch or PR. Use a small test Issue whose changes are safe before trying production work.
4. Create the Issue and add the exact `developer-agent` label. This label is the current GitHub Issue workflow's start gate; an Issue without it will not start development.
5. Review the task details in Coordinator and the resulting Git branch/PR. Confirm development, review, and test results refer to the latest commit.

Organization-level **automatic merge** is a separate policy setting and is disabled by default. For an initial trial, review the PR and test results manually; enable automatic merge only after an explicit organization decision.

## Troubleshooting

| Symptom | Check first |
|---|---|
| Repository cannot be enabled or the identity probe reports an error | Organization approval, the selected repository, the Contents/Issues/Pull requests permissions above, and the `owner/repository` path. |
| GitHub delivery returns 401 | The Webhook Secret must exactly match the value stored in Coordinator; confirm the repository is registered and the Payload URL is correct. |
| Delivery fails, times out, or returns 5xx | HTTPS certificate, reverse-proxy forwarding, public reachability, and Coordinator service status. |
| `ping` succeeds but is shown as ignored | Expected. A `ping` checks the endpoint; it does not verify the Issue/PR workflow. |
| An Issue does not start a task | Confirm the Issue is open, has the exact `developer-agent` label, and the Webhook subscribes to **Issues**. |
| A PR update does not trigger work | Confirm **Pull requests** is selected and inspect that delivery in Recent Deliveries. Incremental updates are the PR `synchronize` action, not a separate `push` event. |
