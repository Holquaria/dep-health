// Deterministically maps a cascade group string to a color from the palette.
// The same group string always produces the same color, independent of order.
const PALETTE = [
  '#6366f1', // indigo
  '#f59e0b', // amber
  '#10b981', // emerald
  '#8b5cf6', // violet
  '#06b6d4', // cyan
  '#ec4899', // pink
]

export function cascadeColor(group) {
  if (!group) return null
  let hash = 0
  for (let i = 0; i < group.length; i++) {
    hash = (hash * 31 + group.charCodeAt(i)) & 0xffff
  }
  return PALETTE[hash % PALETTE.length]
}
