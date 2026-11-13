import { expect, test } from '@playwright/test'

test('默认预览服务不会将未模拟接口转发到运行环境', async ({ request }) => {
  test.skip(Boolean(process.env.PLAYWRIGHT_BASE_URL), '外部测试服务的接口由调用者负责隔离')
  const response = await request.get('/api/v1/setup', { headers: { Accept: 'application/json' } })
  expect(response.status()).toBe(404)
})
