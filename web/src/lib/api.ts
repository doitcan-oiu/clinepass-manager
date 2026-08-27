import type { Account, AppConfig, Batch, BatchPage, CloakUpdate, Job, PoolAccount, PoolPage, RequestLogPage, UsageSyncStatus } from "@/lib/types"

export type AccountBackup = {
  version: number
  accounts: Array<{
    account: string
    password?: string
    auxEmail?: string
    workspaceID?: string
    auth: string
    apiKey?: string
    userID?: string
    loginType?: string
    login_provider?: string
  }>
}

export type ImportResult = {
  imported: number
  updated: number
  skipped: string[]
  sync: UsageSyncStatus
  warning?: string
}

async function json<T>(res: Response): Promise<T> {
  const text = await res.text()
  const data = text ? JSON.parse(text) : null
  if (!res.ok) {
    throw new Error((data as { error?: string } | null)?.error || res.statusText)
  }
  return data as T
}

export const api = {
  config: () => fetch("/api/config").then((r) => json<AppConfig>(r)),
  saveConfig: (body: Partial<AppConfig> & { hero_sms_api_key?: string; cloak_license_key?: string; amzkeys_app_key?: string; amzkeys_private_key?: string }) =>
    fetch("/api/config", {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }).then((r) => json<AppConfig>(r)),
  updateCloak: () =>
    fetch("/api/cloak/update", { method: "POST" }).then((r) => json<CloakUpdate>(r)),
  account: (id: string) => fetch(`/api/accounts/${id}`).then((r) => json<Account>(r)),
  deleteAccount: (id: string) => fetch(`/api/accounts/${id}`, { method: "DELETE" }).then((r) => json<null>(r)),
  loginAccount: (id: string, autoPay = false) =>
    fetch(`/api/accounts/${id}/login`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ auto_pay: autoPay }),
    }).then((r) => json<Job>(r)),
  jobs: () => fetch("/api/jobs").then((r) => json<Job[]>(r)),
  batches: (page = 1, pageSize = 30) =>
    fetch(`/api/batches?page=${page}&page_size=${pageSize}`).then((r) => json<BatchPage>(r)),
  createBatch: async (body: { name: string; text: string; login_provider?: string }) => {
    const res = await fetch("/api/batches", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    })
    const text = await res.text()
    const data = text ? JSON.parse(text) : null
    if (!res.ok) {
      const extra = Array.isArray(data?.errors) && data.errors.length ? `（${data.errors.join("；")}）` : ""
      throw new Error((data?.error || res.statusText) + extra)
    }
    return data as { batch: Batch; errors: string[] }
  },
  batch: (id: string) =>
    fetch(`/api/batches/${id}`).then((r) => json<{ batch: Batch; accounts: Account[] }>(r)),
  deleteBatch: (id: string) =>
    fetch(`/api/batches/${id}`, { method: "DELETE" }).then((r) => json<null>(r)),
  deleteRadarDenied: (id: string) =>
    fetch(`/api/batches/${id}/radar-denied`, { method: "DELETE" }).then((r) =>
      json<{ deleted: number; batch: Batch }>(r)
    ),
  loginBatch: (id: string, autoPay = false) =>
    fetch(`/api/batches/${id}/login`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ auto_pay: autoPay }),
    }).then((r) => json<Job[]>(r)),
  refreshBatch: (id: string, autoPay = false) =>
    fetch(`/api/batches/${id}/refresh`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ auto_pay: autoPay }),
    }).then((r) => json<Job[]>(r)),
  refreshAccount: (id: string, autoPay = false) =>
    fetch(`/api/accounts/${id}/refresh`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ auto_pay: autoPay }),
    }).then((r) => json<Job>(r)),
  amzKeysStatus: () => fetch("/api/amzkeys/status").then((r) => json<AmzKeysStatus>(r)),
  warmAmzKeysCard: () =>
    fetch("/api/amzkeys/cards/warm", { method: "POST" }).then((r) => json<{ ok: boolean; pending: boolean; last4: string }>(r)),
  clearAmzKeysCard: () => fetch("/api/amzkeys/cards", { method: "DELETE" }).then((r) => json<{ ok: boolean }>(r)),
  dispatchBatch: (id: string) =>
    fetch(`/api/batches/${id}/dispatch`, { method: "POST" }).then((r) =>
      json<{ batch: Batch; count: number; filename: string; xlsx: string }>(r)
    ),
  markBatchPaid: (id: string) =>
    fetch(`/api/batches/${id}/paid`, { method: "POST" }).then((r) =>
      json<{ batch: Batch; sync: UsageSyncStatus }>(r)
    ),
  poolAccounts: (page = 1, pageSize = 30, batchId = "") => {
    const q = new URLSearchParams({ page: String(page), page_size: String(pageSize) })
    if (batchId) q.set("batch_id", batchId)
    return fetch(`/api/pool/accounts?${q}`).then((r) => json<PoolPage>(r))
  },
  paidBatches: () => fetch("/api/pool/batches").then((r) => json<Batch[]>(r)),
  createPaidAccount: (body: {
    email: string
    api_key: string
    workspace_id?: string
    cookie_header: string
    password?: string
    recovery_email?: string
    user_id?: string
    login_provider?: string
  }) =>
    fetch("/api/accounts", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }).then((r) => json<Account>(r)),
  refreshAccountUsage: (id: string) =>
    fetch(`/api/accounts/${id}/usage`, { method: "POST" }).then((r) => json<PoolAccount>(r)),
  usageSync: () => fetch("/api/usage/sync").then((r) => json<UsageSyncStatus>(r)),
  startUsageSync: () => fetch("/api/usage/sync", { method: "POST" }).then((r) => json<UsageSyncStatus>(r)),
  logs: (page = 1, pageSize = 50, q: { model?: string; email?: string; status?: string; stream?: string } = {}) => {
    const p = new URLSearchParams({ page: String(page), page_size: String(pageSize) })
    if (q.model) p.set("model", q.model)
    if (q.email) p.set("email", q.email)
    if (q.status) p.set("status", q.status)
    if (q.stream) p.set("stream", q.stream)
    return fetch(`/api/logs?${p}`).then((r) => json<RequestLogPage>(r))
  },
  clearLogs: () => fetch("/api/logs", { method: "DELETE" }).then((r) => json<{ ok: boolean }>(r)),
  exportAccounts: () => fetch("/api/pool/export").then((r) => json<AccountBackup>(r)),
  importAccounts: (body: unknown) =>
    fetch("/api/pool/import", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: typeof body === "string" ? body : JSON.stringify(body),
    }).then((r) => json<ImportResult>(r)),
  heroSMSCatalog: (q: { api_key?: string; service?: string } = {}) => {
    const p = new URLSearchParams()
    if (q.api_key) p.set("api_key", q.api_key)
    if (q.service) p.set("service", q.service)
    const qs = p.toString()
    return fetch(`/api/herosms/catalog${qs ? `?${qs}` : ""}`).then((r) => json<HeroSMSCatalog>(r))
  },
}

export type AmzKeysBalance = { currency: string; available_amount: string; frozen_amount: string }
export type AmzKeysCardType = {
  card_type: number
  new_card_fee: string
  service_fee: string
  min_opencard_amount: string
  min_recharge_amount: string
}
export type AmzKeysStatus = { host: string; balances: AmzKeysBalance[]; card_types: AmzKeysCardType[] }

export type HeroSMSQuote = { price: number; count: number }
export type HeroSMSCountry = { id: number; name: string; phone_code?: number; quotes: HeroSMSQuote[] }
export type HeroSMSCatalog = {
  balance: number
  service: string
  services?: { code: string; name: string }[]
  countries: HeroSMSCountry[]
}
