<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

# AGENTS.md

pacboard: dashboard for Tekton Pipelines-as-Code runs. Go backend with the Vue 3 SPA embedded.
The binding backend/frontend contract is `docs/api.md`; change it first, then both sides.

## Layout

```
cmd/pacboard/        flags (+ PACBOARD_* env), wiring, graceful shutdown
internal/model/      contract types, PaC labels/annotations -> RunSummary/GitInfo, status mapping
internal/dag/        PipelineSpec + TaskRuns -> nodes and edges (runAfter, result refs, finally, matrix)
internal/kube/       client config, informer-backed Store (indexes by repository / PipelineRun), event broker
internal/logs/       step log streaming from pods
internal/auth/       GitHub OAuth (PKCE), encrypted sessions, org gate, CSRF; "none" and dev-token modes
internal/github/     minimal GitHub REST client (user, org membership, PR comment)
internal/api/        HTTP handlers, middleware, security headers
internal/httpx/      JSON helpers
web/                 Vue 3 + TypeScript + Vite + Tailwind 4 SPA; web/embed.go embeds web/dist
deploy/              kustomize manifests (read-only ClusterRole)
docker/Dockerfile    multi-stage image
```

## Commands

```sh
export GOTMPDIR=~/.cache/gotmp   # never /tmp
make web-build                   # web/dist (go:embed needs it only for the real UI)
make check                       # golangci-lint fmt + go vet + golangci-lint
make test                        # go test -race ./...
make run-dev                     # backend against your kubeconfig, no login
cd web && bun run typecheck && bun run test
```

## Rules

- Every file carries the SPDX header used at the top of this file (copyright "The pacboard
  Authors", license AGPL-3.0-or-later). Files that cannot (lockfiles, JSON, fixtures, build output)
  are listed in `REUSE.toml`.
- The cluster is read-only. Never add a write verb to `deploy/clusterrole.yaml`; reruns are GitHub
  comments posted with the user's own token.
- Run annotations are attacker-influenced (a PR's `.tekton` files). URLs go through `model.SafeURL`;
  names that reach a GitHub path or a comment are validated.
- `docs/api.md` wire format is camelCase; GitHub payloads are snake_case (tagliatelle is relaxed there only).
- Dependencies: Kubernetes client libraries only. OAuth, AES-GCM and HKDF use the standard library.
- DAG changes need a case in `internal/dag/dag_test.go`; the golden test uses a recorded real run
  (`go test ./internal/dag -update` rewrites it, review the diff).
- Keep it small and boring: stdlib `net/http` routing, no frameworks, no placeholders.
