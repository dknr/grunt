import { test, expect } from '@playwright/test';
import { login, registerUser, generateInviteCode } from './helpers';

test.describe('Profile Picture (Avatar)', () => {

  const testPngBase64 =
    'iVBORw0KGgoAAAANSUhEUgAAAEAAAABAAQMAAACQp+OdAAAAIGNIUk0AAHomAACAhAAA+gAAAIDoAAB1MAAA6mAAADqYAAAXcJy6UTwAAAAGUExURf8AAP///0EdNBEAAAABYktHRAH/Ai3eAAAAB3RJTUUH6gUfFSwkpE60GQAAACV0RVh0ZGF0ZTpjcmVhdGUAMjAyNi0wNS0zMVQyMTo0NDozNiswMDowMNWAh9QAAAAldEVYdGRhdGU6bW9kaWZ5ADIwMjYtMDUtMzFUMjE6NDQ6MzYrMDA6MDCk3T9oAAAAKHRFWHRkYXRlOnRpbWVzdGFtcAAyMDI2LTA1LTMxVDIxOjQ0OjM2KzAwOjAw88getwAAAA9JREFUKM9jYBgFo4B8AAACQAABjMWrdwAAAABJRU5ErkJggg==';

  const testPngGreenBase64 =
    'iVBORw0KGgoAAAANSUhEUgAAAEAAAABAAQMAAACQp+OdAAAAIGNIUk0AAHomAACAhAAA+gAAAIDoAAB1MAAA6mAAADqYAAAXcJy6UTwAAAAGUExURQD/AP///2+9WFEAAAABYktHRAH/Ai3eAAAAB3RJTUUH6gUfFisTUfQ5iAAAACV0RVh0ZGF0ZTpjcmVhdGUAMjAyNi0wNS0zMVQyMjo0MzoxOSswMDowMD77h/cAAAAldEVYdGRhdGU6bW9kaWZ5ADIwMjYtMDUtMzFUMjI6NDM6MTkrMDA6MDBPpj9LAAAAKHRFWHRkYXRlOnRpbWVzdGFtcAAyMDI2LTA1LTMxVDIyOjQzOjE5KzAwOjAwGLMelAAAAA9JREFUKM9jYBgFo4B8AAACQAABjMWrdwAAAABJRU5ErkJggg==';

  function testPngBuffer(): Buffer {
    return Buffer.from(testPngBase64, 'base64');
  }

  function testPngGreenBuffer(): Buffer {
    return Buffer.from(testPngGreenBase64, 'base64');
  }

  /** Build a multipart body with a PNG file. */
  function multipartBody(boundary: string, filename: string, data: Buffer): Buffer {
    const header =
      `--${boundary}\r\n` +
      `Content-Disposition: form-data; name="avatar"; filename="${filename}"\r\n` +
      `Content-Type: image/png\r\n\r\n`;
    const footer = `\r\n--${boundary}--\r\n`;
    return Buffer.concat([Buffer.from(header), data, Buffer.from(footer)]);
  }

  /** Upload avatar via API (raw multipart). */
  async function uploadAvatar(token: string, data?: Buffer, filename = 'test.png'): Promise<{status: number, body: string}> {
    const boundary = 'TestBoundary' + Math.random().toString(36).slice(2);
    const imgData = data ?? testPngBuffer();
    const body = multipartBody(boundary, filename, imgData);
    const res = await fetch('http://localhost:54765/api/user/avatar', {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${token}`,
        'Content-Type': `multipart/form-data; boundary=${boundary}`,
      },
      body,
    });
    const bodyText = await res.text();
    return { status: res.status, body: bodyText };
  }

  test('upload avatar via settings page', async ({ page }) => {
    const invite = await generateInviteCode();
    const { user, password } = await registerUser(invite);

    await login(page, user, password);
    await page.goto('/settings');

    // Before upload: header should show SVG avatar (no <img>)
    await expect(page.locator('.chat-header .avatar img')).toHaveCount(0);

    // Upload a PNG file via the file input
    const fileInput = page.locator('input[name="avatar"]');
    await fileInput.setInputFiles({
      name: 'test.png',
      mimeType: 'image/png',
      buffer: testPngBuffer(),
    });

    await page.click('form[hx-post="/api/user/avatar"] button[type="submit"]');

    // HTMX swaps the result into #avatar-result
    await expect(page.locator('#avatar-result')).toContainText(/Profile picture updated/i);
  });

  test('avatar appears in header after upload', async ({ page }) => {
    const invite = await generateInviteCode();
    const { user, password, token } = await registerUser(invite);

    // Upload via API
    const { status, body } = await uploadAvatar(token);
    expect(status).toBe(200);

    // Login and check header avatar (page reload fetches avatar from DB)
    await login(page, user, password);
    await page.waitForURL('/');

    const headerImg = page.locator('.chat-header .avatar img');
    await expect(headerImg).toBeVisible();
    await expect(headerImg).toHaveAttribute('src', `/api/user/avatar/${user}`);
  });

  test('avatar image is served correctly', async ({ page }) => {
    const invite = await generateInviteCode();
    const { user, token } = await registerUser(invite);

    // Upload via API
    const { status, body } = await uploadAvatar(token);
    expect(status).toBe(200);

    // Fetch the avatar image
    const imgRes = await fetch(`http://localhost:54765/api/user/avatar/${user}`);
    expect(imgRes.status).toBe(200);
    expect(imgRes.headers.get('Content-Type')).toBe('image/png');
  });

  test('avatar appears in chat messages', async ({ page }) => {
    const invite = await generateInviteCode();
    const { user, password, token } = await registerUser(invite);

    // Upload avatar via API
    const { status, body } = await uploadAvatar(token);
    expect(status).toBe(200);

    // Login and send a message
    await login(page, user, password);
    await page.waitForURL('/');
    // Wait for SSE connection to establish before posting
    await page.waitForSelector('#sse-listener', { state: 'attached', timeout: 5000 });
    await page.waitForTimeout(500);

    await page.fill('textarea[name="content"]', 'Hello from avatar user');
    await page.keyboard.press('Enter');

    // Wait for the message to appear and check avatar
    await page.waitForFunction(
      (expected: string) => {
        const imgs = document.querySelectorAll('.message-row .avatar img');
        return [...imgs].some((img) => img.getAttribute('src') === expected);
      },
      `/api/user/avatar/${user}`,
      { timeout: 10000 },
    );
    const msgImg = page.locator('.message-row .avatar img').first();
    await expect(msgImg).toHaveAttribute('src', `/api/user/avatar/${user}`);
  });

  test('user without avatar shows SVG fallback', async ({ page }) => {
    const invite = await generateInviteCode();
    const { user, password } = await registerUser(invite);

    await login(page, user, password);
    await page.waitForURL('/');

    // No <img> in the header avatar
    await expect(page.locator('.chat-header .avatar img')).toHaveCount(0);

    // SVG should still be present
    await expect(page.locator('.chat-header .avatar svg')).toBeVisible();
  });

  test('avatar endpoint returns 404 for user without avatar', async () => {
    const invite = await generateInviteCode();
    const { token } = await registerUser(invite);

    const res = await fetch('http://localhost:54765/api/user/avatar', {
      headers: { Authorization: `Bearer ${token}` },
    });
    expect(res.status).toBe(404);
  });

  test('avatar endpoint returns 401 without auth', async () => {
    const res = await fetch('http://localhost:54765/api/user/avatar', {
      method: 'POST',
      body: 'not multipart',
    });
    expect(res.status).toBe(401);
  });

  test('other user avatar is visible in chat', async ({ page, context }) => {
    const invite = await generateInviteCode();
    const { user: userA, password: passA, token: tokenA } = await registerUser(invite);

    // User A uploads avatar
    const { status, body } = await uploadAvatar(tokenA);
    expect(status).toBe(200);

    // User B (no avatar) views chat where user A sent a message
    const invite2 = await generateInviteCode();
    const { user: userB, password: passB } = await registerUser(invite2);

    const pageB = await context.newPage();
    await login(pageB, userB, passB);
    await pageB.waitForURL('/');
    // Wait for SSE connection to establish before posting
    await pageB.waitForSelector('#sse-listener', { state: 'attached', timeout: 5000 });
    await pageB.waitForTimeout(500);

    // User A sends a message via API with unique content
    const uniqueMsg = `Avatar test ${Date.now()}_${Math.random().toString(36).slice(2, 8)}`;
    await fetch('http://localhost:54765/api/chat/message', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${tokenA}`,
      },
      body: JSON.stringify({ content: uniqueMsg }),
    });

    // Wait for user B to receive the message via SSE
    await pageB.waitForFunction(
      (text: string) => {
        const bubbles = document.querySelectorAll('.message-bubble p');
        return [...bubbles].some((b) => b.textContent?.includes(text));
      },
      uniqueMsg,
      { timeout: 10000 },
    );

    // Now check the avatar in that specific message row
    const avatarImg = pageB.locator(`.message-row:has(p:text("${uniqueMsg}")) .avatar img`);
    await expect(avatarImg).toBeVisible();
    await expect(avatarImg).toHaveAttribute('src', `/api/user/avatar/${userA}`);
  });

  test('changing avatar updates the image and ETag', async () => {
    const invite = await generateInviteCode();
    const { token } = await registerUser(invite);

    // Upload first avatar (red)
    const { status: s1 } = await uploadAvatar(token);
    expect(s1).toBe(200);

    // Fetch avatar, record ETag
    const res1 = await fetch('http://localhost:54765/api/user/avatar', {
      headers: { Authorization: `Bearer ${token}` },
    });
    expect(res1.status).toBe(200);
    const etag1 = res1.headers.get('ETag');
    expect(etag1).toBeTruthy();
    const body1 = await res1.arrayBuffer();

    // Upload second avatar (green)
    const { status: s2 } = await uploadAvatar(token, testPngGreenBuffer());
    expect(s2).toBe(200);

    // Fetch again — should get different ETag and different image
    const res2 = await fetch('http://localhost:54765/api/user/avatar', {
      headers: { Authorization: `Bearer ${token}` },
    });
    expect(res2.status).toBe(200);
    const etag2 = res2.headers.get('ETag');
    expect(etag2).toBeTruthy();
    expect(etag2).not.toBe(etag1);
    const body2 = await res2.arrayBuffer();
    expect(Buffer.from(body2).equals(Buffer.from(body1))).toBe(false);
  });
});
