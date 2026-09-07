import type { ThemeSlots } from '../types'

/**
 * voltagent 主题的语义色槽位 —— 自 design/voltagent 各页面/组件的硬编码誊抄。
 * 取值 = Voltagent 暗色语义（映射表）：
 * - 时间线 / 事件点：error #f87171 / warning #fbbf24 / success #10b981 / 进行中=live 绿 #00d992
 * - 8 条 sparkline 趋势线：单色数据墨 #bdbdbd（绿是保留色，只给 CTA/pill/live，
 *   绝不作数据填色；原 8 色语义由卡片文字标签承担）
 * - hover 参考线：mute #8b949e（Sparkline 源字面值）
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const voltagentSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#bdbdbd',          // OverviewPage 已创建 MR（数据墨）
      crDone: '#bdbdbd',      // OverviewPage 已完成 CR（数据墨）
      crStart: '#bdbdbd',     // OverviewPage 已发起 CR（数据墨）
      avgDuration: '#bdbdbd', // OverviewPage 成功运行平均耗时（数据墨）
    },
    ratios: {
      success: '#bdbdbd',      // 当前任务成功占比（数据墨）
      firstRun: '#bdbdbd',     // 首次执行成功率（数据墨）
      intervention: '#bdbdbd', // 触发人工介入率（数据墨）
      recovery: '#bdbdbd',     // 受阻后恢复成功率（数据墨）
    },
  },
  timeline: {
    error: '#f87171',   // failed / blocked / unavailable
    warning: '#fbbf24', // human_required / retry_pending / waiting_upstream
    success: '#10b981', // succeeded / approved
    info: '#00d992',    // 其余默认节点 = 进行中 live 绿
  },
  eventType: {
    error: '#f87171',   // codex_error / claude_error
    usage: '#10b981',   // codex_usage（正向 = success 绿）
    info: '#00d992',    // 其余默认事件 = live 绿（无第二 accent）
  },
  sparkline: {
    default: '#bdbdbd', // Sparkline 默认描线（源组件默认值 = 数据墨）
    hoverGrid: '#8b949e', // hover 竖向网格虚线（源字面值 = mute）
  },
}
