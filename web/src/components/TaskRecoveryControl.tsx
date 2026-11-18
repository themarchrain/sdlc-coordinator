import { useEffect, useRef, useState } from 'react'
import { Alert, Button, Checkbox, Descriptions, Input, Modal, Space } from 'antd'

import { adminApi, AdminApiError } from '../api/client'
import type { ExecutionExitConfirmation, RecoverTaskResponse, RecoveryAssessment, UnconfirmedExecution } from '../api/types'
import { useWorkspaceSession } from '../app/workspaceSession'

export interface TaskRecoveryControlProps {
  taskID: string
  status: string
  onRecovered: () => void
}

const RISK_COPY = '系统无法判定失败原因，恢复后可能立即再次阻塞，也可能产生额外模型调用费用'

const classificationLabels: Record<string, string> = {
  transient: '暂时故障',
  prerequisite: '前置条件缺失',
  fatal: '致命错误',
  unknown: '原因不明',
}

let requestSequence = 0

type RecoveryFailureKind = 'stale' | 'unavailable' | 'unknown' | 'refused' | 'missing'

interface RecoveryFailure {
  kind: RecoveryFailureKind
  message: string
  refresh?: boolean
}

// describeRecoveryFailure maps a server answer to what the operator can do next.
// Only a network error or a server failure leaves the outcome unknown.
function describeRecoveryFailure(cause: unknown): RecoveryFailure {
  if (!(cause instanceof AdminApiError)) {
    return { kind: 'unknown', message: '恢复请求结果未知，请先刷新任务后再决定是否重试' }
  }
  switch (cause.status) {
    case 400:
      if (cause.code === 'recovery_risk_confirmation_required') {
        return { kind: 'stale', message: '风险确认要求已变化，正在刷新恢复资格', refresh: true }
      }
      return { kind: 'refused', message: '恢复请求参数无效，请刷新后重试' }
    case 403:
      return { kind: 'refused', message: '当前会话没有恢复权限' }
    case 404:
      return { kind: 'missing', message: '任务不存在或已被删除' }
    case 409:
      if (cause.code === 'execution_still_running') return { kind: 'refused', message: '该执行仍在运行，不能确认退出' }
      if (cause.code === 'execution_exit_not_required') return { kind: 'stale', message: '该执行已有结束记录，无需确认退出', refresh: true }
      if (cause.code === 'execution_status_unknown') return { kind: 'refused', message: 'Runner 返回了无法识别的执行状态，请联系运维核对后再确认' }
      if (cause.code === 'execution_not_found') return { kind: 'stale', message: '任务下已没有这条执行记录，正在刷新恢复资格', refresh: true }
      if (cause.code === 'execution_evidence_changed') return { kind: 'stale', message: '执行证据已变化，正在刷新恢复资格', refresh: true }
      if (cause.code === 'recovery_request_conflict') return { kind: 'stale', message: '该请求编号已用于另一份确认，正在刷新恢复资格', refresh: true }
      return { kind: 'stale', message: '任务状态已变化，正在刷新恢复资格', refresh: true }
    case 503:
      return { kind: 'unavailable', message: '恢复安全检查暂不可用，可重试' }
    default:
      return { kind: 'unknown', message: '恢复请求结果未知，请先刷新任务后再决定是否重试' }
  }
}

function formatCheckedAt(value?: string) {
  if (!value) return '—'
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? '—' : parsed.toLocaleString('zh-CN', { hour12: false })
}

// isRequestConflict reports the one answer that retires a request id: the
// server already tied that id to another confirmation.
function isRequestConflict(cause: unknown) {
  return cause instanceof AdminApiError && cause.status === 409 && cause.code === 'recovery_request_conflict'
}

function newRequestID() {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') return crypto.randomUUID()
  requestSequence += 1
  return `recovery-${Date.now()}-${requestSequence}`
}

type ExecutionExitFeedback = { kind: 'success' | 'info'; message: string }

export function TaskRecoveryControl(props: TaskRecoveryControlProps) {
  const { session, role, superAdmin } = useWorkspaceSession()
  const tenantID = session?.scope.tenant?.id
  const permitted = Boolean(session && tenantID) && (role === 'member' || role === 'tenant_admin' || (superAdmin && Boolean(session?.support?.active)))
  const canConfirmExit = Boolean(session && tenantID) && (role === 'tenant_admin' || (superAdmin && Boolean(session?.support?.active)))
  const identity = `${session?.user.id ?? ''}:${tenantID ?? ''}:${role}:${superAdmin}:${props.taskID}:${props.status}`
  return <TaskRecoveryControlBody key={identity} {...props} permitted={permitted} canConfirmExit={canConfirmExit} />
}

