import type {
  AutomationMetrics,
  AuthSession,
  BatchIdentityLookupRequest,
  BatchIdentityLookupResult,
  Connector,
  ConnectorMetrics,
  ConnectorConfiguration,
  ConnectorConfigurationUpdate,
  ConnectorRecord,
  ConnectorRun,
  CodexExecution,
  CodexExecutionDetail,
  CodexObservabilitySummary,
  ConfigurationStatus,
  ConfigurationUpdateRequest,
  ConfigurationUpdateResult,
  ConfigurationValidationResult,
  ConfigurationView,
  EntityKey,
  Overview,
  Page,
  Person,
  PersonInput,
  IdentityLookupRequest,
  IdentityLookupResult,
  LoginRequest,
  Query,
  RegisteredRepository,
  RepositoryCreateInput,
  RepositoryPatchInput,
  Requirement,
  RequirementInput,
  RequirementSyncResult,
  TaskSummary,
  TickRequest,
  TickResult,
  TimelineEvent,
} from './types'

interface AdminErrorEnvelope {
  error?: {
    code?: string
    message?: string
    request_id?: string
    details?: unknown
  }
  details?: string
}

export class AdminApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string,
    public readonly details?: unknown,
  ) {
    super(message)
    this.name = 'AdminApiError'
  }
}

const entityPaths: Record<EntityKey, string> = {
  runs: 'runs',
  artifacts: 'artifacts',
  dependencies: 'dependencies',
  writebacks: 'writebacks',
  effects: 'effects',
  'inbound-events': 'inbound-events',
  'result-submissions': 'result-submissions',
}

export function queryString(query: Query = {}): string {
  const params = new URLSearchParams()
  Object.entries(query).forEach(([key, value]) => {
    if (value !== undefined && value !== null && value !== '') {
      params.set(key, String(value))
    }
  })
  const encoded = params.toString()
  return encoded ? `?${encoded}` : ''
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  headers.set('Accept', 'application/json')
  if (init.body !== undefined) headers.set('Content-Type', 'application/json')
  const response = await fetch(path, { ...init, headers })
  const text = await response.text()
  let payload: unknown = null
  if (text) {
    try {
      payload = JSON.parse(text)
    } catch {
      payload = { error: { code: 'invalid_response', message: text } }
    }
  }
  if (!response.ok) {
    const envelope = (payload ?? {}) as AdminErrorEnvelope
    throw new AdminApiError(
      response.status,
      envelope.error?.code ?? 'request_failed',
      envelope.error?.message ?? envelope.details ?? `${response.status} ${response.statusText}`,
      envelope.error?.details,
    )
  }
  return payload as T
}

function jsonBody(value: unknown): string {
  return JSON.stringify(value)
}

