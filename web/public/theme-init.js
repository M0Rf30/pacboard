// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

// Sets data-theme before the first paint so a light OS or a stored choice never flashes dark.
// A separate file, not inline: the Content-Security-Policy only allows scripts from the origin.
// Keep the key and the rule in sync with src/lib/theme.ts.
;(function () {
  var theme
  try {
    var stored = localStorage.getItem('pacboard-theme')
    if (stored === 'light' || stored === 'dark') theme = stored
  } catch (e) {
    // storage blocked: follow the OS
  }
  if (!theme) theme = window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark'
  document.documentElement.dataset.theme = theme
})()
