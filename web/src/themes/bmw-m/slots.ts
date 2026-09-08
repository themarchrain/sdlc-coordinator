import type { ThemeSlots } from '../types'

/**
 * bmw-m 主题的语义色槽位 —— 自设计语言 的色彩组在纯黑画布上派生。
 * 取值全部来自规格既有色（不引入新品牌色，规格 Don't）：
 *   M 红 #e22718（motorsport-pace）/ bmw-blue #1c69d4 / m-blue-light #0066b1 /
 *   ink 白 #ffffff / warning #f4b400 / success #0fa336。
 * 趋势线/占比线是数据点缀的合法场景（同 xai 的点缀组思路）；
 * 时间线/事件节点直接写 hex（preset 名会渲染成 antd 标准蓝绿，与品牌不符），
 * 节点为 10px 色点，非文字，暗底可分辨性以色相区分优先。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const bmwMSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#e22718',       // 每日创建 MR = M 红（pace 线）
      crDone: '#ffffff',   // 每日完成 CR = ink 白（完成态落在主色）
      crStart: '#1c69d4',  // 每日发起 CR = bmw-blue
      avgDuration: '#f4b400', // 平均耗时 = warning 琥珀（时长警示族）
    },
    ratios: {
      success: '#0fa336',      // 成功占比 = success
      firstRun: '#f4b400',     // 首次执行成功率 = 琥珀
      intervention: '#e22718', // 人工介入率 = M 红（介入即警示）
      recovery: '#1c69d4',     // 受阻恢复率 = bmw-blue
    },
  },
  timeline: {
    error: '#e22718',     // failed / blocked / unavailable = M 红
    warning: '#f4b400',   // human_required / retry_pending = 琥珀
    success: '#0fa336',   // succeeded / approved = success
    info: '#1c69d4',      // 其余默认 = bmw-blue
  },
  eventType: {
    error: '#e22718',    // codex_error / claude_error
    usage: '#0fa336',    // codex_usage
    info: '#1c69d4',     // 其余默认
  },
  sparkline: {
    default: '#1c69d4',  // 默认描线 = bmw-blue（克制的 M 强调色）
    hoverGrid: '#3c3c3c', // hover 竖向网格虚线 = hairline
  },
}
