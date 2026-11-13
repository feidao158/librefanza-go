import { expect, test } from '@playwright/test'

test('管理员查看数据面板并访问喜欢列表', async ({ page }) => {
  await page.addInitScript(() => window.localStorage.setItem('librefanza-access-token', 'admin-token'))
  await page.route('**/api/v1/setup', route => route.fulfill({ json: { data: { installed: true } } }))
  await page.route('**/api/v1/auth/me', route => route.fulfill({ json: { data: { id: 1, email: 'admin@example.com', is_admin: true } } }))
  await page.route('**/api/v1/admin/dashboard', route => route.fulfill({ json: { data: { movies: 1234, source_items: 2345, errors_24h: 3, queues: [{ queue: 'metadata', status: 'pending', count: 42 }] } } }))
  await page.route('**/api/v1/annotations/likes?*', route => route.fulfill({ json: { data: [{ id: 1, normalized_id: 'LIKE-001', preferred_item: { id: 1, source: 'fanza', title: '喜欢的作品' } }], meta: { page: 1, page_size: 30, total: 1 } } }))
  await page.goto('/admin/dashboard')
  await expect(page.getByRole('heading', { name: '数据面板' })).toBeVisible()
  await expect(page.getByText('1,234')).toBeVisible()
  await expect(page.getByText('42', { exact: true })).toBeVisible()
  const likesLink = page.getByRole('link', { name: '喜欢过' }).first()
  await likesLink.focus()
  await likesLink.press('Enter')
  await expect(page.getByText('LIKE-001')).toBeVisible()
  await expect(page.getByText('喜欢的作品')).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBeTruthy()
})

