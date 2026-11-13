export function isInteractiveShortcutTarget(event: KeyboardEvent): boolean {
  return event.target instanceof Element && Boolean(event.target.closest('a, button, input, textarea, select, [contenteditable]:not([contenteditable="false"]), [role="dialog"]'))
}
