import { test, expect } from '@playwright/test';
import { login, waitForMessage, sendMessage, registerUser, generateInviteCode } from './helpers';

test.describe('SSE Streaming', () => {

  test('SSE connection is established on chat page', async ({ page }) => {
    const invite = await generateInviteCode();
    const { user, password } = await registerUser(invite);

    await login(page, user, password);
    await page.waitForURL('/');
    await expect(page.locator('#sse-listener')).toBeAttached();
  });

  test('live message delivery across two browser contexts', async ({ browser }) => {
    const ctxA = await browser.newContext();
    const pageA = await ctxA.newPage();
    const inviteA = await generateInviteCode();
    const { user: userA, password: passA } = await registerUser(inviteA);
    await login(pageA, userA, passA);
    await pageA.waitForURL('/');

    const ctxB = await browser.newContext();
    const pageB = await ctxB.newPage();
    const inviteB = await generateInviteCode();
    const { user: userB, password: passB } = await registerUser(inviteB);
    await login(pageB, userB, passB);
    await pageB.waitForURL('/');

    const msg = `cross-context ${Date.now()}`;
    await sendMessage(pageA, msg);
    await waitForMessage(pageA, msg);
    await waitForMessage(pageB, msg);

    await ctxA.close();
    await ctxB.close();
  });

  test('join event appears when another user connects', async ({ browser }) => {
    const ctxA = await browser.newContext();
    const pageA = await ctxA.newPage();
    const inviteA = await generateInviteCode();
    const { user: userA, password: passA } = await registerUser(inviteA);
    await login(pageA, userA, passA);
    await pageA.waitForURL('/');

    const ctxB = await browser.newContext();
    const pageB = await ctxB.newPage();
    const inviteB = await generateInviteCode();
    const { user: userB, password: passB } = await registerUser(inviteB);
    await login(pageB, userB, passB);
    await pageB.waitForURL('/');

    await expect(pageA.locator('.sse-event').first()).toContainText(/join/);

    await ctxA.close();
    await ctxB.close();
  });

  test('reconnect after navigation works', async ({ page }) => {
    const invite = await generateInviteCode();
    const { user, password } = await registerUser(invite);

    await login(page, user, password);
    await page.waitForURL('/');

    await page.goto('/settings');
    await expect(page.locator('h1')).toHaveText('Settings');

    await page.goto('/');
    await expect(page.locator('#sse-listener')).toBeAttached();
  });
});
