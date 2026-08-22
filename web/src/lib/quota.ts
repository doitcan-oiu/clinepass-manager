import type { AccountUsage, PoolAccount, UsageWindow } from "@/lib/types"

const WARN = 80
export const MONTHLY_USD = 50

export function poolLeftUSD(totalUsed: number) {
  return Math.max(0, MONTHLY_USD - totalUsed)
}

export function modelCanSpendUSD(used: number, limit: number, totalUsed: number) {
  const poolLeft = poolLeftUSD(totalUsed)
  const cap = limit > 0 ? limit : MONTHLY_USD
  const modelLeft = Math.max(0, cap - used)
  return Math.min(poolLeft, modelLeft)
}

export function modelRemainPct(used: number, limit: number, totalUsed: number) {
  const cap = limit > 0 ? limit : MONTHLY_USD
  if (cap <= 0) return 0
  return (modelCanSpendUSD(used, limit, totalUsed) / cap) * 100
}

export function formatTime(ts: number) {
  if (!ts) return "—"
  const d = new Date(ts * 1000)
  const pad = (n: number) => String(n).padStart(2, "0")
  return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

export function barClass(pct: number | null) {
  if (pct == null) return "bg-muted"
  if (Math.round(pct) >= 100) return "bg-red-500"
  if (pct >= WARN) return "bg-amber-500"
  return "bg-emerald-500"
}

export function pctText(pct: number | null) {
  if (pct == null) return "text-muted-foreground"
  if (Math.round(pct) >= 100) return "text-red-500"
  if (pct >= WARN) return "text-amber-500"
  return "text-muted-foreground"
}

export function windowPct(w: UsageWindow | undefined, synced: boolean): number | null {
  if (!synced || !w?.status) return null
  return w.usage_percent
}

export function health(a: PoolAccount): { label: string; className: string } {
  const u = a.usage
  const synced = !!u?.synced_at
  if (!synced) return { label: "未同步", className: "bg-muted text-muted-foreground" }
  if (u.error && !u.rolling?.status) return { label: "异常", className: "bg-red-500/15 text-red-600" }
  const rows = [u.rolling, u.weekly, u.monthly]
  if (rows.some((w) => w?.status === "rate-limited" || Math.round(w?.usage_percent ?? 0) >= 100)) {
    return { label: "额度用尽", className: "bg-red-500/15 text-red-600" }
  }
  if (rows.some((w) => (w?.usage_percent ?? 0) >= WARN)) {
    return { label: "额度紧张", className: "bg-amber-500/15 text-amber-600" }
  }
  return { label: "正常", className: "bg-emerald-500/15 text-emerald-600" }
}

export function monthlyExpireLabel(u: AccountUsage | undefined) {
  if (!u?.synced_at || !u.monthly?.status || !u.monthly.reset_in_sec || u.monthly.reset_in_sec <= 0) return ""
  return formatTime(u.synced_at + u.monthly.reset_in_sec)
}

export function formatResetAt(resetInSec: number | undefined, syncedAt: number) {
  if (!syncedAt || resetInSec == null || resetInSec < 0) return ""
  return formatTime(syncedAt + resetInSec)
}

export function usageRows(u: AccountUsage | undefined, synced: boolean) {
  const syncedAt = u?.synced_at || 0
  return [
    { key: "r", label: "滚动用量", pct: windowPct(u?.rolling, synced), reset: formatResetAt(u?.rolling?.reset_in_sec, syncedAt) },
    { key: "w", label: "每周用量", pct: windowPct(u?.weekly, synced), reset: formatResetAt(u?.weekly?.reset_in_sec, syncedAt) },
    { key: "m", label: "每月用量", pct: windowPct(u?.monthly, synced), reset: formatResetAt(u?.monthly?.reset_in_sec, syncedAt) },
  ]
}

export function modelTone(used: number, limit = MONTHLY_USD) {
  if (limit <= 0) return "bg-emerald-600 text-white"
  const pct = (used / limit) * 100
  if (pct >= 100) return "bg-red-600 text-white"
  if (pct >= WARN) return "bg-amber-500 text-white"
  return "bg-emerald-600 text-white"
}

export function remainBarClass(remainPct: number | null) {
  if (remainPct == null) return "bg-muted"
  if (remainPct <= 0) return "bg-red-500"
  if (remainPct <= 100 - WARN) return "bg-amber-500"
  return "bg-emerald-500"
}

export function remainText(remainPct: number | null) {
  if (remainPct == null) return "text-muted-foreground"
  if (remainPct <= 0) return "text-red-500"
  if (remainPct <= 100 - WARN) return "text-amber-600"
  return "text-muted-foreground"
}
