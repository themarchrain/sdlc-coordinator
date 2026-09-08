import type { ThemeSlots } from '../types'

/**
 * renault 主题的语义色槽位 —— 规格驱动，无参考实现。
 *
 * 规格（Semantic）的取色纪律：
 * - 黄是唯一品牌强调但「不进正文/数据」→ 图表主色用结构墨 black，
 *   趋势线走「墨 + 灰阶 + 功能蓝」：ink #000000 / mute #666666 / ash #8A8A8A
 *   + info #337AB7（desaturated mid-blue，功能性非装饰）；
 * - 语义四色为规格字面值：error #BE6464 / warning #F0AD4E /
 *   success #8DC572 / info #337AB7。时间线节点是实心填充（近徽标面），
 *   直接用字面值；占比卡 sparkline 是白底细描线，success/warning 字面值
 *   对比不足 3:1 → 同 hue 深化到描线级 #56913D / #A8760D（推导值）。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const renaultSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#000000',        // 每日创建 MR = 结构墨（黄不进数据，黑即结构色）
      crDone: '#337AB7',    // 每日完成 CR = info 功能蓝
      crStart: '#666666',   // 每日发起 CR = mute
      avgDuration: '#8A8A8A', // 每日成功运行平均耗时 = ash（弱曲线）
    },
    ratios: {
      success: '#56913D',   // 当前任务成功占比 = success 深化（#8DC572 白底描线对比不足，推导）
      firstRun: '#000000',  // 首次执行成功率 = 结构墨
      intervention: '#BE6464', // 触发人工介入率 = error 字面值
      recovery: '#A8760D',  // 受阻后恢复成功率 = warning 深化（#F0AD4E 同上，推导）
    },
  },
  timeline: {
    error: '#BE6464',   // failed / blocked / unavailable = error 字面值（实心节点）
    warning: '#F0AD4E', // human_required / retry_pending / waiting_upstream = warning 字面值
    success: '#8DC572', // succeeded / approved = success 字面值
    info: '#337AB7',    // 其余默认 = info 功能蓝
  },
  eventType: {
    error: '#BE6464',  // codex_error / claude_error
    usage: '#8DC572',  // codex_usage = success 字面值
    info: '#337AB7',   // 其余默认
  },
  sparkline: {
    default: '#000000',  // Sparkline 默认描线 = 结构墨（黄不进数据墨水）
    hoverGrid: '#C4C4C4', // hover 竖向网格虚线 = stone（禁用级灰，足见度）
  },
}
