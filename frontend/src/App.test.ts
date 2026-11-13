import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { i18n } from './i18n'
import App from './App.vue'

const auth = vi.hoisted(() => ({
  user: null as null | { email: string; is_admin: boolean },
  restore: vi.fn(async () => {}),
  logout: vi.fn(async () => {}),
}))

vi.mock('./api/client', () => ({ api: vi.fn(async () => ({ data: { installed: true } })) }))
vi.mock('./stores/auth', () => ({ useAuthStore: () => auth }))

const mounted: ReturnType<typeof mount>[] = []

async function mountApp() {
  i18n.global.locale.value = 'zh'
  const empty = { template: '<div />' }
  const router = createRouter({
    history: createMemoryHistory(),
    routes: ['/', '/movies', '/actresses', '/recommendations', '/likes', '/annotations', '/login', '/admin/:section?', '/setup']
      .map(path => ({ path, component: empty })),
  })
  await router.push('/movies')
  await router.isReady()
  const wrapper = mount(App, { global: { plugins: [i18n, router] } })
  mounted.push(wrapper)
  await flushPromises()
  return { wrapper, router }
}

afterEach(() => {
  mounted.splice(0).forEach(wrapper => wrapper.unmount())
  auth.user = null
  vi.clearAllMocks()
})

describe('应用导航', () => {
  it('手机底部提供常用入口，更多菜单保留标注和登录入口', async () => {
    const { wrapper, router } = await mountApp()
    const nav = wrapper.get('nav[aria-label="手机导航"]')
    expect(nav.findAll('a').map(link => link.attributes('href'))).toEqual(['/movies', '/actresses', '/recommendations', '/likes'])
    const more = nav.get('button[aria-expanded]')
    expect(more.attributes('aria-expanded')).toBe('false')
    await more.trigger('click')
    expect(more.attributes('aria-expanded')).toBe('true')
    expect(wrapper.get('#mobile-more a[href="/annotations"]').text()).toContain('标注')
    expect(wrapper.get('#mobile-more a[href="/login"]').text()).toContain('登录')
    await router.push('/annotations')
    await flushPromises()
    expect(more.attributes('aria-expanded')).toBe('false')
  })

  it('管理员可从更多菜单进入管理页面，并可用 Escape 关闭菜单', async () => {
    auth.user = { email: 'admin@example.test', is_admin: true }
    const { wrapper } = await mountApp()
    const more = wrapper.get('nav[aria-label="手机导航"] button[aria-expanded]')
    await more.trigger('click')
    expect(wrapper.get('#mobile-more a[href="/admin/sources"]').text()).toContain('管理')
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await wrapper.vm.$nextTick()
    expect(more.attributes('aria-expanded')).toBe('false')
  })
})
