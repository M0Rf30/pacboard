<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
import { computed } from 'vue'

// Live / Paused (user toggle) / Reconnecting (stream down). A button: pressing it pauses or resumes patching.
const props = defineProps<{ connected: boolean }>()
const paused = defineModel<boolean>('paused', { default: false })

const state = computed(() => (paused.value ? 'paused' : props.connected ? 'live' : 'reconnecting'))
const text = computed(() => ({ live: 'Live', paused: 'Paused', reconnecting: 'Reconnecting' })[state.value])
const title = computed(() =>
  state.value === 'paused'
    ? 'Live updates paused. Click to resume.'
    : state.value === 'live'
      ? 'Receiving live updates. Click to pause.'
      : 'Live updates disconnected, retrying.',
)
</script>

<template>
  <button type="button" class="live" :data-state="state" :aria-pressed="!paused" :title="title" @click="paused = !paused">
    <i aria-hidden="true" />{{ text }}
  </button>
</template>
