<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
import { computed } from 'vue'
import { useNow } from '@/composables/useNow'
import { absoluteTime, relativeTime } from '@/lib/format'

const props = defineProps<{ iso: string | null }>()
const now = useNow()
const text = computed(() => relativeTime(props.iso, now.value))
</script>

<template>
  <time v-if="iso" :datetime="iso" :title="absoluteTime(iso)" class="whitespace-nowrap">{{ text }}</time>
  <span v-else class="text-gray-400" aria-label="unknown">—</span>
</template>
