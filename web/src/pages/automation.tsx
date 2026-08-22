import { useEffect, useState } from "react"
import { Link } from "react-router-dom"
import { ChevronLeft, ChevronRight, MoreHorizontal } from "lucide-react"
import { toast } from "sonner"
import { api } from "@/lib/api"
import type { Batch } from "@/lib/types"
import { batchStatus, waitingCount } from "@/lib/batch-ui"
import { downloadText } from "@/lib/download"
import { AddBatchDialog } from "@/components/accounts/add-batch-dialog"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
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

const PAGE_SIZE = 30

function formatTime(ts: number) {
  if (!ts) return "—"
  const d = new Date(ts * 1000)
  const pad = (n: number) => String(n).padStart(2, "0")
  return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

export function AutomationPage() {
  const [batches, setBatches] = useState<Batch[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [remove, setRemove] = useState<Batch | null>(null)
  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  async function reload(next = page) {
    const res = await api.batches(next, PAGE_SIZE)
    const last = Math.max(1, Math.ceil(res.total / PAGE_SIZE))
    if (next > last) {
      setPage(last)
      const again = await api.batches(last, PAGE_SIZE)
      setBatches(again.items)
      setTotal(again.total)
      return
    }
    setBatches(res.items)
    setTotal(res.total)
  }

  useEffect(() => {
    reload(page).catch((e) => toast.error(e.message))
  }, [page])

  async function confirmRemove() {
    if (!remove) return
    await api.deleteBatch(remove.id)
    setRemove(null)
    toast.success("已删除")
    await reload(page)
  }

  async function loginBatch(b: Batch) {
    try {
      const jobs = await api.loginBatch(b.id)
      if (!jobs?.length) toast.message("没有需要登录的账号")
      else toast.success(`开始生成 ${jobs.length} 条`)
      await reload(page)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "启动失败")
    }
  }

  async function refreshBatch(b: Batch) {
    try {
      const jobs = await api.refreshBatch(b.id)
      if (!jobs?.length) toast.message("没有可刷新的账号")
      else toast.success(`开始刷新 ${jobs.length} 条`)
      await reload(page)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "启动失败")
    }
  }

  async function downloadForStaff(b: Batch) {
    try {
      const res = await api.dispatchBatch(b.id)
      downloadText(`${b.name}-支付链接.txt`, res.text)
      toast.success(`已下载 ${res.count} 条`)
      await reload(page)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "下载失败")
    }
  }

  async function markPaid(b: Batch) {
    try {
      await api.markBatchPaid(b.id)
      toast.success(`${b.name} 开始扫描配额`)
      await reload(page)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "标记失败")
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-2xl font-semibold tracking-tight">支付链接</h1>
        <AddBatchDialog onSaved={() => { setPage(1); reload(1).catch((e) => toast.error(e.message)) }} />
      </div>

      <div className="overflow-hidden rounded-xl border bg-card shadow-sm">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>批次</TableHead>
              <TableHead>状态</TableHead>
              <TableHead>链接</TableHead>
              <TableHead>时间</TableHead>
              <TableHead className="text-right">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {batches.length === 0 ? (
              <TableRow>
                <TableCell colSpan={5} className="h-24 text-center text-muted-foreground">
                  还没有批次
                </TableCell>
              </TableRow>
            ) : (
              batches.map((b) => {
                const s = batchStatus(b)
                const canLogin = waitingCount(b) > 0 || b.failed > 0
                return (
                  <TableRow key={b.id}>
                    <TableCell>
                      <Link to={`/automation/${b.id}`} className="font-medium hover:underline">
                        {b.name}
                      </Link>
                    </TableCell>
                    <TableCell>
                      <span className={`inline-flex rounded-full px-2 py-0.5 text-xs font-medium ${s.color}`}>{s.label}</span>
                      {b.failed ? (
                        <span className="ml-1.5 inline-flex rounded-full bg-red-600 px-2 py-0.5 text-xs font-medium text-white">
                          {b.failed} 失败
                        </span>
                      ) : null}
                    </TableCell>
                    <TableCell>
                      <span className={b.unpaid_pay_count ? "font-medium text-amber-600" : b.pay_count === b.total && b.total ? "font-medium text-emerald-600" : "text-muted-foreground"}>
                        {b.pay_count}/{b.total}
                      </span>
                      {b.paid_count ? (
                        <span className="ml-1.5 text-xs text-violet-600">{b.paid_count} 已付</span>
                      ) : null}
                    </TableCell>
                    <TableCell className="whitespace-nowrap text-muted-foreground">{formatTime(b.created_at)}</TableCell>
                    <TableCell className="text-right">
                      <div className="flex flex-wrap items-center justify-end gap-1">
                        <Button
                          size="sm"
                          variant={s.primary === "login" ? "default" : "outline"}
                          disabled={!canLogin}
                          onClick={() => loginBatch(b)}
                        >
                          生成
                        </Button>
                        <Button
                          size="sm"
                          variant={s.primary === "refresh" ? "default" : "outline"}
                          disabled={!b.unpaid_cookie_count}
                          onClick={() => refreshBatch(b)}
                        >
                          刷新
                        </Button>
                        <Button
                          size="sm"
                          className={s.primary === "download" ? "bg-emerald-600 text-white hover:bg-emerald-700" : ""}
                          variant={s.primary === "download" ? "default" : "outline"}
                          disabled={!b.unpaid_pay_count}
                          onClick={() => downloadForStaff(b)}
                        >
                          下载
                        </Button>
                        <Button
                          size="sm"
                          className={s.primary === "paid" && b.unpaid_cookie_count ? "bg-violet-600 text-white hover:bg-violet-700" : ""}
                          variant={s.primary === "paid" && b.unpaid_cookie_count ? "default" : "outline"}
                          disabled={!b.unpaid_cookie_count}
                          onClick={() => markPaid(b)}
                        >
                          {b.paid_count >= b.total && b.total ? "已付款" : "确认付款"}
                        </Button>
                        <DropdownMenu>
                          <DropdownMenuTrigger asChild>
                            <Button variant="ghost" size="icon-sm">
                              <MoreHorizontal />
                            </Button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent align="end">
                            <DropdownMenuItem asChild>
                              <Link to={`/automation/${b.id}`}>查看账号</Link>
                            </DropdownMenuItem>
                            <DropdownMenuSeparator />
                            <DropdownMenuItem variant="destructive" onClick={() => setRemove(b)}>
                              删除
                            </DropdownMenuItem>
                          </DropdownMenuContent>
                        </DropdownMenu>
                      </div>
                    </TableCell>
                  </TableRow>
                )
              })
            )}
          </TableBody>
        </Table>
      </div>

      <div className="flex items-center justify-between text-sm text-muted-foreground">
        <span>
          共 {total} 批
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
            <AlertDialogTitle>删除批次</AlertDialogTitle>
            <AlertDialogDescription>
              删除 {remove?.name} 及其中 {remove?.total} 个账号？
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction variant="destructive" onClick={confirmRemove}>
              删除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
