import type { AccountStatus } from "@/lib/types"

export function statusLabel(status?: string) {
  return (
    {
      pending: "还没登录",
      queued: "排队登录中",
      running: "正在登录",
      ready: "已有支付链接",
      failed: "登录失败",
    } as Record<string, string>
  )[status || ""] || "还没登录"
}

export function statusVariant(status?: AccountStatus | string): "default" | "secondary" | "destructive" | "outline" {
  if (status === "ready") return "default"
  if (status === "failed") return "destructive"
  if (status === "running" || status === "queued") return "outline"
  return "secondary"
}

export function maskKey(key?: string) {
  if (!key) return "—"
  if (key.length < 12) return key
  return `${key.slice(0, 6)}…${key.slice(-4)}`
}
