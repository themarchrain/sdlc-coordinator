import type { ThemeSlots } from '../types'

/**
 * together 主题的语义色槽位 —— 自设计语言 色板派生。
 *
 * 品牌不维护语义色板，「adopting framework defaults is
 * appropriate」——error/success/warning 直接取 antd 默认（#FF4D4F / #52C41A /
 * #FAAD14）。图表族收敛为「墨 + 灰 + 长春花」的蓝图墨线语汇：
 *   墨 #000000（黑 CTA 同源）= 头部指标与用量事件
 *   Body 灰 #959494（规格唯一次级文字色）= 次级线与默认节点
 *   长春花 #BDBBFF（accent-periwinkle，规格允许的填充位）= 辅助线
 *   深长春花 #8483B3（推导：periwinkle 压深 30%，白底可辨性）
 * 三色渐变丝带（orange→magenta→periwinkle）按规格「never miniaturised /
 * never reduced to a swatch」禁止进入图表线，只留在登录 showcase 的
 * hero 尺度装饰（见 css 片段）。键结构与 default/slots.ts 保持一致，只改值。
 */
export const togetherSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#000000',      // 每日创建 MR = 墨（黑 CTA 同源的主线）
      crDone: '#959494',  // 每日完成 CR = Body 灰
      crStart: '#8483b3', // 每日发起 CR = 深长春花（推导）
      avgDuration: '#bdbbff', // 每日成功运行平均耗时 = 长春花（accent-periwinkle）
    },
    ratios: {
      success: '#52c41a',   // 当前任务成功占比 = 框架默认绿
      firstRun: '#000000',  // 首次执行成功率 = 墨（主指标黑）
      intervention: '#ff4d4f', // 触发人工介入率 = 框架默认红
      recovery: '#faad14',  // 受阻后恢复成功率 = 框架默认琥珀
    },
  },
  timeline: {
    error: '#ff4d4f',   // failed / blocked / unavailable = 框架默认红
    warning: '#faad14', // human_required / retry_pending / waiting_upstream = 框架默认琥珀
    success: '#52c41a', // succeeded / approved = 框架默认绿
    info: '#959494',    // 其余默认 = Body 灰（无彩默认节点，推导位）
  },
  eventType: {
    error: '#ff4d4f', // codex_error / claude_error = 框架默认红
    usage: '#000000', // codex_usage = 墨（黑 CTA 同源的功能主色）
    info: '#959494',  // 其余默认 = Body 灰
  },
  sparkline: {
    default: '#000000',  // Sparkline 默认描线 = 蓝图墨线
    hoverGrid: '#d9d9d9', // hover 竖向网格虚线 = 强发丝线档
  },
}
