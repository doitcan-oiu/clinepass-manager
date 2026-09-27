import { useEffect, useMemo, useRef, useState } from "react"
import { Link, useParams } from "react-router-dom"
import { toast } from "sonner"
import { api } from "@/lib/api"
import type { Account, Batch, Job } from "@/lib/types"
import { Check, CircleAlert, Clock3, LoaderCircle } from "lucide-react"
import { AccountTable } from "@/components/accounts/account-table"
import { AutoPayDialog } from "@/components/accounts/auto-pay-dialog"
import { AutoImport } from "@/components/accounts/auto-import"
import { AutoVerificationStatus, useAutoVerification } from "@/components/accounts/auto-verification"
import { DetailDialog } from "@/components/accounts/detail-dialog"
import { JobLogPanel } from "@/components/accounts/job-log-panel"
import { Button } from "@/components/ui/button"
import { batchStatus, radarDeniedCount, waitingCount } from "@/lib/batch-ui"
import { batchRunCounts, latestJobsByAccount, logsFromJobs } from "@/lib/job-log"
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
  const [jobs, setJobs] = useState<Job[]>([])
  const [now, setNow] = useState(Date.now())
  const [logFilter, setLogFilter] = useState("")
  const [loadError, setLoadError] = useState("")
  const routeRef = useRef(id)
  routeRef.current = id
  const revisionRef = useRef(0)
  const reloadRef = useRef(0)
  const radarCount = radarDeniedCount(accounts)
  const latestJobs = useMemo(() => latestJobsByAccount(jobs), [jobs])
  const logs = useMemo(() => logsFromJobs(jobs), [jobs])
  const counts = useMemo(() => batchRunCounts(accounts, latestJobs), [accounts, latestJobs])
  const verification = useAutoVerification(() => { void reload().catch(() => {}) })

  async function reload() {
    if (!id) return
    const request = ++reloadRef.current
    const revision = revisionRef.current
    let data
    try {
      data = await api.batch(id)
      if (routeRef.current !== id || request !== reloadRef.current || revision !== revisionRef.current) return
      setLoadError("")
    } catch (err) {
      if (routeRef.current !== id || request !== reloadRef.current || revision !== revisionRef.current) return
      setLoadError(err instanceof Error ? err.message : "无法读取远程批次")
      throw err
    }
    setBatch(data.batch)
    setAccounts(data.accounts)
    setDetail((cur) => (cur ? data.accounts.find((a) => a.id === cur.id) || null : null))
    setLogFilter((cur) => (cur && !data.accounts.some((a) => a.id === cur) ? "" : cur))
    return data
  }

  useEffect(() => {
    let cancelled = false
    let timer = 0
    let initialized = false
    setBatch(null)
    setAccounts([])
    setJobs([])
    setLogFilter("")
    setDetail(null)
    setPayAsk(null)
    setPayPending(false)
    setRemove(null)
    setLoadError("")
    revisionRef.current++
    const tick = async () => {
      const revision = revisionRef.current
      const request = ++reloadRef.current
      try {
        const [data, allJobs] = await Promise.all([api.batch(id!), api.jobs()])
        if (cancelled || routeRef.current !== id || revision !== revisionRef.current || request !== reloadRef.current) return
        const ids = new Set(data.accounts.map((a) => a.id))
        const mine = allJobs.filter((j) => ids.has(j.account_id))
        // Each response is a full snapshot. Replacing it retains repeat-count updates.
        setJobs(mine)
        setBatch(data.batch)
        setAccounts(data.accounts)
        setLoadError("")
        setDetail((current) => current ? data.accounts.find((a) => a.id === current.id) || null : null)
        const firstLoad = !initialized
        setLogFilter((current) => {
          if (current && ids.has(current)) return current
          if (firstLoad) return mine.find((j) => j.status === "running")?.account_id || data.accounts.find((a) => a.status === "failed")?.id || data.accounts[0]?.id || ""
          return ""
        })
        initialized = true
      } catch (e) {
        if (!cancelled && routeRef.current === id && revision === revisionRef.current && request === reloadRef.current) setLoadError(e instanceof Error ? e.message : "加载失败")
      } finally {
        if (!cancelled) timer = window.setTimeout(() => void tick(), 1500)
      }
    }
    if (id) void tick()
    const clock = window.setInterval(() => setNow(Date.now()), 1000)
    return () => {
      cancelled = true
      window.clearTimeout(timer)
      window.clearInterval(clock)
      revisionRef.current++
    }
  }, [id])

  function followJobs(nextJobs: Job[]) {
    revisionRef.current++
    const accountIDs = new Set(nextJobs.map((job) => job.account_id))
    setJobs((current) => [...current.filter((job) => !accountIDs.has(job.account_id)), ...nextJobs])
    setAccounts((current) => current.map((account) => accountIDs.has(account.id) ? { ...account, status: "queued", last_error: "" } : account))
    setLogFilter((current) => current || nextJobs[0]?.account_id || "")
  }

  async function startWithAutoPay(autoPay: boolean) {
    if (!id || !payAsk) return
    const requestBatch = id
    revisionRef.current++
    setPayPending(true)
    try {
      if (payAsk.account) {
        setLogFilter(payAsk.account.id)
        const job =
          payAsk.mode === "refresh"
            ? await api.refreshAccount(payAsk.account.id, autoPay)
            : await api.loginAccount(payAsk.account.id, autoPay)
        if (routeRef.current !== requestBatch) return
        followJobs([job])
      } else if (payAsk.mode === "refresh") {
        const jobs = await api.refreshBatch(id, autoPay)
        if (routeRef.current !== requestBatch) return
        if (!jobs?.length) toast.message("这批还没有登录成功的账号，请先生成支付链接")
        else {
          toast.success(`开始刷新支付链接，共 ${jobs.length} 个账号${autoPay ? "，并自动支付" : ""}`)
          followJobs(jobs)
        }
      } else {
        const jobs = await api.loginBatch(id, autoPay)
        if (routeRef.current !== requestBatch) return
        if (!jobs?.length) toast.message("这批账号都已经登录过了")
        else {
          toast.success(`开始登录并生成支付链接，共 ${jobs.length} 个账号${autoPay ? "，并自动支付" : ""}`)
          followJobs(jobs)
        }
      }
      setPayAsk(null)
    } catch (e) {
      if (routeRef.current === requestBatch) toast.error(e instanceof Error ? e.message : "启动失败")
    } finally {
      if (routeRef.current === requestBatch) setPayPending(false)
    }
  }

  async function confirmRemove() {
    if (!remove) return
    try {
      await api.deleteAutoAccount(remove.id)
      setRemove(null)
      toast.success("已从 Auto 删除")
      await reload()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "删除失败")
    }
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
      await verification.start(id)
      toast.success("Auto 正在检查订阅，确认已支付后可导入账号池")
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

      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4" aria-label="批次执行概览">
        {[
          { label: "待处理", value: counts.queued, icon: Clock3, color: "text-muted-foreground", background: "bg-muted" },
          { label: "进行中", value: counts.running, icon: LoaderCircle, color: "text-sky-600 dark:text-sky-400", background: "bg-sky-500/10" },
          { label: "已完成", value: counts.success, icon: Check, color: "text-emerald-600 dark:text-emerald-400", background: "bg-emerald-500/10" },
          { label: "需处理", value: counts.failed, icon: CircleAlert, color: "text-amber-600 dark:text-amber-400", background: "bg-amber-500/10" },
        ].map((item) => <div key={item.label} className="flex items-center justify-between rounded-xl border bg-card px-4 py-4 shadow-sm"><div><p className="text-xs text-muted-foreground">{item.label}</p><p className="mt-1.5 text-2xl font-semibold tabular-nums tracking-tight">{item.value}</p></div><span className={`rounded-lg p-2.5 ${item.background} ${item.color}`}><item.icon className="size-4" /></span></div>)}
      </div>

      {loadError ? <div role="alert" className="space-y-3 rounded-xl border border-destructive/25 bg-destructive/5 p-4"><p className="text-sm">{loadError}</p><div className="flex gap-2"><Button variant="outline" size="sm" onClick={() => void reload().catch(() => {})}>重新加载</Button><Button variant="outline" size="sm" asChild><Link to="/settings?tab=connection">配置 Auto 连接</Link></Button></div></div> : null}
      <AutoImport key={id} batchId={id} disabled={!batch || !!loadError || verification.busy} />
      <AutoVerificationStatus status={verification.status} error={verification.error} />

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
          disabled={!batch?.unpaid_cookie_count || verification.busy}
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
          disabled={!batch?.unpaid_cookie_count || verification.busy}
        >
          {verification.busy ? "正在检查支付状态…" : batch && batch.paid_count >= batch.total && batch.total ? "已付款" : "检查支付状态"}
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
        jobs={latestJobs}
        now={now}
        onLogin={(a) => setPayAsk({ mode: "login", account: a })}
        onRefresh={(a) => setPayAsk({ mode: "refresh", account: a })}
        onDetail={setDetail}
        onRemove={setRemove}
        selectedId={logFilter}
        onSelect={(a) => setLogFilter(a.id)}
      />

      <JobLogPanel
        logs={logs}
        accounts={accounts}
        jobs={latestJobs}
        now={now}
        filterId={logFilter}
        onFilter={setLogFilter}
        onRetry={(a) => setPayAsk({ mode: "login", account: a })}
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

      <DetailDialog account={detail} source="auto" open={!!detail} onOpenChange={(v) => !v && setDetail(null)} />

      <AlertDialog open={!!remove} onOpenChange={(v) => !v && setRemove(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除账号</AlertDialogTitle>
            <AlertDialogDescription>从 Auto 删除 {remove?.email}？已经导入主程序的账号会保留。</AlertDialogDescription>
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
