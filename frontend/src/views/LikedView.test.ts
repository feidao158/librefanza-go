import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { i18n } from '../i18n'
import LikedView from './LikedView.vue'

describe('LikedView', () => {
  it('展示当前用户喜欢的作品并支持分页', async () => {
    const fetchMock = vi.fn(async (_url: string) => ({ ok: true, status: 200, json: async () => ({ data: [{ id: 1, normalized_id: 'LIKE-001' }], meta: { total: 31 } }) }))
    vi.stubGlobal('fetch', fetchMock)
    const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/likes', component: LikedView }] })
    await router.push('/likes')
    await router.isReady()
    const wrapper = mount({ template: '<RouterView />' }, { global: { plugins: [i18n, router], stubs: { MovieCard: { props: ['movie'], template: '<div>{{ movie.normalized_id }}</div>' } } } })
    await vi.waitFor(() => expect(wrapper.text()).toContain('LIKE-001'))
    expect(fetchMock.mock.calls[0][0]).toContain('/annotations/likes?page=1&page_size=30')
    expect(wrapper.text()).toContain('31')
    wrapper.unmount()
    vi.unstubAllGlobals()
  })
})
