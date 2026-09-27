import { useEffect, useId, useRef, useState, type ReactNode } from "react"
import { Activity, ArrowUpRight, CircleDollarSign, Gauge, Info, RefreshCw, Type, Users } from "lucide-react"
import { Link } from "react-router-dom"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { fetchDashboard, type DashboardData, type DashboardRange } from "@/lib/dashboard"
import { cn } from "@/lib/utils"

const ranges: DashboardRange[] = ["24h", "7d", "30d", "90d"]
const colors = ["#0ea5e9", "#10b981", "#8b5cf6", "#f59e0b"]
const fmt = (n: number) => n.toLocaleString("en-US")
const compact = (n: number) => Intl.NumberFormat("zh-CN", { notation: "compact", maximumFractionDigits: 1 }).format(n)
const money = (n: number | null) => n === null ? "—" : n.toLocaleString("en-US", { style: "currency", currency: "USD" })
const percent = (n: number) => `${n.toFixed(1).replace(/\.0$/, "")}%`
const panel = "rounded-2xl bg-card p-5 shadow-xs ring-1 ring-foreground/5 sm:p-6"

function InfoTip({ children }: { children: ReactNode }) {
  return <Tooltip><TooltipTrigger asChild><button type="button" aria-label="查看统计说明" className="rounded text-muted-foreground hover:text-foreground"><Info className="size-3.5" /></button></TooltipTrigger><TooltipContent className="max-w-80">{children}</TooltipContent></Tooltip>
}
function Metric({ title, value, detail, icon, note }: { title: string; value: string; detail: ReactNode; icon: ReactNode; note?: string }) {
  return <section className={cn(panel, "@container min-w-0 sm:p-5")}>
    <div className="flex items-center justify-between gap-2 text-sm text-muted-foreground"><span className="flex items-center gap-1.5">{title}{note && <InfoTip>{note}</InfoTip>}</span>{icon}</div>
    <p className="mt-5 text-[clamp(1.125rem,12cqw,2rem)] leading-none font-medium tracking-tight tabular-nums">{value}</p>
    <div className="mt-3 text-xs leading-relaxed text-muted-foreground">{detail}</div>
  </section>
}

