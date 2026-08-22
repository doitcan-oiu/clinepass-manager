import { useEffect, useRef, useState } from "react"
import { Link, useParams } from "react-router-dom"
import { toast } from "sonner"
import { api } from "@/lib/api"
import type { Account, Batch, Job, JobEvent } from "@/lib/types"
import { AccountTable } from "@/components/accounts/account-table"
import { DetailDialog } from "@/components/accounts/detail-dialog"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { batchStatus, waitingCount } from "@/lib/batch-ui"
import { downloadText } from "@/lib/download"
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
  const [logs, setLogs] = useState<JobEvent[]>([])
  const esRef = useRef<EventSource | null>(null)

  async function reload() {
    if (!id) return
    const data = await api.batch(id)
    setBatch(data.batch)
    setAccounts(data.accounts)
    setDetail((cur) => (cur ? data.accounts.find((a) => a.id === cur.id) || null : null))
  }

  useEffect(() => {
    reload().catch((e) => toast.error(e.message))
    return () => esRef.current?.close()
  }, [id])

  function followJobs(jobs: Job[]) {
    esRef.current?.close()
    setLogs([])
    let i = 0
    const run = () => {
      if (i >= jobs.length) {
        reload().catch(() => {})
        return
      }
      const job = jobs[i++]
      let done = false
      const finish = () => {
        if (done) return
        done = true
        es.close()
        reload().catch(() => {})
        run()
      }
      const es = new EventSource(`/api/jobs/${job.id}/events`)
      esRef.current = es
      es.onmessage = (ev) => {
        const data = JSON.parse(ev.data) as JobEvent
        setLogs((cur) => [...cur, data])
        if (data.level === "error" || /完成/.test(data.message || "")) finish()
      }
      es.onerror = () => finish()
    }
    run()
  }

  async function loginBatch() {
    if (!id) return
    try {
      const jobs = await api.loginBatch(id)
      if (!jobs?.length) {
        toast.message("这批账号都已经登录过了")
        return
      }
      toast.success(`开始登录并生成支付链接，共 ${jobs.length} 个账号`)
      followJobs(jobs)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "启动失败")
    }
  }

  async function refreshBatch() {
    if (!id) return
    try {
      const jobs = await api.refreshBatch(id)
      if (!jobs?.length) {
        toast.message("这批还没有登录成功的账号，请先生成支付链接")
        return
      }
      toast.success(`开始刷新支付链接，共 ${jobs.length} 个账号（不用重新登录）`)
      followJobs(jobs)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "启动失败")
    }
  }

  async function refreshOne(a: Account) {
    try {
      followJobs([await api.refreshAccount(a.id)])
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "刷新失败")
    }
  }

  async function loginOne(a: Account) {
    try {
      followJobs([await api.loginAccount(a.id)])
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "登录失败")
    }
  }

  async function confirmRemove() {
    if (!remove) return
    await api.deleteAccount(remove.id)
    setRemove(null)
    toast.success("已删除")
    await reload()
  }

  async function exportPay() {
    if (!id) return
    try {
      const res = await api.dispatchBatch(id)
      downloadText(`${batch?.name || "batch"}-支付链接.txt`, res.text)
      toast.success(`已下载 ${res.count} 条支付链接，把这个 txt 发给员工即可`)
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
          onClick={loginBatch}
        >
          登录并生成支付链接
        </Button>
        <Button
          variant={batch && batchStatus(batch).primary === "refresh" ? "default" : "outline"}
          disabled={!batch?.unpaid_cookie_count}
          onClick={refreshBatch}
        >
          刷新过期的支付链接
        </Button>
        <Button
          variant={batch && batchStatus(batch).primary === "download" ? "default" : "outline"}
          onClick={exportPay}
          disabled={!batch?.unpaid_pay_count}
        >
          下载支付链接.txt
        </Button>
        <Button
          className={batch && batchStatus(batch).primary === "paid" && batch.unpaid_cookie_count ? "bg-violet-600 text-white hover:bg-violet-700" : ""}
          variant={batch && batchStatus(batch).primary === "paid" && batch.unpaid_cookie_count ? "default" : "outline"}
          onClick={markPaid}
          disabled={!batch?.unpaid_cookie_count}
        >
          {batch && batch.paid_count >= batch.total && batch.total ? "已付款" : "确认付款"}
        </Button>
      </div>

      <AccountTable
        accounts={accounts}
        onLogin={loginOne}
        onRefresh={refreshOne}
        onDetail={setDetail}
        onRemove={setRemove}
      />

      <Card>
        <CardHeader>
          <CardTitle>任务日志</CardTitle>
        </CardHeader>
        <CardContent>
          <pre className="max-h-80 min-h-32 overflow-auto whitespace-pre-wrap font-mono text-xs text-muted-foreground">
            {logs.length
              ? logs.map((l) => `${new Date(l.time).toLocaleTimeString()} ${l.level === "error" ? "!" : "+"} ${l.message}`).join("\n")
              : "还没有运行记录。点上面的按钮开始生成或刷新支付链接。"}
          </pre>
        </CardContent>
      </Card>

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
    </div>
  )
}
