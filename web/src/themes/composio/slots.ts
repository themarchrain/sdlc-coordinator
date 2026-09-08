import type { ThemeSlots } from '../types'

/**
 * composio 主题的语义色槽位 —— 规格驱动，无参考实现。
 * 取值 = 规格色板的暗底可用子集：Composio Blue 提亮变体 #4d79ff
 * （深蓝 #0007cd 在 #181818 卡上对比不足 2:1，文字/描线一律用同色相
 * 提亮版——推导值）+ 语义双色（success #33d17a / error #ff4d4d）+
 * 推导暗底琥珀 #f5b83d（规格未给 warning）。
 * 青 #00d4ff 规格定位「data-flow visualizations 上克制使用」——
 * 平均耗时趋势线是本应用里唯一的 data-flow 曲线，恰好是合法场景。
 * 时间线/事件色直接写字面 hex（非 preset），保证与令牌块严格同源。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const composioSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#4d79ff',        // 每日创建 MR：Composio Blue 提亮变体（趋势主色）
      crDone: '#33d17a',    // 每日完成 CR：规格 success 绿
      crStart: '#f5b83d',   // 每日发起 CR：推导暗底琥珀
      avgDuration: '#00d4ff', // 成功运行平均耗时：accent cyan（规格限定 data-flow 可视化场景）
    },
    ratios: {
      success: '#33d17a',     // 当前任务成功占比：规格绿
      firstRun: '#4d79ff',    // 首次执行成功率：品牌蓝
      intervention: '#ff4d4d', // 触发人工介入率：规格红
      recovery: '#f5b83d',    // 受阻后恢复成功率：推导琥珀
    },
  },
  timeline: {
    error: '#ff4d4d',   // failed / blocked / unavailable（规格 semantic-error）
    warning: '#f5b83d', // human_required / retry_pending / waiting_upstream（推导琥珀）
    success: '#33d17a', // succeeded / approved（规格 semantic-success）
    info: '#4d79ff',    // 其余默认 = 品牌蓝提亮变体
  },
  eventType: {
    error: '#ff4d4d',  // codex_error / claude_error
    usage: '#33d17a',  // codex_usage
    info: '#4d79ff',   // 其余默认
  },
  sparkline: {
    default: '#4d79ff', // Sparkline 默认描线：品牌蓝提亮变体
    hoverGrid: '#333333', // hover 竖向网格虚线：hairline-strong（暗底可见）
  },
}
