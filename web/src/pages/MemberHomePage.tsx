import { Link } from 'react-router-dom'

import { PageHeading } from '../components/PageHeading'

export function MemberHomePage() {
  return (
    <div>
      <PageHeading eyebrow="成员工作台" title="工作台" />
      <section className="surface-panel">
        <p>你当前是组织成员，可以查看任务、数据查询和 Agent 运行情况。写操作由组织管理员执行。</p>
        <ul className="mt-4 flex flex-col gap-2">
          <li><Link to="/tasks">任务查询</Link></li>
          <li><Link to="/explorer">数据查询</Link></li>
          <li><Link to="/agents">Agent 运行</Link></li>
          <li><Link to="/requirements">需求列表（只读）</Link></li>
        </ul>
      </section>
    </div>
  )
}
