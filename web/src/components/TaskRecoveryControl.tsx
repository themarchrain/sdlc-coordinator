import { useEffect, useRef, useState } from 'react'
import { Alert, Button, Checkbox, Descriptions, Input, Modal, Space } from 'antd'

import { adminApi, AdminApiError } from '../api/client'
import type { RecoverTaskResponse, RecoveryAssessment } from '../api/types'
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

function newRequestID() {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') return crypto.randomUUID()
  requestSequence += 1
  return `recovery-${Date.now()}-${requestSequence}`
}

export function TaskRecoveryControl(props: TaskRecoveryControlProps) {
  const { session, role, superAdmin } = useWorkspaceSession()
  const tenantID = session?.scope.tenant?.id
  const permitted = Boolean(session && tenantID) && (role === 'member' || role === 'tenant_admin' || (superAdmin && Boolean(session?.support?.active)))
  const identity = `${session?.user.id ?? ''}:${tenantID ?? ''}:${role}:${superAdmin}:${props.taskID}:${props.status}`
  return <TaskRecoveryControlBody key={identity} {...props} permitted={permitted} />
}

function TaskRecoveryControlBody({ taskID, status, onRecovered, permitted }: TaskRecoveryControlProps & { permitted: boolean }) {
  const [assessment, setAssessment] = useState<RecoveryAssessment>()
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string>()
  const [errorKind, setErrorKind] = useState<'stale' | 'unavailable' | 'unknown'>()
  const [reloadVersion, setReloadVersion] = useState(0)
  const [riskOpen, setRiskOpen] = useState(false)
  const [riskAcknowledged, setRiskAcknowledged] = useState(false)
  const [reason, setReason] = useState('')
  const [formError, setFormError] = useState<string>()
  const [submitting, setSubmitting] = useState(false)
  const [adopted, setAdopted] = useState<RecoverTaskResponse>()
  const sending = useRef(false)
  const mounted = useRef(true)
  const requestID = useRef('')
  const lastRevision = useRef('')
  const scopeRef = useRef('')

  useEffect(() => () => { mounted.current = false }, [])

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
      if (lastRevision.current && lastRevision.current !== next.block_revision) requestID.current = ''
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

  if (!permitted || status !== 'blocked') return null
  if (loading && !assessment) return <Alert type="info" message="正在检查恢复资格" />
  if (error && !assessment) {
    return <Alert type="error" message={error} action={<Button size="small" onClick={() => setReloadVersion((value) => value + 1)}>重试检查</Button>} />
  }
  if (!assessment) return null

  const canRecover = !loading && !error && assessment.eligible && assessment.safety_blockers.length === 0
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
    if (sending.current || !assessment || !assessment.eligible || assessment.safety_blockers.length > 0) return
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
      if (cause instanceof AdminApiError && cause.status === 409) {
        setError('任务状态已变化，正在刷新恢复资格')
        setErrorKind('stale')
        setReloadVersion((value) => value + 1)
      } else if (cause instanceof AdminApiError && cause.status === 503) {
        setError('恢复安全检查暂不可用，可重试')
        setErrorKind('unavailable')
      } else {
        setError('恢复请求结果未知，请先刷新任务后再决定是否重试')
        setErrorKind('unknown')
      }
    } finally {
      sending.current = false
      setSubmitting(false)
    }
  }

  return <section className="drawer-stack" aria-label="任务恢复">
    {adopted ? <Alert type="success" message="恢复已受理" description={`恢复代次 ${adopted.recovery.source_generation} → ${adopted.recovery.target_generation}；任务已进入待派发，尚未开始运行。`} /> : null}
    <Descriptions size="small" bordered column={1} items={[
      { key: 'classification', label: '恢复判断', children: classificationLabels[assessment.classification] ?? assessment.classification },
      { key: 'reason', label: '判断说明', children: assessment.reason_summary || '—' },
      { key: 'failure', label: '原始错误', children: <span className="error-copy">{assessment.failure_summary || '—'}</span> },
    ]} />
    {assessment.safety_blockers.length > 0 ? <Alert type="warning" message="存在未解除的安全阻塞" description={<ul>{assessment.safety_blockers.map((blocker) => <li key={blocker.code}>{blocker.summary}{blocker.remediation ? `；${blocker.remediation}` : ''}</li>)}</ul>} /> : null}
    {!canRecover && assessment.safety_blockers.length === 0 ? <Alert type="warning" message={assessment.reason_summary || '当前任务不能恢复'} /> : null}
    {error ? <Alert type="error" message={error} action={<Button size="small" onClick={() => {
      if (errorKind === 'unavailable') {
        setError(undefined)
        setErrorKind(undefined)
        void submit()
      } else {
        setReloadVersion((value) => value + 1)
      }
    }}>{errorKind === 'unavailable' ? '重试' : errorKind === 'stale' ? '刷新资格' : '刷新任务'}</Button>} /> : null}
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
