import { defineConfig } from '@playwright/test'
import { frontends } from './apps'

// One suite for the auth starter in every frontend: the same steps, by the
// same roles, labels and words, on the apps tug new makes, served as
// they're deployed. setup.ts makes and runs them, which takes minutes the
// first time. Locally the tests use the Chrome that's installed; CI
// installs Playwright's Chromium. Both have the virtual authenticator that
// stands in for a phone's passkeys.
export default defineConfig({
  testDir: 'tests',
  globalSetup: './setup.ts',
  timeout: 60_000,
  expect: { timeout: 10_000 },
  use: {
    channel: process.env.CI ? undefined : 'chrome',
    trace: 'retain-on-failure',
  },
  projects: frontends.map((f) => ({ name: f.name, use: { baseURL: `http://localhost:${f.port}` } })),
})