function UsageChart({ data }: { data: DashboardData }) {
  const [mode, setMode] = useState<"tokens" | "cost">("tokens")
  const [hovered, setHovered] = useState<number | null>(null)
  const gradientID = useId().replace(/:/g, "")
  const points = data.timeline
  const showCost = mode === "cost" && data.billing_available
  const primary = points.map((p) => showCost ? p.cost_usd || 0 : p.total_tokens)
  const maxLeft = Math.max(1, ...primary) * 1.1
  const maxRight = Math.max(1, ...points.map((p) => p.requests)) * 1.1
  const width = 760, height = 294, left = 56, right = 54, top = 20, bottom = 38
  const plotW = width - left - right, plotH = height - top - bottom
  const x = (i: number) => left + i / Math.max(1, points.length - 1) * plotW
  const y = (v: number, max: number) => top + plotH * (1 - v / max)
  const path = primary.map((v, i) => `${i ? "L" : "M"}${x(i)},${y(v, maxLeft)}`).join(" ")
  const requestsPath = points.map((p, i) => `${i ? "L" : "M"}${x(i)},${y(p.requests, maxRight)}`).join(" ")
  const selected = hovered === null ? null : points[hovered]
  const timeLabel = (timestamp: number, full = false) => new Date(timestamp).toLocaleString("zh-CN", { timeZone: "UTC", month: "numeric", day: "numeric", ...(data.range === "24h" || full ? { hour: "2-digit", minute: "2-digit", hour12: false } : {}) })
  const ticks = [...new Set(Array.from({ length: 6 }, (_, i) => Math.round(i / 5 * Math.max(0, points.length - 1))))]
  return <section className={cn(panel, "flex min-w-0 flex-col")}>
    <div className="flex flex-wrap items-center justify-between gap-3"><h2 className="font-medium">用量趋势</h2><div className="flex rounded-lg bg-muted/65 p-0.5" aria-label="趋势指标">
      <button onClick={() => setMode("tokens")} aria-pressed={!showCost} className={cn("rounded-md px-3 py-1 text-xs", !showCost ? "bg-card shadow-xs" : "text-muted-foreground")}>Tokens</button>
      <button onClick={() => setMode("cost")} disabled={!data.billing_available} aria-pressed={showCost} title={data.billing_note} className={cn("rounded-md px-3 py-1 text-xs disabled:opacity-40", showCost ? "bg-card shadow-xs" : "text-muted-foreground")}>计费</button>
    </div></div>
    <div className="mt-6 flex items-center justify-between text-xs text-muted-foreground"><span>{showCost ? "计费 / USD" : "Tokens"}</span><span>请求数</span></div>
    <div className="relative mt-1 min-h-52 flex-1">
      <svg viewBox={`0 0 ${width} ${height}`} className="h-full min-h-52 w-full overflow-visible" role="group" tabIndex={0} aria-label={`${data.range} 用量趋势，左轴${showCost ? "计费" : "Tokens"}，右轴请求数。使用左右方向键查看各时段。`} onFocus={() => setHovered(points.length - 1)} onBlur={() => setHovered(null)} onKeyDown={(e) => { if (e.key === "ArrowLeft" || e.key === "ArrowRight") { e.preventDefault(); setHovered((i) => Math.max(0, Math.min(points.length - 1, (i ?? points.length - 1) + (e.key === "ArrowRight" ? 1 : -1)))) } if (e.key === "Escape") setHovered(null) }} onMouseLeave={() => setHovered(null)}>
        <defs><linearGradient id={gradientID} x1="0" y1="0" x2="0" y2="1"><stop offset="0%" stopColor={showCost ? "#10b981" : "#0ea5e9"} stopOpacity=".2" /><stop offset="100%" stopColor={showCost ? "#10b981" : "#0ea5e9"} stopOpacity=".015" /></linearGradient></defs>
        {[0, 1, 2, 3, 4].map((step) => <g key={step}><line x1={left} x2={width - right} y1={y(step / 4 * maxLeft, maxLeft)} y2={y(step / 4 * maxLeft, maxLeft)} stroke="currentColor" strokeOpacity=".1" strokeDasharray="3 5" /><text x={left - 12} y={y(step / 4 * maxLeft, maxLeft) + 4} textAnchor="end" fontSize="11" className="fill-muted-foreground">{showCost ? `$${(step / 4 * maxLeft).toFixed(2)}` : compact(Math.round(step / 4 * maxLeft))}</text><text x={width - right + 12} y={y(step / 4 * maxLeft, maxLeft) + 4} fontSize="11" className="fill-muted-foreground">{compact(Math.round(step / 4 * maxRight))}</text></g>)}
        {points.length > 0 && <>{showCost ? points.map((p, i) => p.cost_usd !== null && <rect key={p.timestamp} x={x(i) - 3} y={y(p.cost_usd, maxLeft)} width={6} height={Math.max(1, top + plotH - y(p.cost_usd, maxLeft))} rx={2} fill="#10b981" fillOpacity=".6" />) : <><path d={`${path} L${x(points.length - 1)},${top + plotH} L${left},${top + plotH} Z`} fill={`url(#${gradientID})`} /><path d={path} stroke="#0ea5e9" fill="none" strokeWidth="2.5" strokeLinejoin="round" /></>}<path d={requestsPath} stroke="#a78bfa" fill="none" strokeWidth="1.7" strokeDasharray="5 5" strokeLinejoin="round" /></>}
        {ticks.map((i) => points[i] && <text key={i} x={x(i)} y={height - 10} textAnchor="middle" className="fill-muted-foreground" fontSize="11">{timeLabel(points[i].timestamp)}</text>)}
        {points.map((p, i) => <rect key={p.timestamp} x={Math.max(left, x(i) - plotW / Math.max(1, points.length - 1) / 2)} y={top} width={plotW / Math.max(1, points.length - 1)} height={plotH} fill="transparent" aria-hidden="true" onMouseEnter={() => setHovered(i)} />)}
        {hovered !== null && points[hovered] && <g pointerEvents="none"><line x1={x(hovered)} x2={x(hovered)} y1={top} y2={top + plotH} stroke="currentColor" strokeOpacity=".25" strokeDasharray="3 3" />{(!showCost || points[hovered].cost_usd !== null) && <circle cx={x(hovered)} cy={y(primary[hovered], maxLeft)} r="4" fill={showCost ? "#10b981" : "#0ea5e9"} stroke="var(--card)" strokeWidth="2" />}</g>}
      </svg>
      {selected && <div aria-live="polite" className="pointer-events-none absolute top-0 right-2 z-10 rounded-lg border bg-popover p-3 text-xs shadow-lg"><p className="mb-2 font-medium">{timeLabel(selected.timestamp, true)} UTC</p><p className="text-sky-600 dark:text-sky-400">{fmt(selected.total_tokens)} Tokens</p><p className="mt-1 text-violet-500">{fmt(selected.requests)} 次请求</p>{selected.cost_usd !== null && <p className="mt-1 text-emerald-600 dark:text-emerald-400">计费 {money(selected.cost_usd)}</p>}</div>}
      {data.summary.requests === 0 && !showCost && <div className="pointer-events-none absolute inset-x-0 top-1/3 text-center text-sm text-muted-foreground">所选时段暂无请求</div>}
    </div>
    <div className="mt-4 flex justify-center gap-6 text-xs text-muted-foreground"><span className="flex items-center gap-2"><i className="h-0.5 w-4" style={{ background: showCost ? "#10b981" : "#0ea5e9" }} />{showCost ? "计费" : "Tokens"} · 左轴</span><span className="flex items-center gap-2"><i className="w-4 border-t-2 border-dashed border-violet-400" />请求 · 右轴</span></div>
  </section>
}

