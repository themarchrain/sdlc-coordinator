import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { StatusTag } from './StatusTag'

describe('StatusTag', () => {
  it.each([
    ['finished', 'ant-tag-success'],
    ['running', 'ant-tag-processing'],
    ['paused', 'ant-tag-warning'],
    ['waiting_for_confirmation', 'ant-tag-warning'],
    ['error', 'ant-tag-error'],
    ['stuck', 'ant-tag-error'],
    ['skipped', 'ant-tag-default'],
  ])('renders %s with its operational color', (status, className) => {
    render(<StatusTag status={status} />)

    expect(screen.getByText(status)).toHaveClass(className)
  })
})
