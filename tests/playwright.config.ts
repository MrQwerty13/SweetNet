import { defineConfig } from '@playwright/test';
export default defineConfig({
 testDir: './specs', fullyParallel: false, workers: 1, retries: 0,
 timeout: 45_000,
 reporter: [['list']],
 outputDir: '../.local/test-results',
 use: {
  baseURL: process.env.E2E_BASE_URL,
  browserName: 'chromium',
  viewport: { width: 1505, height: 1045 },
  trace: 'off', // Traces could contain generated credentials and private request bodies.
  screenshot: 'only-on-failure',
  launchOptions: process.env.E2E_CHROME_PATH ? { executablePath: process.env.E2E_CHROME_PATH } : {},
 },
});
