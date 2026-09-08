import type { ThemeDefinition, ThemeFamilyId } from './types'

export type { ThemeDefinition } from './types'

import { defaultTheme } from './default'
import { appleTheme } from './apple'
import { claudeTheme } from './claude'
import { minimaxTheme } from './minimax'
import { supabaseTheme } from './supabase'
import { vercelTheme } from './vercel'
import { xaiTheme } from './xai'
import { cursorTheme } from './cursor'
import { ollamaTheme } from './ollama'
import { voltagentTheme } from './voltagent'
import { nintendo2001Theme } from './nintendo-2001'
import { dell1996Theme } from './dell-1996'
import { teslaTheme } from './tesla'
import { nikeTheme } from './nike'
import { thevergeTheme } from './theverge'
import { bmwMTheme } from './bmw-m'
import { discordTheme } from './discord'
import { spotifyTheme } from './spotify'
import { framerTheme } from './framer'
import { elevenlabsTheme } from './elevenlabs'
import { raycastTheme } from './raycast'
import { stripeTheme } from './stripe'
import { lamborghiniTheme } from './lamborghini'
import { resendTheme } from './resend'
import { revolutTheme } from './revolut'
import { sentryTheme } from './sentry'
import { shopifyTheme } from './shopify'
import { krakenTheme } from './kraken'
import { warpTheme } from './warp'
import { composioTheme } from './composio'
import { clickhouseTheme } from './clickhouse'
import { sanityTheme } from './sanity'
import { bugattiTheme } from './bugatti'
import { binanceTheme } from './binance'
import { ferrariTheme } from './ferrari'
import { hashicorpTheme } from './hashicorp'
import { linearTheme } from './linear'
import { renaultTheme } from './renault'
import { slackTheme } from './slack'
import { mintlifyTheme } from './mintlify'
import { togetherTheme } from './together'
import { mistralTheme } from './mistral'
import { expoTheme } from './expo'
import { lovableTheme } from './lovable'
import { mastercardTheme } from './mastercard'
import { starbucksTheme } from './starbucks'
import { metaTheme } from './meta'
import { vodafoneTheme } from './vodafone'
import { wiredTheme } from './wired'
import { notionTheme } from './notion'
import { airbnbTheme } from './airbnb'
import { clayTheme } from './clay'
import { figmaTheme } from './figma'
import { playstationTheme } from './playstation'
import { posthogTheme } from './posthog'
import { spacexTheme } from './spacex'
import { hpTheme } from './hp'
import { ibmTheme } from './ibm'
import { superhumanTheme } from './superhuman'
import { runwayTheme } from './runway'
import { pinterestTheme } from './pinterest'
import { coinbaseTheme } from './coinbase'
import { calTheme } from './cal'
import { airtableTheme } from './airtable'
import { mongodbTheme } from './mongodb'
import { miroTheme } from './miro'
import { replicateTheme } from './replicate'
import { webflowTheme } from './webflow'
import { wiseTheme } from './wise'
import { mobbinTheme } from './mobbin'
import { opencodeTheme } from './opencode'
import { bmwTheme } from './bmw'
import { uberTheme } from './uber'
import { intercomTheme } from './intercom'
import { zapierTheme } from './zapier'
import { cohereTheme } from './cohere'
import { nvidiaTheme } from './nvidia'

/** localStorage 持久化 key（与 web/index.html 首屏脚本、ThemeProvider 同 key）。 */
export const THEME_STORAGE_KEY = 'coordinator-theme'

/** 回退主题 id。 */
export const DEFAULT_THEME_ID = 'default'

