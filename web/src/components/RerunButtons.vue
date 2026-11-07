<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
import { ref } from 'vue'
import { api } from '@/api/client'
import type { RerunMode, RunSummary } from '@/api/types'
import { useToast } from '@/composables/useToast'
import Icon from './Icon.vue'

// Posts /retest or /test as the logged-in user. The parent renders this only when the user may rerun,
// the run may be rerun and it has a PR; the backend checks all of it again.
const props = defineProps<{ summary: RunSummary; login: string }>()

const { show } = useToast()
const busy = ref<RerunMode | ''>('')

function command(mode: RerunMode): string {
  return `/${mode} ${props.summary.prName}`
}

async function post(mode: RerunMode): Promise<void> {
  busy.value = mode
  try {
    const r = await api.rerun(props.summary.namespace, props.summary.name, mode)
    show({
      title: 'Comment posted',
      command: command(mode),
      detail: 'A new run will appear in the list shortly.',
      href: r.commentUrl,
      hrefLabel: 'View the comment',
    })
  } catch (e) {
    show({ title: 'Could not post the comment', detail: e instanceof Error ? e.message : String(e), error: true })
  } finally {
    busy.value = ''
  }
}
</script>

<template>
  <button
    v-if="summary.status === 'Failed'"
    type="button"
    class="btn primary"
    :disabled="busy !== ''"
    :title="`Posts “${command('retest')}” as ${login}`"
    @click="post('retest')"
  >
    <Icon name="retry" />{{ busy === 'retest' ? 'Posting…' : 'Retest failed' }}
  </button>
  <button type="button" class="btn" :disabled="busy !== ''" :title="`Posts “${command('test')}” as ${login}`" @click="post('test')">
    {{ busy === 'test' ? 'Posting…' : 'Test again' }}
  </button>
</template>
