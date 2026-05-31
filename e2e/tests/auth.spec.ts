import { test, expect } from '@playwright/test';
import { login, register, registerUser, generateInviteCode } from './helpers';

test.describe('Authentication & Registration', () => {

  test('login page loads with correct elements', async ({ page }) => {
    await page.goto('/');
    await expect(page).toHaveTitle(/Grunt Login/);
    await expect(page.locator('#login-form')).toBeVisible();
    await expect(page.locator('#register-form')).toHaveClass(/hidden/);
    await expect(page.locator('.toggle-btn')).toBeVisible();
  });

  test('toggle between login and register forms', async ({ page }) => {
    await page.goto('/');

    await page.click('.toggle-btn');
    await expect(page.locator('#register-form')).not.toHaveClass(/hidden/);
    await expect(page.locator('#login-form')).toHaveClass(/hidden/);
    await expect(page.locator('.toggle-btn')).toHaveText(/Already have an account\? Login/);

    await page.click('.toggle-btn');
    await expect(page.locator('#login-form')).toBeVisible();
    await expect(page.locator('#register-form')).toHaveClass(/hidden/);
    await expect(page.locator('.toggle-btn')).toHaveText(/Don't have an account\? Register/);
  });

  test('successful login redirects to chat page', async ({ page }) => {
    const invite = await generateInviteCode();
    const { user, password } = await registerUser(invite);

    await login(page, user, password);
    await page.waitForURL('/');
    await expect(page.locator('h1')).toHaveText('Grunt');
    await expect(page.locator('textarea[name="content"]')).toBeVisible();
  });

  test('failed login shows error message', async ({ page }) => {
    await login(page, 'nonexistent', 'wrong');
    await expect(page.locator('#response .error-message')).toHaveText(/Invalid credentials/);
  });

  test('registration with valid invite code', async ({ page }) => {
    const invite = await generateInviteCode();
    const uid = `reg_${Date.now()}`;
    await register(page, uid, 'pass', invite);
    await page.waitForURL('/');
    await expect(page.locator('h1')).toHaveText('Grunt');
  });

  test('registration with invalid invite code', async ({ page }) => {
    await register(page, `bad_${Date.now()}`, 'pass', 'badbadbadbad');
    await expect(page.locator('#response .error-message')).toHaveText(/Invalid or expired invite code/);
  });

  test('registration with duplicate username', async ({ page }) => {
    const invite = await generateInviteCode();
    const uid = `dup_${Date.now()}`;

    // Register once
    await register(page, uid, 'pass', invite);
    await page.waitForURL('/');

    // Logout
    await page.goto('/logout');

    // Try registering same name again
    const invite2 = await generateInviteCode();
    await register(page, uid, 'pass', invite2);
    await expect(page.locator('#response .error-message')).toHaveText(/User already exists/);
  });

  test('registration with missing fields', async ({ page }) => {
    await page.goto('/');
    await page.click('.toggle-btn');
    // Clear fields (they may already be empty, but ensure it)
    await page.fill('#register-form input[name="user"]', '');
    await page.fill('#register-form input[name="password"]', '');
    await page.fill('#register-form input[name="invite_code"]', '');
    // Submit the form directly via JS to bypass HTML5 validation
    await page.evaluate(() => {
      const form = document.querySelector('#register-form') as HTMLFormElement;
      form.submit();
    });
    await page.waitForURL('/register');
    await expect(page.locator('#response .error-message')).toHaveText(/required/);
  });

  test('logout clears session and redirects', async ({ page }) => {
    const invite = await generateInviteCode();
    const { user, password } = await registerUser(invite);

    await login(page, user, password);
    await page.waitForURL('/');

    await page.goto('/logout');
    await page.waitForURL('/login');
    await page.goto('/');
    await expect(page.locator('#login-form')).toBeVisible();
  });
});
