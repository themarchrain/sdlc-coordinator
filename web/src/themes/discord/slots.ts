import type { ThemeSlots } from '../types'

/**
 * discord 主题的语义色槽位 —— 自设计语言 色彩组在深靛蓝画布上派生。
 * 取值 = 品牌全和弦 + Discord 语义色（green/yellow/red/cuchsia）：
 *   blurple #5865f2 / green #35ed7e / magenta #ec48bd / link cyan #00b0f4 /
 *   idle 黄 #f0b132 / dnd 红 #f23f43（后两者自产品状态语言推导）。
 * 趋势线/占比线是数据点缀的合法场景（同 xai 点缀组思路）；
 * 时间线/事件节点直接写 hex（preset 名会渲染成 antd 标准蓝绿，与品牌不符），
 * 节点为 10px 色点，暗底可分辨性以色相区分优先。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const discordSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#5865f2',       // 每日创建 MR = blurple（主流程落在品牌主色）
      crDone: '#35ed7e',   // 每日完成 CR = green（完成态）
      crStart: '#ec48bd',  // 每日发起 CR = magenta（发起的 playful 计数）
      avgDuration: '#f0b132', // 每日成功运行平均耗时 = 黄（时长警示族）
    },
    ratios: {
      success: '#35ed7e',      // 当前任务成功占比 = green
      firstRun: '#00b0f4',     // 首次执行成功率 = link cyan
      intervention: '#f23f43', // 触发人工介入率 = 红（介入即危急）
      recovery: '#ec48bd',     // 受阻后恢复成功率 = magenta
    },
  },
  timeline: {
    error: '#f23f43',    // failed / blocked / unavailable = 红
    warning: '#f0b132',  // human_required / retry_pending / waiting_upstream = 黄
    success: '#35ed7e',  // succeeded / approved = green
    info: '#5865f2',     // 其余默认 = blurple
  },
  eventType: {
    error: '#f23f43',    // codex_error / claude_error
    usage: '#35ed7e',    // codex_usage
    info: '#5865f2',     // 其余默认
  },
  sparkline: {
    default: '#5865f2',  // Sparkline 默认描线 = blurple
    hoverGrid: '#3a3f7d', // hover 竖向网格虚线 = 强调发丝线阶
  },
}
