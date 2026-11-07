<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
import { eventLabel, isCommandEvent } from '@/lib/events'

// A readable label for the PaC event; the raw identifier stays in the tooltip.
defineProps<{ event: string }>()
</script>

<template>
  <span v-if="event" class="tag" :class="{ 'font-mono': isCommandEvent(event) }" :title="event">{{ eventLabel(event) }}</span>
</template>
