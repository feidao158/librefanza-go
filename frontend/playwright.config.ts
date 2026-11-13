import fs from 'node:fs'
import { defineConfig, devices } from '@playwright/test'

const systemChrome = ['/usr/bin/google-chrome', '/usr/bin/chromium', '/usr/bin/chromium-browser'].find(p => fs.existsSync(p))
const chromiumLaunch = { executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE || systemChrome }
const baseURL = process.env.PLAYWRIGHT_BASE_URL || 'http://127.0.0.1:4174'

export default defineConfig({
  testDir: './e2e',
  timeout: 20_000,
  use: {
    baseURL,
    trace: 'retain-on-failure',
  },
  webServer: process.env.PLAYWRIGHT_BASE_URL ? undefined : {
    command: 'npm run build && npx vite preview --host 127.0.0.1 --port 4174 --strictPort',
    url: baseURL,
    reuseExistingServer: true,
    timeout: 60_000,
  },
  projects: [
    { name: 'desktop', use: { ...devices['Desktop Chrome'], viewport: { width: 1440, height: 900 }, launchOptions: chromiumLaunch } },
    { name: 'mobile', use: { ...devices['iPhone 13'], defaultBrowserType: 'chromium', launchOptions: chromiumLaunch } },
    { name: 'narrow', use: { ...devices['iPhone SE'], defaultBrowserType: 'chromium', viewport: { width: 320, height: 640 }, launchOptions: chromiumLaunch } },
    { name: 'webkit', use: { ...devices['iPhone 13'] } },
  ],
})
