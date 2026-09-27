import { Check, ChevronRight, Clock3, CircleAlert, LoaderCircle, MoreHorizontal } from "lucide-react"
import type { Account, Job } from "@/lib/types"
import { accountRunStatus, elapsedLabel, jobStatusLabel, shortLogText, stageLabel } from "@/lib/job-log"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { loginProviderLabel } from "@/lib/login-provider"
import { cn } from "@/lib/utils"

const statusStyles: Record<string, string> = {
  queued: "bg-muted text-muted-foreground",
  running: "bg-sky-500/10 text-sky-700 dark:text-sky-400",
  success: "bg-emerald-500/10 text-emerald-700 dark:text-emerald-400",
  failed: "bg-amber-500/10 text-amber-700 dark:text-amber-400",
}

export function RunStatus({ status }: { status: string }) {
  const Icon = status === "running" ? LoaderCircle : status === "failed" ? CircleAlert : status === "success" ? Check : Clock3
  return <span className={cn("inline-flex shrink-0 items-center gap-1.5 whitespace-nowrap rounded-full px-2 py-1 text-xs font-medium", statusStyles[status])}><Icon className={cn("size-3 shrink-0", status === "running" && "animate-spin")} />{jobStatusLabel(status)}</span>
}

export function AccountTable({
  accounts, jobs, now, onLogin, onDetail, onRemove, onRefresh, selectedId, onSelect,
}: {
  accounts: Account[]
  jobs: Record<string, Job>
  now: number
  onLogin: (account: Account) => void
  onDetail: (account: Account) => void
  onRemove: (account: Account) => void
  onRefresh?: (account: Account) => void
  selectedId?: string
  onSelect: (account: Account) => void
}) {
  return (
    <section className="flex min-h-0 min-w-0 flex-col overflow-hidden rounded-xl border bg-card shadow-sm lg:h-full" aria-label="账号进度">
      <div className="flex h-14 shrink-0 items-center justify-between gap-3 border-b px-4">
        <h2 className="shrink-0 whitespace-nowrap text-sm font-semibold">账号进度 <span className="ml-1 text-xs font-normal text-muted-foreground">{accounts.length}</span></h2>
        <p className="min-w-0 truncate text-xs text-muted-foreground" title="选择账号，查看阶段与运行记录">选择账号查看记录</p>
      </div>
      {!accounts.length ? <p className="px-5 py-10 text-center text-sm text-muted-foreground">这批还没有账号。</p> : (
        <div className="min-h-0 max-h-[30rem] flex-1 divide-y overflow-y-auto overscroll-contain [scrollbar-gutter:stable] lg:max-h-none">
          {accounts.map((account) => {
            const job = jobs[account.id]
            const status = accountRunStatus(account, job)
            const message = status === "failed" ? shortLogText(job?.error || account.last_error || "任务未完成，请查看运行记录") : shortLogText(job?.logs?.filter((event) => event.level !== "debug").at(-1)?.message || (account.paid_at ? "订阅已确认" : account.payment_url ? "支付链接已生成" : "任务启动后会在这里显示进度"))
            return (
              <div key={account.id} className={cn("relative min-w-0 transition-colors hover:bg-muted/30", selectedId === account.id && "bg-sky-500/5 before:absolute before:inset-y-0 before:left-0 before:w-0.5 before:bg-sky-500")}>
                <button type="button" onClick={() => onSelect(account)} aria-pressed={selectedId === account.id} className="group grid w-full min-w-0 gap-2 px-4 py-3 text-left focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring">
                  <span className="flex h-6 min-w-0 items-center gap-1 pr-8 text-sm font-medium">
                    <span className="truncate" title={`${account.email} · ${account.recovery_email ? `辅助邮箱：${account.recovery_email}` : "无辅助邮箱"}`}>{account.email}</span>
                    <ChevronRight className={cn("size-3.5 shrink-0 text-muted-foreground opacity-0 group-hover:opacity-100", selectedId === account.id && "opacity-100 text-sky-500")} />
                  </span>
                  <span className="flex min-w-0 items-center gap-2">
                    <RunStatus status={status} />
                    <span className="min-w-0 flex-1 truncate text-xs font-medium" title={job ? stageLabel(job) : undefined}>{job ? stageLabel(job) : status === "success" ? "结果已就绪" : status === "failed" ? "上次任务失败" : "等待启动"}</span>
                    <span className="shrink-0 whitespace-nowrap text-xs tabular-nums text-muted-foreground" title="任务总耗时">{elapsedLabel(job, now)}</span>
                  </span>
                  <span className="flex min-w-0 items-center gap-2 text-xs">
                    <Badge variant="secondary" className="rounded-md px-1.5 text-xs font-normal">{loginProviderLabel(account.login_provider)}</Badge>
                    <span className={cn("min-w-0 flex-1 truncate", status === "failed" ? "text-amber-700 dark:text-amber-400" : "text-muted-foreground")} title={message}>{message}</span>
                    <span className="shrink-0 whitespace-nowrap">{account.paid_at ? <span className="text-emerald-600 dark:text-emerald-400">已支付</span> : account.payment_url ? <span className="text-sky-600 dark:text-sky-400">链接已生成</span> : <span className="text-muted-foreground">待提取</span>}</span>
                  </span>
                </button>
                <div className="absolute right-3 top-2">
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild><Button variant="ghost" size="icon" aria-label={`${account.email} 的操作`}><MoreHorizontal /></Button></DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      {account.paid_at ? <DropdownMenuItem disabled>已支付，不再提取链接</DropdownMenuItem> : <>
                        <DropdownMenuItem disabled={status === "running" || job?.status === "queued"} onClick={() => onLogin(account)}>重新登录并生成链接</DropdownMenuItem>
                        {onRefresh && (account.has_cookies || account.cookie_header) ? <DropdownMenuItem disabled={status === "running" || job?.status === "queued"} onClick={() => onRefresh(account)}>刷新过期的支付链接</DropdownMenuItem> : null}
                      </>}
                      <DropdownMenuItem onClick={() => onSelect(account)}>查看运行记录</DropdownMenuItem>
                      <DropdownMenuItem onClick={() => onDetail(account)}>账号详情</DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem variant="destructive" onClick={() => onRemove(account)}>删除</DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
              </div>
            )
          })}
        </div>
      )}
    </section>
  )
}
