import { Link } from "react-router-dom"
import { MoreHorizontal } from "lucide-react"
import type { Batch } from "@/lib/types"
import { batchStatus, waitingCount } from "@/lib/batch-ui"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"

function formatTime(ts: number) {
  if (!ts) return "—"
  const d = new Date(ts * 1000)
  const pad = (n: number) => String(n).padStart(2, "0")
  return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

export function BatchCard({
  batch,
  onLogin,
  onRefresh,
  onDownload,
  onPaid,
  onRemove,
}: {
  batch: Batch
  onLogin: (batch: Batch) => void
  onRefresh: (batch: Batch) => void
  onDownload: (batch: Batch) => void
  onPaid: (batch: Batch) => void
  onRemove: (batch: Batch) => void
}) {
  const s = batchStatus(batch)
  const canLogin = waitingCount(batch) > 0 || batch.failed > 0

  return (
    <article className="flex flex-col gap-2.5 rounded-xl border bg-card p-3 shadow-sm">
      <div className="flex items-start gap-2">
        <div className="min-w-0 flex-1">
          <Link to={`/automation/${batch.id}`} className="block truncate font-medium hover:underline">
            {batch.name}
          </Link>
          <div className="mt-1 flex flex-wrap items-center gap-1.5">
            <span className={`inline-flex rounded-full px-1.5 py-0 text-[11px] font-medium leading-5 ${s.color}`}>{s.label}</span>
            {batch.failed ? (
              <span className="inline-flex rounded-full bg-red-600 px-1.5 py-0 text-[11px] font-medium leading-5 text-white">
                {batch.failed} 失败
              </span>
            ) : null}
            <span className="text-xs text-muted-foreground">{formatTime(batch.created_at)}</span>
          </div>
        </div>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon-xs" className="shrink-0">
              <MoreHorizontal />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem asChild>
              <Link to={`/automation/${batch.id}`}>查看账号</Link>
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem variant="destructive" onClick={() => onRemove(batch)}>
              删除
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      <p className="text-sm">
        <span
          className={
            batch.unpaid_pay_count
              ? "font-medium text-amber-600"
              : batch.pay_count === batch.total && batch.total
                ? "font-medium text-emerald-600"
                : "text-muted-foreground"
          }
        >
          {batch.pay_count}/{batch.total} 链接
        </span>
        {batch.paid_count ? <span className="ml-1.5 text-xs text-violet-600">{batch.paid_count} 已付</span> : null}
      </p>

      <div className="mt-auto flex flex-wrap gap-1">
        <Button size="xs" variant={s.primary === "login" ? "default" : "outline"} disabled={!canLogin} onClick={() => onLogin(batch)}>
          生成
        </Button>
        <Button size="xs" variant={s.primary === "refresh" ? "default" : "outline"} disabled={!batch.unpaid_cookie_count} onClick={() => onRefresh(batch)}>
          刷新
        </Button>
        <Button
          size="xs"
          className={s.primary === "download" ? "bg-emerald-600 text-white hover:bg-emerald-700" : ""}
          variant={s.primary === "download" ? "default" : "outline"}
          disabled={!batch.unpaid_pay_count}
          onClick={() => onDownload(batch)}
        >
          下载
        </Button>
        <Button
          size="xs"
          className={s.primary === "paid" && batch.unpaid_cookie_count ? "bg-violet-600 text-white hover:bg-violet-700" : ""}
          variant={s.primary === "paid" && batch.unpaid_cookie_count ? "default" : "outline"}
          disabled={!batch.unpaid_cookie_count}
          onClick={() => onPaid(batch)}
        >
          {batch.paid_count >= batch.total && batch.total ? "已付款" : "确认付款"}
        </Button>
      </div>
    </article>
  )
}
