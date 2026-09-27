import { useEffect, useMemo, useRef, useState } from "react"
import { ArrowDown, Check, Circle, CircleAlert, Clock3, Info, ListFilter, LoaderCircle, RotateCcw, Terminal } from "lucide-react"
import type { Account, Job, JobEvent } from "@/lib/types"
import { accountRunStatus, displayEvents, elapsedLabel, isTechnicalEvent, jobStages, shortLogText, stageLabel } from "@/lib/job-log"
import { cn } from "@/lib/utils"
import { Button } from "@/components/ui/button"
import { RunStatus } from "./account-table"

function timeLabel(time: number) {
  return new Date(time).toLocaleTimeString("zh-CN", { hour12: false })
}

export function JobLogPanel({ logs, accounts, jobs, now, filterId, onFilter, onRetry }: {
  logs: JobEvent[]
  accounts: Account[]
  jobs: Record<string, Job>
  now: number
  filterId: string
  onFilter: (accountId: string) => void
  onRetry: (account: Account) => void
}) {
  const [technical, setTechnical] = useState(false)
  const [follow, setFollow] = useState(true)
  const scrollerRef = useRef<HTMLDivElement>(null)
  const selected = accounts.find((account) => account.id === filterId)
  const job = selected ? jobs[selected.id] : undefined
  const status = selected ? accountRunStatus(selected, job) : undefined
  const accountLogs = useMemo(() => filterId ? logs.filter((ev) => ev.account_id === filterId) : logs, [logs, filterId])
  const events = useMemo(() => displayEvents(accountLogs, technical), [accountLogs, technical])
  const hiddenCount = accountLogs.filter(isTechnicalEvent).length
  const emails = useMemo(() => Object.fromEntries(accounts.map((account) => [account.id, account.email])), [accounts])
  const seenStages = new Set(job?.logs?.map((event) => event.stage).filter(Boolean))
  const stages = jobStages.filter((stage) => stage.id !== "verification" || seenStages.has("verification") || job?.stage === "verification")
  const failure = selected && status === "failed" ? shortLogText(job?.error || selected.last_error || "本次任务未完成。请查看下方记录后重试。", 350) : ""

  useEffect(() => {
    const el = scrollerRef.current
    if (el && follow) el.scrollTop = el.scrollHeight
  }, [events, filterId, follow])

  return (
    <section className="min-w-0 overflow-hidden rounded-xl border bg-card shadow-sm" aria-label="任务运行记录">
      <header className="flex flex-wrap items-center justify-between gap-3 border-b px-5 py-4">
        <div className="flex items-center gap-2"><Terminal className="size-4 text-muted-foreground" /><h2 className="font-semibold">运行记录</h2><span className="rounded-md bg-muted px-1.5 py-0.5 text-xs tabular-nums text-muted-foreground">{events.length}</span></div>
        <div className="flex min-w-0 max-w-full items-center gap-2">
          <ListFilter className="size-3.5 shrink-0 text-muted-foreground" />
          <select aria-label="选择要查看进度的账号" value={filterId} onChange={(event) => onFilter(event.target.value)} className="h-8 min-w-0 max-w-full rounded-md border bg-background px-2 text-xs sm:max-w-80">
            <option value="">全部账号</option>
            {accounts.map((account) => <option key={account.id} value={account.id}>{account.email}</option>)}
          </select>
        </div>
      </header>

      {selected ? <div className="space-y-4 border-b bg-muted/15 px-5 py-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0 space-y-2"><div className="flex flex-wrap items-center gap-2"><p className="break-all text-sm font-semibold">{selected.email}</p><RunStatus status={status || "queued"} /></div><p className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground"><span>{job ? stageLabel(job) : "暂无运行任务"}</span><span className="inline-flex items-center gap-1"><Clock3 className="size-3" />总耗时 {elapsedLabel(job, now)}</span>{job?.stage_started_at && status === "running" ? <span>当前阶段 {elapsedLabel(job, now, true)}</span> : null}</p></div>
          {status === "failed" && !selected.paid_at ? <Button size="sm" variant="outline" onClick={() => onRetry(selected)}><RotateCcw className="size-3.5" />重试此账号</Button> : null}
        </div>
        {job?.stage ? <ol className="grid grid-cols-2 gap-2 sm:flex sm:flex-wrap" aria-label="执行阶段">
          {stages.map((stage) => {
            const current = job.stage === stage.id
            const visited = seenStages.has(stage.id) && !current
            const completed = stage.id === "done" && status === "success"
            const Icon = current && status === "failed" ? CircleAlert : current && status === "running" ? LoaderCircle : visited || completed ? Check : Circle
            return <li key={stage.id} aria-current={current ? "step" : undefined} className={cn("flex items-center gap-2 rounded-lg border px-3 py-2 text-xs", current ? status === "failed" ? "border-amber-500/25 bg-amber-500/5 text-amber-700 dark:text-amber-400" : "border-sky-500/25 bg-sky-500/5 text-sky-700 dark:text-sky-400" : visited || completed ? "border-transparent bg-muted/60 text-foreground" : "border-transparent text-muted-foreground/70")}><Icon className={cn("size-3.5 shrink-0", current && status === "running" && "animate-spin")} />{stage.label}</li>
          })}
        </ol> : <p className="text-xs text-muted-foreground">{job ? "此任务未提供阶段信息，下方显示实际运行记录。" : "任务开始后，将显示各阶段进度与耗时。"}</p>}
        {failure ? <div role="status" className="flex items-start gap-2.5 rounded-lg border border-amber-500/20 bg-amber-500/5 p-3 text-amber-800 dark:text-amber-300"><CircleAlert className="mt-0.5 size-4 shrink-0" /><div className="min-w-0"><p className="break-words text-sm leading-relaxed [overflow-wrap:anywhere]">{failure}</p><p className="mt-1 text-xs opacity-75">本次任务已停止；处理页面提示或账号问题后，可单独重试。</p></div></div> : null}
      </div> : <div className="border-b bg-muted/15 px-5 py-3 text-xs text-muted-foreground">汇总每个账号最近一次任务。选择账号可查看阶段、耗时及失败原因。</div>}

      <div className="flex flex-wrap items-center justify-between gap-2 border-b px-5 py-2.5">
        <label className="flex cursor-pointer items-center gap-2 text-xs text-muted-foreground"><input type="checkbox" checked={technical} onChange={(event) => setTechnical(event.target.checked)} className="size-3.5 accent-current" />显示技术日志{hiddenCount ? <span className="text-muted-foreground/70">（{hiddenCount}）</span> : null}</label>
        <Button variant={follow ? "secondary" : "ghost"} size="xs" aria-pressed={follow} onClick={() => setFollow(!follow)}><ArrowDown className="size-3" />{follow ? "自动跟随" : "跟随最新"}</Button>
      </div>
      <div ref={scrollerRef} onScroll={() => {
        const el = scrollerRef.current
        if (el && follow && el.scrollHeight - el.scrollTop - el.clientHeight > 48) setFollow(false)
      }} className="max-h-[28rem] min-h-48 overflow-x-hidden overflow-y-auto px-5 py-2 [overflow-anchor:none]" tabIndex={0} aria-label="运行日志时间线">
        {!events.length ? <div className="flex min-h-44 flex-col items-center justify-center gap-2 text-center text-muted-foreground"><Clock3 className="size-6 opacity-40" /><p className="text-sm">{status === "queued" ? "等待任务开始" : "暂无运行记录"}</p><p className="text-xs">{status === "queued" ? "排队中的账号开始执行后，会在这里实时更新。" : hiddenCount ? "打开技术日志可以查看诊断信息。" : "启动任务后，这里会显示关键步骤和执行结果。"}</p></div> : <ol className="divide-y divide-border/50">
          {events.map((event, index) => {
            const error = event.level === "error"
            const warning = event.level === "warning" || event.level === "warn"
            const success = event.stage === "done" || event.level === "success"
            const Icon = error || warning ? CircleAlert : success ? Check : Info
            const stage = jobStages.find((item) => item.id === event.stage)?.label
            return <li key={`${event.job_id}:${event.sequence ?? event.time}:${index}`} className="grid min-w-0 grid-cols-[1rem_minmax(0,1fr)] gap-x-3 py-3 sm:grid-cols-[4.5rem_1rem_minmax(0,1fr)]">
              <time dateTime={new Date(event.time).toISOString()} className="col-start-2 mb-1 text-[11px] tabular-nums text-muted-foreground sm:col-start-auto sm:mb-0 sm:pt-0.5">{timeLabel(event.time)}</time>
              <Icon className={cn("col-start-1 row-start-1 mt-0.5 size-3.5 sm:col-start-auto sm:row-start-auto", error ? "text-destructive" : warning ? "text-amber-500" : success ? "text-emerald-500" : "text-muted-foreground/60")} />
              <div className="col-start-2 min-w-0 sm:col-start-auto">
                {!filterId ? <button type="button" onClick={() => onFilter(event.account_id)} className="mb-1 block max-w-full truncate text-xs font-medium text-muted-foreground hover:text-foreground">{emails[event.account_id] || "未知账号"}</button> : null}
                <div className="flex flex-wrap items-start gap-x-2 gap-y-1"><p className={cn("min-w-0 whitespace-pre-wrap break-words text-sm leading-relaxed [overflow-wrap:anywhere]", error ? "text-destructive" : "text-foreground/85")}>{shortLogText(event.message, 600)}</p>{event.repeat > 1 ? <span className="mt-0.5 shrink-0 rounded-md bg-muted px-1.5 py-0.5 text-[10px] tabular-nums text-muted-foreground" title={`同一步骤重复 ${event.repeat} 次，最近 ${timeLabel(event.lastTime)}`}>×{event.repeat}</span> : null}</div>
                {stage ? <p className="mt-1 text-[10px] text-muted-foreground/75">{stage}</p> : null}
                {technical && event.detail ? <details className="mt-2 text-xs text-muted-foreground"><summary className="cursor-pointer">诊断详情</summary><p className="mt-2 whitespace-pre-wrap break-words rounded-md bg-muted/50 p-2 leading-relaxed [overflow-wrap:anywhere]">{event.detail.slice(0, 4000)}</p></details> : null}
              </div>
            </li>
          })}
        </ol>}
      </div>
      <footer className="flex flex-wrap items-center justify-between gap-2 border-t bg-muted/20 px-5 py-2.5 text-[11px] text-muted-foreground"><span>相同步骤合并显示 · 长链接参数已隐藏</span><span>{follow ? "随新记录滚动" : "自由浏览记录"}</span></footer>
    </section>
  )
}
