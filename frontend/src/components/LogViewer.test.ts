import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import LogViewer from './LogViewer.vue'
import { i18n } from '../i18n'

const request = vi.hoisted(() => vi.fn())
vi.mock('../api/client', () => ({ api: request }))

describe('运行日志', () => {
  beforeEach(() => {
    request.mockReset()
    request.mockResolvedValue({ data: [{ id: 1, created_at: '2026-11-06T06:00:00Z', level: 'error', event: 'task_failed', source: 'jable', task_type: 'source_crawl', task_id: '7281166a-f25c-4dbb-8c06-179d4da365c0', attempt: 2, reason: 'http', http_status: 403, stage: 'fetch', position_type: 'page', position: 4 }], meta: { total: 1 } })
  })
  it('展示失败原因、来源、页码、重试次数并支持筛选', async () => {
    const wrapper = mount(LogViewer, { global: { plugins: [i18n] } })
    await flushPromises()
    expect(wrapper.text()).toContain('任务执行失败')
    expect(wrapper.text()).toContain('HTTP 403')
    expect(wrapper.text()).toContain('第 4 页')
    expect(wrapper.text()).toContain('第 2 次')
    expect(wrapper.text()).toContain('JABLE')
    await wrapper.get('[name="level"]').setValue('error')
    await wrapper.get('[name="source"]').setValue('jable')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()
    expect(request).toHaveBeenLastCalledWith('/admin/logs?page=1&level=error&source=jable')
  })
  it('接口失败时展示错误状态', async () => {
    request.mockRejectedValue(new Error('unavailable'))
    const wrapper = mount(LogViewer, { global: { plugins: [i18n] } })
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('操作未完成')
  })
  it('从任务跳转时按任务编号查询日志', async () => {
    mount(LogViewer, { props: { taskId: '7281166a-f25c-4dbb-8c06-179d4da365c0' }, global: { plugins: [i18n] } })
    await flushPromises()
    expect(request).toHaveBeenCalledWith('/admin/logs?page=1&task_id=7281166a-f25c-4dbb-8c06-179d4da365c0')
  })
})
