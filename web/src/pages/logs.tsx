import { useEffect, useState } from "react"
import { ChevronLeft, ChevronRight, RotateCw, Trash2 } from "lucide-react"
import { toast } from "sonner"
import { api } from "@/lib/api"
import type { RequestLog, RequestLogStats } from "@/lib/types"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
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

const PAGE_SIZES = [20, 50, 100]
const emptyStats: RequestLogStats = {
  rpm_1m: 0,
  tpm_1m: 0,
  rpm_5m: 0,
  tpm_5m: 0,
  requests_1h: 0,
  tokens_1h: 0,
  requests_24h: 0,
  tokens_24h: 0,
  success_1h: 0,
  error_1h: 0,
  processing: 0,
  models: [],
}

function nfmt(n: number) {
  return (n || 0).toLocaleString("en-US")
}

function fmtDur(ms: number) {
  if (!ms) return "—"
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

function fmtTs(ms: number) {
  if (!ms) return "—"
  const d = new Date(ms)
  const pad = (n: number) => String(n).padStart(2, "0")
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

function statusBadge(status: string) {
  if (status === "processing") return { label: "处理中", className: "bg-blue-500/15 text-blue-600" }
  if (status === "completed") return { label: "已完成", className: "bg-emerald-500/15 text-emerald-600" }
  return { label: "失败", className: "bg-red-500/15 text-red-600" }
}

export function LogsPage() {
  const [items, setItems] = useState<RequestLog[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [stats, setStats] = useState<RequestLogStats>(emptyStats)
  const [model, setModel] = useState("")
  const [email, setEmail] = useState("")
  const [emailQ, setEmailQ] = useState("")
  const [status, setStatus] = useState("")
  const [stream, setStream] = useState("")
  const [pageSize, setPageSize] = useState(50)
  const [clearOpen, setClearOpen] = useState(false)
  const [clearing, setClearing] = useState(false)
  const pages = Math.max(1, Math.ceil(total / pageSize))
  const from = total === 0 ? 0 : (page - 1) * pageSize + 1
  const to = Math.min(total, page * pageSize)

  useEffect(() => {
    const t = setTimeout(() => {
      setEmailQ(email.trim())
      setPage(1)
    }, 300)
    return () => clearTimeout(t)
  }, [email])

  async function reload(next = page) {
    const data = await api.logs(next, pageSize, { model, email: emailQ, status, stream })
    const last = Math.max(1, Math.ceil(data.total / pageSize))
    if (next > last) {
      setPage(last)
      const again = await api.logs(last, pageSize, { model, email: emailQ, status, stream })
      setItems(again.items)
      setTotal(again.total)
      setStats(again.stats || emptyStats)
      return
    }
    setItems(data.items)
    setTotal(data.total)
    setStats(data.stats || emptyStats)
  }

  useEffect(() => {
    reload(page).catch((e) => toast.error(e.message))
  }, [page, pageSize, model, emailQ, status, stream])

  useEffect(() => {
    const t = setInterval(() => {
      reload(page).catch(() => {})
    }, 2000)
    return () => clearInterval(t)
  }, [page, pageSize, model, emailQ, status, stream])

  async function clearAll() {
    setClearing(true)
    try {
      await api.clearLogs()
      setPage(1)
      setItems([])
      setTotal(0)
      setStats(emptyStats)
      toast.success("日志已清空")
      setClearOpen(false)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "清空失败")
    } finally {
      setClearing(false)
    }
  }

  const done1h = stats.success_1h + stats.error_1h
  const okRate = done1h ? (stats.success_1h / done1h) * 100 : 0

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">请求日志</h1>
          <p className="text-sm text-muted-foreground">按最近 1 分钟窗口统计 RPM / TPM，便于看转发吞吐。</p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <select
            className="h-9 rounded-lg border bg-card px-2 text-sm shadow-sm"
            value={model}
            onChange={(e) => {
              setModel(e.target.value)
              setPage(1)
            }}
          >
            <option value="">全部模型</option>
            {stats.models.map((m) => (
              <option key={m} value={m}>
                {m}
              </option>
            ))}
          </select>
          <select
            className="h-9 rounded-lg border bg-card px-2 text-sm shadow-sm"
            value={status}
            onChange={(e) => {
              setStatus(e.target.value)
              setPage(1)
            }}
          >
            <option value="">全部状态</option>
            <option value="processing">处理中</option>
            <option value="completed">已完成</option>
            <option value="error">失败</option>
          </select>
          <select
            className="h-9 rounded-lg border bg-card px-2 text-sm shadow-sm"
            value={stream}
            onChange={(e) => {
              setStream(e.target.value)
              setPage(1)
            }}
          >
            <option value="">流式/非流式</option>
            <option value="1">流式</option>
            <option value="0">非流式</option>
          </select>
          <Input
            className="h-9 w-48"
            placeholder="渠道邮箱"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
          <Button variant="outline" disabled={!total || clearing} onClick={() => setClearOpen(true)}>
            <Trash2 />
            清空全部
          </Button>
        </div>
      </div>

      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <StatCard label="RPM" hint="近 1 分钟请求" value={nfmt(stats.rpm_1m)} extra={`5 分钟均值 ${stats.rpm_5m.toFixed(1)}`} />
        <StatCard label="TPM" hint="近 1 分钟词元" value={nfmt(stats.tpm_1m)} extra={`5 分钟均值 ${nfmt(Math.round(stats.tpm_5m))}`} />
        <StatCard label="1 小时" hint="请求 / 词元" value={nfmt(stats.requests_1h)} extra={`词元 ${nfmt(stats.tokens_1h)} · 24 小时 ${nfmt(stats.requests_24h)} 次`} />
        <StatCard
          label="成功率"
          hint="近 1 小时"
          value={`${okRate.toFixed(1)}%`}
          extra={stats.processing ? `${stats.processing} 处理中` : `失败 ${stats.error_1h}`}
        />
      </div>

      {items.length === 0 ? (
        <div className="rounded-xl border border-dashed bg-card p-12 text-center text-muted-foreground">
          还没有转发记录。客户端打 `/v1/chat/completions` 等接口后会出现在这里。
        </div>
      ) : (
        <div className="overflow-hidden rounded-xl border bg-card shadow-sm">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>ID</TableHead>
                <TableHead>模型 ID</TableHead>
                <TableHead>API 格式</TableHead>
                <TableHead>流式</TableHead>
                <TableHead>渠道</TableHead>
                <TableHead>状态</TableHead>
                <TableHead>词元</TableHead>
                <TableHead>读缓存</TableHead>
                <TableHead>写缓存</TableHead>
                <TableHead>耗时</TableHead>
                <TableHead>创建时间</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((row) => {
                const st = statusBadge(row.status)
                const hit = row.input_tokens > 0 && row.cache_read > 0 ? (row.cache_read / row.input_tokens) * 100 : 0
                return (
                  <TableRow key={row.id}>
                    <TableCell className="font-mono text-muted-foreground">#{row.id}</TableCell>
                    <TableCell>
                      <div className="flex items-center gap-1.5">
                        <span className="rounded-md border border-orange-300/70 bg-orange-50 px-1.5 py-0.5 font-mono text-xs text-orange-800 dark:border-orange-500/40 dark:bg-orange-500/10 dark:text-orange-200">
                          {row.model || "—"}
                        </span>
                        {row.retries > 0 ? (
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span className="inline-flex text-orange-500">
                                <RotateCw className="size-3.5" />
                              </span>
                            </TooltipTrigger>
                            <TooltipContent>换号重试 {row.retries} 次</TooltipContent>
                          </Tooltip>
                        ) : null}
                      </div>
                    </TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">{row.api_format || "—"}</TableCell>
                    <TableCell>
                      {row.stream ? <span className="text-emerald-600">流式</span> : <span className="text-muted-foreground">非流式</span>}
                    </TableCell>
                    <TableCell className="max-w-[180px] truncate font-mono text-xs" title={row.account_email}>
                      {row.account_email || "—"}
                    </TableCell>
                    <TableCell>
                      {row.error ? (
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <span className={`inline-flex rounded-full px-2 py-0.5 text-[11px] font-semibold ${st.className}`}>{st.label}</span>
                          </TooltipTrigger>
                          <TooltipContent className="max-w-xs">{row.error}</TooltipContent>
                        </Tooltip>
                      ) : (
                        <span className={`inline-flex rounded-full px-2 py-0.5 text-[11px] font-semibold ${st.className}`}>{st.label}</span>
                      )}
                    </TableCell>
                    <TableCell>
                      {row.total_tokens ? (
                        <div className="leading-tight">
                          <div className="font-medium tabular-nums">总计 {nfmt(row.total_tokens)}</div>
                          <div className="text-[11px] text-muted-foreground">
                            输入 {nfmt(row.input_tokens)} | 输出 {nfmt(row.output_tokens)}
                            {row.reasoning_tokens ? ` · 推理 ${nfmt(row.reasoning_tokens)}` : ""}
                          </div>
                        </div>
                      ) : (
                        <span className="text-muted-foreground">—</span>
                      )}
                    </TableCell>
                    <TableCell>
                      {row.cache_read ? (
                        <div className="leading-tight">
                          <div className="tabular-nums">{nfmt(row.cache_read)}</div>
                          <div className="text-[11px] text-muted-foreground">{hit.toFixed(1)}% 命中</div>
                        </div>
                      ) : (
                        <span className="text-muted-foreground">—</span>
                      )}
                    </TableCell>
                    <TableCell className="tabular-nums">{row.cache_write ? nfmt(row.cache_write) : <span className="text-muted-foreground">—</span>}</TableCell>
                    <TableCell>
                      <div className="leading-tight">
                        <div className="tabular-nums">{fmtDur(row.duration_ms)}</div>
                        {row.stream && row.ttft_ms ? <div className="text-[11px] text-muted-foreground">TTFT {fmtDur(row.ttft_ms)}</div> : null}
                      </div>
                    </TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">{fmtTs(row.created_at)}</TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </div>
      )}

      <div className="flex flex-wrap items-center justify-between gap-3 text-sm text-muted-foreground">
        <span>
          {total ? `第 ${from}–${to} 条，共 ${total} 条` : "共 0 条"}
        </span>
        <div className="flex flex-wrap items-center gap-2">
          <select
            className="h-8 rounded-lg border bg-card px-2 text-sm shadow-sm"
            value={pageSize}
            onChange={(e) => {
              setPageSize(Number(e.target.value))
              setPage(1)
            }}
          >
            {PAGE_SIZES.map((n) => (
              <option key={n} value={n}>
                每页 {n} 条
              </option>
            ))}
          </select>
          <Button variant="outline" size="sm" disabled={page <= 1} onClick={() => setPage(1)}>
            首页
          </Button>
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
          <Button variant="outline" size="sm" disabled={page >= pages} onClick={() => setPage(pages)}>
            末页
          </Button>
        </div>
      </div>

      <AlertDialog open={clearOpen} onOpenChange={setClearOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>清空全部日志</AlertDialogTitle>
            <AlertDialogDescription>
              将删除全部 {total} 条请求日志，不可恢复。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={clearing}>取消</AlertDialogCancel>
            <AlertDialogAction variant="destructive" disabled={clearing} onClick={clearAll}>
              {clearing ? "清空中…" : "清空"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

function StatCard({ label, hint, value, extra }: { label: string; hint: string; value: string; extra: string }) {
  return (
    <div className="rounded-xl border bg-card px-4 py-3 shadow-sm">
      <div className="text-xs text-muted-foreground">{hint}</div>
      <div className="mt-1 flex items-baseline gap-2">
        <span className="text-2xl font-semibold tabular-nums tracking-tight">{value}</span>
        <span className="text-sm font-medium text-muted-foreground">{label}</span>
      </div>
      <div className="mt-1 text-xs text-muted-foreground">{extra}</div>
    </div>
  )
}
