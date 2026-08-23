import { useEffect, useRef } from "react"
import type { JobEvent } from "@/lib/types"
import { groupLogsByAccount, jobStatusLabel } from "@/lib/job-log"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"

function line(ev: JobEvent) {
  const mark = ev.level === "error" ? "!" : "+"
  return `${new Date(ev.time).toLocaleTimeString()} ${mark} ${ev.message}`
}

export function JobLogPanel({
  logs,
  emails,
  statuses,
  filterId,
  onFilter,
}: {
  logs: JobEvent[]
  emails: Record<string, string>
  statuses: Record<string, string>
  filterId: string
  onFilter: (accountId: string) => void
}) {
  const groups = groupLogsByAccount(logs, emails)
  const chips =
    filterId && !groups.some((g) => g.accountId === filterId)
      ? [{ accountId: filterId, email: emails[filterId] || "当前账号", events: [] as JobEvent[] }, ...groups]
      : groups
  const visible = filterId ? groups.filter((g) => g.accountId === filterId) : groups
  const scrollerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const el = scrollerRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [logs, filterId])

  return (
    <Card>
      <CardHeader className="space-y-3">
        <CardTitle>任务日志</CardTitle>
        {chips.length ? (
          <div className="flex flex-wrap gap-1.5">
            <Button size="xs" variant={filterId === "" ? "default" : "outline"} onClick={() => onFilter("")}>
              全部
            </Button>
            {chips.map((g) => (
              <Button
                key={g.accountId}
                size="xs"
                variant={filterId === g.accountId ? "default" : "outline"}
                className="max-w-[14rem] truncate"
                title={g.email}
                onClick={() => onFilter(filterId === g.accountId ? "" : g.accountId)}
              >
                {g.email}
              </Button>
            ))}
          </div>
        ) : null}
      </CardHeader>
      <CardContent>
        {!logs.length && !filterId ? (
          <p className="text-sm text-muted-foreground">
            还没有运行记录。点上面的按钮开始生成或刷新支付链接；刷新页面会自动接上正在跑的任务。点账号表一行，这里只看这个账号。
          </p>
        ) : visible.length === 0 ? (
          <p className="text-sm text-muted-foreground">这个账号还没有任务日志。</p>
        ) : (
          <div ref={scrollerRef} className="max-h-80 min-h-32 space-y-4 overflow-auto">
            {visible.map((g) => (
              <section key={g.accountId}>
                {!filterId ? (
                  <header className="mb-1 flex flex-wrap items-baseline gap-2">
                    <button
                      type="button"
                      className="truncate text-left text-sm font-medium hover:underline"
                      onClick={() => onFilter(g.accountId)}
                    >
                      {g.email}
                    </button>
                    {jobStatusLabel(statuses[g.accountId]) ? (
                      <span className="text-xs text-muted-foreground">{jobStatusLabel(statuses[g.accountId])}</span>
                    ) : null}
                  </header>
                ) : null}
                <pre className="whitespace-pre-wrap font-mono text-xs text-muted-foreground">
                  {g.events.map(line).join("\n")}
                </pre>
              </section>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
