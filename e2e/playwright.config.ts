import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  fullyParallel: false,
  workers: 1,
  timeout: 30000,
  retries: 1,
  use: {
    baseURL: 'http://localhost:54765',
    headless: true,
    viewport: { width: 800, height: 600 },
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
  },
  webServer: {
    command: './dist/grunt serve --port 54765 file::memory:?cache=shared',
    cwd: '/home/dknr/src/grunt',
    port: 54765,
    timeout: 15000,
    reuseExistingServer: true,
    env: {
      GRUNT_INITIAL_INVITE: 'test00000000000000',
    },
  },
  globalSetup: './global-setup.ts',
});
