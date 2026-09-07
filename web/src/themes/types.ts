import type { ThemeConfig } from 'antd'

/**
 * 内置主题系统 · Phase 1 核心骨架的类型契约。
 *
 * 一套主题由三个机制拼成：
 *  1. CSS 变量层 —— web/src/themes/css/<id>.css 提供 [data-theme="<id>"] 令牌块，
 *     骨架样式（styles/index.css）只消费 var()；
 *  2. antd 主题对象 —— 本文件 ThemeDefinition.antd，喂给 ConfigProvider theme；
 *  3. 语义色槽位 —— ThemeDefinition.slots，覆盖组件 TSX 里的图表/时间线等取色。
 *
 * 约束：antd 主色数值派生禁用 var()（antd 需要对 colorPrimary 做算法派生，
 * CSS 变量无法参与计算），antd.ts 里必须写字面 hex，且与 CSS 令牌手工同源。
 */

/** 主题卡片上的五色示意条（外观面板展示用）。 */
export interface ThemeSwatches {
  /** 画布底色 */
  canvas: string
  /** 卡片表面色 */
  surface: string
  /** 主文字色 */
  ink: string
  /** 主操作色 */
  primary: string
  /** 辅助强调色（各主题自定的 accent） */
  accent: string
}

/** OverviewPage 八条趋势线的取色（Sparkline stroke）。 */
export interface ChartTrendSlots {
  /** 每日创建 MR 趋势 */
  mr: string
  /** 每日完成 CR 趋势 */
  crDone: string
  /** 每日发起 CR 趋势 */
  crStart: string
  /** 每日成功运行平均耗时趋势 */
  avgDuration: string
}

/** OverviewPage 四张占比卡的取色（Sparkline stroke）。 */
export interface ChartRatioSlots {
  /** 当前任务成功占比 */
  success: string
  /** 首次执行成功率 */
  firstRun: string
  /** 触发人工介入率 */
  intervention: string
  /** 受阻后恢复成功率 */
  recovery: string
}

/** TasksPage 生命周期时间线的节点色（antd Timeline color，接受 preset 名或色值）。 */
export interface TimelineSlots {
  /** 失败 / 阻塞 / 不可用 */
  error: string
  /** 需人工 / 等待重试 / 等待上游 */
  warning: string
  /** 成功 / 已通过 */
  success: string
  /** 其余默认节点 */
  info: string
}

/** CodexObservabilityPage 执行事件时间线的节点色（antd Timeline color）。 */
export interface EventTypeSlots {
  /** codex_error / claude_error */
  error: string
  /** codex_usage */
  usage: string
  /** 其余默认节点 */
  info: string
}

/** Sparkline 的默认描线色与 hover 网格虚线色。 */
export interface SparklineSlots {
  default: string
  hoverGrid: string
}

/** 组件 TSX 消费语义色槽位（与 CSS/antd 两层正交）。 */
export interface ThemeSlots {
  chart: {
    trends: ChartTrendSlots
    ratios: ChartRatioSlots
  }
  timeline: TimelineSlots
  eventType: EventTypeSlots
  sparkline: SparklineSlots
}

/** 单套内置主题的完整定义。 */
export interface ThemeDefinition {
  /** 主题 id，同时是 [data-theme="<id>"] 与 localStorage 存储值 */
  id: string
  /** 外观面板展示名 */
  label: string
  /** 是否暗色主题（影响卡片徽标与首屏防白闪底色） */
  isDark: boolean
  /** 浏览器 chrome 预设色（meta[name=theme-color] 与首屏底色） */
  chromeColor: string
  /** 外观面板五色示意条 */
  swatches: ThemeSwatches
  /** antd ConfigProvider theme 对象（字面 hex，禁 var()） */
  antd: ThemeConfig
  /** 组件语义色槽位 */
  slots: ThemeSlots
}
