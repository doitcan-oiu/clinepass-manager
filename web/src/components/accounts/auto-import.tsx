import { useState } from "react"
import { Link } from "react-router-dom"
import { toast } from "sonner"
import { api } from "@/lib/api"
import type { AutoImportResult } from "@/lib/types"
import { Button } from "@/components/ui/button"

export function AutoImport({ batchId, disabled = false }: { batchId?: string; disabled?: boolean }) {
  const [pending, setPending] = useState(false)
  const [result, setResult] = useState<AutoImportResult | null>(null)
  const [error, setError] = useState("")

  async function importSuccess() {
    setPending(true)
    setResult(null)
    setError("")
    try {
      const got = await api.importAutoAccounts(batchId)
      setResult(got)
      toast.success(`已导入 ${got.imported} 个，更新 ${got.updated} 个，跳过 ${got.skipped} 个`)
    } catch (err) {
      setError(err instanceof Error ? err.message : "导入失败，请检查 Auto 连接后重试")
    } finally {
      setPending(false)
    }
  }

  return (
    <div className="space-y-3 rounded-xl border bg-card p-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="space-y-1">
          <p className="text-sm font-medium">将成功账号加入主程序</p>
          <p className="text-xs text-muted-foreground">{batchId ? "导入此批次" : "导入所有批次"}中已支付且提取完成的账号。已有账号会更新，不会重复创建。手动付款后请先检查支付状态。</p>
        </div>
        <Button disabled={pending || disabled} onClick={() => void importSuccess()} className="w-fit shrink-0">{pending ? "正在导入…" : "导入已成功账号"}</Button>
      </div>
      {result ? <div role="status" className="space-y-1 border-t pt-3 text-sm">
        <p>新增 <strong>{result.imported}</strong> 个 · 更新 <strong>{result.updated}</strong> 个 · 跳过 <strong>{result.skipped}</strong> 个 <Link to="/account" className="ml-2 text-primary underline underline-offset-4">查看账号池</Link></p>
        {result.warning ? <p className="text-amber-600 dark:text-amber-400">{result.warning}</p> : null}
      </div> : null}
      {error ? <p role="alert" className="border-t pt-3 text-sm text-destructive">{error}</p> : null}
    </div>
  )
}
