import { test, expect } from '@playwright/test';
import { login, sendMessage, waitForMessage, registerUser, generateInviteCode } from './helpers';

test.describe('Mobile Viewport', () => {

  test.use({ viewport: { width: 375, height: 667 } });

  test('chat page renders without horizontal scroll on mobile', async ({ page }) => {
    const invite = await generateInviteCode();
    const { user, password } = await registerUser(invite);

    await login(page, user, password);
    await page.waitForURL('/');

    const overflowX = await page.evaluate(() => {
      return document.documentElement.scrollWidth <= document.documentElement.clientWidth;
    });
    expect(overflowX).toBe(true);

    await expect(page.locator('textarea[name="content"]')).toBeVisible();
    await expect(page.locator('button.send-button')).toBeVisible();
  });

  test('keyboard does not break layout on mobile', async ({ page }) => {
    const invite = await generateInviteCode();
    const { user, password } = await registerUser(invite);

    await login(page, user, password);
    await page.waitForURL('/');

    await page.locator('textarea[name="content"]').focus();
    await page.waitForTimeout(500);

    await expect(page.locator('#messages')).toBeVisible();
    await sendMessage(page, 'Mobile test message');
    await waitForMessage(page, 'Mobile test message');
    await expect(page.locator('.message-bubble p').last()).toContainText('Mobile test message');
  });

  test('send button tap works on mobile', async ({ page }) => {
    const invite = await generateInviteCode();
    const { user, password } = await registerUser(invite);

    await login(page, user, password);
    await page.waitForURL('/');

    await page.fill('textarea[name="content"]', 'Tapped from mobile');
    await page.click('button.send-button');
    await waitForMessage(page, 'Tapped from mobile');
    await expect(page.locator('.message-bubble p').last()).toContainText('Tapped from mobile');
  });

  test('login page is usable on mobile', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('#login-form')).toBeVisible();
    await expect(page.locator('.toggle-btn')).toBeVisible();

    await page.fill('#login-form input[name="user"]', 'mobile');
    await page.fill('#login-form input[name="password"]', 'test');
  });
});
