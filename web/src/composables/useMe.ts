// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import { ref } from 'vue'
import { api } from '@/api/client'
import type { Me } from '@/api/types'

const me = ref<Me | null>(null)

export async function loadMe(): Promise<void> {
  try {
    me.value = await api.me()
  } catch {
    me.value = null
  }
}

export function useMe() {
  return { me, loadMe }
}
