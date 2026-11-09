import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { i18n } from '../i18n'
import Dashboard from './Dashboard.vue'

describe('Dashboard', () => {
  it('展示采集数据、任务队列和近期错误', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, status: 200, json: async () => ({ data: { movies: 42, source_items: 55, errors_24h: 3, queues: [{ queue: 'metadata', status: 'pending', count: 9 }] } }) })))
    const wrapper = mount(Dashboard, { global: { plugins: [i18n], stubs: { RouterLink: { template: '<a><slot /></a>' } } } })
    await vi.waitFor(() => expect(wrapper.text()).toContain('42'))
    expect(wrapper.text()).toContain('55')
    expect(wrapper.text()).toContain('9')
    wrapper.unmount()
    vi.unstubAllGlobals()
  })
})
