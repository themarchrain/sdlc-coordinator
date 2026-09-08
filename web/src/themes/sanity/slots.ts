import type { ThemeSlots } from '../types'

/**
 * sanity 主题的语义色槽位 —— 规格驱动，无参考实现。
 * 取值 = 规格色板的暗底可用子集：coral-red #f36458（唯一品牌强调，头牌曲线
 * 独占）+ link-blue-soft #55beff（暗底链接/信息蓝）+ success #37cd84 +
 * error #dd0000（brand-deep）。
 * 规格缺失 warning：按暗底可读的中明度琥珀 #e6a23c 推导（编辑部式去饱和，
 * 与 coral/error 保持可分辨的色相差）。
 * 近黑画布 (#0b0b0b/#212121) 上的可分辨性已逐色核对：四色全为中高明度，
 * 在 #212121 面板上有 ≥3:1 的描线对比。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const sanitySlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#f36458',        // 每日创建 MR：coral-red = 头牌曲线（每屏一次的品牌强调位）
      crDone: '#55beff',    // 每日完成 CR：link-blue-soft
      crStart: '#37cd84',   // 每日发起 CR：success 绿
      avgDuration: '#e6a23c', // 成功运行平均耗时：推导琥珀（耗时即警示；规格无 warning）
    },
    ratios: {
      success: '#37cd84',     // 当前任务成功占比：语义绿
      firstRun: '#55beff',    // 首次执行成功率：链接蓝
      intervention: '#dd0000', // 触发人工介入率：error（brand-deep）
      recovery: '#e6a23c',    // 受阻后恢复成功率：推导琥珀（恢复带警示余韵）
    },
  },
  timeline: {
    error: '#dd0000',   // failed / blocked / unavailable
    warning: '#e6a23c', // human_required / retry_pending / waiting_upstream（推导琥珀）
    success: '#37cd84', // succeeded / approved
    info: '#55beff',    // 其余默认 = 暗底链接蓝（coral 不进时间线，保持稀缺）
  },
  eventType: {
    error: '#dd0000',  // codex_error / claude_error
    usage: '#37cd84',  // codex_usage
    info: '#55beff',   // 其余默认
  },
  sparkline: {
    default: '#f36458',  // Sparkline 默认描线：coral-red（近黑画布上的单一品牌事件）
    hoverGrid: '#505b6c', // hover 竖向网格虚线：slate-soft（暗底可见的去强调规则线）
  },
}
