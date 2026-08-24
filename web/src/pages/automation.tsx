import { useEffect, useState } from "react"
import { ChevronLeft, ChevronRight } from "lucide-react"
import { toast } from "sonner"
import { api } from "@/lib/api"
import type { Batch } from "@/lib/types"
import { downloadBase64, xlsxMime } from "@/lib/download"
import { AddBatchDialog } from "@/components/accounts/add-batch-dialog"
import { BatchCard } from "@/components/accounts/batch-card"
import { Button } from "@/components/ui/button"
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

const PAGE_SIZE = 48

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
      downloadBase64(res.filename || `${b.name}-支付链接.xlsx`, res.xlsx, xlsxMime)
      toast.success(`已下载 ${res.count} 条，含账号和密码`)
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

      {batches.length === 0 ? (
        <div className="rounded-xl border bg-card px-4 py-16 text-center text-sm text-muted-foreground">
          还没有批次
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
          {batches.map((b) => (
            <BatchCard
              key={b.id}
              batch={b}
              onLogin={loginBatch}
              onRefresh={refreshBatch}
              onDownload={downloadForStaff}
              onPaid={markPaid}
              onRemove={setRemove}
            />
          ))}
        </div>
      )}

      <div className="flex items-center justify-between text-sm text-muted-foreground">
        <span>共 {total} 批</span>
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