function ChannelDistribution({ data }: { data: DashboardData }) {
  const { channels, summary } = data
  return <section className={cn(panel, "min-w-0")}>
    <div className="flex items-center justify-between gap-3"><h2 className="flex items-center gap-2 font-medium">渠道分布<InfoTip>按实际转发接口分组；成功率为成功请求占全部请求（含处理中）的比例。</InfoTip></h2><div className="text-lg tabular-nums">{summary.requests ? percent(summary.success_rate) : "—"}<span className="ml-2 text-xs text-muted-foreground">成功率</span></div></div>
    <div className="my-7 flex h-12 gap-1" aria-label="各渠道请求占比">{Array.from({ length: 48 }, (_, i) => {
      let cumulative = 0
      const channel = channels.findIndex((item) => { cumulative += item.share; return (i + .5) / 48 * 100 <= cumulative })
      return <span key={i} className="min-w-0 flex-1 rounded-xs bg-muted" style={channel >= 0 && summary.requests > 0 ? { background: colors[channel % colors.length] } : undefined} />
    })}</div>
    {channels.length ? <div className="divide-y divide-border/55">{channels.map((item, i) => <div key={item.id} className="flex items-center gap-3 py-4 first:pt-0 last:pb-0"><span className="size-2 shrink-0 rounded-full" style={{ background: colors[i % colors.length] }} /><div className="min-w-0 flex-1"><p className="truncate text-sm">{item.name}</p><p className="mt-1 text-xs text-muted-foreground">成功率 {percent(item.success_rate)} · {compact(item.total_tokens)} Tokens</p></div><div className="text-right text-sm tabular-nums"><p>{fmt(item.requests)}</p><p className="mt-1 text-xs text-muted-foreground">{percent(item.share)}</p></div></div>)}</div> : <p className="py-12 text-center text-sm text-muted-foreground">转发请求后，这里会显示渠道分布</p>}
  </section>
}

