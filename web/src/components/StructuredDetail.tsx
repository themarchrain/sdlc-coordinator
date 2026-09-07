import type { ReactNode } from 'react'
import { Descriptions, Tag } from 'antd'

import type { EntityKey } from '../api/types'
import { JsonViewer } from './JsonViewer'
import { StatusTag } from './StatusTag'

type DataRecord = Record<string, unknown>
type DetailField = { label: string; value: unknown; wide?: boolean }

const jsonFieldsByEntity: Record<EntityKey, string[]> = {
  runs: ['input_json', 'output_json'],
  artifacts: ['metadata_json'],
  dependencies: ['metadata_json'],
  writebacks: ['request_json', 'response_json'],
  effects: ['request_json', 'response_json'],
  'inbound-events': ['payload_json', 'normalized_json'],
  'result-submissions': ['raw_payload_json', 'canonical_json'],
}

const fieldLabels: Record<string, string> = {
  artifact_id: 'Artifact ID', attempt_count: '尝试次数', body: '消息正文', canonical_json: '规范化结果', child_task_id: '下游 Task',
  completed_at: '完成时间', created_at: '创建时间', critical: '关键副作用', dedupe_key: '幂等键', dependency_id: 'Dependency ID',
  effect_id: 'Effect ID', error_message: '错误信息', event_action: '事件动作', event_id: 'Event ID', event_type: '事件类型',
  external_id: '外部 ID', external_system: '外部系统', external_url: '外部地址', kind: '类型', last_reconciled_at: '最近核对时间',
  lease_until: '租约截止', name: '名称', operation_marker: '操作标记', parent_task_id: '上游 Task', payload_hash: 'Payload Hash',
  format: '消息格式', mentions: '通知人员', project_id: '项目 ID', provider: '执行端', raw: '原始响应', received_at: '接收时间', reconcile_attempt_count: '核对次数', relation: '依赖关系', result: '执行结果', role: 'Worker 角色',
  run_id: 'Run ID', source: '来源', status: '状态', submission_id: 'Submission ID', target: '目标', target_iid: '目标 IID',
  target_type: '目标类型', task_id: 'Task ID', tool_name: '工具名称', updated_at: '更新时间', url: '地址', worker_role: 'Worker 角色',
  writeback_id: 'Writeback ID',
}

export function TaskContextDetail({ value }: { value: unknown }) {
  const context = parseRecord(value)
  if (!context) return <EmptyDetail text="任务未保存结构化上下文" />

  const issue = asRecord(context.issue)
  const mergeRequest = asRecord(context.merge_request)
  const originIssue = asRecord(context.origin_issue)
  const reworkRequest = asRecord(context.rework_request)
  const project = asRecord(context.project)
  const repos = asRecordArray(context.repos)
  const hasLineage = hasValue(context.parent_task_id) || hasValue(context.parent_run_id) || hasValue(context.next_task_index) || Boolean(asRecord(context.allowed_repos))

  return (
    <div className="structured-stack">
      {issue ? <WorkItemSection title="Issue 信息" item={issue} linkLabel="打开 Issue" /> : null}
      {mergeRequest ? <MergeRequestSection item={mergeRequest} /> : null}
      {originIssue ? <WorkItemSection title="原始 Issue" item={originIssue} linkLabel="打开原始 Issue" /> : null}
      {reworkRequest ? <ReworkSection request={reworkRequest} /> : null}
      {project ? <ProjectSection project={project} /> : null}
      {repos.length ? <RepositorySection repos={repos} /> : null}
      {hasLineage ? <DerivedTaskSection context={context} /> : null}
      {context.workspace_strategy ? (
        <DetailSection title="执行环境">
          <DetailGrid fields={[{ label: 'Workspace 策略', value: mono(context.workspace_strategy) }]} />
        </DetailSection>
      ) : null}
    </div>
  )
}

