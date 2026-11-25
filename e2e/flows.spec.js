// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 Noschvie
// PMSS API Tester - https://github.com/Noschvie/pmss-api-tester

const { test, expect } = require('@playwright/test');
const {
  login, resetRequests, targetRequests, providerLogouts, runInfo, runChecks,
} = require('./helpers');

test.beforeEach(async ({ request }) => resetRequests(request));

test('sign in, read the API, run checks and log out', async ({ page, request }) => {
  await login(page);
  await expect(page.locator('.auth-status')).toContainText('Read permission granted');
  await runInfo(page);
  await expect(page.locator('.explorer-result')).toContainText('"apiVersion":"1.0"');
  expect(await targetRequests(request)).toEqual([
    { method: 'GET', path: '/api/v1/info', authorization: 'Bearer e2e-read-token' },
  ]);

  await resetRequests(request);
  await runChecks(page);
  for (const name of ['Availability · GET /healthz', 'Authentication · Missing token',
    'Authentication · Invalid token', 'read permission', 'GET /api/v1/info', 'API version']) {
    await expect(page.locator('.check-result').filter({ hasText: name }).getByText('PASS', { exact: true })).toBeVisible();
  }
  const calls = await targetRequests(request);
  expect(calls.map(({ method, path }) => `${method} ${path}`)).toEqual([
    'GET /healthz', 'GET /api/v1/info', 'GET /api/v1/info', 'GET /api/v1/info',
  ]);
  expect(calls[3].authorization).toBe('Bearer e2e-read-token');

  const session = (await page.context().cookies()).find((cookie) => cookie.name === 'pmss_session');
  expect(session).toBeTruthy();
  await page.getByRole('button', { name: 'Log out' }).click();
  await expect(page.getByRole('heading', { name: 'Sign in with your account' })).toBeVisible();
  expect((await page.context().cookies()).some((cookie) => cookie.name === 'pmss_session')).toBe(false);
  const replay = await request.get('http://127.0.0.1:4173/', { headers: { cookie: `pmss_session=${session.value}` } });
  expect(await replay.text()).toContain('Sign in with your account');
  expect(await providerLogouts(request)).toEqual([{ idTokenHint: true }]);
});

test('log out after the session ended on the server', async ({ page, request }) => {
  await login(page);
  // The session is gone on the server (expired with the access token or
  // lost by a restart), the page still shows the logout button.
  const session = (await page.context().cookies()).find((cookie) => cookie.name === 'pmss_session');
  await page.context().addCookies([{ ...session, value: 'unknown-session' }]);
  await page.getByRole('button', { name: 'Log out' }).click();
  await expect(page.getByRole('heading', { name: 'Sign in with your account' })).toBeVisible();
  expect((await page.context().cookies()).some((cookie) => cookie.name === 'pmss_session')).toBe(false);
  expect(await providerLogouts(request)).toEqual([{ idTokenHint: false }]);
});

test('missing read scope blocks protected explorer and checks requests', async ({ page, request }) => {
  await login(page, 'no-read');
  await expect(page.locator('.auth-status')).toContainText('BLOCKED');
  await page.locator('aside[aria-label="Main navigation"]').getByRole('link', { name: 'API Explorer' }).click();
  await expect(page.locator('#operation-getInfo').getByRole('button', { name: 'Run GET' })).toBeDisabled();
  const result = await page.evaluate(async () => {
    const csrf = document.querySelector('input[name="csrf_token"]').value;
    const response = await fetch('/api-explorer/execute', {
      method: 'POST',
      body: new URLSearchParams({ operation_id: 'getInfo', method: 'GET', csrf_token: csrf }),
    });
    return { status: response.status, body: await response.text() };
  });
  expect(result.status).toBe(403);
  expect(result.body).toContain('required read scope');
  expect(await targetRequests(request)).toEqual([]);
  await runChecks(page);
  await expect(page.locator('.check-result').filter({ hasText: 'read permission' })).toContainText('BLOCKED');
  await expect(page.locator('.check-result').filter({ hasText: 'GET /api/v1/info' })).toContainText('BLOCKED');
  const calls = await targetRequests(request);
  expect(calls.map(({ method, path }) => `${method} ${path}`)).toEqual([
    'GET /healthz', 'GET /api/v1/info', 'GET /api/v1/info',
  ]);
  expect(calls.every(({ authorization }) => authorization !== 'Bearer e2e-no-read-token')).toBe(true);
});

test('write operations are blocked in the UI and on forged submissions', async ({ page, request }) => {
  await login(page);
  await page.locator('aside[aria-label="Main navigation"]').getByRole('link', { name: 'API Explorer' }).click();
  const write = page.locator('#operation-createClient');
  await expect(write).toContainText('BLOCKED');
  await expect(write.getByRole('button', { name: 'Run GET' })).toHaveCount(0);
  for (const method of ['POST', 'GET']) {
    const result = await page.evaluate(async (method) => {
      const csrf = document.querySelector('input[name="csrf_token"]').value;
      const response = await fetch('/api-explorer/execute', {
        method: 'POST',
        body: new URLSearchParams({ operation_id: 'createClient', method, csrf_token: csrf }),
      });
      return { status: response.status, body: await response.text() };
    }, method);
    expect(result.status).toBe(403);
    expect(result.body).toContain('Only GET requests');
  }
  expect(await targetRequests(request)).toEqual([]);
});
