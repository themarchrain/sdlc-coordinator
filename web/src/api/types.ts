export interface Page<T> {
  items: T[]
  page: number
  page_size: number
  total: number
}

export interface Overview {
  tasks_by_status: Record<string, number>
  runs_by_status: Record<string, number>
  writebacks_by_status: Record<string, number>
  effects_by_status: Record<string, number>
  recent_failures: Array<Record<string, unknown>>
  business_summary: BusinessSummary
  generated_at: string
}

export interface BusinessSummary {
  window: '7d' | '30d' | 'all'
  merge_requests_created: number
  reviews_completed: number
  reviewer_takeovers: number
  automation_success: RatioMetric
  average_successful_run_seconds: number | null
  human_required_tasks: number
  human_required_added_today: number
  daily_merge_requests_created: DailyCountPoint[]
  daily_reviews_completed: DailyCountPoint[]
  daily_reviewer_takeovers: DailyCountPoint[]
  daily_average_run_seconds: DailyAveragePoint[]
  generated_at: string
}

export interface DailyCountPoint {
  date: string
  count: number
}

export interface DailyAveragePoint {
  date: string
  seconds: number
  sample_count: number
}

export interface DailyRatioPoint {
  date: string
  current_success: number | null
  first_run_success: number | null
  intervention_triggered: number | null
  recovered_after_block: number | null
}

export interface RatioMetric {
  numerator: number
  denominator: number
  value: number | null
}

export interface RunDurationMetric {
  worker_role: string
  sample_count: number
  average_seconds: number
  p50_seconds: number
  p90_seconds: number
}

export interface AutomationMetrics {
  window: '7d' | '30d' | 'all'
  type: string
  task_sample_count: number
  current_success: RatioMetric
  first_run_success: RatioMetric
  intervention_triggered: RatioMetric
  recovered_after_block: RatioMetric
  successful_run_durations: RunDurationMetric[]
  daily_series: DailyRatioPoint[]
  generated_at: string
}

export interface TaskSummary extends Record<string, unknown> {
  task_id: string
  type: string
  role: string
  status: string
  source_repo: string
  target_repo: string
  current_run_id?: string
  last_error?: string
  created_at: string
  updated_at: string
}

export interface TimelineEvent {
  event_id: string
  occurred_at: string
  category: string
  action: string
  status: string
  title: string
  summary: string
  entity_type: string
  entity_id: string
}

export interface Connector extends Record<string, unknown> {
  connector_id: string
  kind: string
  display_name: string
  base_url: string
  enabled: boolean
  target_project_path: string
  capabilities: string[]
  last_health_status: string
  last_health_message: string
  last_health_at?: string
  created_at: string
  updated_at: string
}

export interface ConnectorRun extends Record<string, unknown> {
  run_id: string
  trigger: string
  status: string
  started_at?: string
  completed_at?: string
  discovered: number
  created: number
  updated: number
  skipped: number
  failed: number
  error?: string
}

export interface ConnectorRecord extends Record<string, unknown> {
  record_id: string
  source_type: string
  source_id: string
  source_title?: string
  target_type?: string
  target_id?: string
  target_url?: string
  status: string
  last_synced_at?: string
  metadata?: Record<string, unknown>
}

export interface ConnectorMetrics {
  generated_at?: string
  values: Record<string, number>
  metadata?: Record<string, unknown>
}

export interface TickRequest {
  dispatch_limit?: number
  poll_limit?: number
  reconciliation_limit?: number
  finalization_limit?: number
  recovery_limit?: number
  effect_limit?: number
}

export interface TickResult extends Record<string, unknown> {
  polled_runs: Array<Record<string, unknown>>
  reconciled_writebacks: Array<Record<string, unknown>>
  finalized_results: Array<Record<string, unknown>>
  recoveries: Array<Record<string, unknown>>
  dispatched_tasks: Array<Record<string, unknown>>
  dispatch_failures: Array<Record<string, unknown>>
  executed_effects: Array<Record<string, unknown>>
}

export interface AgentExecutionSummary {
  total_executions: number
  status_counts: Record<string, number>
  average_duration_seconds: number
  p50_duration_seconds: number
  p90_duration_seconds: number
  input_tokens: number
  cached_input_tokens: number
  output_tokens: number
  cache_hit_ratio: number | null
  turn_count: number
  error_count: number
  timeout_count: number
  generated_at: string
}

export interface AgentExecution {
  execution_id: string
  run_id: string
  task_id: string
  task_type: string
  role: string
  repository: string
  provider: string
  status: string
  model: string
  effort: string
  error_code?: string
  error_message?: string
  request_hash: string
  result_hash?: string
  started_at?: string
  finished_at?: string
  created_at: string
  duration_seconds?: number
}

export interface AgentExecutionEvent {
  seq: number
  type: string
  occurred_at: string
  payload: Record<string, unknown>
}

export interface AgentExecutionDetail extends AgentExecution {
  events: AgentExecutionEvent[]
}

export type EntityKey =
  | 'runs'
  | 'artifacts'
  | 'dependencies'
  | 'writebacks'
  | 'effects'
  | 'inbound-events'
  | 'result-submissions'

export type QueryValue = string | number | boolean | null | undefined
export type Query = Record<string, QueryValue>

export interface RegisteredRepository {
  repository_id: string
  display_name: string
  provider: string
  base_url: string
  codehost_project_id: string
  path_with_namespace: string
  token_configured: boolean
  webhook_secret_configured: boolean
  codehost_issue_labels?: string[]
  default_branch: string
  status: string
  last_configuration_error: string
}