export function EntityDetail({ entity, detail }: { entity: EntityKey; detail: DataRecord }) {
  if (entity === 'runs') return <RunDetail detail={detail} />

  const jsonFields = jsonFieldsByEntity[entity]
  const summaryEntries = Object.entries(detail).filter(([key, value]) => !jsonFields.includes(key) && hasValue(value))

  return (
    <div className="drawer-stack">
      <DetailSection title="基础信息">
        <Descriptions bordered size="small" column={{ xs: 1, sm: 2 }} items={summaryEntries.map(([key, value]) => ({
          key,
          label: fieldLabels[key] ?? humanize(key),
          span: isLongValue(key, value) ? { xs: 1, sm: 2 } : 1,
          children: renderKnownValue(key, value),
        }))} />
      </DetailSection>
      {jsonFields.map((key) => hasValue(detail[key]) ? (
        <StructuredPayload key={key} title={payloadTitle(key)} value={detail[key]} />
      ) : null)}
      <AuditJson label={`原始${entityLabel(entity)}数据`} value={detail} />
    </div>
  )
}

export function AuditJson({ label, value }: { label: string; value: unknown }) {
  return (
    <details className="audit-json">
      <summary>{label}</summary>
      <JsonViewer label="JSON" value={value} />
    </details>
  )
}

function RunDetail({ detail }: { detail: DataRecord }) {
  const input = parseRecord(detail.input_json)
  const output = parseRecord(detail.output_json)
  const artifacts = asRecord(output?.artifacts)
  const repos = asRecordArray(input?.repos)
  const testResults = asArray(artifacts?.test_results)
  const changedFiles = asArray(artifacts?.changed_files)

  const common = [
    ['状态', <StatusTag status={String(detail.status ?? '')} />],
    ['Worker 角色', detail.worker_role],
    ['Task ID', mono(detail.task_id)],
    ['外部系统', detail.external_system],
    ['外部 ID', mono(detail.external_id)],
    ['开始时间', formatTime(detail.started_at)],
    ['完成时间', formatTime(detail.completed_at)],
    ['错误信息', detail.error_message],
  ] satisfies Array<[string, ReactNode | unknown]>

  return (
    <div className="drawer-stack">
      <DetailSection title="Run 概况">
        <Descriptions bordered size="small" column={{ xs: 1, sm: 2 }} items={common.filter(([, value]) => hasRenderableValue(value)).map(([label, value]) => ({
          key: label,
          label,
          span: label === '错误信息' ? { xs: 1, sm: 2 } : 1,
          children: value as ReactNode,
        }))} />
        <div className="detail-actions">
          {isURL(detail.external_url) ? <a href={String(detail.external_url)} target="_blank" rel="noreferrer">打开外部执行</a> : null}
        </div>
      </DetailSection>

      {input ? (
        <DetailSection title="执行输入">
          <DetailGrid fields={[
            { label: '任务类型', value: mono(input.type) },
            { label: 'Workspace 策略', value: mono(input.workspace_strategy) },
            { label: '目标', value: input.objective, wide: true },
            { label: '成功标准', value: listText(input.success_criteria), wide: true },
            { label: '回调地址', value: link(input.callback_url), wide: true },
          ]} />
          {repos.length ? <RepositoryList repos={repos} /> : null}
        </DetailSection>
      ) : null}

      {output ? (
        <DetailSection title="执行结果">
          <DetailGrid fields={[
            { label: '结果状态', value: output.status ? <StatusTag status={String(output.status)} /> : undefined },
            { label: '审查结论', value: mono(artifacts?.review_verdict) },
            { label: '交付分支', value: mono(artifacts?.delivery_branch) },
            { label: 'Commit SHA', value: mono(artifacts?.commit_sha) },
            { label: '结果摘要', value: output.summary, wide: true },
            { label: '审查测试摘要', value: artifacts?.review_test_summary, wide: true },
          ]} />
          {changedFiles.length ? <ValueList title="变更文件" values={changedFiles} monoValues /> : null}
          {testResults.length ? <TestResultList values={testResults} /> : null}
          {asArray(artifacts?.review_findings).length ? <ValueList title="审查发现" values={asArray(artifacts?.review_findings)} /> : null}
          {asRecordArray(output.writebacks).length ? <IntentList title="写回意图" values={asRecordArray(output.writebacks)} /> : null}
          {asRecordArray(output.effects).length ? <IntentList title="副作用意图" values={asRecordArray(output.effects)} /> : null}
        </DetailSection>
      ) : null}

      <AuditJson label="原始 Run 数据" value={detail} />
    </div>
  )
}

