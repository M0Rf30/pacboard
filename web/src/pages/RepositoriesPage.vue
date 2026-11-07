<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
import { computed, ref } from 'vue'
import { api } from '@/api/client'
import LiveIndicator from '@/components/LiveIndicator.vue'
import RepoRow from '@/components/RepoRow.vue'
import StateMessage from '@/components/StateMessage.vue'
import StatusIcon from '@/components/StatusIcon.vue'
import { useLive } from '@/composables/useLive'
import { useResource } from '@/composables/useResource'
import { statusStyle } from '@/lib/status'
import { GROUP_TITLE, groupRepos, matchesRepo, repoCounts, type RepoFilter } from '@/lib/repos'

const { data, error, loading, refresh } = useResource((signal) => api.repositories(signal))
const { connected, paused } = useLive(() => void refresh(), (e) => e.kind !== 'TaskRun')

const FILTERS: { key: RepoFilter; label: string }[] = [
  { key: 'all', label: 'All' },
  { key: 'failing', label: 'Failing' },
  { key: 'running', label: 'Running' },
  { key: 'passing', label: 'Passing' },
]
const GROUP_STATUS = { attention: 'Failed', running: 'Running', passing: 'Succeeded' } as const

const filter = ref<RepoFilter>('all')
const text = ref('')

const all = computed(() => data.value ?? [])
const counts = computed(() => repoCounts(all.value))
const sections = computed(() => groupRepos(all.value.filter((r) => matchesRepo(r, filter.value, text.value))))
const empty = computed(() => sections.value.length === 0)
const emptyText = computed(() => {
  if (all.value.length === 0) return 'No repositories found.'
  return text.value.trim() ? `No repository matches “${text.value.trim()}”.` : 'No repository in this state.'
})
</script>

<template>
  <section>
    <div class="mb-4 flex flex-wrap items-center gap-3">
      <h1>Repositories</h1>
      <LiveIndicator v-model:paused="paused" :connected="connected" />
      <div class="ml-auto flex flex-wrap items-center gap-2 max-[900px]:ml-0 max-[900px]:w-full">
        <div class="flex flex-wrap gap-2" role="group" aria-label="Filter by health">
          <button v-for="f in FILTERS" :key="f.key" type="button" class="chip" :aria-pressed="filter === f.key" @click="filter = f.key">
            {{ f.label }} <b>{{ counts[f.key] }}</b>
          </button>
        </div>
        <label class="sr-only" for="repo-filter">Filter repositories</label>
        <input id="repo-filter" v-model="text" data-filter type="search" class="input max-[900px]:flex-1" placeholder="Filter repositories…  ( / )" autocomplete="off" />
      </div>
    </div>

    <StateMessage :error="error" :loading="loading" :empty="empty" :empty-text="emptyText" @retry="refresh" />

    <div v-if="!empty" class="tbl repos" role="table" aria-label="Repositories">
      <div class="thead" role="row">
        <div role="columnheader">Repository</div>
        <div role="columnheader">History</div>
        <div role="columnheader">Last run</div>
        <div role="columnheader">Duration</div>
        <div class="c-start" role="columnheader">Started</div>
      </div>
      <template v-for="s in sections" :key="s.group">
        <div class="group-h" :class="statusStyle(GROUP_STATUS[s.group]).cls" role="row">
          <div class="flex items-center gap-2" role="cell">
            <StatusIcon :status="GROUP_STATUS[s.group]" />
            <span>{{ GROUP_TITLE[s.group] }}</span>
            <span class="font-mono">{{ s.repos.length }}</span>
          </div>
        </div>
        <RepoRow v-for="r in s.repos" :key="`${r.namespace}/${r.name}`" :repo="r" />
      </template>
    </div>
  </section>
</template>
