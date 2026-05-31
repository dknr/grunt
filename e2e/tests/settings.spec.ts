import { test, expect } from '@playwright/test';
import { login, registerUser, generateInviteCode, getAdminSession } from './helpers';

test.describe('Settings & Admin', () => {

  test('settings page loads with password form', async ({ page }) => {
    const invite = await generateInviteCode();
    const { user, password } = await registerUser(invite);

    await login(page, user, password);
    await page.goto('/settings');
    await expect(page.locator('h1')).toHaveText('Settings');
    await expect(page.locator('form[hx-post="/api/user/password"]')).toBeVisible();
    await expect(page.locator('form[hx-post="/api/user/password"] button[type="submit"]')).toContainText('Change Password');
  });

  test('change password succeeds', async ({ page }) => {
    const invite = await generateInviteCode();
    const { user, password } = await registerUser(invite);

    await login(page, user, password);
    await page.goto('/settings');

    await page.fill('input[name="current_password"]', password);
    await page.fill('input[name="new_password"]', 'newpass123');
    await page.fill('input[name="confirm_password"]', 'newpass123');
    await page.click('form[hx-post="/api/user/password"] button[type="submit"]');
    await expect(page.locator('#password-result')).toContainText(/password changed/i);
  });

  test('password mismatch triggers alert', async ({ page }) => {
    const invite = await generateInviteCode();
    const { user, password } = await registerUser(invite);

    let alertText = '';
    page.on('dialog', (dialog) => {
      alertText = dialog.message();
      dialog.accept();
    });

    await login(page, user, password);
    await page.goto('/settings');

    await page.fill('input[name="current_password"]', password);
    await page.fill('input[name="new_password"]', 'newpass1');
    await page.fill('input[name="confirm_password"]', 'newpass2');
    await page.click('form[hx-post="/api/user/password"] button[type="submit"]');
    await expect.poll(() => alertText).toContain('New passwords do not match');
  });

  test('admin can generate invite code', async ({ page }) => {
    const admin = getAdminSession();
    await login(page, admin.user, admin.password);
    await page.goto('/settings');

    await page.click('button[hx-get="/api/user/invite"]');
    await expect(page.locator('#invite-result')).toContainText(/New invite code:/);
  });

  test('admin can create a user', async ({ page }) => {
    const admin = getAdminSession();
    await login(page, admin.user, admin.password);
    await page.goto('/settings');

    const newUser = `bot_${Date.now()}`;
    await page.fill('form[hx-post="/api/admin/users"] input[name="user"]', newUser);
    await page.click('form[hx-post="/api/admin/users"] button[type="submit"]');
    await expect(page.locator('#users-list')).toContainText(newUser);
  });

  test('admin can create and revoke API key', async ({ page }) => {
    const admin = getAdminSession();
    await login(page, admin.user, admin.password);
    await page.goto('/settings');

    const target = `apiuser_${Date.now()}`;
    await page.fill('form[hx-post="/api/admin/users"] input[name="user"]', target);
    await page.click('form[hx-post="/api/admin/users"] button[type="submit"]');
    // Refresh page to update the user select dropdown in the API key form
    await page.goto('/settings');
    await page.waitForTimeout(300);

    await page.selectOption('select[name="user_id"]', target);
    const keyName = `test-key-${Date.now()}`;
    await page.fill('input[name="name"]', keyName);
    await page.click('form[hx-post="/api/admin/api-keys"] button[type="submit"]');
    await expect(page.locator('#keys-table-container')).toContainText(keyName);

    // Click the revoke button in the row containing our specific key name
    await page.click(`#keys-table-container tr:has-text("${keyName}") button[hx-delete^="/api/admin/api-keys/"]`);
    await expect(page.locator('#keys-table-container')).not.toContainText(keyName);
  });

  test('non-admin user sees no admin sections', async ({ page }) => {
    const invite = await generateInviteCode();
    const { user, password } = await registerUser(invite);

    await login(page, user, password);
    await page.goto('/settings');
    await expect(page.locator('button[hx-get="/api/user/invite"]')).not.toBeVisible();
    await expect(page.locator('form[hx-post="/api/admin/users"]')).not.toBeVisible();
    await expect(page.locator('form[hx-post="/api/admin/api-keys"]')).not.toBeVisible();
  });
});
