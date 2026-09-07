import { QuestionCircleOutlined } from '@ant-design/icons'
import { Popover } from 'antd'
import type { ReactNode } from 'react'

/** 术语培训提示：ⓘ 图标 hover 弹出简明解释。 */
export function TermHint({ title, content }: { title: string; content: ReactNode }) {
  return (
    <Popover placement="top" title={title} content={<div className="term-hint__body">{content}</div>} trigger={['hover', 'focus']}>
      <span role="button" tabIndex={0} aria-label={`关于 ${title}`} className="term-hint"><QuestionCircleOutlined /></span>
    </Popover>
  )
}
