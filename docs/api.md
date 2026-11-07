<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

# pacboard HTTP API (v1)

The single contract between the Go backend (`cmd/`, `internal/`) and the Vue SPA (`web/`).
All JSON uses camelCase. Timestamps are RFC 3339 strings or `null`. The SPA is served at `/`
with history-mode fallback; everything below is under the same origin.

## Data sources

- Pipelines-as-Code `Repository` (`pipelinesascode.tekton.dev/v1alpha1`), Tekton `PipelineRun` and
  `TaskRun` (`tekton.dev/v1`), pod logs. Read-only Kubernetes access (get/list/watch, `pods/log`).
- Run ↔ GitHub data comes from PaC labels and annotations on the PipelineRun, keys
  `pipelinesascode.tekton.dev/<key>` (read the annotation first, then the label):
  `repository`, `url-org`, `url-repository`, `sha`, `sha-url`, `sha-title`, `repo-url`,
  `source-repo-url`, `pull-request`, `branch`, `source-branch`, `event-type`, `sender`,
  `original-prname`, `git-provider`, `check-run-id`, `state`, `log-url`.

## Types

```ts
type RunStatus = "Succeeded" | "Failed" | "Running" | "Pending" | "Cancelled" | "Unknown";

interface GitInfo {
  provider: string;          // "github"
  repoUrl: string;           // https://github.com/<org>/<repo>
  sha: string;               // full SHA, "" if unknown
  shortSha: string;          // first 7 chars
  shaUrl: string;            // commit URL ("" if unknown)
  shaTitle: string;          // commit title
  branch: string;            // target branch (refs/heads/ stripped)
  sourceBranch: string;      // PR head branch
  prNumber: number | null;
  prUrl: string;             // repoUrl + "/pull/" + n, "" if no PR
  eventType: string;         // pull_request, push, retest-all-comment, test-all-comment, ...
  sender: string;            // GitHub login that triggered it
  checkRunUrl: string;       // repoUrl + "/runs/" + check-run-id, "" if unknown
}

interface RunSummary {
  namespace: string;
  name: string;              // PipelineRun name
  prName: string;            // original-prname: the .tekton template name, used by "/test <prName>"
  repository: string;        // Repository CR name
  status: RunStatus;
  reason: string;            // Succeeded condition reason (Completed, Failed, PipelineRunTimeout, ...)
  message: string;           // Succeeded condition message
  startTime: string | null;
  completionTime: string | null;
  durationSeconds: number | null; // completion-start, or now-start while running
  git: GitInfo;
}

interface RepositorySummary {
  namespace: string;
  name: string;
  url: string;               // spec.url
  concurrencyLimit: number | null;
  lastRun: RunSummary | null;
  runCount: number;          // PipelineRuns currently in the cluster for this Repository
}

interface Step {
  name: string;
  status: RunStatus;         // from step state: terminated exitCode 0 → Succeeded, else Failed; running → Running; waiting → Pending
  exitCode: number | null;
  reason: string;
  startTime: string | null;
  completionTime: string | null;
}

interface GraphNode {
  id: string;                // pipeline task name (unique in the run)
  taskRunName: string;       // "" when no TaskRun exists (skipped/not started)
  status: RunStatus | "Skipped";
  reason: string;            // TaskRun reason, or skippedTasks reason
  finally: boolean;
  matrixCount: number;       // >1 when the task fans out via matrix (several TaskRuns)
  taskRunNames: string[];    // all TaskRuns of this pipeline task (matrix)
  startTime: string | null;
  completionTime: string | null;
  steps: Step[];             // steps of the first TaskRun
}

interface GraphEdge {
  from: string;              // GraphNode.id
  to: string;
  kind: "runAfter" | "result" | "finally";
}

interface RunDetail {
  summary: RunSummary;
  nodes: GraphNode[];
  edges: GraphEdge[];        // transitive duplicates removed; "finally" edges link every leaf of the main graph to each finally task
  params: { name: string; value: string }[];  // PipelineRun spec.params, values stringified
  canRerun: boolean;         // current user may post rerun comments for this run
}

interface Me {
  authenticated: boolean;
  login: string;             // "" when anonymous
  avatarUrl: string;
  authMode: "github" | "none";
  canRerun: boolean;         // global capability (authMode github + org member, or dev token)
}
```

