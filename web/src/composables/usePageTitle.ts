// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import { computed, onScopeDispose, shallowRef, toValue, type MaybeRefOrGetter } from 'vue'

const sources = shallowRef<readonly (() => string)[]>([])

/**
 * Title of the page that is currently shown ("" while it has none). App.vue turns it into
 * `document.title`, falling back to the route's `meta.title`.
 */
export const pageTitle = computed(() => sources.value.at(-1)?.() ?? '')

/**
 * Overrides the route's fallback title for as long as the calling page lives (e.g. with the loaded
 * run's commit title); an empty value shows the fallback. The newest page wins, so a page entering
 * before the previous one is torn down is not undone by that teardown.
 */
export function usePageTitle(title: MaybeRefOrGetter<string>): void {
  const mine = () => toValue(title)
  sources.value = [...sources.value, mine]
  onScopeDispose(() => {
    sources.value = sources.value.filter((s) => s !== mine)
  })
}
