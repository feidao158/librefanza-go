import { expect, test } from '@playwright/test'

test('内置数据库安装只要求管理员账号并可切换外部数据库', async ({ page }) => {
  await page.route('**/api/v1/setup', route => route.fulfill({ json: { data: { installed: false, managed_database_available: true } } }))
  await page.goto('/')
  await expect(page.getByRole('heading', { name: '管理员账号' })).toBeVisible()
  await expect(page.getByLabel('电子邮箱')).toBeVisible()
  await page.getByRole('button', { name: '使用外部数据库' }).focus()
  await page.keyboard.press('Enter')
  await expect(page.getByLabel('数据库类型')).toBeVisible()
  await page.getByRole('button', { name: '使用内置数据库' }).focus()
  await page.keyboard.press('Enter')
  await expect(page.getByRole('heading', { name: '管理员账号' })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBeTruthy()
})

test('管理页面展示内置推荐服务状态', async ({ page }) => {
  await page.addInitScript(() => window.localStorage.setItem('librefanza-access-token', 'admin-token'))
  await page.route('**/api/v1/setup', route => route.fulfill({ json: { data: { installed: true } } }))
  await page.route('**/api/v1/auth/me', route => route.fulfill({ json: { data: { id: 1, email: 'admin@example.com', is_admin: true } } }))
  await page.route('**/api/v1/admin/settings/gorse', route => route.fulfill({ json: { data: { enabled: true, managed: true, connected: true, sync_error: true, min_feedback_count: 5, sync: { ready: true, phase: 'movies', cursor: 123 } } } }))
  await page.goto('/admin/gorse')
  await expect(page.getByText('内置服务')).toBeVisible()
  await expect(page.getByText('服务已连接')).toBeVisible()
  await expect(page.getByText('历史同步完成')).toBeVisible()
  await expect(page.getByText('同步异常，本地推荐生效')).toBeVisible()
  await expect(page.getByText('影片游标: 123')).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBeTruthy()
})
