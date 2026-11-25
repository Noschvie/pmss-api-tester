// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 Noschvie
// PMSS API Tester - https://github.com/Noschvie/pmss-api-tester

const { expect } = require('@playwright/test');

async function login(page, username = 'alice') {
  await page.goto('/');
  await page.getByRole('link', { name: 'Sign in with Keycloak' }).first().click();
  await expect(page.getByRole('heading', { name: 'Test Keycloak' })).toBeVisible();
  await page.getByLabel('Username').fill(username);
  await page.getByLabel('Password').fill('test-password');
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  await expect(page.getByRole('heading', { name: username, exact: true })).toBeVisible();
}

async function resetRequests(request) {
  expect((await request.delete('http://127.0.0.1:4174/test/requests')).ok()).toBeTruthy();
}

async function targetRequests(request) {
  const response = await request.get('http://127.0.0.1:4174/test/requests');
  expect(response.ok()).toBeTruthy();
  return response.json();
}

async function providerLogouts(request) {
  const response = await request.get('http://127.0.0.1:4174/test/logouts');
  expect(response.ok()).toBeTruthy();
  return response.json();
}

async function runInfo(page) {
  await page.locator('aside[aria-label="Main navigation"]').getByRole('link', { name: 'API Explorer' }).click();
  await page.locator('#operation-getInfo').getByRole('button', { name: 'Run GET' }).click();
  await expect(page.locator('.explorer-result h2')).toContainText('HTTP 200');
  await expect(page.locator('.schema-status')).toHaveText('Response schema: VALID');
}

async function runChecks(page) {
  await page.locator('aside[aria-label="Main navigation"]').getByRole('link', { name: 'Checks' }).click();
  await page.getByRole('button', { name: 'Run read-only checks' }).click();
  await expect(page.locator('.check-results')).toBeVisible();
}

module.exports = { login, resetRequests, targetRequests, providerLogouts, runInfo, runChecks };
