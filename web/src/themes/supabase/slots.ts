import type { ThemeSlots } from '../types'

/**
 * supabase 主题的语义色槽位 —— 自 design/supabase 各页面/组件的硬编码誊抄。
 * 取值规则：祖母绿 #24b47e（深阶，作描线比 #3ecf8e 更稳）承载一切正向趋势；
 * 中性趋势回落墨阶 #9a9a9a / #171717；信号红 #b03a31 与琥珀 #d48806 仅保留
 * 失败/等待语义；时间线中性节点用冷灰 #c7c7c7。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const supabaseSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#24b47e',          // OverviewPage 已创建 MR
      crDone: '#24b47e',      // OverviewPage 已完成 CR
      crStart: '#24b47e',     // OverviewPage 已发起 CR
      avgDuration: '#9a9a9a', // OverviewPage 成功运行平均耗时（中性灰阶）
    },
    ratios: {
      success: '#24b47e',      // 当前任务成功占比
      firstRun: '#24b47e',     // 首次执行成功率
      intervention: '#171717', // 触发人工介入率（墨色单色强调）
      recovery: '#24b47e',     // 受阻后恢复成功率
    },
  },
  timeline: {
    error: '#b03a31',   // failed / blocked / unavailable（signal-danger-ink）
    warning: '#d48806', // human_required / retry_pending / waiting_upstream
    success: '#24b47e', // succeeded / approved（primary-deep）
    info: '#c7c7c7',    // 其余默认节点（hairline-strong 单色灰）
  },
  eventType: {
    error: '#b03a31',  // codex_error / claude_error
    usage: '#24b47e',  // codex_usage（祖母绿=用量）
    info: '#c7c7c7',   // 其余默认
  },
  sparkline: {
    default: '#24b47e',  // Sparkline 默认描线（源组件默认色）
    hoverGrid: '#c7c7c7', // hover 竖向网格虚线
  },
}
