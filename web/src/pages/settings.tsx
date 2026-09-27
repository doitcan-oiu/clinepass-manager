import { useEffect, useMemo, useState, type FormEvent } from "react"
import { useSearchParams } from "react-router-dom"
import { toast } from "sonner"
import { api, type AmzKeysStatus, type HeroSMSCatalog, type HeroSMSCountry } from "@/lib/api"
import type { AppConfig, AutoStatus, ModelCatalogSyncStatus } from "@/lib/types"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"

const selectClass = "h-9 w-full rounded-md border border-input bg-transparent px-3 text-sm shadow-xs outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"

const settingsTabs = [
  { value: "forwarding", label: "转发与负载均衡", description: "请求调度、网络代理与响应格式", owner: "主程序" },
  { value: "usage", label: "用量同步", description: "账号配额、模型用量与官方模型目录", owner: "主程序" },
  { value: "connection", label: "Auto 连接", description: "连接部署在另一台服务器上的 Auto 服务", owner: "主程序" },
  { value: "automation", label: "提取与续期", description: "支付链接提取与 Cookie 续期任务", owner: "Auto 自动化" },
  { value: "browser", label: "浏览器环境", description: "登录浏览器、运行方式与授权", owner: "Auto 自动化" },
  { value: "herosms", label: "接码服务", description: "手机验证的区域、服务和报价", owner: "Auto 自动化" },
  { value: "amzkeys", label: "支付卡台", description: "卡台连接、开卡参数与卡片状态", owner: "Auto 自动化" },
] as const

function formatQuotePrice(n: number) {
  const text = n.toFixed(4).replace(/\.?0+$/, "")
  return text || "0"
}

function lowestQuote(c: HeroSMSCountry) {
  const available = c.quotes.filter((q) => q.count > 0)
  const list = available.length ? available : c.quotes
  if (!list.length) return undefined
  return list.reduce((min, q) => (q.price < min.price ? q : min), list[0])
}

function countryOptionLabel(c: HeroSMSCountry) {
  const parts = [`${c.name}（${c.id}）`]
  if (c.phone_code) parts.push(`+${c.phone_code}`)
  const quote = lowestQuote(c)
  if (quote) parts.push(`最低 ${formatQuotePrice(quote.price)}`)
  return parts.join(" ")
}

function ModelCatalogSyncCard() {
  const [status, setStatus] = useState<ModelCatalogSyncStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState("")
  const [attempt, setAttempt] = useState(0)

  useEffect(() => {
    let active = true
    setLoading(true)
    const load = () => api.modelsSync().then((next) => {
      if (!active) return
      setStatus(next)
      setError("")
    }).catch((err: unknown) => {
      if (active) setError(err instanceof Error ? err.message : "无法读取模型目录同步状态")
    }).finally(() => {
      if (active) setLoading(false)
    })
    void load()
    const timer = window.setInterval(() => { void load() }, 10000)
    return () => { active = false; window.clearInterval(timer) }
  }, [attempt])

  async function refresh() {
    setRefreshing(true)
    setError("")
    try {
      const next = await api.startModelsSync()
      setStatus(next)
      if (!next.syncing) toast.success(`模型目录已更新，共 ${next.model_count} 个模型`)
    } catch (err) {
      setError(err instanceof Error ? err.message : "模型目录同步失败")
      try { setStatus(await api.modelsSync()) } catch { /* Keep the last known catalog status. */ }
    } finally {
      setRefreshing(false)
    }
  }

  const syncing = refreshing || status?.syncing
  const source = status?.source || "https://docs.cline.bot/getting-started/clinepass#models"
  const origin = status?.origin === "remote" ? "官方文档" : status?.origin === "cache" ? "本地缓存" : "内置备用目录"
  const message = error || status?.last_error

  return (
    <Card>
      <CardHeader>
        <CardTitle>模型目录同步</CardTitle>
        <CardDescription>启动时和每小时从官方文档更新可用模型，供 /v1/models 和模型选择列表使用。同步失败时保留已有目录。</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <a href={source} target="_blank" rel="noreferrer" className="inline-block break-all text-sm underline underline-offset-4">查看 ClinePass 官方模型文档</a>
        {loading && !status ? <p role="status" className="text-sm text-muted-foreground">正在读取模型目录状态…</p> : null}
        {status ? (
          <dl className="grid gap-4 rounded-lg border bg-muted/20 p-4 text-sm sm:grid-cols-2">
            <div className="space-y-1"><dt className="text-muted-foreground">模型数量</dt><dd className="font-medium">{status.model_count} 个</dd></div>
            <div className="space-y-1"><dt className="text-muted-foreground">当前目录来源</dt><dd>{origin}</dd></div>
            <div className="space-y-1"><dt className="text-muted-foreground">最近成功同步</dt><dd>{status.last_success_at ? new Date(status.last_success_at * 1000).toLocaleString("zh-CN", { hour12: false }) : "尚未成功同步"}</dd></div>
            <div className="space-y-1"><dt className="text-muted-foreground">同步状态</dt><dd role="status">{syncing ? "正在同步…" : status.last_error ? "同步失败，继续使用已有目录" : "每小时自动同步"}</dd></div>
          </dl>
        ) : null}
        {message ? <p role="alert" className="break-words rounded-lg border border-destructive/25 bg-destructive/5 p-3 text-sm text-destructive">{message}</p> : null}
        <div className="flex flex-wrap gap-2">
          <Button type="button" variant="outline" disabled={loading || !!syncing} onClick={() => void refresh()}>{syncing ? "同步中…" : "立即同步模型目录"}</Button>
          {error ? <Button type="button" variant="ghost" disabled={loading || !!syncing} onClick={() => setAttempt((value) => value + 1)}>重新读取状态</Button> : null}
        </div>
      </CardContent>
    </Card>
  )
}

