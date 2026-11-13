import { defineConfig } from '@playwright/test'

const port = 8093

// The tests drive the built frontend, served by the Go server: run
// `npm run build` first. Locally they use the Chrome that's installed; CI
// installs Playwright's Chromium.
export default defineConfig({
  testDir: 'e2e',
  use: {
    baseURL: `http://127.0.0.1:${port}`,
    channel: process.env.CI ? undefined : 'chrome',
    // A failed test's trace, in test-results/, which CI keeps.
    trace: 'retain-on-failure',
  },
  webServer: {
    command: `ADDR=127.0.0.1:${port} go run .`,
    url: `http://127.0.0.1:${port}/`,
    reuseExistingServer: false,
    timeout: 120_000,
  },
})
