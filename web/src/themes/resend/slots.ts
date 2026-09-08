import type { ThemeSlots } from '../types'

/**
 * resend 主题的语义色槽位 —— 规格驱动，无参考实现。
 * 取值 = 规格 accent 表的实心子集：blue #3b9eff（链接/信息）、green #11ff99
 * （成功状态点）、yellow #ffc53d（行内高亮）、red #ff2047（行内错误红）
 * + 白 #fcfdff / mute #a1a4a5 灰阶。规格约束「solid accent 只作行内高亮与
 * 状态点、不作表面」——图表描线/时间线节点正是行内事件色，属合法用法。
 * 趋势线走「蓝 + 白 + mute + 黄」：白保持稀缺（规格 Iteration「Keep
 * colors.primary scarce」），蓝作为链接/交互主色承头阵。
 * 真黑画布 (#000000/#0a0a0c) 上的可分辨性已逐色核对（描线对比 ≥3:1）。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const resendSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#3B9EFF',          // 每日创建 MR：accent-blue = 链接/交互主色
      crDone: '#FCFDFF',      // 每日完成 CR：白（次重要曲线借「最亮像素」）
      crStart: '#A1A4A5',     // 每日发起 CR：mute 灰
      avgDuration: '#FFC53D', // 每日成功运行平均耗时：accent-yellow（耗时即提示）
    },
    ratios: {
      success: '#11FF99',      // 当前任务成功占比：accent-green（status-dot 同源）
      firstRun: '#3B9EFF',     // 首次执行成功率：蓝
      intervention: '#FF2047', // 触发人工介入率：accent-red
      recovery: '#FFC53D',     // 受阻后恢复成功率：黄
    },
  },
  timeline: {
    error: '#FF2047',   // failed / blocked / unavailable（accent-red）
    warning: '#FFC53D', // human_required / retry_pending / waiting_upstream（accent-yellow）
    success: '#11FF99', // succeeded / approved（accent-green）
    info: '#3B9EFF',    // 其余默认 = accent-blue
  },
  eventType: {
    error: '#FF2047',  // codex_error / claude_error
    usage: '#11FF99',  // codex_usage（accent-green）
    info: '#3B9EFF',   // 其余默认
  },
  sparkline: {
    default: '#3B9EFF',              // Sparkline 默认描线：accent-blue
    hoverGrid: 'rgba(255, 255, 255, 0.14)', // hover 竖向网格虚线：hairline-strong
  },
}
