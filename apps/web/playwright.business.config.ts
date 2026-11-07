
import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  testMatch: 'business.spec.ts',

  timeout: 120_000,
  workers: 1,
  retries: 0,
  fullyParallel: false,

  reporter: 'list',

  use: {
    baseURL: 'http://127.0.0.1:18080',
    trace: 'off',
    screenshot: 'off',
    video: 'off',
  },
})
