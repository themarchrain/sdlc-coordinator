import type { ThemeSlots } from '../types'

/**
 * warp 主题的语义色槽位 —— 规格驱动，无参考实现。
 * 规格（Semantic）明确「营销面不出现独立 error/warning/success 色板，
 * 校验信号来自统一的暖白系统」——因此：
 *   - 默认/info 一律回落品牌暖白 #f7f5f0（ink 即 primary 的规格原文）；
 *   - 必须有彩的槽位（成功/警示/失败）按「暖调低饱和终端色」推导：
 *     绿 #8fbf7f / 琥珀 #d9a253 / 赤陶红 #e07a5f，均与暖棕画布同体温，
 *     在 #383330 面板上有 ≥3:1 的描线对比（推导值）。
 * 趋势线走「暖白 + 文字阶梯」四级（off-white/body/mute），镜像
 * tesla 的 monochrome-plus-one 纪律——Warp 的品牌签名正是无彩色。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const warpSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#f7f5f0',        // 每日创建 MR = 品牌暖白（头部指标独占品牌色）
      crDone: '#dad2c1',    // 每日完成 CR = body-strong（规格文字第二阶）
      crStart: '#c9c0ad',   // 每日发起 CR = body（规格文字第三阶）
      avgDuration: '#aea69c', // 每日成功运行平均耗时 = mute（规格文字第四阶）
    },
    ratios: {
      success: '#8fbf7f',      // 当前任务成功占比 = 推导暖绿
      firstRun: '#f7f5f0',     // 首次执行成功率 = 品牌暖白（中性指标不引入第二彩相）
      intervention: '#e07a5f', // 触发人工介入率 = 推导赤陶红
      recovery: '#d9a253',     // 受阻后恢复成功率 = 推导琥珀
    },
  },
  timeline: {
    error: '#e07a5f',   // failed / blocked / unavailable（推导赤陶红）
    warning: '#d9a253', // human_required / retry_pending / waiting_upstream（推导琥珀）
    success: '#8fbf7f', // succeeded / approved（推导暖绿）
    info: '#f7f5f0',    // 其余默认 = 品牌暖白（规格：默认信号走统一 off-white 系统）
  },
  eventType: {
    error: '#e07a5f',  // codex_error / claude_error
    usage: '#8fbf7f',  // codex_usage
    info: '#f7f5f0',   // 其余默认
  },
  sparkline: {
    default: '#f7f5f0',  // Sparkline 默认描线 = 品牌暖白（无彩色纪律）
    hoverGrid: '#554e47', // hover 竖向网格虚线 = 推导暖强线（暗底可见的发丝线上浮阶）
  },
}