/** 有序主题清单：外观面板按此顺序渲染主题卡。 */
export const THEME_LIST: ThemeDefinition[] = [
  defaultTheme,
  appleTheme,
  claudeTheme,
  minimaxTheme,
  supabaseTheme,
  vercelTheme,
  xaiTheme,
  cursorTheme,
  ollamaTheme,
  voltagentTheme,
  nintendo2001Theme,
  dell1996Theme,
  teslaTheme,
  nikeTheme,
  thevergeTheme,
  bmwMTheme,
  discordTheme,
  spotifyTheme,
  framerTheme,
  elevenlabsTheme,
  raycastTheme,
  stripeTheme,
  lamborghiniTheme,
  resendTheme,
  revolutTheme,
  sentryTheme,
  shopifyTheme,
  krakenTheme,
  warpTheme,
  composioTheme,
  clickhouseTheme,
  sanityTheme,
  bugattiTheme,
  binanceTheme,
  ferrariTheme,
  hashicorpTheme,
  linearTheme,
  renaultTheme,
  slackTheme,
  mintlifyTheme,
  togetherTheme,
  mistralTheme,
  expoTheme,
  lovableTheme,
  mastercardTheme,
  starbucksTheme,
  metaTheme,
  vodafoneTheme,
  wiredTheme,
  notionTheme,
  airbnbTheme,
  clayTheme,
  figmaTheme,
  playstationTheme,
  posthogTheme,
  spacexTheme,
  hpTheme,
  ibmTheme,
  superhumanTheme,
  runwayTheme,
  pinterestTheme,
  coinbaseTheme,
  calTheme,
  airtableTheme,
  mongodbTheme,
  miroTheme,
  replicateTheme,
  webflowTheme,
  wiseTheme,
  mobbinTheme,
  opencodeTheme,
  bmwTheme,
  uberTheme,
  intercomTheme,
  zapierTheme,
  cohereTheme,
  nvidiaTheme,
]

/** id → ThemeDefinition 索引。 */
export const THEMES: Record<string, ThemeDefinition> = Object.fromEntries(THEME_LIST.map((theme) => [theme.id, theme]))

/** 策展家族（多维度分类之一：按设计语言相似度收纳；顺序即面板分节顺序）。 */
export interface ThemeFamily {
  id: ThemeFamilyId
  label: string
  description: string
}

export const THEME_FAMILIES: ThemeFamily[] = [
  { id: 'system', label: '系统默认', description: '现行控制台蓝' },
  { id: 'minimal', label: '极简白', description: '大留白 · 单色克制 · 零装饰' },
  { id: 'dark', label: '暗夜', description: '深底 · 高饱和强调' },
  { id: 'warm', label: '暖纸', description: '奶油暖底 · 编辑感' },
  { id: 'retro', label: '复古时代', description: 'Y2K 金属 · 1996 目录' },
  { id: 'brand', label: '品牌现代', description: '强品牌色 · 装饰丰富' },
]

/** 明暗筛选（多维度分类之二，与家族正交）。 */
export type ThemeLightFilter = 'all' | 'light' | 'dark'

export interface ThemeSection {
  family: ThemeFamily
  themes: ThemeDefinition[]
}

/** 按家族分节 + 明暗过滤 + 关键词过滤（匹配 label 或 id，大小写不敏感）；空节自动剔除，供外观面板直接渲染。 */
export function groupedThemes(filter: ThemeLightFilter = 'all', query = ''): ThemeSection[] {
  const keyword = query.trim().toLowerCase()
  return THEME_FAMILIES.flatMap((family) => {
    const themes = THEME_LIST.filter(
      (theme) =>
        theme.family === family.id &&
        (filter === 'all' || (filter === 'dark') === theme.isDark) &&
        (!keyword || theme.label.toLowerCase().includes(keyword) || theme.id.toLowerCase().includes(keyword)),
    )
    return themes.length ? [{ family, themes }] : []
  })
}

/** 取主题；非法/缺失 id 一律回退 default（与 index.html 首屏脚本同一逻辑）。 */
export function getTheme(id: string | null | undefined): ThemeDefinition {
  return (id && THEMES[id]) || THEMES[DEFAULT_THEME_ID]
}