test('首次访问可选择数据库、跳过 Redis 并填写管理员', async ({ page }) => {
  await page.route('**/api/v1/setup', route => route.fulfill({ json: { data: { installed: false } } }))
  await page.goto('/')
  await expect(page.getByRole('heading', { name: '初始化 LibreFanza' })).toBeVisible()
  await page.getByLabel('用户名').fill('app')
  await page.getByRole('button', { name: '下一步' }).click()
  await expect(page.getByText('启用 Redis')).toBeVisible()
  await page.getByRole('button', { name: '下一步' }).click()
  await page.getByLabel('电子邮箱').fill('admin@example.com')
  await page.getByLabel('密码').fill('test-password')
  await expect(page.getByRole('button', { name: '完成安装' })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBeTruthy()
})

test('安装向导提交 PostgreSQL、可选 Redis 和管理员配置', async ({ page }) => {
  let installed = false
  let databaseChecked = false
  let redisChecked = false
  let request: unknown
  await page.route('**/api/v1/setup', route => route.fulfill({ json: { data: { installed } } }))
  await page.route('**/api/v1/setup/database-checks', route => { databaseChecked = true; return route.fulfill({ json: { data: { connected: true } } }) })
  await page.route('**/api/v1/setup/redis-checks', route => { redisChecked = true; return route.fulfill({ json: { data: { connected: true } } }) })
  await page.route('**/api/v1/setup/installations', route => { request = route.request().postDataJSON(); installed = true; return route.fulfill({ status: 201, json: { data: { installed: true } } }) })
  await page.route('**/api/v1/movies?*', route => route.fulfill({ json: { data: [], meta: { page: 1, page_size: 30, total: 0 } } }))
  await page.goto('/')
  await page.getByLabel('用户名').fill('app')
  await page.getByRole('button', { name: '测试连接' }).click()
  await expect.poll(() => databaseChecked).toBe(true)
  await page.getByRole('button', { name: '下一步' }).click()
  await page.getByLabel('启用 Redis').check()
  await page.getByRole('button', { name: '测试连接' }).click()
  await expect.poll(() => redisChecked).toBe(true)
  await page.getByRole('button', { name: '下一步' }).click()
  await page.getByLabel('电子邮箱').fill('admin@example.com')
  await page.getByLabel('密码').fill('test-password')
  await page.getByRole('button', { name: '完成安装' }).click()
  await expect.poll(() => request).toMatchObject({ database: { driver: 'postgres', port: 5432, user: 'app' }, redis: { enabled: true }, email: 'admin@example.com' })
  await expect(page.getByRole('heading', { name: '影片' })).toBeVisible()
})

test('影片列表可切换英文并展示来源数据', async ({ page, isMobile }) => {
  await page.route('**/api/v1/setup', route => route.fulfill({ json: { data: { installed: true } } }))
  await page.route('**/api/v1/movies?*', route => route.fulfill({ json: { data: [{ id: 1, normalized_id: 'ABC-123', compressed_id: 'ABC-123', is_hidden: false, created_at: '', updated_at: '', preferred_item: { id: 1, source: 'fanza', title: '示例标题', thumbnail_image_url: '', cover_image_url: '' } }], meta: { page: 1, page_size: 30, total: 1 } } }))
  await page.goto('/movies')
  await expect(page.getByText('ABC-123')).toBeVisible()
  await expect(page.getByText('示例标题')).toBeVisible()
  if (isMobile) {
    await page.getByRole('navigation', { name: '手机导航' }).getByRole('button', { name: '更多' }).click()
    await page.getByRole('dialog', { name: '更多' }).getByLabel('语言').selectOption('en')
  } else {
    await page.locator('.header-actions').getByLabel('语言').selectOption('en')
  }
  await expect(page.getByRole('heading', { name: 'Movies' })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBeTruthy()
})

test('影片详情可在不同来源版本之间切换', async ({ page }) => {
  await page.route('**/api/v1/setup', route => route.fulfill({ json: { data: { installed: true } } }))
  const fanza = { id: 1, source: 'fanza', title: 'Fanza title', normalized_id: 'ABC-123', cover_image_url: '', thumbnail_image_url: '', metadata: {} }
  const sod = { id: 2, source: 'sod', title: 'SOD title', normalized_id: 'ABC-123', cover_image_url: '', thumbnail_image_url: '', metadata: {} }
  await page.route('**/api/v1/movies/ABC-123', route => route.fulfill({ json: { data: { id: 1, normalized_id: 'ABC-123', preferred_item: fanza, versions: [fanza, sod] } } }))
  await page.goto('/movies/ABC-123')
  await expect(page.getByRole('heading', { name: 'Fanza title' })).toBeVisible()
  await page.getByRole('link', { name: 'SOD SOD title' }).click()
  await expect(page.getByRole('heading', { name: 'SOD title' })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBeTruthy()
})

test('未收录演员资料时仍能从影片进入作品列表', async ({ page }) => {
  await page.route('**/api/v1/setup', route => route.fulfill({ json: { data: { installed: true } } }))
  const movie = { id: 1, normalized_id: 'ABC-123', preferred_item: { id: 1, source: 'fanza', title: '示例影片', metadata: { cast: [{ name: '出演者 名' }] } } }
  await page.route('**/api/v1/movies/ABC-123', route => route.fulfill({ json: { data: movie } }))
  await page.route('**/api/v1/actress-filmographies?*', route => route.fulfill({ json: { data: { actress: { id: 0, name: '出演者 名', image_url: '' }, movies: [movie] }, meta: { page: 1, page_size: 30, total: 1 } } }))
  await page.goto('/movies/ABC-123')
  await page.getByRole('link', { name: '出演者 名' }).click()
  await expect(page.getByRole('heading', { name: '出演者 名' })).toBeVisible()
  await expect(page.getByText('ABC-123')).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBeTruthy()
})

test('搜索未收录番号时自动提交检索任务', async ({ page }) => {
  let lookups = 0
  await page.route('**/api/v1/setup', route => route.fulfill({ json: { data: { installed: true } } }))
  await page.route('**/api/v1/movies?*', route => route.fulfill({ json: { data: [], meta: { page: 1, page_size: 30, total: 0 } } }))
  await page.route('**/api/v1/product-ids/normalize?*', route => route.fulfill({ json: { data: { valid: true, normalized: 'ABC-123' } } }))
  await page.route('**/api/v1/movies/ABC-123', route => {
    lookups++
    return route.fulfill({ status: 404, json: { error: { code: 'not_found' } } })
  })
  let submitted = 0
  await page.route('**/api/v1/search-jobs', route => { submitted++; return route.fulfill({ status: 202, json: { data: { id: 'job-id' } } }) })
  await page.route('**/api/v1/search-jobs/job-id', route => route.fulfill({ json: { data: { id: 'job-id', status: 'completed', result: { found: false } } } }))
  await page.goto('/movies')
  await page.getByPlaceholder('输入番号或前缀').fill('abc00123')
  await page.getByRole('button', { name: '搜索', exact: true }).click()
  await expect.poll(() => submitted).toBe(1)
  await expect(page.getByRole('heading', { name: '未收录此影片' })).toBeVisible()
  await expect.poll(() => lookups).toBe(2)
})

test('用户可以注册并登录', async ({ page, isMobile }) => {
  await page.route('**/api/v1/setup', route => route.fulfill({ json: { data: { installed: true } } }))
  let registered = false
  await page.route('**/api/v1/users', route => { registered = true; return route.fulfill({ status: 201, json: { data: {} } }) })
  await page.route('**/api/v1/auth/sessions', route => route.fulfill({ json: { data: { access_token: 'test-token', user: { id: 1, email: 'user@example.com', is_admin: false } } } }))
  await page.route('**/api/v1/auth/me', route => route.fulfill({ json: { data: { id: 1, email: 'user@example.com', is_admin: false } } }))
  await page.route('**/api/v1/movies?*', route => route.fulfill({ json: { data: [], meta: { page: 1, page_size: 30, total: 0 } } }))
  await page.goto('/register')
  await page.getByLabel('电子邮箱').fill('user@example.com')
  await page.getByLabel('密码').fill('password123')
  await page.getByRole('button', { name: '注册' }).click()
  await expect.poll(() => registered).toBe(true)
  await expect(page).toHaveURL('/')
  if (isMobile) {
    await page.getByRole('navigation', { name: '手机导航' }).getByRole('button', { name: '更多' }).click()
    await expect(page.getByRole('dialog', { name: '更多' }).getByText('user@example.com')).toBeVisible()
  } else {
    await expect(page.getByTitle('user@example.com')).toBeVisible()
  }
})

test('管理员可以调整 Redis、查看原始页面并重试任务', async ({ page }) => {
  await page.addInitScript(() => window.localStorage.setItem('librefanza-access-token', 'admin-token'))
  await page.route('**/api/v1/setup', route => route.fulfill({ json: { data: { installed: true } } }))
  await page.route('**/api/v1/auth/me', route => route.fulfill({ json: { data: { id: 1, email: 'admin@example.com', is_admin: true } } }))
  let savedRedis: unknown
  await page.route('**/api/v1/admin/settings/redis', route => {
    if (route.request().method() === 'PUT') savedRedis = route.request().postDataJSON()
    return route.fulfill({ json: { data: { enabled: false, address: 'localhost:6379', username: '', password_configured: true, database: 0, tls: false } } })
  })
  await page.route('**/api/v1/admin/settings/redis-checks', route => route.fulfill({ json: { data: { ok: true } } }))
  await page.route('**/api/v1/admin/source-pages?*', route => route.fulfill({ json: { data: [{ id: 1, source: 'javlibrary', url: 'https://example.com/', created_at: '' }], meta: { page: 1, page_size: 30, total: 1 } } }))
  await page.route('**/api/v1/admin/source-pages/1', route => route.fulfill({ json: { data: { id: 1, source: 'javlibrary', raw_html: '<script>window.injected = true</script>' } } }))
  let retried = false
  await page.route('**/api/v1/admin/jobs?*', route => route.fulfill({ json: { data: [{ id: '12345678-0000', type: 'search', status: retried ? 'pending' : 'failed', attempt: 1, created_at: '2026-01-01T00:00:00Z' }], meta: { page: 1, page_size: 30, total: 1 } } }))
  await page.route('**/api/v1/admin/jobs/12345678-0000/retries', route => { retried = true; return route.fulfill({ json: { data: {} } }) })
  await page.goto('/admin/cache')
  await page.getByLabel('启用 Redis').check()
  await page.getByLabel('清除已保存的密码').check()
  await page.getByRole('button', { name: '测试连接' }).click()
  await page.getByRole('button', { name: '保存' }).click()
  await expect.poll(() => savedRedis).toMatchObject({ enabled: true, clear_password: true })
  await page.getByRole('link', { name: '原始页面' }).click()
  await page.getByTitle('查看').click()
  await expect(page.locator('pre')).toContainText('<script>window.injected = true</script>')
  expect(await page.evaluate(() => (window as unknown as { injected?: boolean }).injected)).toBeUndefined()
  await page.getByLabel('关闭').first().click()
  await page.getByRole('link', { name: '任务', exact: true }).click()
  await page.getByRole('row', { name: /12345678/ }).getByTitle('重试').click()
  await expect.poll(() => retried).toBe(true)
  await expect(page.getByRole('row', { name: /12345678/ })).toContainText('等待中')
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBeTruthy()
})
