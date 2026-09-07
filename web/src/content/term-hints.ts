/**
 * 术语培训文案字典：集中维护各页面的 ⓘ 术语解释（TermHint 组件用）。
 *
 * 接入方式（任意页面）：
 *   import { TermHint } from '../components/TermHint'
 *   import { termHints } from '../content/term-hints'
 *
 *   {termHints['运行记录'] ? <TermHint title="运行记录" content={termHints['运行记录']} /> : null}
 *
 * - 键为术语名称（与页面展示文字一致），值为简明解释，可用 \n 换行。
 * - 文案缺失时页面侧不渲染 ⓘ（各页面独立判断，互不影响）。
 */
export const termHints: Record<string, string> = {
  角色: 'DeveloperWorker：执行开发类任务（Issue 开发、MR 接续开发等），修改代码并创建 MR。\nReviewerWorker：对 MR 进行代码审查，给出审查结论与改进建议。',
  运行记录: '一次运行 = 一次 Worker 执行，对应一个 Agent Runner Execution。\n一般情况下一个任务只执行一次；失败后重试，一个任务可能对应多次运行，以最新一次为准。',
  交付产物: '任务产出的提交、MR、审查结论等结构化产物，供下游核对与追溯。',
  任务依赖: '任务之间的上下游关系：上游未就绪时会阻塞下游启动。',
  外部写回: 'Coordinator 对 GitLab 等外部系统的确定性写操作：创建 MR、打标签、写评论等，并核对执行结果。',
  通知与副作用: 'IM 通知等非核心副作用，失败不影响任务主流程，可单独重试。',
  入站事件: 'GitLab Webhook 等外部事件进入 Coordinator 后的接收、规范化与处理记录。',
  结果回传: 'Worker 经 Agent Runner 回传结果后，Coordinator 的校验与落库记录。',
}
