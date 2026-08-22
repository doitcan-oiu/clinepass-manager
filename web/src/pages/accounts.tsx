import { useEffect, useRef, useState } from "react"
import { ChartColumn, ChevronLeft, ChevronRight, Download, RefreshCw, Upload } from "lucide-react"
import { toast } from "sonner"
import { api } from "@/lib/api"
import { downloadJSON } from "@/lib/download"
import type { Batch, ModelSpend, PoolAccount, PoolStats, UsageSyncStatus } from "@/lib/types"
import { barClass, formatTime, health, modelTone, MONTHLY_USD, monthlyExpireLabel, pctText, poolLeftUSD, remainBarClass, remainText, usageRows } from "@/lib/quota"
import { AddPaidDialog } from "@/components/accounts/add-paid-dialog"
import { Button } from "@/components/ui/button"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog"

const PAGE_SIZE = 24

const emptySync: UsageSyncStatus = { running: false, total: 0, done: 0, fail: 0, paid: 0, unpaid: 0, message: "" }
const emptyStats: PoolStats = { total: 0, ok: 0, tight: 0, exhausted: 0, avg_rolling: null, avg_weekly: null, avg_monthly: null }

export function AccountsPage() {
  const [items, setItems] = useState<PoolAccount[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [batchId, setBatchId] = useState("")
  const [batches, setBatches] = useState<Batch[]>([])
  const [sync, setSync] = useState<UsageSyncStatus>(emptySync)
  const [stats, setStats] = useState<PoolStats>(emptyStats)
  const [modelsOf, setModelsOf] = useState<PoolAccount | null>(null)
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
      setTotal(again.total)
      setStats(again.stats || emptyStats)
    } else {
      setItems(pool.items)
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
    if (!sync.running) return
    const t = setInterval(() => {
      reload(page, batchId).catch(() => {})
    }, 2000)
    return () => clearInterval(t)
  }, [sync.running, page, batchId])

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

  async function refreshOne(a: PoolAccount) {
    setBusyId(a.id)
    try {
      const got = await api.refreshAccountUsage(a.id)
      setItems((cur) => cur.map((x) => (x.id === a.id ? { ...x, ...got } : x)))
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "刷新失败")
    } finally {
      setBusyId("")
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
        </div>
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1">
          <span className="text-[11px] uppercase tracking-wide text-muted-foreground">总额度(均值)</span>
          <MiniBar label="滚动" pct={stats.avg_rolling} on={barsOn} />
          <MiniBar label="每周" pct={stats.avg_weekly} on={barsOn} />
          <MiniBar label="每月" pct={stats.avg_monthly} on={barsOn} />
        </div>
      </div>

      {items.length === 0 ? (
        <div className="rounded-xl border border-dashed bg-card p-12 text-center text-muted-foreground">
          还没有已付款账号。在「提取支付链接」点确认付款后，有配额的号会出现在这里。
        </div>
      ) : (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4">
          {items.map((a) => {
            const synced = !!a.usage?.synced_at
            const h = health(a)
            const expire = monthlyExpireLabel(a.usage)
            return (
              <article
                key={a.id}
                className={`group relative flex flex-col rounded-xl border bg-card p-4 shadow-sm transition hover:-translate-y-0.5 hover:border-emerald-500/50 ${
                  h.label === "异常" ? "border-red-500/40" : ""
                }`}
              >
                <div className="mb-2 flex items-start gap-1">
                  <button
                    type="button"
                    className="min-w-0 flex-1 truncate text-left text-sm font-semibold hover:text-emerald-600"
                    onClick={() => setModelsOf(a)}
                    title={`${a.email} · 查看本月模型`}
                  >
                    {a.email}
                  </button>
                  <Button
                    size="icon-sm"
                    variant="ghost"
                    className="text-muted-foreground"
                    title="本月模型"
                    onClick={() => setModelsOf(a)}
                  >
                    <ChartColumn />
                  </Button>
                  <Button
                    size="icon-sm"
                    variant="ghost"
                    className="text-muted-foreground"
                    title="刷新用量"
                    disabled={busyId === a.id}
                    onClick={() => refreshOne(a)}
                  >
                    <RefreshCw className={busyId === a.id ? "animate-spin" : undefined} />
                  </Button>
                </div>
                <div className="mb-3 flex flex-wrap items-center gap-1">
                  <span className={`rounded-full px-2 py-0.5 text-[11px] font-semibold ${h.className}`}>{h.label}</span>
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
                <p className="mt-2 text-[11px] text-muted-foreground">{formatTime(a.usage?.synced_at || 0)}</p>
              </article>
            )
          })}
        </div>
      )}

      <div className="flex items-center justify-between text-sm text-muted-foreground">
        <span>共 {total} 个账号{sync.running && sync.message ? ` · ${sync.message}` : ""}</span>
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
