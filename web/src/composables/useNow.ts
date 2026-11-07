// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import { ref } from 'vue'

// One shared clock for every relative time and live duration: a single timer, started on first use.
const now = ref(Date.now())
let timer: ReturnType<typeof setInterval> | undefined

export function useNow() {
  if (timer === undefined) {
    timer = setInterval(() => {
      now.value = Date.now()
    }, 1000)
  }
  return now
}
