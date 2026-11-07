import { expect, test } from '@playwright/test'

test('管理员查看和筛选采集日志', async ({ page }) => {
  await page.addInitScript(() => window.localStorage.setItem('librefanza-access-token', 'test'))
  let requested = ''
  await page.route('**/api/v1/**', async route => {
    const url = new URL(route.request().url())
    let data: unknown = {}
    if (url.pathname.endsWith('/setup')) data = { installed: true }
    else if (url.pathname.endsWith('/auth/me')) data = { id: 1, email: 'admin@example.com', is_admin: true }
    else if (url.pathname.endsWith('/admin/logs')) {
      requested = url.search
      data = [{ id: 1, created_at: '2026-11-06T06:00:00Z', level: 'error', event: 'task_failed', source: 'jable', task_type: 'source_crawl', task_id: '7281166a-f25c-4dbb-8c06-179d4da365c0', attempt: 2, reason: 'http', http_status: 403, stage: 'fetch', position_type: 'page', position: 4 }]
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data, meta: { page: 1, page_size: 30, total: 1 } }) })
  })
  await page.goto('/admin/logs')
  await expect(page.getByRole('heading', { name: '运行日志' })).toBeVisible()
  await expect(page.getByText('任务执行失败')).toBeVisible()
  await expect(page.getByText('来源返回错误状态 HTTP 403')).toBeVisible()
  await expect(page.getByText('第 4 页')).toBeVisible()
  await page.getByLabel('级别').selectOption('error')
  await page.getByRole('combobox', { name: '来源', exact: true }).selectOption('jable')
  await page.getByRole('button', { name: '筛选' }).click()
  await expect.poll(() => requested).toContain('level=error')
  expect(requested).toContain('source=jable')
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
})

test('从任务列表直接查看对应日志', async ({ page }) => {
  await page.addInitScript(() => window.localStorage.setItem('librefanza-access-token', 'test'))
  const id = '7281166a-f25c-4dbb-8c06-179d4da365c0'
  let requested = ''
  await page.route('**/api/v1/**', async route => {
    const url = new URL(route.request().url())
    let data: unknown = {}
    if (url.pathname.endsWith('/setup')) data = { installed: true }
    else if (url.pathname.endsWith('/auth/me')) data = { id: 1, email: 'admin@example.com', is_admin: true }
    else if (url.pathname.endsWith('/admin/jobs')) data = [{ id, type: 'source_crawl', status: 'failed', attempt: 1, created_at: '2026-11-06T06:00:00Z' }]
    else if (url.pathname.endsWith('/admin/logs')) { requested = url.search; data = [] }
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data, meta: { page: 1, page_size: 30, total: 1 } }) })
  })
  await page.goto('/admin/tasks')
  await page.getByRole('link', { name: '查看任务日志' }).click()
  await expect(page.getByRole('heading', { name: '运行日志' })).toBeVisible()
  await expect.poll(() => requested).toContain(`task_id=${id}`)
  await expect(page.getByRole('textbox', { name: '任务编号' })).toHaveValue(id)
})
