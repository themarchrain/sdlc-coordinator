import type { ThemeSlots } from '../types'

/**
 * hashicorp 主题的语义色槽位 —— 规格驱动，无参考实现。
 * 取色纪律 = 规格「chrome 单色 + 产品色只作身份」：数据视图不混用
 * 产品身份色（规格「Don't combine multiple product accents in the
 * same viewport」），而采用规格功能色集 —— accent-blue #2b89ff
 * （链接/焦点蓝）领趋势主色 + 语义三色（success #00ca8e / warning
 * #ffcf25 / error #e62b1e，规格明文与 Nomad/Vault/Consul 同值，
 * 语义复用即规格认可的功能用法）+ 白/#b2b6bd 单色文字阶。
 * 纯黑画布 (#000000) 与 #15181e 卡面上的描线对比已逐色核对（≥3:1）。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const hashicorpSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#2b89ff',          // 每日创建 MR：accent-blue（chrome 里唯一的功能蓝）
      crDone: '#00ca8e',      // 每日完成 CR：success 绿（= Nomad green）
      crStart: '#b2b6bd',     // 每日发起 CR：ink-muted 单色阶
      avgDuration: '#ffcf25', // 成功运行平均耗时：warning 黄（= Vault yellow，耗时即警示）
    },
    ratios: {
      success: '#00ca8e',     // 当前任务成功占比：绿
      firstRun: '#2b89ff',    // 首次执行成功率：蓝
      intervention: '#e62b1e', // 人工介入率：红（= Consul red）
      recovery: '#ffcf25',    // 受阻后恢复成功率：黄
    },
  },
  timeline: {
    error: '#e62b1e',   // failed / blocked / unavailable（= Consul red）
    warning: '#ffcf25', // human_required / retry_pending / waiting_upstream（= Vault yellow）
    success: '#00ca8e', // succeeded / approved（= Nomad green）
    info: '#2b89ff',    // 其余默认 = accent-blue
  },
  eventType: {
    error: '#e62b1e',  // codex_error / claude_error
    usage: '#00ca8e',  // codex_usage
    info: '#2b89ff',   // 其余默认
  },
  sparkline: {
    default: '#2b89ff',  // Sparkline 默认描线 = 功能蓝（白 CTA 之外的主交互色）
    hoverGrid: '#3b3d45', // hover 竖向网格虚线 = hairline（暗底可见）
  },
}