export interface RepositoryCreateInput {
  display_name: string
  path_with_namespace: string
  provider: string
  base_url: string
  token: string
  token_scope_attested: boolean
  webhook_secret: string
  codehost_issue_labels?: string[]
  reviewer_mentions?: string
}

export interface RepositoryPatchInput {
  display_name?: string
  path_with_namespace?: string
  provider?: string
  base_url?: string
  token?: string
  token_scope_attested?: boolean
  webhook_secret?: string
  codehost_issue_labels?: string[]
  reviewer_mentions?: string
}

export type ConfigurationMode = 'setup_pending' | 'ready' | 'restart_required' | 'configuration_error'
export type ConfigurationSource = 'ui' | 'env_import'

export interface ConfigurationStatus {
  mode: ConfigurationMode
  revision: number
  source?: ConfigurationSource
  applied_at?: string | null
  configuration_error?: string
}

// 连接器配置为插件自定义的不透明 JSON,由连接器自身校验与脱敏。
export type ConnectorConfigurationSettings = Record<string, unknown>

export interface ConnectorConfiguration {
  mode: 'setup_pending' | 'ready'
  version: number
  source?: string
  restart_required: boolean
  settings: ConnectorConfigurationSettings
  secret_fields?: string[]
}

export interface ConnectorConfigurationUpdate {
  settings: ConnectorConfigurationSettings
  preserve_secrets?: string[]
}

export interface EditableRuntimeConfiguration {
  agent_runner: {
    service_url: string
    workspace_root: string
    provider: 'codex-cli' | 'claude-cli'
    provider_base_url: string
    model: string
    effort: 'low' | 'medium' | 'high' | 'xhigh' | 'max'
    task_timeout: string
  }
  codehost: {
    provider: 'gitlab' | 'gitea' | 'github'
    base_url: string
    rework_allowed_user_ids: string[]
  }
  im: {
    provider: '' | 'none' | 'telegram' | 'feishu'
    telegram: {
      api_base?: string
      default_chat_id?: string
      mention_map?: Record<string, string>
    }
    feishu: {
      webhook_url?: string
      mention_map?: Record<string, string>
    }
    reviewer_mentions: string[]
  }
  scheduler: {
    enabled: boolean
    interval: string
    callback_base_url: string
    workspace_root: string
    dispatch_limit: number
    poll_limit: number
    reconciliation_limit: number
    finalization_limit: number
    recovery_limit: number
    effect_limit: number
  }
}

export interface ConfigurationSecretStatus {
  agent_runner_api_key_configured: boolean
  agent_runner_custom_headers_configured: boolean
  telegram_bot_token_configured: boolean
  feishu_secret_configured: boolean
}

export interface ConfigurationView {
  config: EditableRuntimeConfiguration
  secrets: ConfigurationSecretStatus
}

export interface ConfigurationSecretChanges {
  agent_runner_api_key?: string
  agent_runner_custom_headers_json?: string
  telegram_bot_token?: string
  feishu_secret?: string
  clear_agent_runner_api_key?: boolean
  clear_agent_runner_custom_headers_json?: boolean
  clear_telegram_bot_token?: boolean
  clear_feishu_secret?: boolean
}

export interface ConfigurationUpdateRequest {
  config: EditableRuntimeConfiguration
  secrets?: ConfigurationSecretChanges
}

export interface ConfigurationValidationResult {
  valid: boolean
  field_errors: Record<string, string>
}

export interface ConfigurationUpdateResult extends ConfigurationStatus {
  configuration: ConfigurationView
}

export interface CodeHostIdentity {
  identity_id: string
  provider: 'gitlab' | 'gitea' | 'github'
  base_url: string
  external_user_id: string
  username: string
  created_at?: string
  updated_at?: string
}

export interface Person {
  person_id: string
  display_name: string
  email: string
  enabled: boolean
  identities: CodeHostIdentity[]
  created_at: string
  updated_at: string
}

export interface PersonInput {
  display_name: string
  email: string
  enabled: boolean
  identities: Array<Omit<CodeHostIdentity, 'identity_id' | 'created_at' | 'updated_at'> & { identity_id?: string }>
}

export interface IdentityLookupRequest {
  provider: 'gitlab'
  base_url: string
  token: string
  email: string
  username?: string
}

export interface IdentityLookupCandidate {
  external_user_id: string
  username: string
  display_name: string
}

export interface IdentityLookupResult {
  candidates: IdentityLookupCandidate[]
  exact_match_index: number | null
}

export interface BatchIdentityLookupRequest {
  provider: 'gitlab'
  base_url: string
  token: string
}

export interface BatchIdentityLookupItem {
  person_id: string
  display_name: string
  email: string
  identity_id: string
  candidates: IdentityLookupCandidate[]
  exact_match_index: number | null
  error_code?: string
}

export interface BatchIdentityLookupResult {
  items: BatchIdentityLookupItem[]
}

export type RequirementStatus = 'queued' | 'published' | 'done' | 'failed' | 'cancelled'

export interface Requirement {
  requirement_id: string
  title: string
  description: string
  status: RequirementStatus
  repository_id: string
  labels: string[]
  priority: '' | 'high' | 'medium' | 'low'
  issue_iid: number
  issue_url: string
  last_error: string
  published_at: string | null
  completed_at: string | null
  created_at: string
  updated_at: string
}

export interface RequirementInput {
  title: string
  description: string
  repository_id: string
  labels: string[]
  priority: string
}

export interface RequirementSyncResult {
  created: number
  recovered: number
  failed: number
  skipped: number
  mirrored: number
  mirror_errors: number
}
