// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 Noschvie
// PMSS API Tester - https://github.com/Noschvie/pmss-api-tester

// Do not inherit integration or production endpoint settings.
const { spawn } = require('node:child_process');

const child = spawn('go', ['run', './cmd/pmss-api-tester'], {
  stdio: 'inherit',
  env: {
    ...process.env,
    HTTP_ADDR: '127.0.0.1:4173',
    OIDC_ISSUER_URL: 'http://127.0.0.1:4174/realms/test',
    OIDC_INTERNAL_URL: '',
    OIDC_CLIENT_ID: 'e2e-client',
    OIDC_CLIENT_SECRET: 'e2e-secret',
    OIDC_CALLBACK_URL: 'http://127.0.0.1:4173/auth/callback',
    OIDC_COOKIE_SECURE: 'false',
    PMSS_AUDIENCE: 'e2e-pmss',
    PMSS_API_BASE_URL: 'http://127.0.0.1:4174',
    PMSS_ALLOW_INSECURE_HTTP: 'true',
    TZ: 'UTC',
    NO_PROXY: '127.0.0.1,localhost',
    no_proxy: '127.0.0.1,localhost',
  },
});
child.on('error', (error) => { console.error(error); process.exitCode = 1; });
child.on('exit', (code, signal) => { process.exitCode = code ?? (signal ? 1 : 0); });
for (const signal of ['SIGINT', 'SIGTERM']) {
  process.on(signal, () => child.kill(signal));
}
