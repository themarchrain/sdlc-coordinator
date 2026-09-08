import { describe, expect, it } from 'vitest'

import { getTheme, groupedThemes, THEMES, THEME_FAMILIES, THEME_LIST } from './registry'

describe('themes registry', () => {
  it('registers a unique, default-first ordered theme list', () => {
    const ids = THEME_LIST.map((theme) => theme.id)
    // 有序清单即外观面板渲染顺序契约；新增主题时同步追加。
    expect(ids).toEqual(['default', 'apple', 'claude', 'minimax', 'supabase', 'vercel', 'xai', 'cursor', 'ollama', 'voltagent', 'nintendo-2001', 'dell-1996', 'tesla', 'nike', 'theverge', 'bmw-m', 'discord', 'spotify', 'framer', 'elevenlabs', 'raycast', 'stripe', 'lamborghini', 'resend', 'revolut', 'sentry', 'shopify', 'kraken', 'warp', 'composio', 'clickhouse', 'sanity', 'bugatti', 'binance', 'ferrari', 'hashicorp', 'linear', 'renault', 'slack', 'mintlify', 'together', 'mistral', 'expo', 'lovable', 'mastercard', 'starbucks', 'meta', 'vodafone', 'wired', 'notion', 'airbnb', 'clay', 'figma', 'playstation', 'posthog', 'spacex', 'hp', 'ibm', 'superhuman', 'runway', 'pinterest', 'coinbase', 'cal', 'airtable', 'mongodb', 'miro', 'replicate', 'webflow', 'wise', 'mobbin', 'opencode', 'bmw', 'uber', 'intercom', 'zapier', 'cohere', 'nvidia'])
    expect(new Set(ids).size).toBe(ids.length)
  })

  it('gives every theme complete metadata, swatches, antd config and slots', () => {
    for (const theme of THEME_LIST) {
      expect(theme.id, `${theme.id}: id`).toBeTruthy()
      expect(theme.label, `${theme.id}: label`).toBeTruthy()
      expect(typeof theme.isDark, `${theme.id}: isDark`).toBe('boolean')
      expect(theme.chromeColor, `${theme.id}: chromeColor`).toMatch(/^#/)
      expect(theme.swatches, `${theme.id}: swatches`).toMatchObject({
        canvas: expect.stringMatching(/^#/),
        surface: expect.stringMatching(/^#/),
        ink: expect.stringMatching(/^#/),
        primary: expect.stringMatching(/^#/),
        accent: expect.stringMatching(/^#/),
      })
      expect(theme.antd.token, `${theme.id}: antd token`).toMatchObject({
        colorPrimary: expect.stringMatching(/^#/),
        colorText: expect.stringMatching(/^#/),
        borderRadius: expect.any(Number),
        fontFamily: expect.any(String),
      })
      expect(theme.slots.chart.trends, `${theme.id}: chart.trends`).toMatchObject({
        mr: expect.any(String), crDone: expect.any(String), crStart: expect.any(String), avgDuration: expect.any(String),
      })
      expect(theme.slots.chart.ratios, `${theme.id}: chart.ratios`).toMatchObject({
        success: expect.any(String), firstRun: expect.any(String), intervention: expect.any(String), recovery: expect.any(String),
      })
      expect(theme.slots.timeline, `${theme.id}: timeline`).toMatchObject({
        error: expect.any(String), warning: expect.any(String), success: expect.any(String), info: expect.any(String),
      })
      expect(theme.slots.eventType, `${theme.id}: eventType`).toMatchObject({
        error: expect.any(String), usage: expect.any(String), info: expect.any(String),
      })
      expect(theme.slots.sparkline, `${theme.id}: sparkline`).toMatchObject({
        default: expect.any(String), hoverGrid: expect.any(String),
      })
    }
  })

  it('marks only the curated dark themes (tier-1 six + tier-2 fifteen + spacex) as dark', () => {
    expect(THEME_LIST.filter((theme) => theme.isDark).map((theme) => theme.id)).toEqual(['xai', 'voltagent', 'theverge', 'bmw-m', 'discord', 'spotify', 'framer', 'raycast', 'lamborghini', 'resend', 'revolut', 'sentry', 'warp', 'composio', 'clickhouse', 'sanity', 'bugatti', 'binance', 'ferrari', 'hashicorp', 'linear', 'spacex'])
  })

  it('assigns every theme to a defined, non-empty curated family', () => {
    const familyIds = new Set(THEME_FAMILIES.map((family) => family.id))
    for (const theme of THEME_LIST) {
      expect(familyIds.has(theme.family), `${theme.id}: family "${theme.family}" must be defined in THEME_FAMILIES`).toBe(true)
    }
    for (const family of THEME_FAMILIES) {
      expect(THEME_LIST.some((theme) => theme.family === family.id), `${family.id}: family must hold at least one theme`).toBe(true)
    }
  })

  it('groups themes into family sections and applies the light/dark filter', () => {
    const all = groupedThemes()
    expect(all.reduce((total, section) => total + section.themes.length, 0)).toBe(THEME_LIST.length)
    expect(all.every((section) => section.themes.length > 0)).toBe(true)

    const dark = groupedThemes('dark')
    expect(dark.flatMap((section) => section.themes)).toHaveLength(THEME_LIST.filter((theme) => theme.isDark).length)
    expect(dark.flatMap((section) => section.themes).every((theme) => theme.isDark)).toBe(true)

    const light = groupedThemes('light')
    expect(light.flatMap((section) => section.themes).every((theme) => !theme.isDark)).toBe(true)
  })

  it('narrows sections by keyword matching label or id, case-insensitive, composing with the light filter', () => {
    expect(groupedThemes('all', '  ').reduce((total, section) => total + section.themes.length, 0)).toBe(THEME_LIST.length)

    const framer = groupedThemes('all', 'FRAMER')
    expect(framer.flatMap((section) => section.themes.map((theme) => theme.id))).toEqual(['framer'])

    const byId = groupedThemes('all', 'bmw-m')
    expect(byId.flatMap((section) => section.themes.map((theme) => theme.id))).toEqual(['bmw-m'])

    const composed = groupedThemes('light', 'framer')
    expect(composed).toHaveLength(0)

    expect(groupedThemes('all', '不存在的主题')).toHaveLength(0)
  })

  it('falls back to default for unknown or missing ids', () => {
    expect(THEMES.default.id).toBe('default')
    expect(getTheme('default').id).toBe('default')
    expect(getTheme('not-a-theme').id).toBe('default')
    expect(getTheme('').id).toBe('default')
    expect(getTheme(undefined).id).toBe('default')
    expect(getTheme(null).id).toBe('default')
  })
})
