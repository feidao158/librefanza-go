import { describe, expect, it } from 'vitest'
import { decodeSchedule, encodeSchedule } from './crawlSchedule'

describe('采集任务周期', () => {
  it('解析常用周期并保持自定义表达式', () => {
    expect(decodeSchedule('30 9 * * *')).toEqual({ mode: 'daily', hour: 9, minute: 30, weekday: 1 })
    expect(decodeSchedule('15 4 * * 2')).toEqual({ mode: 'weekly', hour: 4, minute: 15, weekday: 2 })
    expect(decodeSchedule('*/5 * * * *').mode).toBe('custom')
  })
  it('生成标准 Cron 表达式', () => {
    expect(encodeSchedule({ mode: 'daily', hour: 6, minute: 15, weekday: 1 })).toBe('15 6 * * *')
    expect(encodeSchedule({ mode: 'weekly', hour: 6, minute: 15, weekday: 2 })).toBe('15 6 * * 2')
    expect(encodeSchedule({ mode: 'hourly', hour: 6, minute: 15, weekday: 2 })).toBe('15 * * * *')
  })
})
