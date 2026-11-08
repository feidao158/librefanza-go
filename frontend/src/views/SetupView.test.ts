import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import SetupView from './SetupView.vue'
import { i18n } from '../i18n'

const request = vi.hoisted(() => vi.fn())
vi.mock('../api/client', () => ({ api: request, jsonBody: (data: unknown) => JSON.stringify(data) }))

describe('安装向导', () => {
  it('可用内置数据库时默认只展示管理员设置，并保留外部数据库入口', async () => {
    request.mockResolvedValue({ data: { installed: false, managed_database_available: true } })
    const wrapper = mount(SetupView, { global: { plugins: [i18n] } })
    await flushPromises()
    expect(wrapper.text()).toContain('管理员账号')
    expect(wrapper.text()).toContain('使用外部数据库')
    expect(wrapper.find('input[type="email"]').exists()).toBe(true)
  })
  it('提交内置安装标志而不依赖浏览器输入数据库凭据', async () => {
    request.mockReset()
    request.mockResolvedValueOnce({ data: { installed: false, managed_database_available: true } })
    request.mockRejectedValueOnce(new Error('安装失败'))
    const wrapper = mount(SetupView, { global: { plugins: [i18n] } })
    await flushPromises()
    await wrapper.get('input[type="email"]').setValue('admin@example.com')
    await wrapper.get('input[type="password"]').setValue('password123')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()
    const [, options] = request.mock.calls[1]
    expect(JSON.parse(options.body).managed_database).toBe(true)
    expect(wrapper.text()).toContain('连接或初始化失败')
  })
})