function WorkItemSection({ title, item, linkLabel }: { title: string; item: DataRecord; linkLabel: string }) {
  return (
    <DetailSection title={title}>
      <DetailGrid fields={[
        { label: 'IID', value: mono(item.iid) },
        { label: '标题', value: item.title },
        { label: '描述', value: item.description, wide: true },
        { label: '目标', value: item.objective, wide: true },
        { label: '验收标准', value: item.acceptance_criteria, wide: true },
        { label: '负责人', value: people(item.assignees) },
      ]} />
      {isURL(item.url) ? <div className="detail-actions"><a href={String(item.url)} target="_blank" rel="noreferrer">{linkLabel}</a></div> : null}
    </DetailSection>
  )
}

function MergeRequestSection({ item }: { item: DataRecord }) {
  return (
    <DetailSection title="MR 信息">
      <DetailGrid fields={[
        { label: 'IID', value: mono(item.iid) },
        { label: '标题', value: item.title },
        { label: '状态', value: item.state ? <StatusTag status={String(item.state)} /> : undefined },
        { label: '审查模式', value: mono(item.review_mode) },
        { label: '源分支', value: mono(item.source_branch) },
        { label: '目标分支', value: mono(item.target_branch) },
        { label: 'Head SHA', value: mono(item.head_sha ?? item.expected_head_sha) },
        { label: 'Base SHA', value: mono(item.base_sha) },
        { label: '作者', value: person(item.author) },
        { label: '描述', value: item.description, wide: true },
      ]} />
      {isURL(item.url) ? <div className="detail-actions"><a href={String(item.url)} target="_blank" rel="noreferrer">打开 MR</a></div> : null}
    </DetailSection>
  )
}

function ReworkSection({ request }: { request: DataRecord }) {
  return (
    <DetailSection title="接续开发请求">
      <DetailGrid fields={[
        { label: '人工要求', value: request.context, wide: true },
        { label: '发起人', value: person(request.requester) },
        { label: 'Discussion ID', value: mono(request.discussion_id) },
        { label: 'Note ID', value: mono(request.note_id) },
        { label: '评论地址', value: link(request.note_url, '打开评论') },
      ]} />
    </DetailSection>
  )
}

function ProjectSection({ project }: { project: DataRecord }) {
  return (
    <DetailSection title="项目">
      <DetailGrid fields={[
        { label: '项目路径', value: mono(project.path) },
        { label: '项目 ID', value: mono(project.id) },
      ]} />
    </DetailSection>
  )
}

function DerivedTaskSection({ context }: { context: DataRecord }) {
  return (
    <DetailSection title="派生任务信息">
      <DetailGrid fields={[
        { label: '上游 Task', value: mono(context.parent_task_id) },
        { label: '上游 Run', value: mono(context.parent_run_id) },
        { label: '任务序号', value: mono(context.next_task_index) },
        { label: '仓库权限范围', value: repositoryScope(context.allowed_repos), wide: true },
      ]} />
    </DetailSection>
  )
}

function RepositorySection({ repos }: { repos: DataRecord[] }) {
  return <DetailSection title="代码仓库"><RepositoryList repos={repos} /></DetailSection>
}

function RepositoryList({ repos }: { repos: DataRecord[] }) {
  return (
    <div className="repo-list">
      {repos.map((repo, index) => (
        <div className="repo-row" key={`${String(repo.name ?? repo.url ?? 'repo')}-${index}`}>
          <div><strong>{String(repo.name ?? '未命名仓库')}</strong><span>{link(repo.url)}</span></div>
          <div><span className="mono">{String(repo.branch ?? '—')}</span><Tag>{String(repo.access ?? 'unknown')}</Tag></div>
        </div>
      ))}
    </div>
  )
}

