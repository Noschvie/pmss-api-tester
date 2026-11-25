// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 Noschvie
// PMSS API Tester - https://github.com/Noschvie/pmss-api-tester

const { defineConfig } = require('@playwright/test');

// Playwright's readiness probes must not go through an inherited HTTP proxy.
for (const name of ['NO_PROXY', 'no_proxy']) {
  process.env[name] = [process.env[name], '127.0.0.1', 'localhost'].filter(Boolean).join(',');
}

module.exports = defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  workers: 1,
  retries: 0,
  timeout: 30_000,
  expect: { timeout: 10_000 },
  use: {
    baseURL: 'http://127.0.0.1:4173',
    browserName: 'chromium',
    viewport: { width: 1440, height: 900 },
    deviceScaleFactor: 1,
    colorScheme: 'light',
    locale: 'en-US',
    timezoneId: 'UTC',
    reducedMotion: 'reduce',
    trace: 'retain-on-failure',
  },
  projects: [
    { name: 'chromium', testMatch: '**/flows.spec.js' },
    { name: 'visual', testMatch: '**/visual.spec.js' },
  ],
  webServer: [
    {
      command: 'node e2e/mock-server.js',
      url: 'http://127.0.0.1:4174/test/health',
      reuseExistingServer: false,
      timeout: 30_000,
    },
    {
      command: 'node e2e/start-app.js',
      url: 'http://127.0.0.1:4173/healthz',
      reuseExistingServer: false,
      timeout: 180_000,
    },
  ],
});
