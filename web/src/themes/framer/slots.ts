import type { ThemeSlots } from '../types'

/**
 * framer 主题的语义色槽位 —— 自设计语言 语义段派生。
 *
 * 规格：调色板 = 单色（ink #ffffff / ink-muted #999999）
 * + 唯一信号蓝 #0099ff（链接 / focus / 选中指示）+ 渐变家族
 * （magenta #d44df0 / violet #6a4cf5 / orange #ff7a3d / coral #ff5577）。
 * 语义色仅 success #22c55e 在规格内；warning/error 无对应物 →
 * 取渐变家族 orange/coral（保持「不引入第二种蓝或额外彩相」的纪律）。
 * 趋势线走「白 + 蓝 + 灰 + 渐变族 violet」四色，遵守二元文字律的同时
 * 保证暗画布上的可分辨性。键结构与 default/slots.ts 保持一致，只改值。
 */
export const framerSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#ffffff',         // 每日创建 MR = ink 白（头部指标独占最高层级）
      crDone: '#0099ff',     // 每日完成 CR = 信号蓝（数据指示是蓝的合法用途）
      crStart: '#999999',    // 每日发起 CR = ink-muted（二元律的次级档）
      avgDuration: '#6a4cf5', // 每日平均耗时 = 渐变族 violet（规格渐变基色，推导）
    },
    ratios: {
      success: '#22c55e',    // 成功占比 = 规格 checkmark 绿
      firstRun: '#0099ff',   // 首次执行成功率 = 信号蓝
      intervention: '#ff5577', // 人工介入率 = 渐变族 coral（error 同源，推导）
      recovery: '#ff7a3d',   // 恢复成功率 = 渐变族 sunset-orange（warning 同源，推导）
    },
  },
  timeline: {
    error: '#ff5577',   // failed / blocked / unavailable = coral（推导）
    warning: '#ff7a3d', // human_required / retry_pending / waiting_upstream = orange（推导）
    success: '#22c55e', // succeeded / approved = 规格 success 绿
    info: '#0099ff',    // 其余默认 = 信号蓝（selected indicator 合法场景）
  },
  eventType: {
    error: '#ff5577',  // codex_error / claude_error
    usage: '#22c55e',  // codex_usage = 规格 success 绿
    info: '#0099ff',   // 其余默认
  },
  sparkline: {
    default: '#0099ff',  // Sparkline 默认描线 = 信号蓝
    hoverGrid: '#404040', // hover 竖向网格虚线 = 强发丝线（推导：比 hairline #262626 高一档）
  },
}
