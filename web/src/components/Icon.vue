<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
withDefaults(defineProps<{ name: string; sm?: boolean; spin?: boolean }>(), { sm: false, spin: false })
</script>

<template>
  <svg class="ic" :class="{ sm, spin }" aria-hidden="true" focusable="false"><use :href="`#i-${name}`" /></svg>
</template>
