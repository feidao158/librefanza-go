import { mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import RecommendationsView from './RecommendationsView.vue'
import { i18n } from '../i18n'

describe('RecommendationsView', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('推荐服务不可用时不显示标注不足', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: false, status: 503, json: async () => ({ error: { code: 'recommendation_unavailable' } }) })))
    const wrapper = mount(RecommendationsView, { global: { plugins: [i18n], stubs: { RouterLink: { template: '<a><slot /></a>' } } } })
    await vi.waitFor(() => expect(wrapper.find('.alert-error').exists()).toBe(true))
    expect(wrapper.text()).toContain('推荐服务暂不可用')
    expect(wrapper.text()).not.toContain('推荐算法正在学习中')
    wrapper.unmount()
  })

  it('renders prompt when annotations threshold is not met', async () => {
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve({
      ok: true,
      status: 200,
      json: () => Promise.resolve({
        data: {
          ready: false,
          stats: {
            total: 2,
            liked: 1,
            disliked: 1,
            uncertain: 0,
            min_required: 5,
            ready: false,
          },
          movies: [],
        },
      }),
    })))

    const wrapper = mount(RecommendationsView, {
      global: {
        plugins: [i18n],
        stubs: { RouterLink: { template: '<a><slot /></a>' } },
      },
    })

    await vi.waitFor(() => {
      expect(wrapper.text()).toContain('推荐算法正在学习中')
      expect(wrapper.text()).toContain('前往人工标注')
    })
  })

})
