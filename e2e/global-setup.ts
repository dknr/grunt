import type { FullConfig } from '@playwright/test';

const BASE_URL = 'http://localhost:54765';
const INITIAL_INVITE = 'test00000000000000';

export default async function globalSetup(_config: FullConfig) {
  // Register the first admin user using the initial invite code.
  // This user gets admin privileges automatically (first user on the server).
  const uid = `admin`;
  const pw = 'pass';
  const regRes = await fetch(`${BASE_URL}/api/user`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ user: uid, password: pw, invite_code: INITIAL_INVITE }),
  });

  if (regRes.status !== 201) {
    // Server already has users — admin already exists
    console.warn('Admin already registered, using existing session');
  }

  // Login to get an admin token
  const loginRes = await fetch(`${BASE_URL}/api/user/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ user: uid, password: pw }),
  });

  if (loginRes.status === 200) {
    const body = await loginRes.json();
    process.env.ADMIN_TOKEN = body.token as string;
    process.env.ADMIN_USER = uid;
    process.env.ADMIN_PASS = pw;
  }
}
