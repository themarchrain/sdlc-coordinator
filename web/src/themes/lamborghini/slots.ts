import type { ThemeSlots } from '../types'

/**
 * lamborghini 主题的语义色槽位 —— 规格驱动，无参考实现。
 * 取值 = 规格色板的「约 5 个活跃色」子集：金 #FFC000/#FFCE3E、Cyan Pulse
 * #29ABE2、Teal #1EAEDB、Link Blue #3860BE + 白/Smoke/Ash/Steel 灰阶。
 * 无语义色码：success #34D399 / error #FF453A 为暗底标准语义克制化
 * 推导（照 tesla「语义态跟随浏览器默认→克制化」先例），warning 直接采用
 * 品牌金琥珀（金即暖琥珀系）。
 * 趋势线走「金 + 白 + 灰 + 青」：mr 独占金（头部指标 = 唯一彩事件，呼应
 * 规格「金只落在最重要之物上」），其余白/灰/青保持 monochrome-plus-gold 纪律。
 * 真黑画布 (#000000/#202020) 上的可分辨性已逐色核对（描线对比 ≥3:1）。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const lamborghiniSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#FFC000',          // 每日创建 MR：Giallo 金 = 唯一彩事件（头部指标独占）
      crDone: '#FFFFFF',      // 每日完成 CR：Pure White
      crStart: '#969696',     // 每日发起 CR：Steel
      avgDuration: '#29ABE2', // 每日成功运行平均耗时：Cyan Pulse（信息类曲线）
    },
    ratios: {
      success: '#34D399',      // 当前任务成功占比：推导语义绿
      firstRun: '#29ABE2',     // 首次执行成功率：Cyan Pulse
      intervention: '#FF453A', // 触发人工介入率：推导语义红
      recovery: '#FFC000',     // 受阻后恢复成功率：品牌金琥珀
    },
  },
  timeline: {
    error: '#FF453A',   // failed / blocked / unavailable
    warning: '#FFC000', // human_required / retry_pending / waiting_upstream（金琥珀）
    success: '#34D399', // succeeded / approved
    info: '#29ABE2',    // 其余默认 = Cyan Pulse
  },
  eventType: {
    error: '#FF453A', // codex_error / claude_error
    usage: '#34D399', // codex_usage
    info: '#29ABE2',  // 其余默认
  },
  sparkline: {
    default: '#FFC000',   // Sparkline 默认描线：金（黑画布上的单一金事件）
    hoverGrid: '#494949', // hover 竖向网格虚线：Graphite 强线（暗底可见）
  },
}
