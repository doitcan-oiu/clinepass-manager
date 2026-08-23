import type { Job, JobEvent } from "@/lib/types"

export function latestStepByAccount(logs: JobEvent[]): Record<string, string> {
  const out: Record<string, string> = {}
  for (const ev of logs) {
    const id = ev.account_id
    if (!id || !ev.message) continue
    out[id] = ev.message
  }
  return out
}

export function emailByAccount(accounts: { id: string; email: string }[], jobs: Job[]): Record<string, string> {
  const out: Record<string, string> = {}
  for (const a of accounts) {
    if (a.id) out[a.id] = a.email
  }
  for (const j of jobs) {
    if (j.account_id && j.email && !out[j.account_id]) out[j.account_id] = j.email
  }
  return out
}

function jobRank(status?: string) {
  if (status === "running") return 3
  if (status === "queued") return 2
  return 1
}

export function jobStatusByAccount(jobs: Job[]): Record<string, string> {
  const best: Record<string, Job> = {}
  for (const j of jobs) {
    if (!j.account_id) continue
    const cur = best[j.account_id]
    if (
      !cur ||
      jobRank(j.status) > jobRank(cur.status) ||
      (jobRank(j.status) === jobRank(cur.status) && (j.started_at || 0) >= (cur.started_at || 0))
    ) {
      best[j.account_id] = j
    }
  }
  const out: Record<string, string> = {}
  for (const [id, j] of Object.entries(best)) out[id] = j.status
  return out
}

export function groupLogsByAccount(
  logs: JobEvent[],
  emails: Record<string, string>
): { accountId: string; email: string; events: JobEvent[] }[] {
  const order: string[] = []
  const map = new Map<string, JobEvent[]>()
  for (const ev of logs) {
    const id = ev.account_id || "_"
    if (!map.has(id)) {
      map.set(id, [])
      order.push(id)
    }
    map.get(id)!.push(ev)
  }
  return order.map((id) => ({
    accountId: id,
    email: emails[id] || "未知账号",
    events: map.get(id) || [],
  }))
}

export function jobStatusLabel(status?: string) {
  return (
    {
      queued: "排队中",
      running: "进行中",
      success: "成功",
      failed: "失败",
    } as Record<string, string>
  )[status || ""] || ""
}
