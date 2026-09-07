import type { ThemeSlots } from '../types'

/**
 * MiniMax 主题的语义色槽位 —— 值自 minimax 源 TSX 硬编码誊抄
 * （OverviewPage trendColor / TasksPage timelineColor /
 *   CodexObservabilityPage eventTypeColor / Sparkline 默认色与 hover 网格）。
 * 时间线/事件节点用 antd preset 名（red/orange/green/blue），
 * 由 themes/css/minimax.css 的 [data-theme="minimax"]
 * .ant-timeline-item-color-* 覆写对齐品牌令牌，两层各司其职。
 */
export const minimaxSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#1ba673',       /* 源：success 绿 */
      crDone: '#1456f0',   /* 源：brand blue */
      crStart: '#a855f7',  /* 源：brand purple */
      avgDuration: '#ff5530', /* 源：brand coral */
    },
    ratios: {
      success: '#1ba673',
      firstRun: '#1456f0',
      intervention: '#ff5530',
      recovery: '#a855f7',
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
    default: '#1456f0', /* 源 Sparkline 默认描线 = --mm-blue */
    hoverGrid: '#c9ccd2',
  },
}