function TaskRecoveryControlBody({ taskID, status, onRecovered, permitted, canConfirmExit }: TaskRecoveryControlProps & { permitted: boolean; canConfirmExit: boolean }) {
  const [assessment, setAssessment] = useState<RecoveryAssessment>()
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string>()
  const [errorKind, setErrorKind] = useState<RecoveryFailureKind>()
  const [reloadVersion, setReloadVersion] = useState(0)
  const [riskOpen, setRiskOpen] = useState(false)
  const [riskAcknowledged, setRiskAcknowledged] = useState(false)
  const [reason, setReason] = useState('')
  const [formError, setFormError] = useState<string>()
  const [submitting, setSubmitting] = useState(false)
  const [adopted, setAdopted] = useState<RecoverTaskResponse>()
  const [exitFeedback, setExitFeedback] = useState<ExecutionExitFeedback>()
  const sending = useRef(false)
  const mounted = useRef(true)
  const requestID = useRef('')
  const lastRevision = useRef('')
  const scopeRef = useRef('')

  useEffect(() => {
    mounted.current = true
    return () => { mounted.current = false }
  }, [])

  useEffect(() => {
    scopeRef.current = `${taskID}:${assessment?.block_revision ?? ''}`
  }, [taskID, assessment?.block_revision])

  useEffect(() => {
    if (!permitted || status !== 'blocked') {
      setLoading(false)
      return
    }
    let current = true
    const controller = new AbortController()
    setLoading(true)
    setError(undefined)
    adminApi.taskRecovery(taskID, controller.signal).then((next) => {
      if (!current) return
      if (lastRevision.current && lastRevision.current !== next.block_revision) {
        requestID.current = ''
        setRiskOpen(false)
        setRiskAcknowledged(false)
        setReason('')
        setFormError(undefined)
      }
      lastRevision.current = next.block_revision
      setAssessment(next)
      setLoading(false)
    }).catch((cause) => {
      if (!current || controller.signal.aborted) return
      setError(cause instanceof Error ? cause.message : '读取恢复资格失败')
      setLoading(false)
    })
    return () => { current = false; controller.abort() }
  }, [taskID, status, permitted, reloadVersion])

  useEffect(() => {
    if (!riskOpen || !assessment) return
    if (assessment.eligible && (assessment.safety_blockers ?? []).length === 0) return
    setRiskOpen(false)
    setRiskAcknowledged(false)
    setReason('')
    setFormError(undefined)
  }, [riskOpen, assessment])

  if (!permitted || status !== 'blocked') return null
  if (loading && !assessment) return <Alert type="info" message="正在检查恢复资格" />
  if (error && !assessment) {
    return <Alert type="error" message={error} action={<Button size="small" onClick={() => setReloadVersion((value) => value + 1)}>重试检查</Button>} />
  }
  if (!assessment) return null

  const safetyBlockers = assessment.safety_blockers ?? []
  const canRecover = !loading && !error && assessment.eligible && safetyBlockers.length === 0
  const buttonLabel = assessment.requires_risk_confirmation ? '风险恢复' : '恢复任务'

  const openRisk = () => {
    setRiskAcknowledged(false)
    setReason('')
    setFormError(undefined)
    setRiskOpen(true)
  }

  const closeRisk = () => {
    setRiskOpen(false)
    setRiskAcknowledged(false)
    setReason('')
    setFormError(undefined)
  }

  const submit = async () => {
    if (sending.current || !assessment) return
    if (!assessment.eligible || safetyBlockers.length > 0) {
      closeRisk()
      setError('当前任务已不能恢复，请查看最新的恢复资格')
      setErrorKind('refused')
      return
    }
    if (assessment.requires_risk_confirmation && (!riskAcknowledged || !reason.trim())) {
      setFormError('请确认风险并填写恢复说明')
      return
    }
    sending.current = true
    setSubmitting(true)
    setFormError(undefined)
    if (!requestID.current) requestID.current = newRequestID()
    const scopeAtStart = scopeRef.current
    try {
      const result = await adminApi.recoverTask(taskID, {
        request_id: requestID.current,
        block_revision: assessment.block_revision,
        reason: reason.trim(),
        acknowledge_unknown_risk: assessment.requires_risk_confirmation && riskAcknowledged,
      })
      if (!mounted.current || scopeRef.current !== scopeAtStart) return
      setAdopted(result)
      setRiskOpen(false)
      onRecovered()
    } catch (cause) {
      if (!mounted.current || scopeRef.current !== scopeAtStart) return
      const failure = describeRecoveryFailure(cause)
      setError(failure.message)
      setErrorKind(failure.kind)
      if (failure.refresh) setReloadVersion((value) => value + 1)
    } finally {
      sending.current = false
      setSubmitting(false)
    }
  }

  return <section className="drawer-stack" aria-label="任务恢复">
    {adopted ? <Alert type="success" message="恢复已受理" description={`恢复代次 ${adopted.recovery.source_generation} → ${adopted.recovery.target_generation}；任务已进入待派发，尚未开始运行。`} /> : null}
    {exitFeedback ? <Alert type={exitFeedback.kind} message={exitFeedback.message} /> : null}
    <Descriptions size="small" bordered column={1} items={[
      { key: 'classification', label: '恢复判断', children: classificationLabels[assessment.classification] ?? assessment.classification },
      { key: 'reason', label: '判断说明', children: assessment.reason_summary || '—' },
      { key: 'stage', label: '失败阶段', children: assessment.failure_stage || '—' },
      { key: 'source_run', label: '来源执行', children: assessment.source_run_id || '—' },
      { key: 'checked_at', label: '资格检查时间', children: formatCheckedAt(assessment.checked_at) },
      { key: 'failure', label: '原始错误', children: <span className="error-copy">{assessment.failure_summary || '—'}</span> },
    ]} />
    {safetyBlockers.length > 0 ? <Alert type="warning" message="存在未解除的安全阻塞" description={<>
      <ul>{safetyBlockers.map((blocker) => <li key={blocker.code}>{blocker.summary}{blocker.remediation ? `；${blocker.remediation}` : ''}</li>)}</ul>
      {canConfirmExit && safetyBlockers.some((blocker) => blocker.code === 'execution_unconfirmed')
        ? <ExecutionExitConfirm taskID={taskID} executions={assessment.unconfirmed_executions ?? []}
          onPick={() => setExitFeedback(undefined)}
          onConfirmed={(feedback) => { if (feedback) setExitFeedback(feedback); setReloadVersion((value) => value + 1) }} />
        : null}
    </>} /> : null}
    {!canRecover && safetyBlockers.length === 0 ? <Alert type="warning" message={assessment.reason_summary || '当前任务不能恢复'} /> : null}
    {error ? <Alert type="error" message={error} action={errorKind === 'stale' || errorKind === 'unavailable' || errorKind === 'unknown' ? <Button size="small" onClick={() => {
      if (errorKind === 'unavailable') {
        setError(undefined)
        setErrorKind(undefined)
        void submit()
      } else {
        setReloadVersion((value) => value + 1)
      }
    }}>{errorKind === 'unavailable' ? '重试' : errorKind === 'stale' ? '刷新资格' : '刷新任务'}</Button> : undefined} /> : null}
    {canRecover && !adopted ? <Space>
      <Button type={assessment.requires_risk_confirmation ? 'default' : 'primary'} danger={assessment.requires_risk_confirmation} loading={submitting} disabled={submitting} onClick={assessment.requires_risk_confirmation ? openRisk : submit}>{buttonLabel}</Button>
    </Space> : null}
    <Modal open={riskOpen} title="确认风险恢复" okText="确认恢复" cancelText="取消" confirmLoading={submitting} okButtonProps={{ disabled: !riskAcknowledged || !reason.trim() || submitting }} onCancel={closeRisk} onOk={submit}>
      <Space direction="vertical" size="middle" style={{ width: '100%' }}>
        <Alert type="warning" message={RISK_COPY} />
        <Checkbox checked={riskAcknowledged} onChange={(event) => setRiskAcknowledged(event.target.checked)}>我已了解并确认风险</Checkbox>
        <Input.TextArea aria-label="恢复说明" value={reason} onChange={(event) => setReason(event.target.value)} maxLength={1000} showCount rows={4} placeholder="说明为什么现在恢复，以及已核实的风险" />
        {formError ? <span className="error-copy">{formError}</span> : null}
      </Space>
    </Modal>
  </section>
}

