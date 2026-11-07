<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
import { computed } from 'vue'
import type { RunSummary } from '@/api/types'
import { longestDuration } from '@/lib/timing'
import RunRow from './RunRow.vue'

const props = defineProps<{
  runs: RunSummary[]
  /** The repository's typical duration for a run (null when unknown), for the slow marker. */
  typicalOf: (run: RunSummary) => number | null
  showRepo?: boolean
}>()

const longest = computed(() => longestDuration(props.runs))
</script>

<template>
  <div class="tbl runs" role="table" aria-label="Runs, newest first">
    <div class="thead" role="row">
      <div role="columnheader">Status</div>
      <div role="columnheader">Commit</div>
      <div role="columnheader">Event</div>
      <div class="c-sender" role="columnheader">Sender</div>
      <div class="c-start" role="columnheader">Started</div>
      <div role="columnheader">Duration</div>
    </div>
    <RunRow v-for="r in runs" :key="`${r.namespace}/${r.name}`" :run="r" :longest="longest" :typical="typicalOf(r)" :show-repo="showRepo" />
  </div>
</template>
