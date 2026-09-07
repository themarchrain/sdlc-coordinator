import { createContext, useContext, type ReactNode } from 'react'
import { createPortal } from 'react-dom'

export const PageHeadingHostContext = createContext<HTMLElement | null>(null)

export function PageHeading({ eyebrow, title, action }: { eyebrow: string; title: string; action?: ReactNode }) {
  const host = useContext(PageHeadingHostContext)
  const heading = (
    <header className="page-heading">
      <div>
        <div className="page-heading__eyebrow">{eyebrow}</div>
      </div>
      {action ? <div className="page-heading__action">{action}</div> : null}
    </header>
  )
  return host ? createPortal(heading, host) : heading
}
