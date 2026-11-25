// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 Noschvie
// PMSS API Tester - https://github.com/Noschvie/pmss-api-tester

const { test, expect } = require('@playwright/test');
const { login, runInfo, runChecks } = require('./helpers');

test.beforeAll(() => {
  if (process.env.E2E_VISUAL_CONTAINER !== '1' || process.platform !== 'linux') {
    throw new Error('Visual tests must run in the documented pinned Playwright container.');
  }
});

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('pmss-theme', 'light'));
  await login(page);
  await page.evaluate(() => document.fonts.ready);
});

test('overview', async ({ page }) => {
  await expect(page.locator('.inline-status')).toContainText('Ready');
  await page.evaluate(() => window.scrollTo(0, 0));
  await expect(page).toHaveScreenshot('overview.png', {
    animations: 'disabled',
    mask: [page.locator('.auth-status')],
  });
});

test('explorer result', async ({ page }) => {
  await runInfo(page);
  await page.evaluate(() => window.scrollTo(0, 0));
  await expect(page).toHaveScreenshot('explorer.png', {
    animations: 'disabled',
    mask: [page.locator('.duration'), page.locator('.response-headers')],
  });
});

test('check results', async ({ page }) => {
  await runChecks(page);
  await expect(page.locator('.check-result.check-fail')).toHaveCount(0);
  await page.evaluate(() => window.scrollTo(0, 0));
  await expect(page).toHaveScreenshot('checks.png', { animations: 'disabled' });
});
