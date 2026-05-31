import { Page, expect } from '@playwright/test';

const BASE_URL = 'http://localhost:54765';

// ─── Admin session (registered once in global setup) ───

interface AdminSession {
  user: string;
  password: string;
  token: string;
}

function getAdmin(): AdminSession {
  const token = process.env.ADMIN_TOKEN;
  const user = process.env.ADMIN_USER || 'admin';
  const pass = process.env.ADMIN_PASS || 'pass';
  if (!token) throw new Error('ADMIN_TOKEN not set — globalSetup may not have run');
  return { user, password: pass, token };
}

// ─── API helpers ───

export interface UserSession {
  user: string;
  password: string;
  token: string;
}

/** Register a fresh user with a given invite code. */
export async function registerUser(inviteCode: string): Promise<UserSession> {
  const uid = `test_${Date.now()}_${Math.random().toString(36).slice(2, 8)}`;
  const pw = 'pass';

  const regRes = await fetch(`${BASE_URL}/api/user`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ user: uid, password: pw, invite_code: inviteCode }),
  });
  expect(regRes.status).toBe(201);

  const loginRes = await fetch(`${BASE_URL}/api/user/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ user: uid, password: pw }),
  });
  const body = await loginRes.json();
  return { user: uid, password: pw, token: body.token as string };
}

/** Generate a fresh invite code using the shared admin token. */
export async function generateInviteCode(): Promise<string> {
  const admin = getAdmin();
  const res = await fetch(`${BASE_URL}/api/user/invite`, {
    headers: { Authorization: `Bearer ${admin.token}` },
  });
  const body = await res.json();
  return body.code as string;
}

/** Get the admin session (for direct API calls). */
export function getAdminSession(): AdminSession {
  return getAdmin();
}

// ─── Login page helpers ───

/** Fill and submit the login form. Redirects to /chat on success. */
export async function login(page: Page, user: string, password: string) {
  await page.goto('/');
  await page.fill('#login-form input[name="user"]', user);
  await page.fill('#login-form input[name="password"]', password);
  await page.click('#login-form button[type="submit"]');
}

/** Fill and submit the register form. Redirects to /chat on success. */
export async function register(page: Page, user: string, password: string, code: string) {
  await page.goto('/');
  await page.click('.toggle-btn');
  await page.waitForSelector('#register-form:not(.hidden)');
  await page.fill('#register-form input[name="user"]', user);
  await page.fill('#register-form input[name="password"]', password);
  await page.fill('#register-form input[name="invite_code"]', code);
  await page.click('#register-form button[type="submit"]');
}

// ─── Chat page helpers ───

/** Send a message via the chat textarea (Enter key). */
export async function sendMessage(page: Page, text: string) {
  await page.fill('textarea[name="content"]', text);
  await page.keyboard.press('Enter');
}

/** Wait for a message with the given text to appear in the chat. */
export async function waitForMessage(page: Page, text: string, timeout = 10000) {
  await page.waitForFunction(
    (expected: string) => {
      const bubbles = document.querySelectorAll('.message-bubble p');
      return [...bubbles].some((b) => b.textContent?.includes(expected));
    },
    text,
    { timeout },
  );
}

/** Count visible message rows. */
export async function messageCount(page: Page): Promise<number> {
  return page.locator('.message-row').count();
}
