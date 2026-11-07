<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
// Every icon of the UI, referenced by <Icon name="…"> through <use>. Mounted once in App.vue.
// 24px grid, stroke drawn by the `.ic` class (1.75, round caps).
</script>

<template>
  <svg width="0" height="0" style="position: absolute" aria-hidden="true" focusable="false">
    <defs>
      <symbol id="i-succeeded" viewBox="0 0 24 24"><circle cx="12" cy="12" r="9" /><path d="m8 12.5 2.8 2.8 5.7-5.8" /></symbol>
      <symbol id="i-failed" viewBox="0 0 24 24"><rect x="3.5" y="3.5" width="17" height="17" rx="4" /><path d="m9 9 6 6M15 9l-6 6" /></symbol>
      <symbol id="i-running" viewBox="0 0 24 24"><circle cx="12" cy="12" r="9" opacity=".3" /><path d="M21 12a9 9 0 0 0-9-9" /></symbol>
      <symbol id="i-pending" viewBox="0 0 24 24"><circle cx="12" cy="12" r="9" stroke-dasharray="2.4 3.6" /></symbol>
      <symbol id="i-cancelled" viewBox="0 0 24 24"><circle cx="12" cy="12" r="9" /><path d="m6 18 12-12" /></symbol>
      <symbol id="i-skipped" viewBox="0 0 24 24"><path d="M5 12h9m-4-5 5 5-5 5M19 6v12" /></symbol>
      <symbol id="i-unknown" viewBox="0 0 24 24"><circle cx="12" cy="12" r="9" /><path d="M9.6 9.6a2.5 2.5 0 1 1 3.5 2.3c-.7.4-1.1.9-1.1 1.7M12 17h.01" /></symbol>
      <symbol id="i-pr" viewBox="0 0 24 24"><circle cx="6" cy="6" r="2.5" /><circle cx="6" cy="18" r="2.5" /><circle cx="18" cy="18" r="2.5" /><path d="M6 8.5v7M18 15.5V10a3 3 0 0 0-3-3h-3m2.5-2.5L12 7l2.5 2.5" /></symbol>
      <symbol id="i-branch" viewBox="0 0 24 24"><circle cx="6" cy="5.5" r="2.5" /><circle cx="6" cy="18.5" r="2.5" /><circle cx="18" cy="8.5" r="2.5" /><path d="M6 8v8M18 11c0 4-6 3-12 5" /></symbol>
      <symbol id="i-commit" viewBox="0 0 24 24"><circle cx="12" cy="12" r="3.5" /><path d="M3 12h5.5M15.5 12H21" /></symbol>
      <symbol id="i-ext" viewBox="0 0 24 24"><path d="M14 4h6v6M20 4l-9 9M18 14v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V7a1 1 0 0 1 1-1h5" /></symbol>
      <symbol id="i-chev" viewBox="0 0 24 24"><path d="m9 6 6 6-6 6" /></symbol>
      <symbol id="i-sun" viewBox="0 0 24 24"><circle cx="12" cy="12" r="4" /><path d="M12 2.5V5M12 19v2.5M2.5 12H5M19 12h2.5M5.3 5.3l1.8 1.8M16.9 16.9l1.8 1.8M5.3 18.7l1.8-1.8M16.9 7.1l1.8-1.8" /></symbol>
      <symbol id="i-moon" viewBox="0 0 24 24"><path d="M20 14.5A8 8 0 0 1 9.5 4a8 8 0 1 0 10.5 10.5z" /></symbol>
      <symbol id="i-retry" viewBox="0 0 24 24"><path d="M20 12a8 8 0 1 1-2.6-5.9M20 4v5h-5" /></symbol>
      <symbol id="i-down" viewBox="0 0 24 24"><path d="M12 4v11m-5-5 5 5 5-5M5 20h14" /></symbol>
      <symbol id="i-clock" viewBox="0 0 24 24"><circle cx="12" cy="12" r="9" /><path d="M12 7v5l3 2" /></symbol>
      <symbol id="i-user" viewBox="0 0 24 24"><circle cx="12" cy="8" r="3.5" /><path d="M5 20c0-3.5 3-6 7-6s7 2.5 7 6" /></symbol>
      <symbol id="i-warn" viewBox="0 0 24 24"><path d="M12 3.5 21.5 20h-19z" /><path d="M12 10v4.5M12 17.5h.01" /></symbol>
      <symbol id="i-fit" viewBox="0 0 24 24"><path d="M4 9V4h5M20 9V4h-5M4 15v5h5M20 15v5h-5" /></symbol>
      <symbol id="i-plus" viewBox="0 0 24 24"><path d="M12 5v14M5 12h14" /></symbol>
      <symbol id="i-minus" viewBox="0 0 24 24"><path d="M5 12h14" /></symbol>
      <marker id="arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
        <path d="M1 1.5 9 5 1 8.5z" fill="context-stroke" />
      </marker>
    </defs>
  </svg>
</template>
