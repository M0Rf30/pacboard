// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import { createApp } from 'vue'
import App from './App.vue'
import { router } from './router'
import { loginUrl, setUnauthorizedHandler } from './api/client'
import { useMe } from './composables/useMe'
import './style.css'

const { me } = useMe()

// A 401 in github mode means the session is missing or expired: send the user to GitHub and back here.
setUnauthorizedHandler(() => {
  if (me.value?.authMode === 'none') return
  window.location.assign(loginUrl(router.currentRoute.value.fullPath))
})

createApp(App).use(router).mount('#app')
