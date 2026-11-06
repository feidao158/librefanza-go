import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import MoviePlayer from './MoviePlayer.vue'
import { i18n } from '../i18n'

const request = vi.hoisted(() => vi.fn())
vi.mock('../api/client', () => ({ api: request }))

describe('影片播放', () => {
  beforeEach(() => { request.mockReset() })
  it('按需检索并提示没有可播放来源', async () => {
    request.mockResolvedValue({ data: [] })
    const wrapper = mount(MoviePlayer, { props: { id: 'ABC-123' }, global: { plugins: [i18n] } })
    expect(request).not.toHaveBeenCalled()
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(request).toHaveBeenCalledWith('/movies/ABC-123/playbacks')
    expect(wrapper.text()).toContain('未找到可播放的来源')
  })
  it('来源请求失败时展示错误状态', async () => {
    request.mockRejectedValue(new Error('source unavailable'))
    const wrapper = mount(MoviePlayer, { props: { id: 'ABC-123' }, global: { plugins: [i18n] } })
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('播放来源不可用')
  })
})