function ExecutionExitConfirm({ taskID, executions, onPick, onConfirmed }: { taskID: string; executions: UnconfirmedExecution[]; onPick: () => void; onConfirmed: (feedback?: ExecutionExitFeedback) => void }) {
  const [target, setTarget] = useState<UnconfirmedExecution>()
  const [acknowledged, setAcknowledged] = useState(false)
  const [reason, setReason] = useState('')
  const [error, setError] = useState<string>()
  const [submitting, setSubmitting] = useState(false)
  const sending = useRef(false)
  const pending = useRef({ id: '', key: '' })

  const close = () => {
    setTarget(undefined)
    setAcknowledged(false)
    setReason('')
    setError(undefined)
  }

  const submit = async () => {
    if (!target || sending.current || !acknowledged || !reason.trim()) return
    sending.current = true
    setSubmitting(true)
    setError(undefined)
    // 请求编号绑定实际发送的完整确认内容：同一内容重试复用编号，
    // 换任务、执行、指纹或说明都算新请求。
    const confirmation = {
      execution_id: target.execution_id, expected_fingerprint: target.fingerprint, reason: reason.trim(),
    }
    const key = JSON.stringify([taskID, confirmation.execution_id, confirmation.expected_fingerprint, confirmation.reason])
    if (!pending.current.id || pending.current.key !== key) pending.current = { id: newRequestID(), key }
    try {
      const stored = await adminApi.confirmExecutionExit(taskID, { request_id: pending.current.id, ...confirmation })
      pending.current = { id: '', key: '' }
      close()
      onConfirmed({ kind: 'success', message: remoteOutcomeCopy(stored) })
    } catch (cause) {
      if (isRequestConflict(cause)) {
        // 服务端已判定该编号属于另一份确认：废弃它，人工再确认时换新编号。
        pending.current = { id: '', key: '' }
      }
      const failure = describeRecoveryFailure(cause)
      if (failure.refresh) {
        // 证据已变化：关窗刷新，但把原因交给父组件留在页面上，
        // 否则阻断一解除子组件就卸载，操作员不知道为何被拒。
        close()
        onConfirmed({ kind: 'info', message: failure.message })
        return
      }
      setError(failure.message)
    } finally {
      sending.current = false
      setSubmitting(false)
    }
  }

  if (executions.length === 0) return null

  return <>
    <ul>
      {executions.map((execution) => <li key={execution.execution_id}>
        <code>{execution.execution_id}</code>（Run {execution.run_id}，状态 {execution.status}）
        {' '}<Button size="small" onClick={() => { onPick(); setTarget(execution) }}>确认已退出</Button>
      </li>)}
    </ul>
    <Modal open={Boolean(target)} title="确认该执行已退出" okText="提交确认" cancelText="取消" confirmLoading={submitting} okButtonProps={{ disabled: !acknowledged || !reason.trim() || submitting }} onCancel={close} onOk={submit}>
      <Space direction="vertical" size="middle" style={{ width: '100%' }}>
        <Alert type="warning" message="该确认只记录你核实过的证据，不会停止或改写执行记录，也不会直接恢复任务。" />
        {target ? <span>确认对象：<code>{target.execution_id}</code>（Run {target.run_id}，状态 {target.status}）</span> : null}
        <Checkbox checked={acknowledged} onChange={(event) => setAcknowledged(event.target.checked)}>我确认该执行进程已经退出，且不会再回写结果</Checkbox>
        <Input.TextArea aria-label="确认说明" value={reason} onChange={(event) => setReason(event.target.value)} maxLength={1000} showCount rows={3} placeholder="说明在哪里、用什么方式确认了该执行已经退出" />
        {error ? <span className="error-copy">{error}</span> : null}
      </Space>
    </Modal>
  </>
}

// remoteOutcomeCopy reports what the runner answered, so the operator can tell
// an attested exit from one the runner confirmed.
function remoteOutcomeCopy(stored: ExecutionExitConfirmation) {
  const detail = stored.remote_message ? `：${stored.remote_message}` : ''
  switch (stored.remote_state) {
    case 'terminal':
      return `已记录退出确认；Runner 回报执行已结束${detail}`
    case 'not_found':
      return `已记录退出确认；Runner 没有这条执行记录${detail}`
    default:
      return `已记录退出确认；未能读取 Runner 执行状态${detail}`
  }
}