function StructuredPayload({ title, value }: { title: string; value: unknown }) {
  const parsed = parseMaybeJSON(value)
  if (!asRecord(parsed)) return <DetailSection title={title}><div className="detail-empty">{String(parsed ?? '—')}</div></DetailSection>
  return <DetailSection title={title}><GenericRecord record={parsed as DataRecord} /></DetailSection>
}

function GenericRecord({ record }: { record: DataRecord }) {
  return (
    <div className="generic-record">
      {Object.entries(record).map(([key, value]) => {
        const nested = asRecord(value)
        if (nested) return <div className="generic-record__group" key={key}><strong>{fieldLabels[key] ?? humanize(key)}</strong><GenericRecord record={nested} /></div>
        if (Array.isArray(value)) return <ValueList key={key} title={fieldLabels[key] ?? humanize(key)} values={value} />
        return <DetailGrid key={key} fields={[{ label: fieldLabels[key] ?? humanize(key), value: renderKnownValue(key, value) }]} />
      })}
    </div>
  )
}

function DetailSection({ title, children }: { title: string; children: ReactNode }) {
  return <section className="detail-section"><h3>{title}</h3>{children}</section>
}

function DetailGrid({ fields }: { fields: DetailField[] }) {
  const visible = fields.filter((field) => hasRenderableValue(field.value))
  if (!visible.length) return <EmptyDetail text="暂无可展示字段" />
  return (
    <dl className="detail-grid">
      {visible.map((field) => <div className={field.wide ? 'detail-grid__wide' : undefined} key={field.label}><dt>{field.label}</dt><dd>{field.value as ReactNode}</dd></div>)}
    </dl>
  )
}

function ValueList({ title, values, monoValues = false }: { title: string; values: unknown[]; monoValues?: boolean }) {
  return (
    <div className="value-list">
      <div className="value-list__title">{title}</div>
      <ul>{values.map((value, index) => <li className={monoValues ? 'mono' : ''} key={index}>{renderListValue(value)}</li>)}</ul>
    </div>
  )
}

function TestResultList({ values }: { values: unknown[] }) {
  return (
    <div className="value-list">
      <div className="value-list__title">测试结果</div>
      <div className="test-result-list">{values.map((value, index) => {
        const result = asRecord(value)
        if (!result) return <div key={index}>{String(value)}</div>
        return <div className="test-result" key={index}><div><strong className="mono">{String(result.command ?? `测试 ${index + 1}`)}</strong>{typeof result.passed === 'boolean' ? <StatusTag status={result.passed ? 'succeeded' : 'failed'} /> : null}</div><p>{String(result.summary ?? result.output ?? '—')}</p></div>
      })}</div>
    </div>
  )
}

function IntentList({ title, values }: { title: string; values: DataRecord[] }) {
  return (
    <div className="value-list">
      <div className="value-list__title">{title}</div>
      <div className="intent-list">{values.map((value, index) => (
        <div className="intent-row" key={`${String(value.kind ?? 'intent')}-${index}`}>
          <strong>{String(value.kind ?? '未标记类型')}</strong>
          <span>{String(value.project_id ?? value.target ?? value.dedupe_key ?? '—')}</span>
        </div>
      ))}</div>
    </div>
  )
}

function EmptyDetail({ text }: { text: string }) {
  return <div className="detail-empty">{text}</div>
}

function parseRecord(value: unknown): DataRecord | undefined {
  return asRecord(parseMaybeJSON(value))
}

function parseMaybeJSON(value: unknown): unknown {
  if (typeof value !== 'string') return value
  const trimmed = value.trim()
  if (!trimmed) return undefined
  try { return JSON.parse(trimmed) } catch { return value }
}

function asRecord(value: unknown): DataRecord | undefined {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as DataRecord : undefined
}

function asRecordArray(value: unknown): DataRecord[] {
  return Array.isArray(value) ? value.map(asRecord).filter((item): item is DataRecord => Boolean(item)) : []
}

function asArray(value: unknown): unknown[] {
  return Array.isArray(value) ? value : []
}

