
import { defineConfig, devices } from '@playwright/test'

const baseURL = 'http://127.0.0.1:5173'

export default defineConfig({
  testDir: './e2e',

  timeout: 30_000,

  expect: {
    timeout: 10_000,
  },

  // Menghindari banyak login paralel yang
  // dapat memicu rate limiter backend.
  fullyParallel: false,
  workers: 1,
  retries: 0,

  reporter: 'list',

  use: {
    baseURL,
    trace: 'off',
    screenshot: 'off',
    video: 'off',
  },

  projects: [
    {
      name: 'chromium',
      use: {
        ...devices['Desktop Chrome'],
      },
    },
  ],

  // Hanya menjalankan Vite.
  // Backend Go dan PostgreSQL harus sudah aktif.
  webServer: {
    command: 'npm run dev',
    url: `${baseURL}/login`,
    reuseExistingServer: true,
    timeout: 60_000,
    stdout: 'ignore',
    stderr: 'pipe',
  },
})
