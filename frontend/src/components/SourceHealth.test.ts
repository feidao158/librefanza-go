import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import SourceHealth from './SourceHealth.vue'
import { i18n } from '../i18n'

describe('来源状态', () => {
  it('显示冷却原因、HTTP 状态和下次重试时间，并列出尚未检查的来源', () => {
    const wrapper = mount(SourceHealth, {
      props: { health: [{ source: 'javdb', status: 'cooldown', failures: 3, reason: 'http', http_status: 403, last_checked: '2026-11-07T01:00:00Z', next_retry: '2026-11-07T02:00:00Z' }] },
      global: { plugins: [i18n] },
    })
    const row = wrapper.get('[data-source="javdb"]')
    expect(row.text()).toContain('冷却中')
    expect(row.text()).toContain('HTTP 403')
    expect(row.text()).toContain('来源返回错误状态')
    expect(row.text()).toContain('2026')
    expect(wrapper.get('[data-source="javquery"]').text()).toContain('尚未检查')
  })
})