function people(value: unknown): ReactNode {
  const values = asArray(value).map(person).filter(Boolean)
  return values.length ? <div className="people-list">{values.map((value, index) => <Tag key={index}>{value}</Tag>)}</div> : undefined
}

function person(value: unknown): string | undefined {
  const entry = asRecord(value)
  if (!entry) return typeof value === 'string' ? value : undefined
  const name = String(entry.name ?? '').trim()
  const username = String(entry.username ?? '').trim()
  const email = String(entry.email ?? '').trim()
  const identity = name && username && name !== username ? `${name}（${username}）` : username || name || email || String(entry.id ?? '')
  if (!identity) return undefined
  return email && email !== identity ? `${identity} · ${email}` : identity
}

function repositoryScope(value: unknown): ReactNode {
  const scope = asRecord(value)
  if (!scope) return undefined
  const writable = asArray(scope.writable).map(String)
  const readable = asArray(scope.readable).map(String)
  if (!writable.length && !readable.length) return undefined
  return <div className="scope-list">{writable.map((repo) => <span key={`w-${repo}`}><Tag color="processing">可写</Tag><span className="mono">{repo}</span></span>)}{readable.map((repo) => <span key={`r-${repo}`}><Tag>只读</Tag><span className="mono">{repo}</span></span>)}</div>
}

function mono(value: unknown): ReactNode {
  return hasValue(value) ? <span className="mono detail-break">{String(value)}</span> : undefined
}

function link(value: unknown, label?: string): ReactNode {
  return isURL(value) ? <a href={String(value)} target="_blank" rel="noreferrer">{label ?? String(value)}</a> : undefined
}

function listText(value: unknown): ReactNode {
  const values = asArray(value)
  return values.length ? values.map(String).join('；') : undefined
}

function renderKnownValue(key: string, value: unknown): ReactNode {
  if (!hasValue(value)) return '—'
  if (key === 'status') return <StatusTag status={String(value)} />
  if (key.endsWith('_at')) return formatTime(value)
  if (key === 'external_url' || key === 'url') return link(value)
  if (typeof value === 'boolean') return value ? '是' : '否'
  if (key.endsWith('_id') || key === 'dedupe_key' || key === 'payload_hash' || key === 'operation_marker') return mono(value)
  return String(value)
}

function renderListValue(value: unknown): ReactNode {
  if (isURL(value)) return link(value)
  if (asRecord(value)) return Object.entries(value as DataRecord).map(([key, child]) => `${fieldLabels[key] ?? humanize(key)}: ${String(child ?? '—')}`).join('；')
  return String(value ?? '—')
}

function hasValue(value: unknown) {
  return value !== undefined && value !== null && value !== ''
}

function hasRenderableValue(value: unknown) {
  return value !== undefined && value !== null && value !== '' && value !== false
}

function isURL(value: unknown) {
  return typeof value === 'string' && /^https?:\/\//i.test(value)
}

function isLongValue(key: string, value: unknown) {
  return ['error_message', 'dedupe_key', 'external_url', 'operation_marker'].includes(key) || String(value ?? '').length > 64
}

function formatTime(value: unknown) {
  if (!value) return '—'
  return new Date(String(value)).toLocaleString('zh-CN', { hour12: false })
}

function humanize(value: string) {
  return value.replaceAll('_', ' ').replace(/\b\w/g, (letter) => letter.toUpperCase())
}

function payloadTitle(key: string) {
  return ({
    metadata_json: '元数据', request_json: '请求内容', response_json: '响应内容', payload_json: '原始事件 Payload',
    normalized_json: '规范化事件', raw_payload_json: '原始回传 Payload', canonical_json: '规范化回传结果',
  } as Record<string, string>)[key] ?? humanize(key)
}

function entityLabel(entity: EntityKey) {
  return ({
    runs: ' Run ', artifacts: ' Artifact ', dependencies: ' Dependency ', writebacks: ' Writeback ', effects: ' Effect ',
    'inbound-events': ' Inbound Event ', 'result-submissions': ' Result Submission ',
  } as Record<EntityKey, string>)[entity]
}
