import { useEffect, useRef, useState } from "react"
import { ChartColumn, ChevronDown, ChevronLeft, ChevronRight, Cookie, Download, Play, RefreshCw, Trash2, Upload } from "lucide-react"
import { toast } from "sonner"
import { api, type AccountTestResult } from "@/lib/api"
import { downloadJSON } from "@/lib/download"
import type { Batch, ModelSpend, PoolAccount, PoolStats, UsageSyncStatus, UsageWindow } from "@/lib/types"
import { barClass, formatRefreshAt, formatResetAt, health, isCookieExpired, modelTone, MONTHLY_USD, monthlyExpireLabel, pctText, poolLeftUSD, remainBarClass, remainText, shelfOf, usageRows, windowPct } from "@/lib/quota"
import { AddPaidDialog } from "@/components/accounts/add-paid-dialog"
import { loginProviderLabel, normalizeLoginProvider } from "@/lib/login-provider"
import { Button } from "@/components/ui/button"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
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

const PAGE_SIZE = 24

const emptySync: UsageSyncStatus = { running: false, total: 0, done: 0, fail: 0, paid: 0, unpaid: 0, message: "" }
const emptyStats: PoolStats = { total: 0, ok: 0, tight: 0, exhausted: 0, inflight: 0, avg_rolling: null, avg_weekly: null, avg_monthly: null }

