import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { Sparkline } from './Sparkline'

beforeEach(() => {
  vi.spyOn(SVGElement.prototype, 'getBoundingClientRect').mockReturnValue({ left: 0, width: 100 } as DOMRect)
})
afterEach(() => vi.restoreAllMocks())

describe('Sparkline', () => {
  it('renders a polyline for two or more points', () => {
    const { container } = render(<Sparkline data={[
      { date: '2026-08-12', value: 1 },
      { date: '2026-08-13', value: 3 },
      { date: '2026-08-14', value: 2 },
    ]} />)

    expect(screen.queryByText('暂无趋势')).not.toBeInTheDocument()
    expect(container.querySelector('polyline')).toHaveAttribute('points')
    expect(container.querySelectorAll('circle')).toHaveLength(3)
  })

  it('shows an empty placeholder when fewer than two points are present', () => {
    render(<Sparkline data={[
      { date: '2026-08-14', value: 2 },
      { date: '2026-08-15', value: null },
    ]} />)

    expect(screen.getByText('暂无趋势')).toBeInTheDocument()
  })

  it('reveals the hovered point value and date on mouse move', () => {
    render(<Sparkline data={[
      { date: '2026-08-12', value: 1 },
      { date: '2026-08-13', value: 3 },
      { date: '2026-08-14', value: 2 },
    ]} format={(value) => `${value} 个`} />)

    const svg = document.querySelector('.sparkline svg')!
    fireEvent.mouseMove(svg, { clientX: 50 })

    expect(screen.getByText('2026-08-13')).toBeInTheDocument()
    expect(screen.getByText('3 个')).toBeInTheDocument()
  })

  it('clears the tooltip when the mouse leaves', () => {
    render(<Sparkline data={[
      { date: '2026-08-12', value: 1 },
      { date: '2026-08-13', value: 3 },
    ]} />)

    const svg = document.querySelector('.sparkline svg')!
    fireEvent.mouseMove(svg, { clientX: 10 })
    expect(screen.getByText('2026-08-12')).toBeInTheDocument()

    fireEvent.mouseLeave(svg)
    expect(screen.queryByText('2026-08-12')).not.toBeInTheDocument()
  })
})
