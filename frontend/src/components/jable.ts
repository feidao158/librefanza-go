export function jableURL(id: string): string {
  const normalized = id.trim().toLowerCase()
  return /^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(normalized) ? `https://jable.tv/videos/${normalized}/` : ''
}