export function AccountsPage() {
  const [items, setItems] = useState<PoolAccount[]>([])
  const [weeklyLimited, setWeeklyLimited] = useState<PoolAccount[]>([])
  const [rollingLimited, setRollingLimited] = useState<PoolAccount[]>([])
  const [cookieExpired, setCookieExpired] = useState<PoolAccount[]>([])
  const [weeklyOpen, setWeeklyOpen] = useState(false)
  const [rollingOpen, setRollingOpen] = useState(false)
  const [cookieOpen, setCookieOpen] = useState(false)
  const [testOf, setTestOf] = useState<PoolAccount | null>(null)
  const [testModels, setTestModels] = useState<{ id: string; name: string }[]>([])
  const [testModel, setTestModel] = useState("")
  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState<AccountTestResult | null>(null)
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [batchId, setBatchId] = useState("")
  const [batches, setBatches] = useState<Batch[]>([])
  const [sync, setSync] = useState<UsageSyncStatus>(emptySync)
  const [stats, setStats] = useState<PoolStats>(emptyStats)
  const [modelsOf, setModelsOf] = useState<PoolAccount | null>(null)
  const [remove, setRemove] = useState<PoolAccount | null>(null)
  const [busyId, setBusyId] = useState("")
  const [barsOn, setBarsOn] = useState(false)
  const [importing, setImporting] = useState(false)
  const fileRef = useRef<HTMLInputElement>(null)
  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  async function reload(next = page, filter = batchId) {
    const [pool, paid, st] = await Promise.all([
      api.poolAccounts(next, PAGE_SIZE, filter),
      api.paidBatches(),
      api.usageSync(),
    ])
    const last = Math.max(1, Math.ceil(pool.total / PAGE_SIZE))
    if (next > last) {
      setPage(last)
      const again = await api.poolAccounts(last, PAGE_SIZE, filter)
      setItems(again.items)
      setWeeklyLimited(again.weekly_limited || [])
      setRollingLimited(again.rolling_limited || [])
      setCookieExpired(again.cookie_expired || [])
      setTotal(again.total)
      setStats(again.stats || emptyStats)
    } else {
      setItems(pool.items)
      setWeeklyLimited(pool.weekly_limited || [])
      setRollingLimited(pool.rolling_limited || [])
      setCookieExpired(pool.cookie_expired || [])
      setTotal(pool.total)
      setStats(pool.stats || emptyStats)
    }
    setBatches(paid)
    setSync(st)
    requestAnimationFrame(() => setBarsOn(true))
  }

  useEffect(() => {
    setBarsOn(false)
    reload(page, batchId).catch((e) => toast.error(e.message))
  }, [page, batchId])

  useEffect(() => {
    const t = setInterval(() => {
      reload(page, batchId).catch(() => {})
    }, 3000)
    return () => clearInterval(t)
  }, [page, batchId])

  async function refreshAll() {
    try {
      await api.startUsageSync()
      toast.success("开始刷新用量")
      await reload(page, batchId)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "刷新失败")
    }
  }

  async function exportAll() {
    try {
      const data = await api.exportAccounts()
      downloadJSON("clinepass-backup.json", data)
      toast.success(`已导出 ${data.accounts?.length || 0} 个账号`)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "导出失败")
    }
  }

  async function importFile(file: File) {
    setImporting(true)
    try {
      const text = await file.text()
      const res = await api.importAccounts(text)
      const parts = [`新增 ${res.imported}`, `更新 ${res.updated}`]
      if (res.skipped?.length) parts.push(`跳过 ${res.skipped.length}`)
      toast.success(parts.join("，") + "，正在补齐用量")
      if (res.warning) toast.error(res.warning)
      if (res.skipped?.length) toast.message(res.skipped.slice(0, 5).join("；"))
      setPage(1)
      await reload(1, batchId)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "导入失败")
    } finally {
      setImporting(false)
      if (fileRef.current) fileRef.current.value = ""
    }
  }

  async function confirmRemove() {
    if (!remove) return
    try {
      await api.deleteAccount(remove.id)
      toast.success("已删除")
      setRemove(null)
      await reload(page, batchId)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "删除失败")
    }
  }

  async function refreshOne(a: PoolAccount) {
    setBusyId(a.id)
    try {
      const got = await api.refreshAccountUsage(a.id)
      const next = { ...a, ...got }
      if (shelfOf(a) !== shelfOf(next)) {
        await reload(page, batchId)
        return
      }
      setItems((cur) => cur.map((x) => (x.id === a.id ? next : x)))
      setWeeklyLimited((cur) => cur.map((x) => (x.id === a.id ? next : x)))
      setRollingLimited((cur) => cur.map((x) => (x.id === a.id ? next : x)))
      setCookieExpired((cur) => cur.map((x) => (x.id === a.id ? next : x)))
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "刷新失败")
    } finally {
      setBusyId("")
    }
  }

  async function markExpired(a: PoolAccount, expired: boolean) {
    setBusyId(a.id)
    try {
      await api.setCookieExpired(a.id, expired)
      toast.success(expired ? "已标记 Cookie 过期：仍按 Key 调度，滚动/周限以 429 为准" : "已清除 Cookie 过期标记")
      await reload(page, batchId)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "标记失败")
    } finally {
      setBusyId("")
    }
  }

  async function openTest(a: PoolAccount) {
    setTestOf(a)
    setTestResult(null)
    try {
      const got = await api.models()
      const list = got.models || []
      setTestModels(list)
      setTestModel((cur) => cur || list[0]?.id || "")
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "加载模型失败")
    }
  }

  async function runTest() {
    if (!testOf || !testModel) return
    setTesting(true)
    setTestResult(null)
    try {
      const got = await api.testAccount(testOf.id, testModel)
      setTestResult(got)
      if (got.ok) toast.success(`测试成功 ${got.latency_ms}ms`)
      else toast.error(got.error || "测试失败")
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "测试失败")
    } finally {
      setTesting(false)
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-2xl font-semibold tracking-tight">账号</h1>
        <div className="flex flex-wrap items-center gap-2">
          <select
            className="h-9 rounded-lg border bg-card px-2 text-sm shadow-sm"
            value={batchId}
            onChange={(e) => {
              setBatchId(e.target.value)
              setPage(1)
            }}
          >
            <option value="">全部批次</option>
            {batches.map((b) => (
              <option key={b.id} value={b.id}>
                {b.name}
              </option>
            ))}
          </select>
          <Button variant="outline" disabled={sync.running} onClick={refreshAll}>
            {sync.running ? `刷新中 ${sync.done}/${sync.total}` : "刷新用量"}
          </Button>
          <input
            ref={fileRef}
            type="file"
            accept="application/json,.json"
            className="hidden"
            onChange={(e) => {
              const f = e.target.files?.[0]
              if (f) importFile(f).catch((err) => toast.error(err.message))
            }}
          />
          <Button variant="outline" disabled={importing} onClick={() => fileRef.current?.click()}>
            <Upload />
            {importing ? "导入中" : "导入"}
          </Button>
          <Button variant="outline" onClick={exportAll}>
            <Download />
            导出
          </Button>
          <AddPaidDialog
            onSaved={() => {
              setPage(1)
              reload(1, batchId).catch((e) => toast.error(e.message))
            }}
          />
        </div>
      </div>

      <div className="flex flex-wrap items-center justify-between gap-x-6 gap-y-2 rounded-xl border bg-card px-4 py-2.5 text-xs shadow-sm">
        <div className="flex flex-wrap items-center gap-x-5 gap-y-1">
          <Stat label="总账号" value={stats.total} />
          <Stat label="正常" value={stats.ok} className="text-emerald-600" />
          <Stat label="紧张" value={stats.tight} className={stats.tight ? "text-amber-600" : ""} />
          <Stat label="用尽" value={stats.exhausted} className={stats.exhausted ? "text-red-600" : ""} />
          <Stat label="在途" value={stats.inflight || 0} className={stats.inflight ? "text-sky-600" : ""} />
        </div>
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1">
          <span className="text-[11px] uppercase tracking-wide text-muted-foreground">总额度(均值)</span>
          <MiniBar label="滚动" pct={stats.avg_rolling} on={barsOn} />
          <MiniBar label="每周" pct={stats.avg_weekly} on={barsOn} />
          <MiniBar label="每月" pct={stats.avg_monthly} on={barsOn} />
        </div>
        <div className="flex flex-wrap items-center gap-x-3 text-[11px] text-muted-foreground">
          <span>{sync.running ? `刷新中 ${sync.done}/${sync.total}` : formatRefreshAt(sync.finished_at || 0)}</span>
          <span>每 {sync.interval_sec || 60} 秒</span>
          <span>并发 {sync.concurrency || 10}</span>
        </div>
      </div>

      {items.length === 0 && weeklyLimited.length === 0 && rollingLimited.length === 0 && cookieExpired.length === 0 ? (
        <div className="rounded-xl border border-dashed bg-card p-12 text-center text-muted-foreground">
          还没有已付款账号。在「提取支付链接」点确认付款后，有配额的号会出现在这里。
        </div>
      ) : items.length === 0 ? (
        <div className="rounded-xl border border-dashed bg-card p-8 text-center text-muted-foreground">
          当前没有用量正常的账号。Cookie 过期的号仍可调度，额度已满的号收在下面。
        </div>
      ) : (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4">
          {items.map((a) => (
            <AccountCard
              key={a.id}
              a={a}
              barsOn={barsOn}
              busy={busyId === a.id}
              onModels={() => setModelsOf(a)}
              onTest={() => openTest(a)}
              onMarkExpired={() => markExpired(a, true)}
              onClearExpired={() => markExpired(a, false)}
              onRefresh={() => refreshOne(a)}
              onRemove={() => setRemove(a)}
            />
          ))}
        </div>
      )}

      {cookieExpired.length > 0 ? (
        <QuotaFold
          title="Cookie 已过期"
          hint="邮箱可能登不上了，仍按 API Key 调度；只有 429 带重置时间才冷却"
          pctLabel="滚动"
          accounts={cookieExpired}
          windowOf={(a) => a.usage?.rolling}
          open={cookieOpen}
          onOpenChange={setCookieOpen}
          busyId={busyId}
          onModels={setModelsOf}
          onTest={openTest}
          onClearExpired={(a) => markExpired(a, false)}
          onRefresh={refreshOne}
          onRemove={setRemove}
        />
      ) : null}

      {rollingLimited.length > 0 ? (
        <QuotaFold
          title="滚动冷却"
          hint="429 正文里的重置时间未到，不调度"
          pctLabel="滚动"
          accounts={rollingLimited}
          windowOf={(a) => a.usage?.rolling}
          open={rollingOpen}
          onOpenChange={setRollingOpen}
          busyId={busyId}
          onModels={setModelsOf}
          onTest={openTest}
          onMarkExpired={(a) => markExpired(a, true)}
          onRefresh={refreshOne}
          onRemove={setRemove}
        />
      ) : null}

      {weeklyLimited.length > 0 ? (
        <QuotaFold
          title="周限冷却"
          hint="按 429「resets in 3d 7h」这类剩余时间停车，不按用量百分比收起"
          pctLabel="周限"
          accounts={weeklyLimited}
          windowOf={(a) => a.usage?.weekly}
          open={weeklyOpen}
          onOpenChange={setWeeklyOpen}
          busyId={busyId}
          onModels={setModelsOf}
          onTest={openTest}
          onMarkExpired={(a) => markExpired(a, true)}
          onRefresh={refreshOne}
          onRemove={setRemove}
        />
      ) : null}

      <div className="flex items-center justify-between text-sm text-muted-foreground">
        <span>
          共 {stats.total} 个账号
          {cookieExpired.length ? ` · Cookie 过期 ${cookieExpired.length} 个` : ""}
          {rollingLimited.length ? ` · 滚动 ${rollingLimited.length} 个已收起` : ""}
          {weeklyLimited.length ? ` · 周限 ${weeklyLimited.length} 个已收起` : ""}
          {sync.running && sync.message ? ` · ${sync.message}` : ""}
        </span>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>
            <ChevronLeft />
            上一页
          </Button>
          <span className="tabular-nums">
            {page} / {pages}
          </span>
          <Button variant="outline" size="sm" disabled={page >= pages} onClick={() => setPage((p) => p + 1)}>
            下一页
            <ChevronRight />
          </Button>
        </div>
      </div>

      <AlertDialog open={!!remove} onOpenChange={(v) => !v && setRemove(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除账号</AlertDialogTitle>
            <AlertDialogDescription>删除 {remove?.email}？删除后无法从账号池调度。</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction variant="destructive" onClick={confirmRemove}>
              删除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <Dialog open={!!testOf} onOpenChange={(v) => !v && setTestOf(null)}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>测试 {testOf?.email}</DialogTitle>
          </DialogHeader>
          <div className="grid gap-2">
            <label className="text-sm text-muted-foreground" htmlFor="test-model">模型</label>
            <select
              id="test-model"
              className="h-9 rounded-lg border bg-card px-2 text-sm shadow-sm"
              value={testModel}
              onChange={(e) => setTestModel(e.target.value)}
            >
              {testModels.map((m) => (
                <option key={m.id} value={m.id}>
                  {m.name} · {m.id}
                </option>
              ))}
            </select>
            {testResult ? (
              <div className={`rounded-lg border px-3 py-2 text-xs ${testResult.ok ? "border-emerald-500/40 bg-emerald-500/10" : "border-red-500/40 bg-red-500/10"}`}>
                <p className="font-medium">{testResult.ok ? `成功 ${testResult.latency_ms}ms` : `失败 ${testResult.status || ""}`.trim()}</p>
                {testResult.content ? <p className="mt-1 whitespace-pre-wrap text-muted-foreground">{testResult.content}</p> : null}
                {testResult.error ? <p className="mt-1 text-red-600">{testResult.error}</p> : null}
              </div>
            ) : (
              <p className="text-xs text-muted-foreground">用这个账号的 API Key 直打上游，不走负载均衡。</p>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setTestOf(null)}>关闭</Button>
            <Button disabled={!testModel || testing} onClick={runTest}>
              {testing ? "测试中…" : "发送测试"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!modelsOf} onOpenChange={(v) => !v && setModelsOf(null)}>
        <DialogContent className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>{modelsOf?.email} 本月模型</DialogTitle>
          </DialogHeader>
          {modelsOf?.usage?.models?.length ? (
            <ModelMonthSummary models={modelsOf.usage.models} />
          ) : null}
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>模型</TableHead>
                <TableHead>已用</TableHead>
                <TableHead>占比</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {!modelsOf?.usage?.models?.length ? (
                <TableRow>
                  <TableCell colSpan={3} className="h-16 text-center text-muted-foreground">
                    {modelsOf?.usage?.error || "还没有模型用量"}
                  </TableCell>
                </TableRow>
              ) : (
                modelsOf.usage.models.map((m) => {
                  const totalUsed = modelsOf.usage.models.reduce((n, x) => n + x.usd, 0)
                  const share = totalUsed > 0 ? (m.usd / totalUsed) * 100 : 0
                  return (
                    <TableRow key={m.model}>
                      <TableCell className="font-mono text-xs">{m.model}</TableCell>
                      <TableCell>
                        <span className={`inline-flex rounded-full px-2 py-0.5 text-xs font-medium ${modelTone(m.usd, MONTHLY_USD)}`}>
                          ${m.usd.toFixed(2)}
                        </span>
                      </TableCell>
                      <TableCell>
                        <div className="min-w-24">
                          <div className="mb-0.5 font-mono text-xs text-muted-foreground">{share.toFixed(0)}%</div>
                          <div className="h-1.5 w-full overflow-hidden rounded-full bg-muted">
                            <div className="h-full rounded-full bg-emerald-500" style={{ width: `${Math.min(share, 100)}%` }} />
                          </div>
                        </div>
                      </TableCell>
                    </TableRow>
                  )
                })
              )}
            </TableBody>
          </Table>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function AccountCard({
  a,
  barsOn,
  busy,
  onModels,
  onTest,
  onMarkExpired,
  onClearExpired,
  onRefresh,
  onRemove,
}: {
  a: PoolAccount
  barsOn: boolean
  busy: boolean
  onModels: () => void
  onTest: () => void
  onMarkExpired: () => void
  onClearExpired: () => void
  onRefresh: () => void
  onRemove: () => void
}) {
  const synced = !!a.usage?.synced_at
  const h = health(a)
  const expire = monthlyExpireLabel(a.usage)
  const stale = isCookieExpired(a)
  return (
    <article
      className={`group relative flex flex-col rounded-xl border bg-card p-4 shadow-sm transition hover:-translate-y-0.5 hover:border-emerald-500/50 ${
        h.label === "异常" || stale ? "border-orange-500/40" : ""
      }`}
    >
      <div className="mb-2 flex items-start gap-1">
        <button
          type="button"
          className="min-w-0 flex-1 truncate text-left text-sm font-semibold hover:text-emerald-600"
          onClick={onModels}
          title={`${a.email} · 查看本月模型`}
        >
          {a.email}
        </button>
        <Button size="icon-sm" variant="ghost" className="text-muted-foreground" title="测试对话" disabled={busy || !a.api_key} onClick={onTest}>
          <Play />
        </Button>
        <Button size="icon-sm" variant="ghost" className="text-muted-foreground" title="本月模型" onClick={onModels}>
          <ChartColumn />
        </Button>
        <Button
          size="icon-sm"
          variant="ghost"
          className={stale ? "text-orange-600" : "text-muted-foreground"}
          title={stale ? "清除 Cookie 过期标记" : "标记 Cookie 已过期"}
          disabled={busy}
          onClick={stale ? onClearExpired : onMarkExpired}
        >
          <Cookie />
        </Button>
        <Button size="icon-sm" variant="ghost" className="text-muted-foreground" title="刷新用量" disabled={busy} onClick={onRefresh}>
          <RefreshCw className={busy ? "animate-spin" : undefined} />
        </Button>
        <Button size="icon-sm" variant="ghost" className="text-muted-foreground hover:text-destructive" title="删除账号" onClick={onRemove}>
          <Trash2 />
        </Button>
      </div>
      <div className="mb-3 flex flex-wrap items-center gap-1">
        <span className={`rounded-full px-2 py-0.5 text-[11px] font-semibold ${h.className}`}>{h.label}</span>
        <span className={`rounded-full px-2 py-0.5 text-[11px] font-medium ${normalizeLoginProvider(a.login_provider) === "microsoft" ? "bg-sky-100 text-sky-800 dark:bg-sky-950 dark:text-sky-200" : "bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-200"}`}>
          {loginProviderLabel(a.login_provider)}
        </span>
        <span className={`rounded-full px-2 py-0.5 text-[11px] font-medium ${(a.inflight || 0) > 0 ? "bg-sky-100 text-sky-800 dark:bg-sky-950 dark:text-sky-200" : "bg-muted text-muted-foreground"}`}>
          在途 {a.inflight || 0}
        </span>
        {a.batch_name ? <span className="rounded-full bg-muted px-2 py-0.5 text-[11px] text-muted-foreground">{a.batch_name}</span> : null}
        {expire ? (
          <span className="rounded-full bg-muted px-2 py-0.5 text-[11px] text-muted-foreground">
            到期 {expire}
          </span>
        ) : null}
      </div>
      <div className="space-y-2">
        {usageRows(a.usage, synced).map((row) => (
          <div key={row.key}>
            <div className="mb-0.5 flex items-center justify-between text-[11px]">
              <span className="text-muted-foreground">
                {row.label}
                {row.reset ? ` (${row.reset})` : ""}
              </span>
              <span className={`font-mono ${pctText(row.pct)}`}>{row.pct == null ? "—" : `${Math.round(row.pct)}%`}</span>
            </div>
            <div className="h-1.5 w-full overflow-hidden rounded-full bg-muted">
              <div
                className={`h-full rounded-full transition-[width] duration-700 ${barClass(row.pct)}`}
                style={{ width: `${barsOn && row.pct != null ? Math.min(row.pct, 100) : 0}%` }}
              />
            </div>
          </div>
        ))}
      </div>
      {a.usage?.error ? <p className="mt-2 truncate text-[11px] text-red-600" title={a.usage.error}>{a.usage.error}</p> : null}
      <p className="mt-2 text-[11px] text-muted-foreground">{formatRefreshAt(a.usage?.synced_at || 0)}</p>
    </article>
  )
}

function QuotaFold({
  title,
  hint,
  pctLabel,
  accounts,
  windowOf,
  open,
  onOpenChange,
  busyId,
  onModels,
  onTest,
  onMarkExpired,
  onClearExpired,
  onRefresh,
  onRemove,
}: {
  title: string
  hint?: string
  pctLabel: string
  accounts: PoolAccount[]
  windowOf: (a: PoolAccount) => UsageWindow | undefined
  open: boolean
  onOpenChange: (open: boolean) => void
  busyId: string
  onModels: (a: PoolAccount) => void
  onTest?: (a: PoolAccount) => void
  onMarkExpired?: (a: PoolAccount) => void
  onClearExpired?: (a: PoolAccount) => void
  onRefresh: (a: PoolAccount) => void
  onRemove: (a: PoolAccount) => void
}) {
  return (
    <div className="overflow-hidden rounded-xl border bg-card shadow-sm">
      <button
        type="button"
        className="flex w-full items-center justify-between gap-3 px-4 py-3 text-left"
        onClick={() => onOpenChange(!open)}
        aria-expanded={open}
      >
        <span className="text-sm font-medium">
          {title}
          <span className="ml-2 text-muted-foreground">{accounts.length} 个账号{hint ? `，${hint}` : ""}</span>
        </span>
        <ChevronDown className={`size-4 text-muted-foreground transition-transform ${open ? "rotate-180" : ""}`} />
      </button>
      {open ? (
        <ul className="divide-y border-t">
          {accounts.map((a) => {
            const w = windowOf(a)
            const pct = windowPct(w, !!a.usage?.synced_at)
            const reset = formatResetAt(w?.reset_in_sec, a.usage?.synced_at || 0)
            return (
              <li key={a.id} className="flex items-center gap-3 px-4 py-2.5">
                <button
                  type="button"
                  className="min-w-0 flex-1 truncate text-left text-sm hover:text-emerald-600"
                  onClick={() => onModels(a)}
                  title={a.email}
                >
                  {a.email}
                </button>
                <span className="hidden text-[11px] text-muted-foreground sm:inline">{a.batch_name || ""}</span>
                <span className={`shrink-0 font-mono text-[11px] ${(a.inflight || 0) > 0 ? "text-sky-600" : "text-muted-foreground"}`}>在途 {a.inflight || 0}</span>
                <span className={`font-mono text-[11px] ${pctText(pct)}`}>{pct == null ? "—" : `${pctLabel} ${Math.round(pct)}%`}</span>
                <span className="hidden w-36 text-right text-[11px] text-muted-foreground lg:inline">{formatRefreshAt(a.usage?.synced_at || 0)}</span>
                <span className="hidden w-28 text-right text-[11px] text-muted-foreground md:inline">{reset ? `重置 ${reset}` : ""}</span>
                {onTest ? (
                  <Button size="icon-sm" variant="ghost" className="text-muted-foreground" title="测试对话" disabled={busyId === a.id || !a.api_key} onClick={() => onTest(a)}>
                    <Play />
                  </Button>
                ) : null}
                {onMarkExpired && !isCookieExpired(a) ? (
                  <Button size="icon-sm" variant="ghost" className="text-muted-foreground" title="标记 Cookie 已过期" disabled={busyId === a.id} onClick={() => onMarkExpired(a)}>
                    <Cookie />
                  </Button>
                ) : null}
                {onClearExpired && isCookieExpired(a) ? (
                  <Button size="icon-sm" variant="ghost" className="text-orange-600" title="清除 Cookie 过期标记" disabled={busyId === a.id} onClick={() => onClearExpired(a)}>
                    <Cookie />
                  </Button>
                ) : null}
                <Button size="icon-sm" variant="ghost" className="text-muted-foreground" title="刷新用量" disabled={busyId === a.id} onClick={() => onRefresh(a)}>
                  <RefreshCw className={busyId === a.id ? "animate-spin" : undefined} />
                </Button>
                <Button size="icon-sm" variant="ghost" className="text-muted-foreground hover:text-destructive" title="删除账号" onClick={() => onRemove(a)}>
                  <Trash2 />
                </Button>
              </li>
            )
          })}
        </ul>
      ) : null}
    </div>
  )
}

function Stat({ label, value, className = "" }: { label: string; value: number; className?: string }) {
  return (
    <span className="flex items-center gap-1.5">
      <span className="text-muted-foreground">{label}</span>
      <span className={`font-mono text-sm font-semibold ${className}`}>{value}</span>
    </span>
  )
}

function MiniBar({ label, pct, on }: { label: string; pct: number | null; on: boolean }) {
  const n = pct == null ? null : Math.round(pct)
  return (
    <div className="flex items-center gap-1.5">
      <span className="text-[11px] text-muted-foreground">{label}</span>
      <div className="h-1.5 w-14 overflow-hidden rounded-full bg-muted">
        <div className={`h-full rounded-full ${barClass(n)}`} style={{ width: `${on && n != null ? Math.min(n, 100) : 0}%` }} />
      </div>
      <span className={`w-8 font-mono text-[11px] ${pctText(n)}`}>{n == null ? "—" : `${n}%`}</span>
    </div>
  )
}

function ModelMonthSummary({ models }: { models: ModelSpend[] }) {
  const used = models.reduce((n, m) => n + m.usd, 0)
  const left = poolLeftUSD(used)
  const remain = (left / MONTHLY_USD) * 100
  return (
    <div className="rounded-lg border bg-muted/40 px-3 py-2 text-xs">
      <div className="mb-1 flex items-center justify-between">
        <span className="text-muted-foreground">月配额 ${MONTHLY_USD}</span>
        <span className={`font-mono ${remainText(remain)}`}>
          已用 ${used.toFixed(2)} · 剩余 ${left.toFixed(2)}
        </span>
      </div>
      <div className="h-1.5 w-full overflow-hidden rounded-full bg-muted">
        <div className={`h-full rounded-full ${remainBarClass(remain)}`} style={{ width: `${Math.min(remain, 100)}%` }} />
      </div>
    </div>
  )
}