function ModelRanking({ data }: { data: DashboardData }) {
  return <section className={cn(panel, "min-w-0")}>
    <div className="flex flex-wrap items-center justify-between gap-3"><h2 className="font-medium">{data.billing_available ? "模型计费" : "模型用量"} Top 10</h2><span className="text-xs text-muted-foreground">{data.billing_available ? "按已同步计费排序" : "按 Tokens 排序"}</span></div>
    <div className="mt-6 overflow-x-auto"><table className="w-full min-w-[460px] text-sm"><thead><tr className="border-b border-border/60 text-xs text-muted-foreground"><th className="pb-3 text-left font-normal">模型</th><th className="pb-3 text-right font-normal text-emerald-600 dark:text-emerald-400">计费</th><th className="pb-3 text-right font-normal text-violet-500">Tokens</th><th className="pb-3 text-right font-normal text-sky-600 dark:text-sky-400">请求</th></tr></thead>
      <tbody>{data.models.map((m) => <tr key={m.id} className="border-b border-border/50 last:border-0"><td className="py-4 pr-4"><p className="font-medium">{m.id.replace(/^cline-pass\//, "")}</p><p className="mt-1.5 max-w-100 text-[11px] leading-relaxed text-muted-foreground">输入 {compact(m.input_tokens)} · 输出 {compact(m.output_tokens)}{m.cache_read > 0 && ` · 缓存 ${compact(m.cache_read)}`}{m.reasoning_tokens > 0 && ` · 推理 ${compact(m.reasoning_tokens)}`}</p></td><td className="py-4 pl-3 text-right text-emerald-600 tabular-nums dark:text-emerald-400">{money(m.cost_usd)}</td><td className="py-4 pl-5 text-right text-violet-500 tabular-nums">{compact(m.total_tokens)}</td><td className="py-4 pl-5 text-right text-sky-600 tabular-nums dark:text-sky-400">{fmt(m.requests)}</td></tr>)}</tbody>
    </table></div>
    {data.models.length === 0 && <div className="flex min-h-72 flex-col items-center justify-center gap-2 text-sm text-muted-foreground"><Activity className="mb-1 size-7 opacity-40" /><p>暂无模型用量</p><p className="text-xs">开始转发请求后，模型统计将显示在这里</p></div>}
  </section>
}

function ActivityHeatmap({ data }: { data: DashboardData }) {
  const cells = data.activity
  const [focusedDay, setFocusedDay] = useState(179)
  const dayRefs = useRef<Array<HTMLButtonElement | null>>([])
  const total = cells.reduce((sum, day) => sum + day.requests, 0)
  const max = Math.max(1, ...cells.map((day) => day.requests))
  const firstDay = cells[0] ? new Date(`${cells[0].date}T00:00:00Z`).getUTCDay() : 0
  const leading = (firstDay + 6) % 7
  const heatColors = ["bg-emerald-500/10", "bg-emerald-500/25", "bg-emerald-500/45", "bg-emerald-500/70", "bg-emerald-500"]
  const dateLabel = (date: string) => date.slice(5).replace("-", "/")
  return <section className={panel}>
    <div className="flex items-center justify-between"><h2 className="font-medium">请求活跃度</h2><span className="text-xs text-muted-foreground">近 180 天</span></div>
    <div className="mt-6 flex items-baseline justify-between gap-3"><p className="text-2xl font-medium tracking-tight tabular-nums">{fmt(total)}<span className="ml-2 text-xs font-normal text-muted-foreground">次请求</span></p><span className="text-xs text-muted-foreground">{cells[0] && dateLabel(cells[0].date)} – {cells.at(-1) && dateLabel(cells.at(-1)!.date)}</span></div>
    <div className="mt-5 overflow-x-auto pb-1"><div className="grid min-w-72 grid-flow-col grid-rows-7 gap-1" aria-label="过去 180 天每日请求次数">{Array.from({ length: leading }, (_, i) => <span key={`pad-${i}`} />)}{cells.map((day, index) => {
      const level = day.requests === 0 ? 0 : Math.min(4, Math.ceil(day.requests / max * 4))
      return <Tooltip key={day.date}><TooltipTrigger asChild><button ref={(element) => { dayRefs.current[index] = element }} tabIndex={index === focusedDay ? 0 : -1} onFocus={() => setFocusedDay(index)} onKeyDown={(event) => { const offset = ({ ArrowLeft: -7, ArrowRight: 7, ArrowUp: -1, ArrowDown: 1 } as Record<string, number>)[event.key]; if (offset !== undefined) { event.preventDefault(); dayRefs.current[Math.max(0, Math.min(cells.length - 1, index + offset))]?.focus() } }} className={cn("aspect-square min-w-1 rounded-[3px] transition-opacity hover:opacity-70 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none", heatColors[level])} aria-label={`${day.date}：${fmt(day.requests)} 次请求`} /></TooltipTrigger><TooltipContent>{day.date} · {fmt(day.requests)} 次请求</TooltipContent></Tooltip>
    })}</div></div>
    <div className="mt-3 flex items-center justify-end gap-1 text-[10px] text-muted-foreground"><span className="mr-1">少</span>{heatColors.map((color) => <span key={color} className={cn("size-2.5 rounded-xs", color)} />)}<span className="ml-1">多</span></div>
  </section>
}

function Availability({ data }: { data: DashboardData }) {
  const a = data.availability
  const rate = a.accounts_total ? a.accounts_available / a.accounts_total * 100 : 0
  const rows = [
    { title: "可用账号", value: a.accounts_available, total: a.accounts_total, color: "#10b981", detail: "可转发" },
    { title: "不可用账号", value: a.accounts_unavailable, total: a.accounts_total, color: "#cbd5e1", detail: "暂不可用" },
    { title: "已启用模型", value: a.models_enabled, total: a.models_total, color: "#8b5cf6", detail: "已启用" },
    { title: "可用密钥", value: a.keys_available, total: a.keys_total, color: "#0ea5e9", detail: "可用" },
  ]
  return <section className={panel}>
    <div className="flex items-center justify-between"><h2 className="flex items-center gap-2 font-medium">资源可用性<InfoTip>按当前账号池及额度状态统计，与时间范围无关。暂不可用包括凭据缺失、冷却或配额耗尽等。</InfoTip></h2><Link to="/account" className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground">管理账号<ArrowUpRight className="size-3" /></Link></div>
    <div className="mt-7 grid items-center gap-6 sm:grid-cols-[minmax(120px,1fr)_1.2fr]">
      <div className="relative mx-auto aspect-square w-full max-w-48"><svg viewBox="0 0 160 160" className="size-full -rotate-90" role="img" aria-label={`账号可用率 ${percent(rate)}`}><circle cx="80" cy="80" r="65" stroke="currentColor" className="text-muted" strokeWidth="19" fill="none" /><circle cx="80" cy="80" r="65" stroke="#10b981" strokeWidth="19" fill="none" strokeDasharray={`${rate / 100 * 408.407} 408.407`} /></svg><div className="absolute inset-0 flex flex-col items-center justify-center"><span className="text-3xl font-medium tracking-tight">{a.accounts_total ? `${Math.round(rate)}%` : "—"}</span><span className="mt-2 text-xs text-muted-foreground">账号可用率</span></div></div>
      <div className="divide-y divide-border/55">{rows.map((row) => <div key={row.title} className="flex items-center gap-2.5 py-3 first:pt-0 last:pb-0"><span className="size-2 shrink-0 rounded-full" style={{ background: row.color }} /><div className="flex-1"><p className="text-sm">{row.title}</p><p className="mt-1 text-xs text-muted-foreground">{fmt(row.value)} / {fmt(row.total)} {row.detail}</p></div><span className="text-sm tabular-nums">{fmt(row.value)}</span></div>)}</div>
    </div>
  </section>
}

export function DashboardPage() {
  const [range, setRange] = useState<DashboardRange>("30d")
  const [data, setData] = useState<DashboardData | null>(null)
  const [revision, setRevision] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")
  useEffect(() => {
    const controller = new AbortController()
    setLoading(true)
    setError("")
    fetchDashboard(range, controller.signal).then(setData).catch((err: unknown) => {
      if (!controller.signal.aborted) setError(err instanceof Error ? err.message : "统计数据加载失败")
    }).finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [range, revision])
  const visible = data?.range === range ? data : null
  const s = visible?.summary
  return <div className="space-y-5 pb-6 sm:space-y-6">
    <div className="flex flex-wrap items-center justify-between gap-4"><div><h1 className="text-2xl font-semibold tracking-tight">仪表盘</h1><p className="mt-1 text-xs text-muted-foreground">查看请求用量、模型表现与账号池状态</p></div><div className="flex items-center gap-3"><div className="flex rounded-full bg-muted/75 p-1" aria-label="统计时间范围">{ranges.map((item) => <button key={item} onClick={() => setRange(item)} aria-pressed={item === range} className={cn("rounded-full px-3.5 py-1.5 text-xs transition-colors sm:px-4", item === range ? "bg-card font-medium text-foreground shadow-sm" : "text-muted-foreground hover:text-foreground")}>{item}</button>)}</div><Button variant="outline" size="sm" className="rounded-full border-0 bg-card shadow-xs" disabled={loading} onClick={() => setRevision((n) => n + 1)}><RefreshCw className={cn("size-3.5", loading && "animate-spin")} />刷新</Button></div></div>
    {error && <div role="alert" className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-destructive/25 bg-destructive/5 px-4 py-3 text-sm"><span>{error}{visible ? "，当前显示上次成功加载的数据。" : ""}</span><Button size="sm" variant="outline" onClick={() => setRevision((n) => n + 1)}>重新加载</Button></div>}
    {!visible && loading && <div aria-label="正在加载统计数据" className="space-y-6"><div className="grid grid-cols-2 gap-4 lg:grid-cols-5">{[0, 1, 2, 3, 4].map((item) => <Skeleton key={item} className="h-36 rounded-2xl" />)}</div><div className="grid gap-6 lg:grid-cols-[3fr_2fr]"><Skeleton className="h-96 rounded-2xl" /><Skeleton className="h-96 rounded-2xl" /><Skeleton className="h-96 rounded-2xl" /><Skeleton className="h-96 rounded-2xl" /></div></div>}
    {visible && s && <div className={cn("space-y-5 transition-opacity sm:space-y-6", loading && "opacity-60")} aria-busy={loading}>
      <div className="grid grid-cols-1 gap-3 min-[400px]:grid-cols-2 sm:gap-4 lg:grid-cols-5">
        <Metric title="账号数" value={fmt(s.accounts)} detail={<>当前账号池 · {fmt(visible.availability.accounts_available)} 个可用</>} icon={<Users className="size-4" />} />
        <Metric title="请求数" value={fmt(s.requests)} detail={<>成功率 {s.requests ? percent(s.success_rate) : "—"}{s.processing > 0 ? ` · ${s.processing} 处理中` : ` · ${fmt(s.error)} 次失败`}</>} icon={<Activity className="size-4" />} />
        <Metric title="总 Tokens" value={fmt(s.total_tokens)} detail={<>缓存命中率 {s.input_tokens ? percent(s.cache_hit_rate) : "—"}</>} icon={<Type className="size-4" />} />
        <Metric title="计费" value={money(s.cost_usd)} detail={s.avg_request_cost_usd !== null ? `平均每次请求 ${money(s.avg_request_cost_usd)}` : visible.billing_available ? "已同步的账号日账单" : "暂无可用计费数据"} icon={<CircleDollarSign className="size-4" />} note={visible.billing_note} />
        <Metric title="平均首包" value={s.avg_ttft_ms === null ? "—" : `${(s.avg_ttft_ms / 1000).toFixed(2)} s`} detail={<>平均输出 {s.avg_output_tps === null ? "—" : `${s.avg_output_tps.toFixed(1)} Token/s`} · {fmt(s.ttft_samples)} 个样本</>} icon={<Gauge className="size-4" />} note={`成功流式请求的首个响应包耗时。首包样本 ${fmt(s.ttft_samples)} 个，输出速度样本 ${fmt(s.output_samples)} 个。`} />
      </div>
      <div className="grid gap-5 sm:gap-6 xl:grid-cols-[3fr_2fr]"><UsageChart data={visible} /><ChannelDistribution data={visible} /></div>
      <div className="grid items-start gap-5 sm:gap-6 xl:grid-cols-[3fr_2fr]"><ModelRanking data={visible} /><div className="grid gap-5 sm:gap-6"><ActivityHeatmap data={visible} /><Availability data={visible} /></div></div>
      <div className="flex flex-wrap items-start justify-between gap-2 px-1 text-[11px] leading-relaxed text-muted-foreground"><p className="max-w-3xl">按 UTC 汇总。{visible.history_partial ? "历史仅包含已记录数据，升级前已清理的请求无法补回。" : "请求明细清理后，历史汇总仍会保留。"}{visible.billing_note}</p><p className="shrink-0">更新于 {new Date(visible.generated_at).toLocaleTimeString("zh-CN", { hour12: false })}</p></div>
    </div>}
  </div>
}
