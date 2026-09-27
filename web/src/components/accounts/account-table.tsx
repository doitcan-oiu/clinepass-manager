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
  return <span className={cn("inline-flex shrink-0 items-center gap-1.5 rounded-full px-2 py-1 text-xs font-medium", statusStyles[status])}><Icon className={cn("size-3", status === "running" && "animate-spin")} />{jobStatusLabel(status)}</span>
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
    <section className="overflow-hidden rounded-xl border bg-card shadow-sm" aria-label="账号进度">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b px-5 py-4">
        <h2 className="font-semibold">账号进度 <span className="ml-1.5 text-sm font-normal text-muted-foreground">{accounts.length}</span></h2>
        <p className="text-xs text-muted-foreground">选择账号，查看阶段与运行记录</p>
      </div>
      <div className="hidden grid-cols-[minmax(0,1.2fr)_minmax(0,1.4fr)_6rem_6rem_2.5rem] gap-4 bg-muted/40 px-5 py-2.5 text-xs text-muted-foreground md:grid" aria-hidden="true">
        <span>账号</span><span>执行进度</span><span>总耗时</span><span>支付状态</span><span />
      </div>
      {!accounts.length ? <p className="px-5 py-10 text-center text-sm text-muted-foreground">这批还没有账号。</p> : (
        <div className="divide-y">
          {accounts.map((account) => {
            const job = jobs[account.id]
            const status = accountRunStatus(account, job)
            const message = status === "failed" ? shortLogText(job?.error || account.last_error || "任务未完成，请查看运行记录") : shortLogText(job?.logs?.filter((event) => event.level !== "debug").at(-1)?.message || (account.paid_at ? "订阅已确认" : account.payment_url ? "支付链接已生成" : "任务启动后会在这里显示进度"))
            return (
              <div key={account.id} className={cn("relative grid min-w-0 grid-cols-[minmax(0,1fr)_2.5rem] items-center gap-x-4 gap-y-3 px-5 py-4 transition-colors hover:bg-muted/30 md:grid-cols-[minmax(0,1.2fr)_minmax(0,1.4fr)_6rem_6rem_2.5rem]", selectedId === account.id && "bg-sky-500/5 before:absolute before:inset-y-0 before:left-0 before:w-0.5 before:bg-sky-500")}>
                <button type="button" onClick={() => onSelect(account)} aria-pressed={selectedId === account.id} className="group min-w-0 text-left focus-visible:outline-2 focus-visible:outline-ring">
                  <span className="flex min-w-0 items-center gap-1 text-sm font-medium"><span className="truncate" title={account.email}>{account.email}</span><ChevronRight className={cn("size-3.5 shrink-0 text-muted-foreground opacity-0 group-hover:opacity-100", selectedId === account.id && "opacity-100 text-sky-500")} /></span>
                  <span className="mt-1.5 flex flex-wrap items-center gap-2 text-xs text-muted-foreground"><Badge variant="secondary" className="rounded-md px-1.5 text-[10px] font-normal">{loginProviderLabel(account.login_provider)}</Badge>{account.recovery_email ? "已设置辅助邮箱" : "无辅助邮箱"}</span>
                </button>
                <button type="button" onClick={() => onSelect(account)} className="col-start-1 row-start-2 min-w-0 text-left md:col-auto md:row-auto">
                  <span className="flex flex-wrap items-center gap-2"><RunStatus status={status} /><span className="truncate text-xs font-medium">{job ? stageLabel(job) : status === "success" ? "结果已就绪" : status === "failed" ? "上次任务失败" : "等待启动"}</span></span>
                  <span className={cn("mt-1.5 block truncate text-xs", status === "failed" ? "text-amber-700 dark:text-amber-400" : "text-muted-foreground")} title={message}>{message}</span>
                </button>
                <div className="hidden text-xs tabular-nums text-muted-foreground md:block">{elapsedLabel(job, now)}</div>
                <div className="hidden text-xs md:block">{account.paid_at ? <span className="text-emerald-600 dark:text-emerald-400">已支付</span> : account.payment_url ? <span className="text-sky-600 dark:text-sky-400">链接已生成</span> : <span className="text-muted-foreground">待提取</span>}</div>
                <div className="col-start-2 row-start-1 self-start md:col-auto md:row-auto md:self-center">
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
