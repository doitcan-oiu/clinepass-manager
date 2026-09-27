import { useEffect, useRef, useState } from "react"
import { api } from "@/lib/api"
import type { UsageSyncStatus } from "@/lib/types"

export function useAutoVerification(onComplete: () => void) {
  const [status, setStatus] = useState<UsageSyncStatus | null>(null)
  const [checkingBatch, setCheckingBatch] = useState("")
  const [error, setError] = useState("")
  const callback = useRef(onComplete)
  callback.current = onComplete

  useEffect(() => {
    let active = true
    api.autoUsageSync().then((next) => { if (active) setStatus(next) }).catch(() => {})
    return () => { active = false }
  }, [])

  useEffect(() => {
    if (!status?.running) return
    let active = true
    let busy = false
    const timer = window.setInterval(async () => {
      if (busy) return
      busy = true
      try {
        const next = await api.autoUsageSync()
        if (!active) return
        setStatus(next)
        setError("")
        if (!next.running) callback.current()
      } catch (err) {
        if (active) setError(err instanceof Error ? err.message : "无法读取支付检查进度")
      } finally {
        busy = false
      }
    }, 2000)
    return () => { active = false; window.clearInterval(timer) }
  }, [status?.running])

  async function start(batchId: string) {
    setCheckingBatch(batchId)
    setError("")
    try {
      const result = await api.markBatchPaid(batchId)
      setStatus(result.sync)
      if (!result.sync.running) callback.current()
      return result
    } finally {
      setCheckingBatch("")
    }
  }

  return { status, checkingBatch, error, start, busy: !!checkingBatch || !!status?.running }
}

export function AutoVerificationStatus({ status, error }: { status: UsageSyncStatus | null; error: string }) {
  if (!status?.running && !status?.finished_at && !error) return null
  return <div role={error ? "alert" : "status"} className="space-y-1 rounded-xl border bg-muted/20 p-4 text-sm">
    <p className="font-medium">{status?.running ? "Auto 正在检查支付状态" : "Auto 支付状态检查已完成"}</p>
    {status ? <p className="text-muted-foreground">已检查 {status.done} / {status.total} · 已支付 {status.paid} · 未支付 {status.unpaid} · 失败 {status.fail}{status.running ? "，完成后可导入成功账号。" : ""}</p> : null}
    {error ? <p className="text-destructive">{error}，正在尝试恢复连接。</p> : null}
  </div>
}
