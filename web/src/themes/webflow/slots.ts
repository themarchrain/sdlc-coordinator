import type { ThemeSlots } from '../types'

/**
 * webflow 主题的语义色槽位 —— 自设计语言 语义段派生。
 *
 * 规格：五停 chromatic 类别色（紫 #7A3DFF / 粉 #ED52CB /
 * 蓝 #3B89FF / 橙 #FF6B00 / 绿 #00D722）只作整面填充，不作交互件底色；
 * 管理台的图表/时间线取色即「mini 类别卡」填充位，可全饱和使用。
 * 语义三色取规格字面（红 #EE1D36 / 黄 #FFAE13 / 绿 #00D722），
 * 但白画布上的细描线（Sparkline 1.5px stroke）与带文字的节点按对比度
 * 压深一档：#00A31A（绿 L≈32%）/ #B37500（黄 L≈35%），与 antd 同源。
 * 趋势线走「紫 + 墨阶」：mr 独占类别紫（设计/构建 = 管理台的本职域），
 * 其余三级用 Ink/Body/Mute 墨阶，呼应两色转换层级纪律。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const webflowSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#7a3dff',      // 每日创建 MR = Accent Purple（design/build 类别色，头部指标独占彩色）
      crDone: '#080808',  // 每日完成 CR = Ink Black
      crStart: '#5a5a5a', // 每日发起 CR = Body-Mid
      avgDuration: '#898989', // 每日成功运行平均耗时 = Mute
    },
    ratios: {
      success: '#00a31a',      // 当前任务成功占比 = 规格 #00D722 压深（细描线可读性）
      firstRun: '#7a3dff',     // 首次执行成功率 = Accent Purple
      intervention: '#ee1d36', // 触发人工介入率 = Error Red 规格字面
      recovery: '#b37500',     // 受阻后恢复成功率 = 规格 #FFAE13 压深（细描线可读性）
    },
  },
  timeline: {
    error: '#ee1d36',   // failed / blocked / unavailable = Error Red 规格字面
    warning: '#b37500', // human_required / retry_pending / waiting_upstream（#FFAE13 压深，节点带标签）
    success: '#00a31a', // succeeded / approved（#00D722 压深，与 antd colorSuccess 同源）
    info: '#146ef5',    // 其余默认 = badge-info 蓝（accent-blue-info）
  },
  eventType: {
    error: '#ee1d36',  // codex_error / claude_error
    usage: '#00a31a',  // codex_usage（#00D722 压深）
    info: '#146ef5',   // 其余默认 = badge-info 蓝
  },
  sparkline: {
    default: '#7a3dff',  // Sparkline 默认描线 = Accent Purple（数据墨水的类别填充位）
    hoverGrid: '#d8d8d8', // hover 竖向网格虚线 = 规格 hairline
  },
}
