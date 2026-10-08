// accountRegistry 是用户中心多账号切换的客户端注册表：登录后把当前账号的
// 会话 token 与用户身份存入 localStorage，切换/批量退出时凭此列表操作。
// 安全边界：token 明文存 localStorage，XSS 可窃取——本系统无第三方脚本、
// 同源自托管，风险可接受；浏览器单 cookie 约束下这是唯一可行形态。

const ACCOUNT_REGISTRY_KEY = 'coordinator_portal_accounts'
export const MAX_SAVED_ACCOUNTS = 5

export interface SavedAccount {
  token: string
  userId: number
  username: string
  savedAt: string
}

function readRaw(): SavedAccount[] {
  try {
    const raw = window.localStorage.getItem(ACCOUNT_REGISTRY_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw) as unknown
    if (!Array.isArray(parsed)) return []
    return parsed.filter(
      (item): item is SavedAccount =>
        typeof item === 'object' &&
        item !== null &&
        typeof (item as SavedAccount).token === 'string' &&
        typeof (item as SavedAccount).userId === 'number' &&
        typeof (item as SavedAccount).username === 'string',
    )
  } catch {
    // 注册表损坏时按空表处理：丢失的只是快捷切换入口，当前会话不受影响。
    return []
  }
}

function writeRaw(accounts: SavedAccount[]): void {
  try {
    window.localStorage.setItem(ACCOUNT_REGISTRY_KEY, JSON.stringify(accounts))
  } catch {
    // 写入失败（配额/隐私模式）只损失快捷切换入口，不得打断登录主流程。
  }
}

// listSavedAccounts 返回全部已保存账号（不含当前 cookie 会话，除非已被登记）。
export function listSavedAccounts(): SavedAccount[] {
  return readRaw()
}

// 会话 cookie 是 HttpOnly，脚本侧不可读；token 一律来自登录/注册/选择组织
// 响应体里的 session_token 字段（颁发时刻下发）。此常量仅供测试对齐。
export const SESSION_COOKIE_NAME = 'coordinator_portal_session'

// saveCurrentAccount 把颁发时刻的会话 token 登记进注册表：按 userId 去重
// （同人重登覆盖旧 token），容量封顶 MAX_SAVED_ACCOUNTS（先淘汰最早保存的）。
// token 为空（后端未回 session_token）时不做任何事。
export function saveCurrentAccount(userId: number, username: string, token: string): void {
  const trimmed = token.trim()
  if (!trimmed) return
  const rest = readRaw().filter((item) => item.userId !== userId)
  const next = [
    { token: trimmed, userId, username, savedAt: new Date().toISOString() },
    ...rest,
  ].slice(0, MAX_SAVED_ACCOUNTS)
  writeRaw(next)
}

// removeSavedAccount 按用户 id 剔除（该账号被单独退出/失效时调用）。
export function removeSavedAccount(userId: number): void {
  writeRaw(readRaw().filter((item) => item.userId !== userId))
}

// clearSavedAccounts 清空注册表（退出所有账号 / 修改密码后调用）。
export function clearSavedAccounts(): void {
  try {
    window.localStorage.removeItem(ACCOUNT_REGISTRY_KEY)
  } catch {
    // 同 writeRaw：清除失败不阻断退出流程。
  }
}

// otherSavedAccounts 返回注册表中除当前登录账号外的可切换账号。
export function otherSavedAccounts(currentUserId: number | null): SavedAccount[] {
  if (currentUserId === null) return readRaw()
  return readRaw().filter((item) => item.userId !== currentUserId)
}