export function SettingsPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const [configLoaded, setConfigLoaded] = useState(false)
  const [loadingConfig, setLoadingConfig] = useState(true)
  const [configError, setConfigError] = useState("")
  const [configAttempt, setConfigAttempt] = useState(0)
  const activeTab = searchParams.get("tab") || "forwarding"
  const [autoConfigLoaded, setAutoConfigLoaded] = useState(false)
  const [loadingAutoConfig, setLoadingAutoConfig] = useState(false)
  const [autoConfigError, setAutoConfigError] = useState("")
  const [autoConfigAttempt, setAutoConfigAttempt] = useState(0)
  const [autoURL, setAutoURL] = useState("")
  const [autoToken, setAutoToken] = useState("")
  const [autoTokenConfigured, setAutoTokenConfigured] = useState(false)
  const [connectionLoaded, setConnectionLoaded] = useState(false)
  const [connectionLoading, setConnectionLoading] = useState(true)
  const [connectionError, setConnectionError] = useState("")
  const [connectionAttempt, setConnectionAttempt] = useState(0)
  const [savingConnection, setSavingConnection] = useState(false)
  const [testingConnection, setTestingConnection] = useState(false)
  const [draftStatus, setDraftStatus] = useState<AutoStatus | null>(null)
  const [autoStatus, setAutoStatus] = useState<AutoStatus | null>(null)
  const [checkingAuto, setCheckingAuto] = useState(false)
  const [proxy, setProxy] = useState("")
  const [autoProxy, setAutoProxy] = useState("")
  const [headless, setHeadless] = useState(true)
  const [maxConcurrent, setMaxConcurrent] = useState(1)
  const [cookieKeepEnabled, setCookieKeepEnabled] = useState(true)
  const [cookieKeepHour, setCookieKeepHour] = useState(4)
  const [cookieKeepConcurrency, setCookieKeepConcurrency] = useState(4)
  const [cookieKeepLastDate, setCookieKeepLastDate] = useState("")
  const [keepingCookies, setKeepingCookies] = useState(false)
  const [maxRetries, setMaxRetries] = useState(3)
  const [accountRpm, setAccountRpm] = useState(5)
  const [apiProxy, setApiProxy] = useState(true)
  const [usageRefreshSec, setUsageRefreshSec] = useState(60)
  const [modelUsageRefreshSec, setModelUsageRefreshSec] = useState(600)
  const [usageRefreshConcurrency, setUsageRefreshConcurrency] = useState(10)
  const [providerMode, setProviderMode] = useState<"keep" | "hide" | "replace">("keep")
  const [providerValue, setProviderValue] = useState("")
  const [cloakVersion, setCloakVersion] = useState("151.0.7922.108.2")
  const [cloakLicense, setCloakLicense] = useState("")
  const [cloakConfigured, setCloakConfigured] = useState(false)
  const [apiKey, setApiKey] = useState("")
  const [configured, setConfigured] = useState(false)
  const [service, setService] = useState("ot")
  const [country, setCountry] = useState(0)
  const [maxPrice, setMaxPrice] = useState(0)
  const [catalog, setCatalog] = useState<HeroSMSCatalog | null>(null)
  const [pending, setPending] = useState(false)
  const [checkingUpdate, setCheckingUpdate] = useState(false)
  const [loadingCatalog, setLoadingCatalog] = useState(false)
  const [amzHost, setAmzHost] = useState("https://testapi.amzkeys.com")
  const [amzAppID, setAmzAppID] = useState("")
  const [amzAppKey, setAmzAppKey] = useState("")
  const [amzPrivateKey, setAmzPrivateKey] = useState("")
  const [amzCardType, setAmzCardType] = useState(467845)
  const [amzAmount, setAmzAmount] = useState(20)
  const [amzConfigured, setAmzConfigured] = useState(false)
  const [amzLast4, setAmzLast4] = useState("")
  const [amzPending, setAmzPending] = useState(false)
  const [amzPayCount, setAmzPayCount] = useState(0)
  const [amzMaxPays, setAmzMaxPays] = useState(3)
  const [amzNextLast4, setAmzNextLast4] = useState("")
  const [amzNextPending, setAmzNextPending] = useState(false)
  const [amzCardError, setAmzCardError] = useState("")
  const [amzStatus, setAmzStatus] = useState<AmzKeysStatus | null>(null)
  const [checkingAmz, setCheckingAmz] = useState(false)
  const [clearingCard, setClearingCard] = useState(false)

  const currentTab = settingsTabs.find((tab) => tab.value === activeTab) || settingsTabs[0]

  async function checkAutoStatus() {
    setCheckingAuto(true)
    try {
      setAutoStatus(await api.autoStatus())
    } catch (err) {
      setAutoStatus((previous) => ({
        connected: false,
        url: previous?.url || "",
        error: err instanceof Error ? err.message : "无法获取 Auto 服务状态",
      }))
    } finally {
      setCheckingAuto(false)
    }
  }

  useEffect(() => {
    if (currentTab.owner === "Auto 自动化" && autoStatus === null && !checkingAuto) {
      void checkAutoStatus()
    }
  }, [currentTab.owner, autoStatus, checkingAuto])

  useEffect(() => {
    let active = true
    setConnectionLoading(true)
    setConnectionError("")
    api.autoConnection().then((cfg) => {
      if (!active) return
      setAutoURL(cfg.url || "")
      setAutoTokenConfigured(cfg.token_configured)
      setConnectionLoaded(true)
    }).catch((err) => {
      if (active) setConnectionError(err instanceof Error ? err.message : "无法读取连接设置")
    }).finally(() => {
      if (active) setConnectionLoading(false)
    })
    return () => { active = false }
  }, [connectionAttempt])

  async function saveConnection(e: FormEvent) {
    e.preventDefault()
    setSavingConnection(true)
    try {
      const cfg = await api.saveAutoConnection({ url: autoURL.trim(), ...(autoToken.trim() ? { token: autoToken.trim() } : {}) })
      setAutoURL(cfg.url)
      setAutoToken("")
      setAutoTokenConfigured(cfg.token_configured)
      setDraftStatus(null)
      setAutoStatus(null)
      setAutoConfigLoaded(false)
      setAutoConfigError("")
      setCatalog(null)
      setAmzStatus(null)
      setAmzPending(false)
      setAmzNextPending(false)
      setAutoConfigAttempt((value) => value + 1)
      toast.success("Auto 连接设置已保存")
      void checkAutoStatus()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "保存连接失败")
    } finally {
      setSavingConnection(false)
    }
  }

  async function testConnection() {
    setTestingConnection(true)
    setDraftStatus(null)
    try {
      const status = await api.testAutoConnection({ url: autoURL.trim(), ...(autoToken.trim() ? { token: autoToken.trim() } : {}) })
      setDraftStatus(status)
    } catch (err) {
      setDraftStatus({ connected: false, url: autoURL.trim(), error: err instanceof Error ? err.message : "连接测试失败" })
    } finally {
      setTestingConnection(false)
    }
  }

  function applyAmzCard(cfg: AppConfig) {
    setAmzLast4(cfg.amzkeys_card_last4 || "")
    setAmzPending(!!cfg.amzkeys_card_pending)
    setAmzPayCount(cfg.amzkeys_card_pay_count || 0)
    setAmzMaxPays(cfg.amzkeys_card_max_pays || 3)
    setAmzNextLast4(cfg.amzkeys_card_next_last4 || "")
    setAmzNextPending(!!cfg.amzkeys_card_next_pending)
    setAmzCardError(cfg.amzkeys_card_error || "")
  }

  useEffect(() => {
    let active = true
    setLoadingConfig(true)
    setConfigError("")
    api
      .config()
      .then((cfg) => {
        if (!active) return
        setProxy(cfg.proxy || "")
        setMaxRetries(Number.isFinite(cfg.max_retries) && cfg.max_retries >= 0 ? cfg.max_retries : 3)
        setAccountRpm(cfg.account_rpm >= 1 ? cfg.account_rpm : 5)
        setApiProxy(cfg.api_proxy !== false)
        setUsageRefreshSec(cfg.usage_refresh_sec >= 15 ? cfg.usage_refresh_sec : 60)
        setModelUsageRefreshSec(cfg.model_usage_refresh_sec >= 15 ? cfg.model_usage_refresh_sec : 600)
        setUsageRefreshConcurrency(cfg.usage_refresh_concurrency >= 1 ? cfg.usage_refresh_concurrency : 10)
        setProviderMode(cfg.provider_mode === "hide" || cfg.provider_mode === "replace" ? cfg.provider_mode : "keep")
        setProviderValue(cfg.provider_value || "")
        setConfigLoaded(true)
      })
      .catch((err: unknown) => {
        if (active) setConfigError(err instanceof Error ? err.message : "无法读取设置")
      })
      .finally(() => {
        if (active) setLoadingConfig(false)
      })
    return () => { active = false }
  }, [configAttempt])

  const remoteSelected = currentTab.owner === "Auto 自动化"
  useEffect(() => {
    if (!remoteSelected) return
    let active = true
    setLoadingAutoConfig(true)
    setAutoConfigError("")
    api.autoConfig().then((cfg) => {
        if (!active) return
        setAutoProxy(cfg.proxy || "")
        setHeadless(cfg.headless !== false)
        setMaxConcurrent(cfg.max_concurrent >= 1 ? cfg.max_concurrent : 1)
        setCookieKeepEnabled(cfg.cookie_keep_enabled !== false)
        setCookieKeepHour(Number.isFinite(cfg.cookie_keep_hour) && cfg.cookie_keep_hour >= 0 && cfg.cookie_keep_hour <= 23 ? cfg.cookie_keep_hour : 4)
        setCookieKeepConcurrency(Number.isFinite(cfg.cookie_keep_concurrency) && cfg.cookie_keep_concurrency >= 1 ? cfg.cookie_keep_concurrency : 4)
        setCookieKeepLastDate(cfg.cookie_keep_last_date || "")
        setCloakVersion(cfg.cloak_version || "151.0.7922.108.2")
        setCloakLicense(cfg.cloak_license_key || "")
        setCloakConfigured(!!cfg.cloak_license_configured)
        setApiKey(cfg.hero_sms_api_key || "")
        setConfigured(!!cfg.hero_sms_configured)
        setService(cfg.hero_sms_service || "ot")
        setCountry(cfg.hero_sms_country || 0)
        setMaxPrice(cfg.hero_sms_max_price || 0)
        setAmzHost(cfg.amzkeys_host || "https://testapi.amzkeys.com")
        setAmzAppID(cfg.amzkeys_app_id || "")
        setAmzAppKey(cfg.amzkeys_app_key || "")
        setAmzPrivateKey(cfg.amzkeys_private_key || "")
        setAmzCardType(cfg.amzkeys_card_type || 467845)
        setAmzAmount(cfg.amzkeys_card_amount || 20)
        setAmzConfigured(!!cfg.amzkeys_configured)
        applyAmzCard(cfg)
        setAutoConfigLoaded(true)
        if (cfg.hero_sms_configured) {
          loadCatalog(cfg.hero_sms_service || "ot", "").catch(() => {})
        }
      })
      .catch((err: unknown) => {
        if (active) {
          setAutoConfigLoaded(false)
          setAutoConfigError(err instanceof Error ? err.message : "无法读取 Auto 设置")
        }
      })
      .finally(() => {
        if (active) setLoadingAutoConfig(false)
      })
    return () => { active = false }
  }, [remoteSelected, autoConfigAttempt])

  useEffect(() => {
    if (!amzPending && !amzNextPending) return
    let active = true
    const timer = window.setInterval(() => {
      api
        .autoConfig()
        .then((cfg) => {
          if (!active) return
          setAmzConfigured(!!cfg.amzkeys_configured)
          applyAmzCard(cfg)
        })
        .catch(() => {})
    }, 3000)
    return () => { active = false; window.clearInterval(timer) }
  }, [amzPending, amzNextPending])

  async function loadCatalog(nextService = service, key = apiKey) {
    setLoadingCatalog(true)
    try {
      const cat = await api.heroSMSCatalog({
        api_key: key.includes("********") ? undefined : key,
        service: nextService,
      })
      setCatalog(cat)
      if (cat.service) setService(cat.service)
      return cat
    } catch (err) {
      setCatalog(null)
      throw err
    } finally {
      setLoadingCatalog(false)
    }
  }

  async function onFetchCatalog() {
    try {
      const cat = await loadCatalog(service)
      toast.success(`余额 ${cat.balance}，${cat.countries.length} 个区域`)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "拉取报价失败")
    }
  }

  const countries = catalog?.countries || []
  const selected: HeroSMSCountry | undefined = useMemo(
    () => countries.find((c) => c.id === country),
    [countries, country]
  )
  const quotes = selected?.quotes || []

  async function saveAutomation(e?: FormEvent) {
    e?.preventDefault()
    setPending(true)
    try {
      const n = Math.floor(Number(maxConcurrent))
      if (!Number.isFinite(n) || n < 1) {
        toast.error("并发数至少为 1")
        return
      }
      const hour = Math.floor(Number(cookieKeepHour))
      if (!Number.isFinite(hour) || hour < 0 || hour > 23) {
        toast.error("续 Cookie 小时须在 0–23")
        return
      }
      const keepConc = Math.floor(Number(cookieKeepConcurrency))
      if (!Number.isFinite(keepConc) || keepConc < 1 || keepConc > 32) {
        toast.error("续 Cookie 并发须在 1–32")
        return
      }
      await api.saveAutoConfig({
        max_concurrent: n,
        cookie_keep_enabled: cookieKeepEnabled,
        cookie_keep_hour: hour,
        cookie_keep_concurrency: keepConc,
      })
      setMaxConcurrent(n)
      setCookieKeepHour(hour)
      setCookieKeepConcurrency(keepConc)
      toast.success("提取与续期设置已保存")
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "保存失败")
    } finally {
      setPending(false)
    }
  }

  async function saveForwarding(e?: FormEvent) {
    e?.preventDefault()
    setPending(true)
    try {
      const retries = Math.floor(Number(maxRetries))
      if (!Number.isFinite(retries) || retries < 0 || retries > 32) {
        toast.error("失败换号次数须在 0–32")
        return
      }
      const rpm = Math.floor(Number(accountRpm))
      if (!Number.isFinite(rpm) || rpm < 1 || rpm > 1000) {
        toast.error("单账号 RPM 须在 1–1000")
        return
      }
      await api.saveConfig({
        proxy: proxy.trim(),
        max_retries: retries,
        account_rpm: rpm,
        api_proxy: apiProxy,
        provider_mode: providerMode,
        provider_value: providerValue,
      })
      setMaxRetries(retries)
      setAccountRpm(rpm)
      toast.success("转发与负载均衡设置已保存")
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "保存失败")
    } finally {
      setPending(false)
    }
  }

  async function saveUsage(e?: FormEvent) {
    e?.preventDefault()
    setPending(true)
    try {
      const refreshSec = Math.floor(Number(usageRefreshSec))
      if (!Number.isFinite(refreshSec) || refreshSec < 15 || refreshSec > 86400) {
        toast.error("套餐配额刷新间隔须在 15–86400 秒")
        return
      }
      const modelRefreshSec = Math.floor(Number(modelUsageRefreshSec))
      if (!Number.isFinite(modelRefreshSec) || modelRefreshSec < 15 || modelRefreshSec > 86400) {
        toast.error("模型用量刷新间隔须在 15–86400 秒")
        return
      }
      const refreshConc = Math.floor(Number(usageRefreshConcurrency))
      if (!Number.isFinite(refreshConc) || refreshConc < 1 || refreshConc > 64) {
        toast.error("用量刷新并发须在 1–64")
        return
      }
      await api.saveConfig({
        usage_refresh_sec: refreshSec,
        model_usage_refresh_sec: modelRefreshSec,
        usage_refresh_concurrency: refreshConc,
      })
      setUsageRefreshSec(refreshSec)
      setModelUsageRefreshSec(modelRefreshSec)
      setUsageRefreshConcurrency(refreshConc)
      toast.success("用量同步设置已保存")
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "保存失败")
    } finally {
      setPending(false)
    }
  }

  async function saveCloak(e?: FormEvent) {
    e?.preventDefault()
    setPending(true)
    try {
      const version = cloakVersion.trim()
      if (!version) {
        toast.error("请填写 CloakBrowser 版本")
        return
      }
      const body: Parameters<typeof api.saveAutoConfig>[0] = { cloak_version: version, headless, proxy: autoProxy.trim() }
      if (cloakLicense && !cloakLicense.includes("********")) {
        body.cloak_license_key = cloakLicense.trim()
      }
      const cfg = await api.saveAutoConfig(body)
      setCloakVersion(cfg.cloak_version || version)
      setCloakLicense(cfg.cloak_license_key || "")
      setCloakConfigured(!!cfg.cloak_license_configured)
      toast.success("CloakBrowser 设置已保存")
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "保存失败")
    } finally {
      setPending(false)
    }
  }

  async function checkCloakUpdate() {
    setCheckingUpdate(true)
    try {
      if (cloakLicense && !cloakLicense.includes("********")) {
        const cfg = await api.saveAutoConfig({ cloak_license_key: cloakLicense.trim() })
        setCloakLicense(cfg.cloak_license_key || "")
        setCloakConfigured(!!cfg.cloak_license_configured)
      }
      const out = await api.updateCloak()
      setCloakVersion(out.latest || out.current)
      if (out.updated) {
        toast.success(`发现新版本 ${out.latest}，正在后台下载并启用`)
      } else {
        toast.success(`已是最新版本 ${out.latest}`)
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "检测更新失败")
    } finally {
      setCheckingUpdate(false)
    }
  }

  async function saveAmzKeys(e?: FormEvent) {
    e?.preventDefault()
    const type = Math.floor(Number(amzCardType))
    const amount = Number(amzAmount)
    if (!Number.isFinite(type) || type <= 0) {
      toast.error("卡段须为正整数")
      return false
    }
    if (!Number.isFinite(amount) || amount <= 0) {
      toast.error("开卡金额须大于 0")
      return false
    }
    setPending(true)
    try {
      const body: Parameters<typeof api.saveAutoConfig>[0] = {
        amzkeys_host: amzHost.trim() || "https://testapi.amzkeys.com",
        amzkeys_app_id: amzAppID.trim(),
        amzkeys_card_type: type,
        amzkeys_card_amount: amount,
      }
      if (amzAppKey && !amzAppKey.includes("********")) {
        body.amzkeys_app_key = amzAppKey.trim()
      }
      if (amzPrivateKey && !amzPrivateKey.includes("********")) {
        body.amzkeys_private_key = amzPrivateKey.trim()
      }
      const cfg = await api.saveAutoConfig(body)
      setAmzHost(cfg.amzkeys_host || body.amzkeys_host || "")
      setAmzAppID(cfg.amzkeys_app_id || "")
      setAmzAppKey(cfg.amzkeys_app_key || "")
      setAmzPrivateKey(cfg.amzkeys_private_key || "")
      setAmzCardType(cfg.amzkeys_card_type || type)
      setAmzAmount(cfg.amzkeys_card_amount || amount)
      setAmzConfigured(!!cfg.amzkeys_configured)
      applyAmzCard(cfg)
      window.setTimeout(() => {
        api
          .autoConfig()
          .then((next) => {
            setAmzConfigured(!!next.amzkeys_configured)
            applyAmzCard(next)
          })
          .catch(() => {})
      }, 1000)
      toast.success("amzkeys卡台已保存")
      return true
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "保存失败")
      return false
    } finally {
      setPending(false)
    }
  }

  async function clearAmzCard() {
    setClearingCard(true)
    try {
      await api.clearAmzKeysCard()
      setAmzLast4("")
      setAmzPending(true)
      setAmzPayCount(0)
      setAmzNextLast4("")
      setAmzNextPending(false)
      setAmzCardError("")
      toast.success("已弃用当前卡，后台会再提前备一张")
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "弃用失败")
    } finally {
      setClearingCard(false)
    }
  }

  async function warmAmzCard() {
    setClearingCard(true)
    try {
      await api.warmAmzKeysCard()
      const cfg = await api.autoConfig()
      applyAmzCard(cfg)
      toast.success("已提交开卡。测试环境大约 2–5 分钟，页面会自动刷新状态")
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "开卡失败")
    } finally {
      setClearingCard(false)
    }
  }

  async function checkAmzKeys() {
    setCheckingAmz(true)
    try {
      if (!(await saveAmzKeys())) return
      const st = await api.amzKeysStatus()
      setAmzStatus(st)
      const usd = st.balances.find((b) => b.currency === "USD")
      toast.success(usd ? `连接成功，USD 可用 ${usd.available_amount}` : "连接成功")
    } catch (err) {
      setAmzStatus(null)
      toast.error(err instanceof Error ? err.message : "检测失败")
    } finally {
      setCheckingAmz(false)
    }
  }

  async function saveHeroSMS(e?: FormEvent) {
    e?.preventDefault()
    setPending(true)
    try {
      const body: Parameters<typeof api.saveAutoConfig>[0] = {
        hero_sms_service: service,
        hero_sms_country: country,
        hero_sms_max_price: maxPrice,
      }
      if (apiKey && !apiKey.includes("********")) {
        body.hero_sms_api_key = apiKey
      }
      const cfg = await api.saveAutoConfig(body)
      setApiKey(cfg.hero_sms_api_key || "")
      setConfigured(!!cfg.hero_sms_configured)
      toast.success("接码设置已保存")
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "保存失败")
    } finally {
      setPending(false)
    }
  }

  return (
    <div className="mx-auto w-full max-w-5xl space-y-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">设置</h1>
        <p className="mt-1 text-sm text-muted-foreground">按功能管理主程序与 Auto 自动化，每个标签独立保存。</p>
      </div>
      {!remoteSelected && activeTab !== "connection" && loadingConfig ? (
        <div role="status" className="rounded-xl border bg-muted/20 p-6 text-sm text-muted-foreground">正在读取已保存的设置…</div>
      ) : null}
      {!remoteSelected && activeTab !== "connection" && configError ? (
        <div role="alert" className="flex flex-col gap-4 rounded-xl border border-destructive/25 bg-destructive/5 p-5 sm:flex-row sm:items-center sm:justify-between">
          <div className="space-y-1">
            <p className="text-sm font-medium">设置加载失败</p>
            <p className="text-sm text-muted-foreground">{configError}。成功读取当前配置后才能修改或执行操作。</p>
          </div>
          <Button type="button" variant="outline" disabled={loadingConfig} onClick={() => setConfigAttempt((attempt) => attempt + 1)} className="w-fit shrink-0">重新加载</Button>
        </div>
      ) : null}
      <Tabs value={currentTab.value} onValueChange={(value) => setSearchParams({ tab: value }, { replace: true })}>
        <TabsList aria-label="设置分类" className="grid w-full grid-cols-1 gap-4 bg-transparent p-0 group-data-horizontal/tabs:h-auto lg:grid-cols-[3fr_4fr]">
          {(["主程序", "Auto 自动化"] as const).map((owner) => (
            <div key={owner} className="min-w-0 space-y-2">
              <p className="px-1 text-xs font-medium tracking-wide text-muted-foreground">{owner}</p>
              <div className={owner === "主程序" ? "grid grid-cols-2 gap-1 rounded-xl bg-muted p-1 sm:grid-cols-3" : "grid grid-cols-2 gap-1 rounded-xl bg-muted p-1 sm:grid-cols-4"}>
                {settingsTabs.filter((tab) => tab.owner === owner).map((tab) => (
                  <TabsTrigger key={tab.value} value={tab.value} className={"min-h-10 px-3 py-2 " + (tab.value === "forwarding" ? "col-span-2 sm:col-span-1" : "")}>{tab.label}</TabsTrigger>
                ))}
              </div>
            </div>
          ))}
        </TabsList>
        <div className="mt-5 flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
          <span className="rounded-md border bg-muted/40 px-2 py-0.5 text-xs">{currentTab.owner}</span>
          <span>{currentTab.description}</span>
        </div>
        {currentTab.owner === "Auto 自动化" ? (
          <div role="status" className="mt-2 flex flex-col gap-3 rounded-xl border bg-muted/20 p-4 sm:flex-row sm:items-center sm:justify-between">
            <div className="min-w-0 space-y-1">
              <p className="flex items-center gap-2 text-sm font-medium">
                <span className={"size-2 shrink-0 rounded-full " + (checkingAuto ? "bg-muted-foreground animate-pulse" : autoStatus?.connected ? "bg-emerald-500" : "bg-amber-500")} />
                {checkingAuto ? "正在检测 Auto 服务…" : autoStatus?.connected ? "Auto 服务已连接" : "Auto 服务未连接"}
              </p>
              {autoStatus?.url ? <p className="break-all text-xs text-muted-foreground">{autoStatus.url}</p> : null}
              {!checkingAuto && autoStatus && !autoStatus.connected ? (
                <p className="text-xs text-muted-foreground">请在「Auto 连接」中设置远程服务地址和访问令牌。{autoStatus.error ? "（" + autoStatus.error + "）" : ""}</p>
              ) : null}
            </div>
            <Button type="button" variant="outline" size="sm" disabled={checkingAuto} onClick={() => { void checkAutoStatus(); setAutoConfigAttempt((value) => value + 1) }} className="shrink-0 self-start sm:self-auto">
              {checkingAuto ? "检测中…" : "检测连接"}
            </Button>
          </div>
        ) : null}
        {remoteSelected && loadingAutoConfig ? (
          <div role="status" className="mt-3 rounded-xl border bg-muted/20 p-6 text-sm text-muted-foreground">正在读取 Auto 服务器的设置…</div>
        ) : null}
        {remoteSelected && autoConfigError ? (
          <div role="alert" className="mt-3 space-y-3 rounded-xl border border-destructive/25 bg-destructive/5 p-5">
            <p className="text-sm font-medium">无法读取 Auto 设置</p>
            <p className="break-words text-sm text-muted-foreground">{autoConfigError}</p>
            <div className="flex flex-wrap gap-2">
              <Button type="button" variant="outline" size="sm" disabled={loadingAutoConfig} onClick={() => setAutoConfigAttempt((value) => value + 1)}>重新加载</Button>
              <Button type="button" variant="outline" size="sm" onClick={() => setSearchParams({ tab: "connection" }, { replace: true })}>配置 Auto 连接</Button>
            </div>
          </div>
        ) : null}
        <TabsContent value="connection" className="mt-3">
          <Card>
            <CardHeader>
              <CardTitle>连接 Auto 服务器</CardTitle>
              <CardDescription>Auto 在另一台服务器运行。保存连接后，可在本程序创建提取任务、自动支付、查看日志，并将成功账号导入主程序账号池。</CardDescription>
            </CardHeader>
            <CardContent className="space-y-5">
              {connectionLoading ? <p role="status" className="text-sm text-muted-foreground">正在读取连接设置…</p> : null}
              {connectionError ? <div role="alert" className="space-y-2 text-sm"><p className="text-destructive">{connectionError}</p><Button variant="outline" disabled={connectionLoading} onClick={() => setConnectionAttempt((value) => value + 1)}>重新读取连接设置</Button></div> : null}
              <form onSubmit={saveConnection}>
                <fieldset disabled={!connectionLoaded || connectionLoading || savingConnection || testingConnection} className="grid min-w-0 gap-5">
                  <div className="grid gap-2">
                    <Label htmlFor="auto-url">Auto 服务地址</Label>
                    <Input id="auto-url" type="url" placeholder="https://auto.example.com:8081" value={autoURL} onChange={(e) => { setAutoURL(e.target.value); setDraftStatus(null) }} />
                    <p className="text-xs text-muted-foreground">填写主程序服务器能访问的 HTTP / HTTPS 地址，无需添加 /api。留空可断开连接。</p>
                  </div>
                  <div className="grid gap-2">
                    <Label htmlFor="auto-token">访问令牌</Label>
                    <Input id="auto-token" type="password" autoComplete="new-password" value={autoToken} onChange={(e) => { setAutoToken(e.target.value); setDraftStatus(null) }} placeholder={autoTokenConfigured ? "已保存，留空保持现有令牌" : "填写 Auto 服务器的访问令牌"} />
                    <p className="text-xs text-muted-foreground">填写 Auto 首次启动显示的密钥（保存在 Auto 数据目录的 auto-token 文件），或该服务器设置的 AUTO_TOKEN。</p>
                  </div>
                  <div className="flex flex-wrap gap-2">
                    <Button type="submit">{savingConnection ? "保存中…" : "保存连接"}</Button>
                    <Button type="button" variant="outline" disabled={!autoURL.trim()} onClick={() => void testConnection()}>{testingConnection ? "测试中…" : "测试连接"}</Button>
                  </div>
                </fieldset>
              </form>
              {draftStatus ? (
                <div role={draftStatus.connected ? "status" : "alert"} className={"rounded-lg border p-3 text-sm " + (draftStatus.connected ? "border-emerald-500/30 bg-emerald-500/5" : "border-destructive/25 bg-destructive/5")}>
                  <p className="font-medium">{draftStatus.connected ? "连接测试成功" : "连接测试失败"}</p>
                  <p className="mt-1 break-words text-muted-foreground">{draftStatus.connected ? "当前填写的地址和令牌可以连接。保存后应用到自动化操作。" : draftStatus.error || "无法连接 Auto 服务，请检查地址、令牌和服务器网络。"}</p>
                </div>
              ) : null}
              {autoStatus && !draftStatus ? <p role="status" className="break-words text-sm text-muted-foreground">{checkingAuto ? "正在检测已保存的连接…" : autoStatus.connected ? "已保存的 Auto 服务已连接" : `已保存的 Auto 服务未连接：${autoStatus.error || "请检查服务器地址和访问令牌"}`}</p> : null}
            </CardContent>
          </Card>
        </TabsContent>
        <TabsContent value="forwarding" className="mt-3">
          <fieldset disabled={!configLoaded || loadingConfig} hidden={!configLoaded} aria-busy={loadingConfig} className="min-w-0">
          <Card>
            <CardHeader>
              <CardTitle>转发与负载均衡</CardTitle>
              <CardDescription>设置网络出口、单账号速率和失败切换策略，以及返回给客户端的响应格式。</CardDescription>
            </CardHeader>
            <CardContent>
              <form onSubmit={saveForwarding} className="grid gap-6">
                <div className="grid gap-2">
                  <Label htmlFor="proxy">主程序网络代理</Label>
                  <Input
                    id="proxy"
                    placeholder="socks5://user:pass@host:1080"
                    value={proxy}
                    onChange={(e) => setProxy(e.target.value)}
                  />
                  <p className="text-xs text-muted-foreground">
                    留空则直连。用于主程序的用量同步；API 转发由下方开关单独控制。Auto 浏览器代理在「浏览器环境」中单独配置。
                  </p>
                </div>
                <div className="flex items-center justify-between rounded-lg border p-3">
                  <div>
                    <Label htmlFor="api-proxy">转发 API 走全局代理</Label>
                    <p className="text-xs text-muted-foreground">关闭后转发 Cline API 直连，用量同步仍使用上方代理。</p>
                  </div>
                  <Switch id="api-proxy" checked={apiProxy} onCheckedChange={setApiProxy} />
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="account-rpm">单账号 RPM</Label>
                  <Input
                    id="account-rpm"
                    type="number"
                    min={1}
                    max={1000}
                    step={1}
                    value={accountRpm}
                    onChange={(e) => setAccountRpm(Number(e.target.value))}
                  />
                  <p className="text-xs text-muted-foreground">
                    每个账号每分钟最多转发多少个请求，默认 5。达到上限后会换其他号。
                  </p>
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="max-retries">失败换号次数</Label>
                  <Input
                    id="max-retries"
                    type="number"
                    min={0}
                    max={32}
                    step={1}
                    value={maxRetries}
                    onChange={(e) => setMaxRetries(Number(e.target.value))}
                  />
                  <p className="text-xs text-muted-foreground">
                    上游账号额度 429/5xx 时再换几个号，0 表示不换号。边缘 HTML 429 和模型不支持图片这类错误不会换号，避免把限流打得更猛。
                  </p>
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="provider-mode">响应提供商标识（provider）</Label>
                  <select
                    id="provider-mode"
                    className={selectClass}
                    value={providerMode}
                    onChange={(e) => setProviderMode(e.target.value as "keep" | "hide" | "replace")}
                  >
                    <option value="keep">保留上游原值</option>
                    <option value="hide">从响应中删除</option>
                    <option value="replace">改成固定值</option>
                  </select>
                  {providerMode === "replace" ? (
                    <Input
                      id="provider-value"
                      aria-label="替换后的提供商标识"
                      placeholder="例如 OpenAI"
                      value={providerValue}
                      onChange={(e) => setProviderValue(e.target.value)}
                    />
                  ) : null}
                  <p className="text-xs text-muted-foreground">
                    只改转发给客户端的 JSON，流式和非流式都生效。默认不改。
                  </p>
                </div>
                <Button type="submit" disabled={pending} className="w-fit">
                  保存转发设置
                </Button>
              </form>
            </CardContent>
          </Card>
        </fieldset>
        </TabsContent>
        <TabsContent value="usage" className="mt-3 space-y-4">
          <ModelCatalogSyncCard />
          <fieldset disabled={!configLoaded || loadingConfig} hidden={!configLoaded} aria-busy={loadingConfig} className="min-w-0">
          <Card>
            <CardHeader>
              <CardTitle>用量同步</CardTitle>
              <CardDescription>由主程序同步账号配额与模型用量，为调度和仪表盘提供数据。修改后下一轮生效。</CardDescription>
            </CardHeader>
            <CardContent>
              <form onSubmit={saveUsage} className="grid gap-6">
                <div className="grid gap-2">
                  <Label htmlFor="usage-refresh-sec">套餐配额刷新间隔（秒）</Label>
                  <Input
                    id="usage-refresh-sec"
                    type="number"
                    min={15}
                    max={86400}
                    step={1}
                    value={usageRefreshSec}
                    onChange={(e) => setUsageRefreshSec(Number(e.target.value))}
                  />
                  <p className="text-xs text-muted-foreground">后台拉 5 小时 / 每周 / 每月配额的间隔，默认 60 秒。改完下一轮生效。</p>
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="model-usage-refresh-sec">模型用量刷新间隔（秒）</Label>
                  <Input
                    id="model-usage-refresh-sec"
                    type="number"
                    min={15}
                    max={86400}
                    step={1}
                    value={modelUsageRefreshSec}
                    onChange={(e) => setModelUsageRefreshSec(Number(e.target.value))}
                  />
                  <p className="text-xs text-muted-foreground">各模型花费单独刷新，默认 600 秒。手动点刷新用量仍会立刻拉一次。</p>
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="usage-refresh-concurrency">用量刷新并发</Label>
                  <Input
                    id="usage-refresh-concurrency"
                    type="number"
                    min={1}
                    max={64}
                    step={1}
                    value={usageRefreshConcurrency}
                    onChange={(e) => setUsageRefreshConcurrency(Number(e.target.value))}
                  />
                  <p className="text-xs text-muted-foreground">一次自动/手动刷新同时打多少个账号，默认 10，范围 1–64。</p>
                </div>
                <Button type="submit" disabled={pending} className="w-fit">
                  保存用量同步
                </Button>
              </form>
            </CardContent>
          </Card>
        </fieldset>
        </TabsContent>
        <TabsContent value="automation" className="mt-3">
          <fieldset disabled={!autoConfigLoaded || loadingAutoConfig} hidden={!autoConfigLoaded} aria-busy={loadingAutoConfig} className="min-w-0">
          <Card>
            <CardHeader>
              <CardTitle>提取与续期</CardTitle>
              <CardDescription>由独立 Auto 程序执行登录、支付链接提取与 Cookie 续期。这里分别设置提取和续期的并发。</CardDescription>
            </CardHeader>
            <CardContent>
              <form onSubmit={saveAutomation} className="grid gap-6">
                <div className="grid gap-2">
                  <Label htmlFor="max-concurrent">提取支付链接并发</Label>
                  <Input
                    id="max-concurrent"
                    type="number"
                    min={1}
                    step={1}
                    value={maxConcurrent}
                    onChange={(e) => setMaxConcurrent(Number(e.target.value))}
                  />
                  <p className="text-xs text-muted-foreground">同时打开多少个付费 Cloak 浏览器去提取链接，最少 1。改完立即生效。续 Cookie 不占这个席位。</p>
                </div>
                <div className="flex items-center justify-between rounded-lg border p-3">
                  <div>
                    <Label htmlFor="cookie-keep">每天续 Cookie</Label>
                    <p className="text-xs text-muted-foreground">尽量用免费 Cloak 续 Cookie，方便看滚动/周限重置时间。这些是一次性 $50 号，不保证邮箱还能登满 8 天；Cookie 没了仍按 API Key 调度。</p>
                  </div>
                  <Switch id="cookie-keep" checked={cookieKeepEnabled} onCheckedChange={setCookieKeepEnabled} />
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="cookie-keep-hour">每日开始时间（Auto 服务本地时间，0–23 点）</Label>
                  <Input
                    id="cookie-keep-hour"
                    type="number"
                    min={0}
                    max={23}
                    step={1}
                    value={cookieKeepHour}
                    onChange={(e) => setCookieKeepHour(Number(e.target.value))}
                    disabled={!cookieKeepEnabled}
                  />
                  <p className="text-xs text-muted-foreground">
                    {cookieKeepLastDate ? `上次入队日期 ${cookieKeepLastDate}。` : "今天还没跑过。"}
                    到点后单独入队，和提号互不抢付费席位。
                  </p>
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="cookie-keep-concurrency">续 Cookie 并发（1–32）</Label>
                  <Input
                    id="cookie-keep-concurrency"
                    type="number"
                    min={1}
                    max={32}
                    step={1}
                    value={cookieKeepConcurrency}
                    onChange={(e) => setCookieKeepConcurrency(Number(e.target.value))}
                    disabled={!cookieKeepEnabled}
                  />
                  <p className="text-xs text-muted-foreground">同时开多少个免费 Cloak 去续 Cookie。同一账号仍不会和提号并行。</p>
                </div>
                <Button
                  type="button"
                  variant="outline"
                  disabled={pending || keepingCookies}
                  className="w-fit"
                  onClick={async () => {
                    setKeepingCookies(true)
                    try {
                      const got = await api.keepPoolCookies()
                      toast.success(got.count ? `已入队 ${got.count} 个账号续 Cookie` : "没有可续的有效账号")
                      const cfg = await api.autoConfig()
                      setCookieKeepLastDate(cfg.cookie_keep_last_date || "")
                    } catch (err) {
                      toast.error(err instanceof Error ? err.message : "入队失败")
                    } finally {
                      setKeepingCookies(false)
                    }
                  }}
                >
                  {keepingCookies ? "正在入队…" : "立即续一次 Cookie"}
                </Button>
                <Button type="submit" disabled={pending} className="w-fit">
                  保存提取与续期
                </Button>
              </form>
            </CardContent>
          </Card>
        </fieldset>
        </TabsContent>
        <TabsContent value="browser" className="mt-3">
          <fieldset disabled={!autoConfigLoaded || loadingAutoConfig} hidden={!autoConfigLoaded} aria-busy={loadingAutoConfig} className="min-w-0">
          <Card>
            <CardHeader>
              <CardTitle>浏览器环境</CardTitle>
              <CardDescription>Auto 使用 CloakBrowser 完成登录与支付提取，在这里管理显示模式、版本和授权。</CardDescription>
            </CardHeader>
            <CardContent>
              <form onSubmit={saveCloak} className="grid gap-6">
                <div className="grid gap-2">
                  <Label htmlFor="auto-proxy">Auto 浏览器网络代理</Label>
                  <Input id="auto-proxy" placeholder="socks5://user:pass@host:1080" value={autoProxy} onChange={(e) => setAutoProxy(e.target.value)} />
                  <p className="text-xs text-muted-foreground">从 Auto 服务器访问此代理，用于登录、支付链接提取与 Cookie 续期。留空则由 Auto 服务器直连。</p>
                </div>
                <div className="flex items-center justify-between rounded-lg border p-3">
                  <div>
                    <Label htmlFor="headless">无头模式</Label>
                    <p className="text-xs text-muted-foreground">开启后不弹出浏览器窗口；需要手动处理验证码时请关闭。</p>
                  </div>
                  <Switch id="headless" checked={headless} onCheckedChange={setHeadless} />
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="cloak-version">当前版本</Label>
                  <Input
                    id="cloak-version"
                    placeholder="151.0.7922.108.2"
                    value={cloakVersion}
                    onChange={(e) => setCloakVersion(e.target.value)}
                  />
                  <p className="text-xs text-muted-foreground">
                    151 必须带 license。也可以点检测更新，有新版本会自动下载并切过去。
                  </p>
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="cloak-license">License</Label>
                  <Input
                    id="cloak-license"
                    type="password"
                    autoComplete="off"
                    placeholder={cloakConfigured ? "已保存，留空不改" : "CloakBrowser Pro license key"}
                    value={cloakLicense}
                    onChange={(e) => setCloakLicense(e.target.value)}
                  />
                  <p className="text-xs text-muted-foreground">
                    缺 key 时官方包装会报 license invalid/expired/missing。免费 key 在 cloakbrowser.dev/free。
                  </p>
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  <Button type="submit" disabled={pending} className="w-fit">
                    保存浏览器设置
                  </Button>
                  <Button type="button" variant="outline" disabled={pending || checkingUpdate} onClick={checkCloakUpdate}>
                    {checkingUpdate ? "检测中…" : "检测更新"}
                  </Button>
                </div>
              </form>
            </CardContent>
          </Card>
        </fieldset>
        </TabsContent>
        <TabsContent value="herosms" className="mt-3">
          <fieldset disabled={!autoConfigLoaded || loadingAutoConfig} hidden={!autoConfigLoaded} aria-busy={loadingAutoConfig} className="min-w-0">
          <Card>
            <CardHeader>
              <CardTitle>接码服务 · Hero SMS</CardTitle>
              <CardDescription>登录遇到手机验证时，按这里选好的区域和报价取号。</CardDescription>
            </CardHeader>
            <CardContent>
              <form onSubmit={saveHeroSMS} className="grid gap-6">
                <div className="grid gap-2">
                  <Label htmlFor="hero-key">API Key</Label>
                  <Input
                    id="hero-key"
                    type="password"
                    autoComplete="off"
                    placeholder={configured ? "已保存，留空不改" : "Hero SMS API Key"}
                    value={apiKey}
                    onChange={(e) => setApiKey(e.target.value)}
                  />
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  <Button type="button" variant="outline" disabled={loadingCatalog} onClick={onFetchCatalog}>
                    {loadingCatalog ? "拉取中…" : "拉取区域和报价"}
                  </Button>
                  {catalog ? (
                    <span className="text-sm text-muted-foreground">余额 {catalog.balance}</span>
                  ) : null}
                </div>
                {catalog?.services?.length ? (
                  <div className="grid gap-2">
                    <Label htmlFor="hero-service">服务</Label>
                    <select
                      id="hero-service"
                      className={selectClass}
                      value={service}
                      onChange={(e) => {
                        const next = e.target.value
                        setService(next)
                        loadCatalog(next).catch((err) => toast.error(err.message))
                      }}
                    >
                      {catalog.services.map((s) => (
                        <option key={s.code} value={s.code}>
                          {s.name} ({s.code})
                        </option>
                      ))}
                    </select>
                  </div>
                ) : (
                  <div className="grid gap-2">
                    <Label htmlFor="hero-service-input">服务代码</Label>
                    <Input id="hero-service-input" value={service} onChange={(e) => setService(e.target.value)} placeholder="ot" />
                    <p className="text-xs text-muted-foreground">AuthKit 手机验证一般用 ot（其他）。拉取报价后会列出可选服务。</p>
                  </div>
                )}
                <div className="grid gap-2">
                  <Label htmlFor="hero-country">区域</Label>
                  <select
                    id="hero-country"
                    className={selectClass}
                    value={country || ""}
                    onChange={(e) => {
                      const id = Number(e.target.value)
                      setCountry(id)
                      const c = countries.find((x) => x.id === id)
                      setMaxPrice(c ? lowestQuote(c)?.price || 0 : 0)
                    }}
                    disabled={!countries.length}
                  >
                    <option value="">{countries.length ? "请选择区域" : "先拉取报价"}</option>
                    {countries.map((c) => (
                      <option key={c.id} value={c.id}>
                        {countryOptionLabel(c)}
                      </option>
                    ))}
                  </select>
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="hero-quote">报价</Label>
                  <select
                    id="hero-quote"
                    className={selectClass}
                    value={maxPrice || ""}
                    onChange={(e) => setMaxPrice(Number(e.target.value))}
                    disabled={!quotes.length}
                  >
                    <option value="">{quotes.length ? "请选择报价" : "先选择区域"}</option>
                    {quotes.map((q) => (
                      <option key={String(q.price)} value={q.price}>
                        {q.price} {q.count ? `· 库存 ${q.count}` : ""}
                      </option>
                    ))}
                  </select>
                </div>
                <Button type="submit" disabled={pending || !country} className="w-fit">
                  保存接码设置
                </Button>
              </form>
            </CardContent>
          </Card>
        </fieldset>
        </TabsContent>
        <TabsContent value="amzkeys" className="mt-3">
          <fieldset disabled={!autoConfigLoaded || loadingAutoConfig} hidden={!autoConfigLoaded} aria-busy={loadingAutoConfig} className="min-w-0">
          <Card>
            <CardHeader>
              <CardTitle>支付卡台 · amzkeys</CardTitle>
              <CardDescription>
                一张卡大约能付 3 个账户（每个 5.3 美金）。测试环境用官方文档凭据，不用填自己的 AppID/密钥。生产才需要商务给的那套，并把公钥配到卡台。
              </CardDescription>
            </CardHeader>
            <CardContent>
              <form onSubmit={saveAmzKeys} className="grid gap-6">
                <div className="grid gap-2">
                  <Label htmlFor="amz-host">API Host</Label>
                  <Input
                    id="amz-host"
                    placeholder="https://testapi.amzkeys.com"
                    value={amzHost}
                    onChange={(e) => setAmzHost(e.target.value)}
                  />
                  <p className="text-xs text-muted-foreground">
                    测试用 https://testapi.amzkeys.com，不用填 AppID/AppKey/私钥。切生产改成 https://ymapi.amzkeys.com:15970，再填商务给的凭据，并在卡台网页配好 RSA 公钥和本机出口 IP 白名单。
                  </p>
                </div>
                {!/testapi\.amzkeys\.com/i.test(amzHost.trim() || "https://testapi.amzkeys.com") ? (
                  <>
                <div className="grid gap-2">
                  <Label htmlFor="amz-app-id">AppID</Label>
                  <Input
                    id="amz-app-id"
                    placeholder="商务提供的生产 AppID"
                    value={amzAppID}
                    onChange={(e) => setAmzAppID(e.target.value)}
                  />
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="amz-app-key">AppKey</Label>
                  <Input
                    id="amz-app-key"
                    type="password"
                    autoComplete="off"
                    placeholder={amzConfigured ? "已保存，留空不改" : "商务提供的生产 AppKey"}
                    value={amzAppKey}
                    onChange={(e) => setAmzAppKey(e.target.value)}
                  />
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="amz-private-key">RSA2 私钥</Label>
                  <Textarea
                    id="amz-private-key"
                    rows={6}
                    autoComplete="off"
                    placeholder={amzConfigured ? "已保存，留空不改" : "生产私钥，公钥要先配到卡台网页"}
                    value={amzPrivateKey}
                    onChange={(e) => setAmzPrivateKey(e.target.value)}
                  />
                </div>
                  </>
                ) : (
                  <p className="rounded-lg border p-3 text-xs text-muted-foreground">
                    当前是测试环境，已自动使用官方文档里的测试 AppID、AppKey 和私钥。你自己生成或商务给的那套只在生产有效。
                  </p>
                )}
                <div className="grid gap-2">
                  <Label htmlFor="amz-card-type">卡段</Label>
                  <Input
                    id="amz-card-type"
                    type="number"
                    min={1}
                    step={1}
                    value={amzCardType}
                    onChange={(e) => setAmzCardType(Number(e.target.value))}
                  />
                  <p className="text-xs text-muted-foreground">
                    测试卡段 467845。生产卡段不是这个，点「检测连接」看可开卡列表再填，金额不要低于卡台返回的最低开卡额。
                  </p>
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="amz-amount">每张开卡金额（USD）</Label>
                  <Input
                    id="amz-amount"
                    type="number"
                    min={1}
                    step={0.01}
                    value={amzAmount}
                    onChange={(e) => setAmzAmount(Number(e.target.value))}
                  />
                  <p className="text-xs text-muted-foreground">
                    只在开新卡时扣这一笔。建议不少于 16（3×5.3），默认 20 刚好够付 3 个账户。
                  </p>
                </div>
                <div className="flex flex-col gap-4 rounded-lg border p-3 sm:flex-row sm:items-center sm:justify-between">
                  <div>
                    <Label>当前在用的卡</Label>
                    <p className="text-xs text-muted-foreground">
                      {amzLast4
                        ? `**** ${amzLast4}，已付 ${amzPayCount}/${amzMaxPays || 3} 个账户${
                            amzNextLast4
                              ? `，下一张 **** ${amzNextLast4} 已备好`
                              : amzNextPending
                                ? "，下一张正在开（大约 2–5 分钟）"
                                : (amzMaxPays || 3) - amzPayCount <= 1
                                  ? "，快用完了会自动再开一张"
                                  : ""
                          }`
                        : amzPending
                          ? "开卡任务已提交，测试环境大约要 2–5 分钟，不会再开第二张。登录抽链接时会一起等。"
                          : amzCardError
                            ? `上次开卡失败：${amzCardError}`
                            : "还没有卡。保存卡台后会自动提前备一张"}
                    </p>
                  </div>
                  <div className="flex flex-wrap gap-2">
                    <Button type="button" variant="outline" disabled={!amzConfigured || !!amzLast4 || amzPending || clearingCard} onClick={warmAmzCard}>
                      {amzPending ? "开卡中…" : "提前开一张"}
                    </Button>
                    <Button type="button" variant="outline" disabled={!amzLast4 || clearingCard} onClick={clearAmzCard}>
                      {clearingCard ? "处理中…" : "弃用当前卡"}
                    </Button>
                  </div>
                </div>
                {amzStatus?.balances?.length ? (
                  <p className="text-sm text-muted-foreground">
                    {amzStatus.balances.map((b) => `${b.currency} 可用 ${b.available_amount}`).join("，")}
                    {amzStatus.card_types?.length
                      ? `；可开卡段 ${amzStatus.card_types.map((c) => c.card_type).join("、")}`
                      : ""}
                  </p>
                ) : null}
                <div className="flex flex-wrap items-center gap-2">
                  <Button type="submit" disabled={pending} className="w-fit">
                    保存 amzkeys卡台
                  </Button>
                  <Button type="button" variant="outline" disabled={pending || checkingAmz} onClick={checkAmzKeys}>
                    {checkingAmz ? "检测中…" : "检测连接"}
                  </Button>
                </div>
              </form>
            </CardContent>
          </Card>
        </fieldset>
        </TabsContent>
      </Tabs>
    </div>
  )
}
