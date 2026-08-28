export type AccountStatus = "pending" | "queued" | "running" | "ready" | "failed"

export type UsageWindow = {
  status: string
  reset_in_sec: number
  usage_percent: number
}

export type ModelSpend = {
  model: string
  usd: number
  limit_usd: number
}

export type AccountUsage = {
  rolling: UsageWindow
  weekly: UsageWindow
  monthly: UsageWindow
  models: ModelSpend[]
  synced_at: number
  error: string
}

export type Account = {
  id: string
  email: string
  recovery_email: string
  status: AccountStatus
  workspace_id: string
  api_key: string
  user_id: string
  cookies_json?: string
  cookie_header?: string
  payment_url: string
  has_cookies?: boolean
  has_api_key?: boolean
  last_error: string
  last_login_at: number
  created_at: number
  updated_at: number
  paid_at: number
  batch_id: string
  batch_name?: string
  login_provider?: string
}

export type PoolAccount = Account & {
  usage: AccountUsage
  inflight?: number
}

export type PoolPage = {
  items: PoolAccount[]
  weekly_limited: PoolAccount[]
  rolling_limited: PoolAccount[]
  total: number
  page: number
  page_size: number
  stats: PoolStats
}

export type PoolStats = {
  total: number
  ok: number
  tight: number
  exhausted: number
  inflight: number
  avg_rolling: number | null
  avg_weekly: number | null
  avg_monthly: number | null
}

export type UsageSyncStatus = {
  running: boolean
  total: number
  done: number
  fail: number
  paid: number
  unpaid: number
  message: string
  started_at?: number
  finished_at?: number
  interval_sec?: number
  concurrency?: number
}

export type AppConfig = {
  platform: string
  headless: boolean
  proxy: string
  invite_url: string
  max_concurrent: number
  max_retries: number
  account_rpm: number
  api_proxy: boolean
  usage_refresh_sec: number
  usage_refresh_concurrency: number
  cloak_version: string
  cloak_license_key: string
  cloak_license_configured: boolean
  hero_sms_api_key: string
  hero_sms_configured: boolean
  hero_sms_service: string
  hero_sms_country: number
  hero_sms_max_price: number
  provider_mode: "keep" | "hide" | "replace"
  provider_value: string
  amzkeys_host: string
  amzkeys_app_id: string
  amzkeys_app_key: string
  amzkeys_private_key: string
  amzkeys_card_type: number
  amzkeys_card_amount: number
  amzkeys_configured: boolean
  amzkeys_card_last4: string
  amzkeys_card_pending: boolean
  amzkeys_card_pay_count: number
  amzkeys_card_max_pays: number
  amzkeys_card_next_last4: string
  amzkeys_card_next_pending: boolean
  amzkeys_card_error: string
  cookie_keep_enabled: boolean
  cookie_keep_hour: number
  cookie_keep_last_date: string
}

export type CloakUpdate = {
  current: string
  latest: string
  updated: boolean
  downloading: boolean
}

export type Job = {
  id: string
  account_id: string
  email?: string
  kind?: string
  status: string
  error?: string
  logs?: JobEvent[]
  started_at?: number
  ended_at?: number
}

export type Batch = {
  id: string
  name: string
  created_at: number
  updated_at: number
  exported_at: number
  exported_count: number
  paid_at: number
  total: number
  pending: number
  ready: number
  failed: number
  pay_count: number
  cookie_count: number
  paid_count: number
  unpaid_pay_count: number
  unpaid_cookie_count: number
}

export type BatchPage = {
  items: Batch[]
  total: number
  page: number
  page_size: number
}

export type RequestLog = {
  id: number
  created_at: number
  model: string
  api_format: string
  stream: boolean
  account_id: string
  account_email: string
  status: string
  http_status: number
  input_tokens: number
  output_tokens: number
  reasoning_tokens: number
  cache_read: number
  cache_write: number
  total_tokens: number
  duration_ms: number
  ttft_ms: number
  retries: number
  error: string
}

export type RequestLogStats = {
  rpm_1m: number
  tpm_1m: number
  rpm_5m: number
  tpm_5m: number
  requests_1h: number
  tokens_1h: number
  requests_24h: number
  tokens_24h: number
  success_1h: number
  error_1h: number
  processing: number
  models: string[]
}

export type RequestLogPage = {
  items: RequestLog[]
  total: number
  page: number
  page_size: number
  stats: RequestLogStats
}

export type JobEvent = {
  job_id: string
  account_id: string
  level: string
  message: string
  time: number
}
