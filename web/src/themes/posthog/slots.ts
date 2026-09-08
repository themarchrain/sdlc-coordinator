import type { ThemeSlots } from '../types'

/**
 * posthog 主题的语义色槽位 —— 自设计语言 colors 表 + 语义段派生。
 *
 * 规格语义词汇 = 四彩通告族（banner-tip）：blue #2c84e0（💡 Tip/Info）、
 * green #2c8c66（✅ Success）、red #cd4239（⚠️ Warning/Caution —— 规格把
 * 「警示」绑在红族上）、purple #7c44a6（📘 Note/Reference）。
 * 迁移取舍：
 *   - error = accent-red 字面值（规格警示即红）；timeline warning 需要
 *     与 error 区分的节点色 → 派生标准琥珀 #97650d（规格无真琥珀）；
 *   - 趋势/占比线走「四彩 accent 家族 + 墨阶」——产品分析品牌的图表本位
     语汇（规格 Known Gaps：in-product 图表未 captured，按通告族角色推导）；
 *   - 黄橙 #f7a501 按规格 Do「Reserve for the primary CTA pill only」
 *     不进图表与节点。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const posthogSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#2c84e0',      // 每日创建 MR = accent-blue（Tip/Info 族，头部指标）
      crDone: '#23251d',  // 每日完成 CR = ink 深橄榄炭（数据墨主轨）
      crStart: '#6c6e63', // 每日发起 CR = mute（静默说明档）
      avgDuration: '#7c44a6', // 每日成功运行平均耗时 = accent-purple（四彩家族第四相）
    },
    ratios: {
      success: '#2c8c66',     // 当前任务成功占比 = accent-green（✅ 族字面值）
      firstRun: '#2c84e0',    // 首次执行成功率 = accent-blue
      intervention: '#cd4239', // 触发人工介入率 = accent-red（⚠️ 族字面值）
      recovery: '#7c44a6',    // 受阻后恢复成功率 = accent-purple（📘 族第四相）
    },
  },
  timeline: {
    error: '#cd4239',   // failed / blocked / unavailable（accent-red 字面值）
    warning: '#97650d', // human_required / retry_pending / waiting_upstream（派生琥珀，与红节点区分）
    success: '#2c8c66', // succeeded / approved（accent-green 字面值）
    info: '#2c84e0',    // 其余默认 = accent-blue
  },
  eventType: {
    error: '#cd4239', // codex_error / claude_error（accent-red）
    usage: '#2c8c66', // codex_usage（accent-green）
    info: '#2c84e0',  // 其余默认（accent-blue）
  },
  sparkline: {
    default: '#2c84e0',  // Sparkline 默认描线 = accent-blue
    hoverGrid: '#dcdfd2', // hover 竖向网格虚线 = hairline-soft 字面值
  },
}
