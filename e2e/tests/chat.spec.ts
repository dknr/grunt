import { test, expect } from '@playwright/test';
import { login, sendMessage, waitForMessage, messageCount, registerUser, generateInviteCode, getAdminSession } from './helpers';

test.describe('Chat & Messaging', () => {

  let token: string;

  test.beforeAll(async () => {
    const invite = await generateInviteCode();
    const session = await registerUser(invite);
    token = session.token;
  });

  test('chat page renders correctly after login', async ({ page }) => {
    const invite = await generateInviteCode();
    const { user, password } = await registerUser(invite);

    await login(page, user, password);
    await page.waitForURL('/');

    await expect(page.locator('header h1')).toHaveText('Grunt');
    await expect(page.locator('#messages')).toBeVisible();
    await expect(page.locator('textarea[name="content"]')).toBeFocused();
    await expect(page.locator('#sse-listener')).toBeAttached();
  });

  test('send a message via textarea and Enter key', async ({ page }) => {
    const invite = await generateInviteCode();
    const { user, password } = await registerUser(invite);

    await login(page, user, password);
    await page.waitForURL('/');

    await sendMessage(page, 'Hello, Playwright!');
    await waitForMessage(page, 'Hello, Playwright!');
    await expect(page.locator('textarea[name="content"]')).toHaveValue('');
  });

  test('send message via click on send button', async ({ page }) => {
    const invite = await generateInviteCode();
    const { user, password } = await registerUser(invite);

    await login(page, user, password);
    await page.waitForURL('/');

    await page.fill('textarea[name="content"]', 'Button click test');
    await page.click('button.send-button');
    await waitForMessage(page, 'Button click test');
  });

  test('Shift+Enter inserts newline instead of sending', async ({ page }) => {
    const invite = await generateInviteCode();
    const { user, password } = await registerUser(invite);

    await login(page, user, password);
    await page.waitForURL('/');

    await page.fill('textarea[name="content"]', 'line1');
    await page.keyboard.press('Shift+Enter');
    await page.keyboard.type('line2');
    await page.keyboard.press('Enter');
    await waitForMessage(page, 'line1');
    await waitForMessage(page, 'line2');
  });

  test('messages appear via SSE from another user', async ({ page, context }) => {
    const invite = await generateInviteCode();
    const { user: userA, password: passA } = await registerUser(invite);

    const pageA = await context.newPage();
    await login(pageA, userA, passA);
    await pageA.waitForURL('/');
    // Wait for SSE connection to establish before posting
    await pageA.waitForSelector('#sse-listener', { state: 'attached', timeout: 5000 });
    await pageA.waitForTimeout(500);

    const msg = `SSE test ${Date.now()}`;
    await fetch('http://localhost:54765/api/chat/message', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({ content: msg }),
    });

    await waitForMessage(pageA, msg);
    await pageA.close();
  });

  test('messages from same user are grouped', async ({ page }) => {
    const invite = await generateInviteCode();
    const { user, password } = await registerUser(invite);

    await login(page, user, password);
    await page.waitForURL('/');

    await sendMessage(page, 'First message');
    await waitForMessage(page, 'First message');
    await sendMessage(page, 'Second message');
    await waitForMessage(page, 'Second message');

    // Find the message-row that contains "Second message" and verify it's grouped
    const secondMsgRow = page.locator('.message-row').filter({ hasText: 'Second message' });
    await expect(secondMsgRow).toHaveClass(/message-row--grouped/);
  });

  test('message ordering is correct (ascending timestamps)', async ({ page }) => {
    const invite = await generateInviteCode();
    const { user, password } = await registerUser(invite);

    await login(page, user, password);
    await page.waitForURL('/');

    await sendMessage(page, 'Earliest');
    await waitForMessage(page, 'Earliest');
    await page.waitForTimeout(100);
    await sendMessage(page, 'Middle');
    await waitForMessage(page, 'Middle');
    await page.waitForTimeout(100);
    await sendMessage(page, 'Latest');
    await waitForMessage(page, 'Latest');

    // Get all message bubbles, find the ones matching our test messages
    const bubbles = page.locator('.message-bubble p');
    const allTexts = await bubbles.allTextContents();
    const ourTexts = allTexts.filter(t => t.includes('Earliest') || t.includes('Middle') || t.includes('Latest'));

    expect(ourTexts[0]).toContain('Earliest');
    expect(ourTexts[1]).toContain('Middle');
    expect(ourTexts[2]).toContain('Latest');
  });

  test('10KB message limit is enforced', async ({ page }) => {
    const invite = await generateInviteCode();
    const { user, password } = await registerUser(invite);

    await login(page, user, password);
    await page.waitForURL('/');

    const before = await messageCount(page);
    const big = 'x'.repeat(11000);
    await sendMessage(page, big);
    await page.waitForTimeout(500);
    const after = await messageCount(page);
    expect(after).toBe(before);
  });

  test('empty message is not sent', async ({ page }) => {
    const invite = await generateInviteCode();
    const { user, password } = await registerUser(invite);

    await login(page, user, password);
    await page.waitForURL('/');

    const before = await messageCount(page);
    await page.click('button.send-button');
    await page.waitForTimeout(500);
    const after = await messageCount(page);
    expect(after).toBe(before);
  });
});
