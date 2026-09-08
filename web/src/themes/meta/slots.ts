import type { ThemeSlots } from '../types'

/**
 * meta 主题的语义色槽位 —— 自设计语言 colors 表 + 语义段派生。
 *
 * 规格（frontmatter colors + ）：success #31a24c（「In stock」
 * 类确认）、attention #f2a918（中优提醒）、warning #f7b928（促销横幅底）、
 * critical #e41e3f（校验错误/破坏性反馈）。钴蓝 #0064e0 = 产品内交互色，
 * info/默认节点一律回落钴蓝（Deep Cobalt #0457cb 亦为 active link color）。
 * 迁移取舍：
 *   - attention/warning 黄在白底上作节点/文字对比不足 → 深化 #97690f（同 hue 文字级）；
 *   - recovery 占比描线用 attention #f2a918 原值（badge 色作 2px 描线可读，非文本位）；
 *   - 趋势线第四色相 = Oculus Purple #a121ce —— 偏离点：规格紫仅 Quest 品类
 *     强调位（「VR product accent … for category emphasis」），白底上钴蓝/
 *     近黑/钢灰三色已尽，按「品类强调」角色将紫引入为第四趋势色。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const metaSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#0064e0',      // 每日创建 MR = Cobalt（产品内交互蓝，头部指标）
      crDone: '#0a1317',  // 每日完成 CR = Deep Ink（黑主轨数据墨）
      crStart: '#5d6c7b', // 每日发起 CR = Steel（静默说明档）
      avgDuration: '#a121ce', // 每日成功运行平均耗时 = Oculus Purple（偏离见上：品类强调角色）
    },
    ratios: {
      success: '#31a24c',     // 当前任务成功占比 = 规格 success
      firstRun: '#0064e0',    // 首次执行成功率 = Cobalt
      intervention: '#e41e3f', // 触发人工介入率 = Critical
      recovery: '#f2a918',    // 受阻后恢复成功率 = Attention（badge 色作描线）
    },
  },
  timeline: {
    error: '#e41e3f',   // failed / blocked / unavailable
    warning: '#97690f', // human_required / retry_pending / waiting_upstream（attention 深化，推导）
    success: '#31a24c', // succeeded / approved
    info: '#0064e0',    // 其余默认 = 钴蓝
  },
  eventType: {
    error: '#e41e3f', // codex_error / claude_error（critical-strong 同族）
    usage: '#31a24c', // codex_usage
    info: '#0064e0',  // 其余默认
  },
  sparkline: {
    default: '#0064e0',  // Sparkline 默认描线 = Cobalt
    hoverGrid: '#ced0d4', // hover 竖向网格虚线 = Hairline Gray（规格字面值）
  },
}
