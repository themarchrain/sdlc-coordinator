import { useEffect, useRef, useState } from 'react'
import { Alert, Button, Descriptions } from 'antd'
import { adminApi } from '../api/client'
import type { StopStatus, StopTarget } from '../api/types'
import { useWorkspaceSession } from '../app/workspaceSession'

export interface StopControlProps {
  target: StopTarget
  id: string
  canStop: boolean
  initialStop?: StopStatus | null
  onChanged?: () => void
}

function isConfirmed(stop?: StopStatus | null) {
  return stop?.delivery_status === 'confirmed' && (stop.confirmation_kind === 'no_remote_target' || stop.confirmation_kind === 'create_blocked' || (stop.confirmation_kind === 'execution_exited' && stop.exit_confirmed))
}

export function StopControl(props: StopControlProps) {
  const { role, session, superAdmin } = useWorkspaceSession()
  const permitted = role === 'tenant_admin' || ((superAdmin || role === 'super_admin') && Boolean(session?.support?.active && session.scope.tenant))
  const identity = `${session?.user.id ?? ''}:${session?.scope.tenant?.id ?? ''}:${role}:${props.target}:${props.id}`
  return <StopControlBody key={identity} {...props} permitted={permitted} />
}

function StopControlBody({ target, id, canStop, initialStop, onChanged, permitted }: StopControlProps & { permitted: boolean }) {
  const [stop, setStop] = useState(initialStop)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string>()
  const sending = useRef(false)
  const active = useRef(true)

  useEffect(() => { active.current = true; return () => { active.current = false } }, [])
  useEffect(() => { if (initialStop) setStop(initialStop) }, [initialStop])

  useEffect(() => {
    if (!stop || isConfirmed(stop)) return
    let current = true
    let timer: ReturnType<typeof setTimeout>
    const controller = new AbortController()
    const poll = async () => {
      try {
        const detail = await adminApi.getTask(stop.task_id, controller.signal)
        if (!current) return
        const next = detail.stop as StopStatus | undefined
        if (next?.stop_id === stop.stop_id && next.task_id === stop.task_id) {
          setStop(next)
          if (isConfirmed(next)) { onChanged?.(); return }
        }
      } catch (reason) {
        if (!current) return
        setError(reason instanceof Error ? reason.message : '读取停止状态失败')
      }
      if (current) timer = setTimeout(poll, 2000)
    }
    timer = setTimeout(poll, 2000)
    return () => { current = false; clearTimeout(timer); controller.abort() }
  }, [stop, onChanged])

  const stopNow = async () => {
    if (sending.current) return
    sending.current = true
    setBusy(true)
    setError(undefined)
    try {
      const result = await adminApi.stopTarget(target, id)
      if (!active.current) return
      setStop(result)
      onChanged?.()
    } catch (reason) {
      if (active.current) setError(reason instanceof Error ? reason.message : '停止请求失败')
    } finally {
      sending.current = false
      if (active.current) setBusy(false)
    }
  }

  const label = stop?.confirmation_kind === 'no_remote_target' ? '无需取消执行'
    : stop?.confirmation_kind === 'create_blocked' ? '未启动，已阻止启动'
      : stop?.confirmation_kind === 'execution_exited' && stop.exit_confirmed ? '执行已退出'
        : stop?.scope === 'execution_cleanup' ? '正在停止残留执行，等待退出确认' : '已中断，执行停止待确认'

  if (!stop && !error && (!permitted || !canStop)) return null
  return <section className="drawer-stack" aria-label="停止管理">
    {permitted && canStop && !stop ? <Button danger loading={busy} disabled={busy} onClick={stopNow}>停止</Button> : null}
    {error ? <Alert type="error" message={error} /> : null}
    {stop ? <>
      <Alert type={isConfirmed(stop) ? 'info' : 'warning'} message={label} />
      {stop.scope === 'execution_cleanup' ? <p>仅清理残留执行，保留已完成交付。</p> : null}
      <Descriptions size="small" bordered column={1} items={[
        { key: 'actor', label: '停止人', children: stop.requested_by },
        { key: 'time', label: '停止时间', children: new Date(stop.requested_at).toLocaleString() },
        { key: 'reason', label: '原因', children: stop.reason || '—' },
      ]} />
      {(stop.unknown_writebacks || stop.unknown_messages) ? <p>待核实外部动作：代码写回 {stop.unknown_writebacks || 0} 条，IM 消息 {stop.unknown_messages || 0} 条。</p> : null}
    </> : null}
  </section>
}
