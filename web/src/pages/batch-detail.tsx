import { useEffect, useMemo, useRef, useState } from "react"
import { Link, useParams } from "react-router-dom"
import { toast } from "sonner"
import { api } from "@/lib/api"
import type { Account, Batch, Job, JobEvent } from "@/lib/types"
import { AccountTable } from "@/components/accounts/account-table"
import { AutoPayDialog } from "@/components/accounts/auto-pay-dialog"
import { DetailDialog } from "@/components/accounts/detail-dialog"
import { JobLogPanel } from "@/components/accounts/job-log-panel"
import { Button } from "@/components/ui/button"
import { batchStatus, radarDeniedCount, waitingCount } from "@/lib/batch-ui"
import { emailByAccount, jobStatusByAccount, latestStepByAccount } from "@/lib/job-log"
import { downloadBase64, xlsxMime } from "@/lib/download"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"

export function BatchDetailPage() {
  const { id } = useParams<{ id: string }>()
  const [batch, setBatch] = useState<Batch | null>(null)
  const [accounts, setAccounts] = useState<Account[]>([])
  const [detail, setDetail] = useState<Account | null>(null)
  const [remove, setRemove] = useState<Account | null>(null)
  const [removeRadar, setRemoveRadar] = useState(false)
  const [removingRadar, setRemovingRadar] = useState(false)
  const [payAsk, setPayAsk] = useState<null | { mode: "login" | "refresh"; account?: Account }>(null)
  const [payPending, setPayPending] = useState(false)
  const [logs, setLogs] = useState<JobEvent[]>([])
  const [jobs, setJobs] = useState<Job[]>([])
  const [logFilter, setLogFilter] = useState("")
  const esRef = useRef<{ close: () => void } | null>(null)
  const radarCount = radarDeniedCount(accounts)
  const emails = useMemo(() => emailByAccount(accounts, jobs), [accounts, jobs])
  const jobStatuses = useMemo(() => jobStatusByAccount(jobs), [jobs])
  const currentSteps = useMemo(() => latestStepByAccount(logs), [logs])

  async function reload() {
    if (!id) return
    const data = await api.batch(id)
    setBatch(data.batch)
    setAccounts(data.accounts)
    setDetail((cur) => (cur ? data.accounts.find((a) => a.id === cur.id) || null : null))
    setLogFilter((cur) => (cur && !data.accounts.some((a) => a.id === cur) ? "" : cur))
    return data
  }

  useEffect(() => {
    let cancelled = false
    ;(async () => {
      try {
        const data = await reload()
        if (cancelled || !data) return
        const jobs = await api.jobs()
        if (cancelled) return
        const ids = new Set(data.accounts.map((a) => a.id))
        const mine = jobs.filter((j) => ids.has(j.account_id))
        if (mine.length) followJobs(mine, false)
      } catch (e) {
        toast.error(e instanceof Error ? e.message : "加载失败")
      }
    })()
    return () => {
      cancelled = true
      esRef.current?.close()
    }
  }, [id])

  function followJobs(nextJobs: Job[], reset = true) {
    esRef.current?.close()
    if (reset) setLogs([])
    setJobs(nextJobs)
    if (!nextJobs.length) return
    const ids = new Set(nextJobs.map((j) => j.id))
    const seen = new Set<string>()
    let reloadTimer = 0
    const scheduleReload = () => {
      if (reloadTimer) return
      reloadTimer = window.setTimeout(() => {
        reloadTimer = 0
        reload().catch(() => {})
      }, 800)
    }
    const tick = async () => {
      try {
        const all = await api.jobs()
        const mine = all.filter((j) => ids.has(j.id))
        setJobs(mine)
        const next: JobEvent[] = []
        for (const j of mine) {
          for (const ev of j.logs || []) {
            const key = `${ev.job_id}:${ev.time}:${ev.message}`
            if (seen.has(key)) continue
            seen.add(key)
            next.push(ev)
          }
        }
        if (next.length) {
          setLogs((cur) => [...cur, ...next].sort((a, b) => a.time - b.time))
          scheduleReload()
        }
        if (mine.length && mine.every((j) => j.status === "success" || j.status === "failed")) {
          window.clearInterval(poll)
          reload().catch(() => {})
        }
      } catch {
        /* 登录进行中接口偶发失败时下一秒再拉 */
      }
    }
    const poll = window.setInterval(() => {
      tick().catch(() => {})
    }, 1000)
    esRef.current = {
      close: () => {
        window.clearInterval(poll)
        if (reloadTimer) window.clearTimeout(reloadTimer)
      },
    }
    tick().catch(() => {})
    scheduleReload()
  }

  async function startWithAutoPay(autoPay: boolean) {
    if (!id || !payAsk) return
    setPayPending(true)
    try {
      if (payAsk.account) {
        setLogFilter(payAsk.account.id)
        const job =
          payAsk.mode === "refresh"
            ? await api.refreshAccount(payAsk.account.id, autoPay)
            : await api.loginAccount(payAsk.account.id, autoPay)
        followJobs([job])
      } else if (payAsk.mode === "refresh") {
        const jobs = await api.refreshBatch(id, autoPay)
        if (!jobs?.length) toast.message("这批还没有登录成功的账号，请先生成支付链接")
        else {
          toast.success(`开始刷新支付链接，共 ${jobs.length} 个账号${autoPay ? "，并自动支付" : ""}`)
          followJobs(jobs)
        }
      } else {
        const jobs = await api.loginBatch(id, autoPay)
        if (!jobs?.length) toast.message("这批账号都已经登录过了")
        else {
          toast.success(`开始登录并生成支付链接，共 ${jobs.length} 个账号${autoPay ? "，并自动支付" : ""}`)
          followJobs(jobs)
        }
      }
      setPayAsk(null)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "启动失败")
    } finally {
      setPayPending(false)
    }
  }

  async function confirmRemove() {
    if (!remove) return
    await api.deleteAccount(remove.id)
    setRemove(null)
    toast.success("已删除")
    await reload()
  }

  async function confirmRemoveRadar() {
    if (!id) return
    setRemovingRadar(true)
    try {
      const res = await api.deleteRadarDenied(id)
      setRemoveRadar(false)
      if (res.deleted === 0) {
        toast.message("这批没有 AuthKit Radar 拦截账户")
      } else {
        toast.success(`已删除 ${res.deleted} 个 AuthKit Radar 拦截账户`)
      }
      await reload()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "删除失败")
    } finally {
      setRemovingRadar(false)
    }
  }

  async function exportPay() {
    if (!id) return
    try {
      const res = await api.dispatchBatch(id)
      downloadBase64(res.filename || `${batch?.name || "batch"}-支付链接.xlsx`, res.xlsx, xlsxMime)
      toast.success(`已下载 ${res.count} 条，含账号和密码`)
      await reload()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "下载失败")
    }
  }

  async function markPaid() {
    if (!id) return
    try {
      await api.markBatchPaid(id)
      toast.success("开始扫描配额，有配额的会标成已支付")
      await reload()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "标记失败")
    }
  }

  return (
    <div className="space-y-6">
      <div>
        <Button variant="ghost" size="sm" asChild className="-ml-2 mb-1">
          <Link to="/automation">← 返回批次列表</Link>
        </Button>
        <h1 className="text-2xl font-semibold tracking-tight">{batch?.name || "批次"}</h1>
        {batch ? (
          <div className="mt-2 flex flex-wrap items-center gap-2">
            <span className={`inline-flex rounded-full px-2 py-0.5 text-xs font-medium ${batchStatus(batch).color}`}>
              {batchStatus(batch).label}
            </span>
            <span className="text-sm text-muted-foreground">
              {batch.pay_count}/{batch.total} 链接
              {batch.paid_count ? ` · ${batch.paid_count} 已支付` : ""}
              {batch.failed ? ` · ${batch.failed} 失败` : ""}
            </span>
          </div>
        ) : null}
      </div>

      <div className="flex flex-wrap gap-2">
        <Button
          variant={batch && batchStatus(batch).primary === "login" ? "default" : "outline"}
          disabled={!batch || !(waitingCount(batch) > 0 || batch.failed > 0)}
          onClick={() => setPayAsk({ mode: "login" })}
        >
          登录并生成支付链接
        </Button>
        <Button
          variant={batch && batchStatus(batch).primary === "refresh" ? "default" : "outline"}
          disabled={!batch?.unpaid_cookie_count}
          onClick={() => setPayAsk({ mode: "refresh" })}
        >
          刷新过期的支付链接
        </Button>
        <Button
          variant={batch && batchStatus(batch).primary === "download" ? "default" : "outline"}
          onClick={exportPay}
          disabled={!batch?.unpaid_pay_count}
        >
          下载 Excel
        </Button>
        <Button
          className={batch && batchStatus(batch).primary === "paid" && batch.unpaid_cookie_count ? "bg-violet-600 text-white hover:bg-violet-700" : ""}
          variant={batch && batchStatus(batch).primary === "paid" && batch.unpaid_cookie_count ? "default" : "outline"}
          onClick={markPaid}
          disabled={!batch?.unpaid_cookie_count}
        >
          {batch && batch.paid_count >= batch.total && batch.total ? "已付款" : "确认付款"}
        </Button>
        <Button
          variant="destructive"
          disabled={radarCount === 0 || removingRadar}
          onClick={() => setRemoveRadar(true)}
        >
          {radarCount
            ? `一键删除 AuthKit Radar 拦截账户（${radarCount}）`
            : "一键删除 AuthKit Radar 拦截账户"}
        </Button>
      </div>

      <AccountTable
        accounts={accounts}
        onLogin={(a) => setPayAsk({ mode: "login", account: a })}
        onRefresh={(a) => setPayAsk({ mode: "refresh", account: a })}
        onDetail={setDetail}
        onRemove={setRemove}
        currentSteps={currentSteps}
        selectedId={logFilter}
        onSelect={(a) => setLogFilter((cur) => (cur === a.id ? "" : a.id))}
      />

      <JobLogPanel
        logs={logs}
        emails={emails}
        statuses={jobStatuses}
        filterId={logFilter}
        onFilter={setLogFilter}
      />

      <AutoPayDialog
        open={!!payAsk}
        title={payAsk?.mode === "refresh" ? "刷新支付链接" : "登录并生成支付链接"}
        description={
          payAsk?.mode === "refresh"
            ? "用已有 Cookie 重新抽出支付链接。勾选自动支付后会立刻用 AmzKeys 虚拟卡去付 Stripe。"
            : "登录成功后抽出支付链接。勾选自动支付后不再下载 Excel，会开虚拟卡填 Stripe。"
        }
        pending={payPending}
        onOpenChange={(v) => !v && !payPending && setPayAsk(null)}
        onConfirm={startWithAutoPay}
      />

      <DetailDialog account={detail} open={!!detail} onOpenChange={(v) => !v && setDetail(null)} />

      <AlertDialog open={!!remove} onOpenChange={(v) => !v && setRemove(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除账号</AlertDialogTitle>
            <AlertDialogDescription>删除 {remove?.email}？</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction variant="destructive" onClick={confirmRemove}>
              删除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={removeRadar} onOpenChange={(v) => !v && !removingRadar && setRemoveRadar(false)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除 AuthKit Radar 拦截账户</AlertDialogTitle>
            <AlertDialogDescription>
              将删除本批 {radarCount} 个因 AuthKit Radar 拦截（policy_denied）失败的账户，其它账户不受影响。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={removingRadar}>取消</AlertDialogCancel>
            <AlertDialogAction variant="destructive" disabled={removingRadar} onClick={confirmRemoveRadar}>
              {removingRadar ? "删除中…" : "确认删除"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
