import type { Account, Job, JobEvent, JobStage } from "./types.ts"

export const jobStages: { id: JobStage; label: string }[] = [
  { id: "queued", label: "等待执行" },
  { id: "browser", label: "准备浏览器" },
  { id: "login", label: "登录账号" },
  { id: "verification", label: "身份验证" },
  { id: "payment", label: "提取 / 支付" },
  { id: "saving", label: "保存结果" },
  { id: "done", label: "完成" },
]

// Older Auto servers may still return complete OAuth URLs. Never render their query or fragment.
export function safeLogText(value = ""): string {
  return value.replace(/https?:\/\/[^\s<>"'，。；）]+/gi, (raw) => {
    const clean = raw.split(/[?#]/, 1)[0]
    let safe = clean
    try { const url = new URL(clean); safe = `${url.origin}${url.pathname}` } catch { safe = clean.replace(/\/\/[^/]*@/, "//") }
    return safe.length > 120 ? `${safe.slice(0, 117)}…` : safe
  }).replace(/\b(authorization|cookie)\s*[:=]\s*[^\r\n]+/gi, "$1=[已隐藏]")
    .replace(/\b(password|passwd|access_token|refresh_token|id_token|client_secret|license_key)\s*[:=]\s*(?:"[^"]*"|'[^']*'|[^\s,;]+)/gi, "$1=[已隐藏]")
}

export function shortLogText(value = "", limit = 150): string {
  const clean = safeLogText(value).replace(/[，,]?\s*当前\s*URL\s*=\s*https?:\/\/\S+/gi, "").trim()
  return clean.length > limit ? `${clean.slice(0, limit - 1)}…` : clean
}

function firstEventTime(job: Job) {
  return job.logs?.reduce((earliest, ev) => Math.min(earliest, ev.time), Infinity) ?? Infinity
}

export function latestJobsByAccount(jobs: Job[]): Record<string, Job> {
  const result: Record<string, Job> = {}
  for (const job of jobs) {
    if (!job.account_id) continue
    const current = result[job.account_id]
    const time = (job.started_at || job.stage_started_at || 0) - (current?.started_at || current?.stage_started_at || 0)
    // Prefer the new empty queued job over a completed job created in the same second.
    const left = firstEventTime(job)
    const right = current ? firstEventTime(current) : 0
    const tie = left === right ? 0 : left > right ? 1 : -1
    if (!current || time > 0 || (time === 0 && (tie > 0 || (tie === 0 && job.id.localeCompare(current.id) > 0)))) {
      result[job.account_id] = job
    }
  }
  return result
}

export function logsFromJobs(jobs: Job[]): JobEvent[] {
  return Object.values(latestJobsByAccount(jobs)).flatMap((job) => job.logs || [])
    .sort((a, b) => a.time - b.time || (a.sequence || 0) - (b.sequence || 0))
}

export type DisplayEvent = JobEvent & { repeat: number; lastTime: number }

export function isTechnicalEvent(event: JobEvent): boolean {
  if (event.level === "error" || event.level === "warning" || event.level === "warn") return false
  return event.level === "debug" || event.level === "trace" || /(?:当前\s*URL\s*=|正在正常关闭浏览器|释放 Cloak 会话|浏览器可执行|CloakBrowser.*(?:版本|就绪)|^\s*(?:Traceback|File "))/i.test(event.message)
}

export function displayEvents(logs: JobEvent[], technical = false): DisplayEvent[] {
  const result: DisplayEvent[] = []
  const lastByAccount = new Map<string, DisplayEvent>()
  for (const event of logs) {
    if (!technical && isTechnicalEvent(event)) continue
    const message = safeLogText(event.message)
    const detail = safeLogText(event.detail)
    const repeat = Math.max(1, event.repeat || 1)
    const previous = lastByAccount.get(event.account_id)
    if (previous && previous.job_id === event.job_id && previous.message === message && previous.detail === detail && previous.level === event.level && previous.stage === event.stage) {
      previous.repeat += repeat
      previous.lastTime = event.time
      continue
    }
    const item = { ...event, message, detail, repeat, lastTime: event.time }
    result.push(item)
    lastByAccount.set(event.account_id, item)
  }
  return result
}

export function stageLabel(job?: Job): string {
  if (!job) return "等待启动"
  if (job.status === "success") return "任务已完成"
  return shortLogText(job.stage_label || jobStages.find((s) => s.id === job.stage)?.label || (job.status === "queued" ? "等待执行" : job.status === "failed" ? "任务未完成" : "正在处理"))
}

export function accountRunStatus(account: Account, job?: Job): "queued" | "running" | "success" | "failed" {
  if (job && ["queued", "running", "success", "failed"].includes(job.status)) return job.status as ReturnType<typeof accountRunStatus>
  if (account.status === "running") return "running"
  if (account.status === "failed") return "failed"
  if (account.paid_at || account.status === "ready") return "success"
  return "queued"
}

export function batchRunCounts(accounts: Account[], jobs: Record<string, Job>) {
  const counts = { queued: 0, running: 0, success: 0, failed: 0 }
  for (const account of accounts) counts[accountRunStatus(account, jobs[account.id])]++
  return counts
}

export function elapsedLabel(job?: Job, now = Date.now(), stage = false): string {
  const start = stage ? job?.stage_started_at : job?.started_at
  if (!start) return "—"
  const seconds = Math.max(0, Math.floor(((job?.ended_at ? job.ended_at * 1000 : now) - start * 1000) / 1000))
  if (seconds < 60) return `${seconds} 秒`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes} 分 ${seconds % 60} 秒`
  return `${Math.floor(minutes / 60)} 小时 ${minutes % 60} 分`
}

export function jobStatusLabel(status?: string) {
  return ({ queued: "待处理", running: "进行中", success: "已完成", failed: "需处理" } as Record<string, string>)[status || ""] || "待处理"
}
