<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

# Security policy

Report vulnerabilities privately through
[GitHub security advisories](https://github.com/M0Rf30/pacboard/security/advisories/new),
not in public issues. Only the latest release is supported.

Relevant to the threat model: pacboard reads the cluster with get/list/watch only, PipelineRun
annotations are treated as attacker-controlled (a pull request's `.tekton/` files set them), and
reruns are PR comments posted with the logged-in user's own GitHub token.
