let n = 0;

/**
 * Per-instance id suffix for components that carry inline SVG <defs>.
 * Several app-shell mocks render on one page, and a repeated gradient id
 * would make every later sparkline resolve to the first one's definition.
 */
export function nextId(prefix: string): string {
  n += 1;
  return `${prefix}-${n}`;
}
