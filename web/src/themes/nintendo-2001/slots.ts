import type { ThemeSlots } from '../types'

/**
 * nintendo-2001 语义色槽位 —— 自规格语义色按角色派生，全字面 hex。
 *
 * 宿主面约束：趋势/占比 Sparkline 画在 ratio-card / 指标卡上，其底是
 * --canvas-soft（Pale Sky #9fbee7）与白卡；亮暖色（amber #ecab37 /
 * nav-gold / signal）在 Pale Sky 上对比不足 1.5:1，描线会消失，
 * 因此图表线一律取规格深色阶：games-red / systems-teal / chrome-indigo /
 * carbon / muted-indigo / amber 压深的金棕。
 * 时间线/事件节点是实心色点（非文字），恢复使用饱和原值（品牌红 / nav-gold）。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const nintendo2001Slots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#a7282b',          // 每日创建 MR（games-red，F-1 hero 域）
      crDone: '#206479',      // 每日完成 CR（systems-teal）
      crStart: '#3d4f97',     // 每日发起 CR（chrome-indigo）
      avgDuration: '#21242e', // 成功运行平均耗时（carbon）
    },
    ratios: {
      success: '#206479',      // 当前任务成功占比（systems-teal）
      firstRun: '#60619c',     // 首次执行成功率（muted-indigo）
      intervention: '#a7282b', // 触发人工介入率（games-red）
      recovery: '#8c5c00',     // 受阻后恢复成功率（amber 压深金棕）
    },
  },
  timeline: {
    error: '#e60012',   // failed / blocked / unavailable（Nintendo Red = 规格 error）
    warning: '#e48600', // human_required / retry_pending / waiting_upstream（nav-gold）
    success: '#206479', // succeeded / approved（systems-teal）
    info: '#3d4f97',    // 其余默认（chrome-indigo）
  },
  eventType: {
    error: '#e60012',  // codex_error / claude_error
    usage: '#206479',  // codex_usage（systems-teal）
    info: '#3d4f97',   // 其余默认（chrome-indigo）
  },
  sparkline: {
    default: '#3d4f97',  // Sparkline 默认描线（chrome-indigo，Pale Sky 面上 ≈4:1）
    hoverGrid: '#5a5f8c', // hover 竖向网格虚线（hairline）
  },
}