## Graph edges

Built from the run's resolved spec (`status.pipelineSpec`, falling back to the referenced Pipeline):
- `runAfter`: each entry of `tasks[].runAfter`.
- `result`: any `$(tasks.<x>.results.<r>)` or `$(tasks.<x>.status)` reference found in a task's
  `params`, `when` (input and values), `matrix` params/include, or `workspaces[].subPath`.
- `finally`: every main-graph task with no outgoing edge → each `finally` task. Finally tasks may also
  reference main tasks' results (`result` edges) — keep both kinds.
Remove an edge A→C when another path A→…→C exists (transitive reduction) so the drawing stays readable.

## Endpoints

| Method | Path | Response |
|---|---|---|
| GET | `/api/v1/me` | `Me` (always 200) |
| GET | `/api/v1/repositories` | `RepositorySummary[]`, sorted by lastRun.startTime desc |
| GET | `/api/v1/repositories/{namespace}/{name}/runs?limit=50&pr=<n>&branch=<b>&status=<s>` | `RunSummary[]`, newest first |
| GET | `/api/v1/runs?limit=100&status=<s>` | `RunSummary[]` across all repositories, newest first |
| GET | `/api/v1/runs/{namespace}/{name}` | `RunDetail` |
| GET | `/api/v1/runs/{namespace}/{name}/tasks/{task}/logs?taskRun=<n>&step=<step>&follow=1` | `text/plain; charset=utf-8`, streamed (chunked) while `follow=1` and the step runs. `step` omitted → all steps concatenated with `==> step: <name> <==` separators. `taskRun` selects a matrix TaskRun (defaults to the first). 404 when the pod is gone (`{"error":"logs no longer available"}`). |
| GET | `/api/v1/events` | `text/event-stream`. Event `change`, data `{"kind":"PipelineRun"|"TaskRun"|"Repository","namespace":"..","name":"..","repository":"..","run":".."}` (`run` = owning PipelineRun for TaskRuns). Heartbeat comment every 20s. |
| POST | `/api/v1/runs/{namespace}/{name}/rerun` | body `{"mode":"retest"|"test"}`. Posts a PR comment as the logged-in GitHub user: `retest` → `/retest <prName>`, `test` → `/test <prName>`. 201 `{"commentUrl":"..."}`; 400 if the run has no PR; 401 anonymous; 403 not allowed. Requires header `X-Pacboard-CSRF` equal to cookie `pacboard_csrf`. |
| GET | `/auth/login?next=<path>` | 302 to GitHub OAuth (`read:org repo` scopes for an OAuth App; a GitHub App's user authorization needs none) |
| GET | `/auth/callback` | sets session, 302 to `next` (same-origin paths only) |
| POST | `/auth/logout` | clears session, 204 (CSRF header required) |
| GET | `/healthz`, `/readyz` | 200 when informers synced (`readyz`) |

Errors: JSON `{"error": "<message>"}` with the HTTP status.

## Auth modes

- `github` (default in production): GitHub OAuth web flow. Session = encrypted, HttpOnly, `SameSite=Lax`
  cookie (`pacboard_session`) holding login, avatar and the user's GitHub token. Access requires
  membership of one of `--allowed-orgs` (checked at login and cached 10 min). All `/api/v1/*` need a
  session; `/auth/*`, `/healthz`, `/readyz` and static assets do not.
- `none`: no login, read-only, `canRerun=false`. For local development only.
- Dev rerun override: `--dev-github-token-env=NAME` reads a token from that env var and acts as its
  user (`authMode` stays `none`, `canRerun=true`). Logged loudly at startup; never for deployments.

Rerun authorization is PaC's own ACL: the comment is posted as the real user, and PaC decides.
