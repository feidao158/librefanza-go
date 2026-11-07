export interface ScheduleEditor {
  mode: 'interval' | 'hourly' | 'daily' | 'weekly' | 'custom'
  interval?: number
  hour: number
  minute: number
  weekday: number
}

export function decodeSchedule(value: string): ScheduleEditor {
  const parts = value.trim().split(/\s+/)
  const interval = /^\*\/(5|10|15|20|30)$/.exec(parts[0] || '')
  if (parts.length === 5 && interval && parts.slice(1).every(part => part === '*')) return { mode: 'interval', interval: Number(interval[1]), hour: 0, minute: 0, weekday: 1 }
  const minute = Number(parts[0])
  const hour = Number(parts[1])
  const validMinute = /^\d+$/.test(parts[0] || '') && minute <= 59
  if (parts.length === 5 && validMinute && parts[2] === '*' && parts[3] === '*') {
    if (parts[1] === '*' && parts[4] === '*') return { mode: 'hourly', hour: 0, minute, weekday: 1 }
    if (/^\d+$/.test(parts[1]) && hour <= 23 && parts[4] === '*') return { mode: 'daily', hour, minute, weekday: 1 }
    const weekday = Number(parts[4])
    if (/^\d+$/.test(parts[1]) && hour <= 23 && /^[0-6]$/.test(parts[4])) return { mode: 'weekly', hour, minute, weekday }
  }
  return { mode: 'custom', hour: 0, minute: 0, weekday: 1 }
}

export function encodeSchedule(value: ScheduleEditor): string {
  if (value.mode === 'interval') return `*/${value.interval || 30} * * * *`
  if (value.mode === 'hourly') return `${value.minute} * * * *`
  if (value.mode === 'weekly') return `${value.minute} ${value.hour} * * ${value.weekday}`
  return `${value.minute} ${value.hour} * * *`
}
