export type DashboardRange = "24h" | "7d" | "30d" | "90d"
export type DashboardData = {
  range: DashboardRange
  generated_at: number
  since: number
  until: number
  timezone: string
  history_since: number
  history_partial: boolean
  billing_available: boolean
  billing_note: string
  summary: {
    accounts: number; requests: number; success: number; error: number; processing: number
    success_rate: number; total_tokens: number; input_tokens: number; output_tokens: number
    cache_read: number; cache_write: number; cache_hit_rate: number
    cost_usd: number | null; avg_request_cost_usd: number | null
    avg_ttft_ms: number | null; ttft_samples: number; avg_output_tps: number | null; output_samples: number
  }
  timeline: Array<{ timestamp: number; requests: number; total_tokens: number; cost_usd: number | null }>
  channels: Array<{ id: string; name: string; requests: number; success: number; success_rate: number; total_tokens: number; share: number }>
  models: Array<{ id: string; requests: number; input_tokens: number; output_tokens: number; reasoning_tokens: number; cache_read: number; cache_write: number; total_tokens: number; cost_usd: number | null }>
  activity: Array<{ date: string; requests: number }>
  availability: { accounts_total: number; accounts_available: number; accounts_unavailable: number; models_total: number; models_enabled: number; keys_total: number; keys_available: number }
}
export async function fetchDashboard(range: DashboardRange, signal: AbortSignal): Promise<DashboardData> {
  const response = await fetch(`/api/dashboard?range=${range}`, { signal })
  if (!response.ok) {
    const data = await response.json().catch(() => null)
    throw new Error(data?.error || `统计数据加载失败（${response.status}）`)
  }
  return response.json()
}
