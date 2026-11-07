<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

# pacboard Helm chart

[pacboard](https://github.com/M0Rf30/pacboard) is a dashboard for Tekton Pipelines-as-Code runs:
repositories, runs linked to their pull request, commit and check run, the task graph (including
result-reference, `finally` and matrix tasks), live logs, and reruns posted as `/test` or `/retest`
comments by the logged-in user. It only reads the cluster.

## Install

```sh
helm install pacboard oci://ghcr.io/m0rf30/charts/pacboard \
  --namespace pacboard --create-namespace \
  --set 'namespaces={tekton-ci}'
```

Requirements: Tekton Pipelines (v1 API) and Pipelines-as-Code in the cluster.

## Authentication

- `auth.mode: none` (default): no login, read-only, rerun buttons hidden. Keep it behind a VPN
  or an authenticating proxy.
- `auth.mode: github`: GitHub OAuth login limited to members of `auth.github.allowedOrgs`; reruns
  are PR comments posted with the user's own token, so Pipelines-as-Code's ACL decides. Create a
  GitHub OAuth App with callback `<publicUrl>/auth/callback` and a Secret:

  ```sh
  kubectl -n pacboard create secret generic pacboard-github \
    --from-literal=github-client-id=... \
    --from-literal=github-client-secret=... \
    --from-literal=session-key="$(openssl rand -hex 32)"
  ```

  then set `auth.github.existingSecret=pacboard-github`, `auth.github.publicUrl` and
  `auth.github.allowedOrgs`.

## Link GitHub check runs to pacboard

In the `pipelines-as-code` ConfigMap:

```yaml
custom-console-name: pacboard
custom-console-url: https://pacboard.example.com
custom-console-url-namespace: https://pacboard.example.com/runs
custom-console-url-pr-details: https://pacboard.example.com/runs/{{ namespace }}/{{ pr }}
custom-console-url-pr-tasklog: https://pacboard.example.com/runs/{{ namespace }}/{{ pr }}?task={{ task }}&step={{ firstFailedStep }}
```

## Values

See [`values.yaml`](values.yaml); `values.schema.json` validates them.
