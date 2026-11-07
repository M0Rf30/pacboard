<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
// External link (GitHub): always opens in a new tab without leaking the opener. An empty or non-http(s)
// href renders the slot as plain text, so callers need no conditionals for unknown URLs. The URLs come
// from PipelineRun annotations, which a PR's .tekton template can influence: never trust the scheme.
import { computed } from 'vue'
import { safeHttpUrl } from '../lib/format'

const props = defineProps<{ href: string }>()
const safeHref = computed(() => safeHttpUrl(props.href))
</script>

<template>
  <a
    v-if="safeHref"
    :href="safeHref"
    target="_blank"
    rel="noopener noreferrer"
    class="text-blue-700 hover:underline dark:text-blue-400"
  >
    <slot />
  </a>
  <span v-else><slot /></span>
</template>
