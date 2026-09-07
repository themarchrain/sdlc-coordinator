import type { ThemeSlots } from '../types'

/**
 * cursor 主题的语义色槽位 —— 自 design/cursor 各页面/组件的硬编码誊抄。
 * 取值规则（剩余 hex 清点）：
 *   - Overview 计数趋势（trends 四条）一律 ink #26251e：黑白 chrome，
 *     数据面板不引入彩色；
 *   - 占比卡（ratios）只在语义绿 #1f8a65 / 语义红 #cf2d56 之间取值；
 *   - TasksPage 生命周期时间线 = agent 时间线语义 hex：
 *     错误 #cf2d56、等待类 #dfa88f（桃 = timeline-thinking）、
 *     成功 #c08532（金 = timeline-done）、进行中 #9fbbe0（蓝 = timeline-read）；
 *   - Codex 事件时间线：错误 #cf2d56、usage #9fc9a2（薄荷 = timeline-grep）、
 *     其余 #9fbbe0；
 *   - Sparkline 默认描线 ink #26251e，hover 网格虚线 hairline-strong #cfcdc4。
 * 粉彩仅限 agent 执行时间线，绝不当通用动作色/状态色。
 * 键结构与 default/slots.ts 保持一致，只改值。
 */
export const cursorSlots: ThemeSlots = {
  chart: {
    trends: {
      mr: '#26251e',          // OverviewPage 已创建 MR
      crDone: '#26251e',      // OverviewPage 已完成 CR
      crStart: '#26251e',     // OverviewPage 已发起 CR
      avgDuration: '#26251e', // OverviewPage 成功运行平均耗时
    },
    ratios: {
      success: '#1f8a65',      // 当前任务成功占比
      firstRun: '#1f8a65',     // 首次执行成功率
      intervention: '#cf2d56', // 触发人工介入率
      recovery: '#1f8a65',     // 受阻后恢复成功率
    },
  },
  timeline: {
    error: '#cf2d56',   // failed / blocked / unavailable
    warning: '#dfa88f', // human_required / retry_pending / waiting_upstream（桃 = thinking）
    success: '#c08532', // succeeded / approved（金 = done）
    info: '#9fbbe0',    // 其余默认节点（蓝 = read）
  },
  eventType: {
    error: '#cf2d56',  // codex_error / claude_error
    usage: '#9fc9a2',  // codex_usage（薄荷 = grep）
    info: '#9fbbe0',   // 其余默认节点（蓝 = read）
  },
  sparkline: {
    default: '#26251e',  // Sparkline 默认描线 = ink
    hoverGrid: '#cfcdc4', // hover 竖向网格虚线 = hairline-strong
  },
}
