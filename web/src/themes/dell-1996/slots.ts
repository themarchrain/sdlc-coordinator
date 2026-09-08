import type { ThemeSlots } from '../types'

/**
 * dell-1996 主题的语义色槽位 —— 规格语义色派生，无参考实现。
 * 取值 = 规格闭色板内优先：Dell red #e91d2a（error/注意）、经典 Mosaic 蓝
 * #0000ee（info/链接语义）、purple-stripe #6a26a4、periwinkle #8c9ae0、
 * peach #e6915d（压深为 #c2693b 保描线可辨）；规格未给绿/黄语义，按时代
 * HTML 基色推导：success #008000、warning #8a6a00（yellow-sticker 压深）。
 * 全部取色保证在纯白画布 / pale sage 指标卡上可分辨。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const dell1996Slots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#008000',         // OverviewPage 已创建 MR（语义绿）
      crDone: '#6a26a4',     // OverviewPage 已完成 CR（purple-stripe 缎带）
      crStart: '#0000ee',    // OverviewPage 已发起 CR（经典链接蓝）
      avgDuration: '#e91d2a', // OverviewPage 成功运行平均耗时（Dell red）
    },
    ratios: {
      success: '#008000',      // 当前任务成功占比（语义绿）
      firstRun: '#8c9ae0',     // 首次执行成功率（tint-periwinkle）
      intervention: '#e91d2a', // 触发人工介入率（Dell red = 最高注意级）
      recovery: '#c2693b',     // 受阻后恢复成功率（tint-peach 压深，保描线对比）
    },
  },
  timeline: {
    error: '#e91d2a',   // failed / blocked / unavailable（Dell red）
    warning: '#8a6a00', // human_required / retry_pending / waiting_upstream（深贴纸黄）
    success: '#008000', // succeeded / approved（语义绿）
    info: '#0000ee',    // 其余默认节点（经典链接蓝）
  },
  eventType: {
    error: '#e91d2a',  // codex_error / claude_error（Dell red）
    usage: '#008000',  // codex_usage（正向 = 语义绿）
    info: '#0000ee',   // 其余默认事件（经典链接蓝）
  },
  sparkline: {
    default: '#000000', // Sparkline 默认描线 = 目录黑数据墨（GIF 折线图）
    hoverGrid: '#b3b3b3', // hover 竖向网格虚线（抖动灰）
  },
}
