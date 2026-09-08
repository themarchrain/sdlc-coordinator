import type { ThemeSlots } from '../types'

/**
 * expo 主题的语义色槽位 —— 自设计语言 语义段派生。
 *
 * 规格：语义色有字面值 —— success #16A34A、
 * warning #AB6400（docs callout 琥珀）、error #EB8E90（软红）；蓝色
 * #0D74CE 只做行内链接色，绝不落 CTA。迁移策略：
 *   成功/警示照抄字面值；软红 #EB8E90 保留给「点」形态（时间线节点），
 *   描线形态（占比卡 Sparkline）加深为 #D97074 保证白底可读 —— 行内注明。
 * 默认节点（timeline.info / eventType.info）回落 Ink #171717 —— 蓝只属于
 * 链接，中性默认才是「quietly-confident」语汇。
 * 趋势线走「黑 + 灰阶 + 一条链接蓝」：mr 纯黑（唯一电压承载数据墨水）、
 * crDone/crStart 走 Body/Muted 灰阶、avgDuration 用链接蓝补第四级可分辨性
 * （图表描线是数据读取面，非 CTA，不违反「蓝不上按钮」纪律）。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const expoSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#000000',      // 每日创建 MR = 纯黑（唯一电压 = 头部数据墨水）
      crDone: '#60646c',  // 每日完成 CR = Body 灰
      crStart: '#999999', // 每日发起 CR = Muted 灰
      avgDuration: '#0d74ce', // 每日成功运行平均耗时 = Text Link Blue（数据蓝）
    },
    ratios: {
      success: '#16a34a',      // 当前任务成功占比 = 语义绿（字面值）
      firstRun: '#0d74ce',     // 首次执行成功率 = 链接蓝
      intervention: '#d97074', // 触发人工介入率 = 软红加深一档（描线可读，派生）
      recovery: '#ab6400',     // 受阻后恢复成功率 = 琥珀（字面值）
    },
  },
  timeline: {
    error: '#eb8e90',   // failed / blocked / unavailable（规格软红，点形态保留）
    warning: '#ab6400', // human_required / retry_pending / waiting_upstream
    success: '#16a34a', // succeeded / approved
    info: '#171717',    // 其余默认 = Ink（蓝只属于链接）
  },
  eventType: {
    error: '#eb8e90', // codex_error / claude_error
    usage: '#16a34a', // codex_usage
    info: '#171717',  // 其余默认
  },
  sparkline: {
    default: '#000000',  // Sparkline 默认描线 = 纯黑数据墨水
    hoverGrid: '#dcdee0', // hover 竖向网格虚线 = Hairline Strong
  },
}
