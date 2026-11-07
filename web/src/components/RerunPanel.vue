<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
import { computed, ref } from 'vue'
import { api } from '@/api/client'
import type { RerunMode, RunSummary } from '@/api/types'
import ExtLink from './ExtLink.vue'

const props = defineProps<{ summary: RunSummary; login: string }>()

const dialog = ref<HTMLDialogElement | null>(null)
const pending = ref<RerunMode | null>(null)
const busy = ref(false)
const error = ref('')
const commentUrl = ref('')

const target = computed(() => props.summary.prName || props.summary.name)
const command = computed(() => (pending.value ? `/${pending.value} ${target.value}` : ''))

function ask(mode: RerunMode): void {
  pending.value = mode
  error.value = ''
  dialog.value?.showModal()
}

function cancel(): void {
  dialog.value?.close()
  pending.value = null
}

async function confirm(): Promise<void> {
  if (!pending.value) return
  busy.value = true
  error.value = ''
  try {
    const res = await api.rerun(props.summary.namespace, props.summary.name, pending.value)
    commentUrl.value = res.commentUrl
    cancel()
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e)
  } finally {
    busy.value = false
  }
}

const btn =
  'rounded border border-gray-300 bg-white px-3 py-1 text-sm font-medium hover:bg-gray-100 dark:border-gray-700 dark:bg-gray-900 dark:hover:bg-gray-800'
</script>

<template>
  <div class="flex flex-wrap items-center gap-2">
    <button type="button" :class="btn" @click="ask('retest')">Retest failed</button>
    <button type="button" :class="btn" @click="ask('test')">Test again</button>
    <p v-if="commentUrl" role="status" class="text-sm text-green-700 dark:text-green-400">
      Comment posted:
      <ExtLink :href="commentUrl">view on GitHub</ExtLink>
    </p>

    <dialog
      ref="dialog"
      class="m-auto w-full max-w-md rounded-lg border border-gray-300 bg-white p-4 text-gray-900 backdrop:bg-black/50 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-100"
      aria-labelledby="rerun-title"
      @cancel.prevent="cancel"
    >
      <h2 id="rerun-title" class="text-base font-semibold">Post a comment on the pull request?</h2>
      <p class="mt-2 text-sm">
        This posts
        <code class="rounded bg-gray-100 px-1 py-0.5 font-mono dark:bg-gray-800">{{ command }}</code>
        on
        <ExtLink :href="summary.git.prUrl">pull request #{{ summary.git.prNumber }}</ExtLink>
        as <strong>{{ login || 'you' }}</strong>. Pipelines-as-Code decides whether it is allowed to run.
      </p>
      <p v-if="error" role="alert" class="mt-2 text-sm text-red-700 dark:text-red-300">{{ error }}</p>
      <div class="mt-4 flex justify-end gap-2">
        <button type="button" :class="btn" :disabled="busy" @click="cancel">Cancel</button>
        <button type="button" class="rounded bg-blue-700 px-3 py-1 text-sm font-medium text-white hover:bg-blue-800 disabled:opacity-50" :disabled="busy" @click="confirm">
          {{ busy ? 'Posting…' : 'Post comment' }}
        </button>
      </div>
    </dialog>
  </div>
</template>
