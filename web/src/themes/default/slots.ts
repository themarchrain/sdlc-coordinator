import type { ThemeSlots } from '../types'

/**
 * default 主题的语义色槽位 —— 值为 develop 现行硬编码，视觉零改动。
 * Phase 2 誊抄其他主题时保持键结构不变，只改值。
 */
export const defaultSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#16a34a',
      crDone: '#00CED1',
      crStart: '#7c3aed',
      avgDuration: '#d9534f',
    },
    ratios: {
      success: '#16a34a',
      firstRun: '#DAA520',
      intervention: '#DC143C',
      recovery: '#D2691E',
    },
  },
  timeline: {
    error: 'red',
    warning: 'orange',
    success: 'green',
    info: 'blue',
  },
  eventType: {
    error: 'red',
    usage: 'green',
    info: 'blue',
  },
  sparkline: {
    default: '#2f6bff',
    hoverGrid: '#c5cfdd',
  },
}