export const adminApi = {
	login: (value: LoginRequest) => request<AuthSession>('/api/auth/login', { method: 'POST', body: jsonBody(value) }),
	session: (signal?: AbortSignal) => request<AuthSession>('/api/auth/session', { signal }),

	configurationStatus: (signal?: AbortSignal) => request<ConfigurationStatus>('/api/admin/configuration/status', { signal }),
	configuration: (signal?: AbortSignal) => request<ConfigurationView>('/api/admin/configuration', { signal }),
	validateConfiguration: (value: ConfigurationUpdateRequest) => request<ConfigurationValidationResult>('/api/admin/configuration/validate', { method: 'POST', body: jsonBody(value) }),
	updateConfiguration: (value: ConfigurationUpdateRequest) => request<ConfigurationUpdateResult>('/api/admin/configuration', { method: 'PUT', body: jsonBody(value) }),
	restartConfiguration: () => request<{ status: string }>('/api/admin/configuration/restart', { method: 'POST', body: jsonBody({}) }),

  listPeople: (query: Query = {}, signal?: AbortSignal) => request<Page<Person>>(`/api/admin/people${queryString(query)}`, { signal }),
  getPerson: (personID: string, signal?: AbortSignal) => request<Person>(`/api/admin/people/${encodeURIComponent(personID)}`, { signal }),
  createPerson: (value: PersonInput) => request<Person>('/api/admin/people', { method: 'POST', body: jsonBody(value) }),
  updatePerson: (personID: string, value: PersonInput) => request<Person>(`/api/admin/people/${encodeURIComponent(personID)}`, { method: 'PATCH', body: jsonBody(value) }),
  deletePerson: (personID: string) => request<void>(`/api/admin/people/${encodeURIComponent(personID)}`, { method: 'DELETE' }),
  lookupCodeHostIdentity: (value: IdentityLookupRequest) => request<IdentityLookupResult>('/api/admin/people/codehost-identities/lookup', { method: 'POST', body: jsonBody(value) }),
  lookupCodeHostIdentities: (value: BatchIdentityLookupRequest) => request<BatchIdentityLookupResult>('/api/admin/people/codehost-identities/lookup-batch', { method: 'POST', body: jsonBody(value) }),

  overview: (query: Query = {}, signal?: AbortSignal) => request<Overview>(`/api/admin/overview${queryString(query)}`, { signal }),
  metrics: (query: Query = {}, signal?: AbortSignal) => request<AutomationMetrics>(`/api/admin/metrics${queryString(query)}`, { signal }),
	  codexSummary: (query: Query = {}, signal?: AbortSignal) => request<CodexObservabilitySummary>(`/api/admin/agents/codex/summary${queryString(query)}`, { signal }),
	  listCodexExecutions: (query: Query = {}, signal?: AbortSignal) => request<Page<CodexExecution>>(`/api/admin/agents/codex/executions${queryString(query)}`, { signal }),
	  getCodexExecution: (executionID: string, signal?: AbortSignal) => request<CodexExecutionDetail>(`/api/admin/agents/codex/executions/${encodeURIComponent(executionID)}`, { signal }),
  listTasks: (query: Query, signal?: AbortSignal) => request<Page<TaskSummary>>(`/api/admin/tasks${queryString(query)}`, { signal }),
  getTask: (taskID: string, signal?: AbortSignal) => request<Record<string, unknown>>(`/api/admin/tasks/${encodeURIComponent(taskID)}`, { signal }),
  taskTimeline: (taskID: string, signal?: AbortSignal) => request<{ items: TimelineEvent[] }>(`/api/admin/tasks/${encodeURIComponent(taskID)}/timeline`, { signal }),

  listEntities: (entity: EntityKey, query: Query, signal?: AbortSignal) =>
    request<Page<Record<string, unknown>>>(`/api/admin/${entityPaths[entity]}${queryString(query)}`, { signal }),
  getEntity: (entity: EntityKey, id: string, signal?: AbortSignal) =>
    request<Record<string, unknown>>(`/api/admin/${entityPaths[entity]}/${encodeURIComponent(id)}`, { signal }),

  listConnectors: (query: Query = {}, signal?: AbortSignal) => request<Page<Connector>>(`/api/admin/connectors${queryString(query)}`, { signal }),
  createConnector: (value: Partial<Connector>) => request<Connector>('/api/admin/connectors', { method: 'POST', body: jsonBody(value) }),
  updateConnector: (connectorID: string, value: Partial<Connector>) =>
    request<Connector>(`/api/admin/connectors/${encodeURIComponent(connectorID)}`, { method: 'PATCH', body: jsonBody(value) }),
  deleteConnector: (connectorID: string) => request<void>(`/api/admin/connectors/${encodeURIComponent(connectorID)}`, { method: 'DELETE' }),
  probeConnector: (connectorID: string) => request<Record<string, unknown>>(`/api/admin/connectors/${encodeURIComponent(connectorID)}/probe`, { method: 'POST' }),
  triggerConnector: (connectorID: string) => request<{ request_id: string; run: ConnectorRun }>(`/api/admin/connectors/${encodeURIComponent(connectorID)}/runs`, { method: 'POST' }),
  connectorRuns: (connectorID: string, query: Query, signal?: AbortSignal) =>
    request<Page<ConnectorRun>>(`/api/admin/connectors/${encodeURIComponent(connectorID)}/runs${queryString(query)}`, { signal }),
  connectorRecords: (connectorID: string, query: Query, signal?: AbortSignal) =>
    request<Page<ConnectorRecord>>(`/api/admin/connectors/${encodeURIComponent(connectorID)}/records${queryString(query)}`, { signal }),
  connectorMetrics: (connectorID: string, signal?: AbortSignal) =>
    request<ConnectorMetrics>(`/api/admin/connectors/${encodeURIComponent(connectorID)}/metrics`, { signal }),
  connectorConfiguration: (connectorID: string, signal?: AbortSignal) =>
    request<ConnectorConfiguration>(`/api/admin/connectors/${encodeURIComponent(connectorID)}/configuration`, { signal }),
  updateConnectorConfiguration: (connectorID: string, value: ConnectorConfigurationUpdate) =>
    request<ConnectorConfiguration>(`/api/admin/connectors/${encodeURIComponent(connectorID)}/configuration`, { method: 'PUT', body: jsonBody(value) }),

  listRepositories: (signal?: AbortSignal) => request<{ items: RegisteredRepository[] }>('/api/admin/repositories', { signal }),
  getRepository: (repositoryID: string, signal?: AbortSignal) =>
    request<RegisteredRepository>(`/api/admin/repositories/${encodeURIComponent(repositoryID)}`, { signal }),
  createRepository: (value: RepositoryCreateInput) => request<RegisteredRepository>('/api/admin/repositories', { method: 'POST', body: jsonBody(value) }),
  patchRepository: (repositoryID: string, value: RepositoryPatchInput) =>
    request<RegisteredRepository>(`/api/admin/repositories/${encodeURIComponent(repositoryID)}`, { method: 'PATCH', body: jsonBody(value) }),
  disableRepository: (repositoryID: string) =>
    request<RegisteredRepository>(`/api/admin/repositories/${encodeURIComponent(repositoryID)}/disable`, { method: 'POST' }),
  enableRepository: (repositoryID: string) =>
    request<RegisteredRepository>(`/api/admin/repositories/${encodeURIComponent(repositoryID)}/enable`, { method: 'POST' }),

  listRequirements: (query: Query = {}, signal?: AbortSignal) => request<Page<Requirement>>(`/api/admin/requirements${queryString(query)}`, { signal }),
  createRequirement: (value: RequirementInput) => request<Requirement>('/api/admin/requirements', { method: 'POST', body: jsonBody(value) }),
  patchRequirement: (requirementID: string, value: Partial<RequirementInput>) =>
    request<Requirement>(`/api/admin/requirements/${encodeURIComponent(requirementID)}`, { method: 'PATCH', body: jsonBody(value) }),
  publishRequirement: (requirementID: string) =>
    request<Requirement>(`/api/admin/requirements/${encodeURIComponent(requirementID)}/publish`, { method: 'POST' }),
  cancelRequirement: (requirementID: string) =>
    request<Requirement>(`/api/admin/requirements/${encodeURIComponent(requirementID)}/cancel`, { method: 'POST' }),
  completeRequirement: (requirementID: string) =>
    request<Requirement>(`/api/admin/requirements/${encodeURIComponent(requirementID)}/complete`, { method: 'POST' }),
  triggerRequirementSync: () => request<{ result: RequirementSyncResult }>('/api/admin/requirements/sync', { method: 'POST', body: jsonBody({}) }),

  tickDefaults: () => request<TickRequest>('/api/admin/scheduler/defaults'),
  tick: (value: TickRequest) => request<TickResult>('/api/admin/scheduler/tick', { method: 'POST', body: jsonBody(value) }),
  retryEffect: (effectID: string) => request<Record<string, unknown>>(`/api/admin/effects/${encodeURIComponent(effectID)}/retry`, { method: 'POST' }),

}
