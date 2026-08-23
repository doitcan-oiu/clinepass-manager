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

function statusTone(label: string) {
  return (
    {
      已付款: "text-violet-600",
      部分付款: "text-violet-600",
      失败: "text-red-600",
      待生成: "text-amber-600",
      可下载: "text-emerald-600",
      已下载: "text-sky-600",
    } as Record<string, string>
  )[label] || "text-muted-foreground"
}

function linkTone(batch: Batch) {
  if (batch.unpaid_pay_count) return "text-amber-600"
  if (batch.pay_count === batch.total && batch.total) return "text-emerald-600"
  return "text-muted-foreground"
}

function Dot() {
  return <span className="text-border select-none">·</span>
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
          <p className="mt-1 flex min-w-0 items-baseline gap-1.5 overflow-hidden whitespace-nowrap text-[13px] leading-5">
            <span className={`shrink-0 font-medium ${statusTone(s.label)}`}>{s.label}</span>
            {batch.failed ? (
              <>
                <Dot />
                <span className="shrink-0 font-medium text-red-600">{batch.failed} 失败</span>
              </>
            ) : null}
            <Dot />
            <span className="shrink-0 tabular-nums text-muted-foreground">{formatTime(batch.created_at)}</span>
            <Dot />
            <span className={`shrink-0 tabular-nums font-medium ${linkTone(batch)}`}>
              {batch.pay_count}/{batch.total}
            </span>
            <span className="shrink-0 text-muted-foreground">链接</span>
            {batch.paid_count ? (
              <>
                <Dot />
                <span className="shrink-0 text-violet-600">{batch.paid_count} 已付</span>
              </>
            ) : null}
          </p>
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
