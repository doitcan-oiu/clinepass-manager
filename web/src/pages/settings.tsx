import { useEffect, useMemo, useState, type FormEvent } from "react"
import { toast } from "sonner"
import { api, type AmzKeysStatus, type HeroSMSCatalog, type HeroSMSCountry } from "@/lib/api"
import type { AppConfig } from "@/lib/types"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"

const selectClass = "h-9 w-full rounded-md border border-input bg-transparent px-3 text-sm shadow-xs outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"

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

export function SettingsPage() {
  const [proxy, setProxy] = useState("")
  const [headless, setHeadless] = useState(true)
  const [maxConcurrent, setMaxConcurrent] = useState(1)
  const [cookieKeepEnabled, setCookieKeepEnabled] = useState(true)
  const [cookieKeepHour, setCookieKeepHour] = useState(4)
  const [cookieKeepLastDate, setCookieKeepLastDate] = useState("")
  const [keepingCookies, setKeepingCookies] = useState(false)
  const [maxRetries, setMaxRetries] = useState(3)
  const [accountRpm, setAccountRpm] = useState(5)
  const [apiProxy, setApiProxy] = useState(true)
  const [usageRefreshSec, setUsageRefreshSec] = useState(60)
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
    api
      .config()
      .then((cfg) => {
        setProxy(cfg.proxy || "")
        setHeadless(cfg.headless !== false)
        setMaxConcurrent(cfg.max_concurrent >= 1 ? cfg.max_concurrent : 1)
        setCookieKeepEnabled(cfg.cookie_keep_enabled !== false)
        setCookieKeepHour(Number.isFinite(cfg.cookie_keep_hour) && cfg.cookie_keep_hour >= 0 && cfg.cookie_keep_hour <= 23 ? cfg.cookie_keep_hour : 4)
        setCookieKeepLastDate(cfg.cookie_keep_last_date || "")
        setMaxRetries(Number.isFinite(cfg.max_retries) && cfg.max_retries >= 0 ? cfg.max_retries : 3)
        setAccountRpm(cfg.account_rpm >= 1 ? cfg.account_rpm : 5)
        setApiProxy(cfg.api_proxy !== false)
        setUsageRefreshSec(cfg.usage_refresh_sec >= 15 ? cfg.usage_refresh_sec : 60)
        setUsageRefreshConcurrency(cfg.usage_refresh_concurrency >= 1 ? cfg.usage_refresh_concurrency : 10)
        setProviderMode(cfg.provider_mode === "hide" || cfg.provider_mode === "replace" ? cfg.provider_mode : "keep")
        setProviderValue(cfg.provider_value || "")
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
        if (cfg.hero_sms_configured) {
          loadCatalog(cfg.hero_sms_service || "ot").catch(() => {})
        }
      })
      .catch((e) => toast.error(e.message))
  }, [])

  useEffect(() => {
    if (!amzPending && !amzNextPending) return
    const timer = window.setInterval(() => {
      api
        .config()
        .then((cfg) => {
          setAmzConfigured(!!cfg.amzkeys_configured)
          applyAmzCard(cfg)
        })
        .catch(() => {})
    }, 3000)
    return () => window.clearInterval(timer)
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

  async function saveRuntime(e?: FormEvent) {
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
      await api.saveConfig({
        proxy,
        headless,
        max_concurrent: n,
        provider_mode: providerMode,
        provider_value: providerValue,
        cookie_keep_enabled: cookieKeepEnabled,
        cookie_keep_hour: hour,
      })
      setMaxConcurrent(n)
      setCookieKeepHour(hour)
      toast.success("运行环境已保存")
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "保存失败")
    } finally {
      setPending(false)
    }
  }

  async function saveAccount(e?: FormEvent) {
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
      const refreshSec = Math.floor(Number(usageRefreshSec))
      if (!Number.isFinite(refreshSec) || refreshSec < 15 || refreshSec > 86400) {
        toast.error("用量刷新间隔须在 15–86400 秒")
        return
      }
      const refreshConc = Math.floor(Number(usageRefreshConcurrency))
      if (!Number.isFinite(refreshConc) || refreshConc < 1 || refreshConc > 64) {
        toast.error("用量刷新并发须在 1–64")
        return
      }
      await api.saveConfig({
        max_retries: retries,
        account_rpm: rpm,
        api_proxy: apiProxy,
        usage_refresh_sec: refreshSec,
        usage_refresh_concurrency: refreshConc,
      })
      setMaxRetries(retries)
      setAccountRpm(rpm)
      setUsageRefreshSec(refreshSec)
      setUsageRefreshConcurrency(refreshConc)
      toast.success("账号设置已保存")
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
      const body: Parameters<typeof api.saveConfig>[0] = { cloak_version: version }
      if (cloakLicense && !cloakLicense.includes("********")) {
        body.cloak_license_key = cloakLicense.trim()
      }
      const cfg = await api.saveConfig(body)
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
        const cfg = await api.saveConfig({ cloak_license_key: cloakLicense.trim() })
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
      const body: Parameters<typeof api.saveConfig>[0] = {
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
      const cfg = await api.saveConfig(body)
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
          .config()
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
      const cfg = await api.config()
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
      const body: Parameters<typeof api.saveConfig>[0] = {
        hero_sms_service: service,
        hero_sms_country: country,
        hero_sms_max_price: maxPrice,
      }
      if (apiKey && !apiKey.includes("********")) {
        body.hero_sms_api_key = apiKey
      }
      const cfg = await api.saveConfig(body)
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
    <div className="mx-auto w-full max-w-2xl space-y-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">设置</h1>
        <p className="text-sm text-muted-foreground">按分组切换，只改当前这一组。</p>
      </div>
      <Tabs defaultValue="runtime">
        <TabsList className="flex h-auto w-full flex-wrap justify-start gap-1">
          <TabsTrigger value="runtime">运行环境</TabsTrigger>
          <TabsTrigger value="cloak">CloakBrowser</TabsTrigger>
          <TabsTrigger value="account">账号设置</TabsTrigger>
          <TabsTrigger value="herosms">Hero SMS</TabsTrigger>
          <TabsTrigger value="amzkeys">amzkeys卡台</TabsTrigger>
        </TabsList>
        <TabsContent value="runtime" className="mt-4">
          <Card>
            <CardHeader>
              <CardTitle>运行环境</CardTitle>
              <CardDescription>出现验证码时请关闭无头模式。</CardDescription>
            </CardHeader>
            <CardContent>
              <form onSubmit={saveRuntime} className="grid gap-6">
                <div className="grid gap-2">
                  <Label htmlFor="proxy">全局代理</Label>
                  <Input
                    id="proxy"
                    placeholder="socks5://user:pass@host:1080"
                    value={proxy}
                    onChange={(e) => setProxy(e.target.value)}
                  />
                  <p className="text-xs text-muted-foreground">
                    留空则直连。登录提取支付、用量刷新会走这里。转发 Cline API 是否走代理由「账号设置」里的开关决定。带账密的 SOCKS5（例如 1024proxy）Chrome 不支持，会自动起本地中继再出去。
                  </p>
                </div>
                <div className="flex items-center justify-between rounded-lg border p-3">
                  <div>
                    <Label htmlFor="headless">无头模式</Label>
                    <p className="text-xs text-muted-foreground">开启后不弹出浏览器窗口。</p>
                  </div>
                  <Switch id="headless" checked={headless} onCheckedChange={setHeadless} />
                </div>
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
                  <p className="text-xs text-muted-foreground">同时打开多少个浏览器去提取链接，最少 1，没有上限。改完立即生效。每日续 Cookie 也走这条队列，提号时会排队。</p>
                </div>
                <div className="flex items-center justify-between rounded-lg border p-3">
                  <div>
                    <Label htmlFor="cookie-keep">每天续 Cookie</Label>
                    <p className="text-xs text-muted-foreground">对有效（已付款）账号用浏览器打开 dashboard，换新 Cookie。和提号共用并发。</p>
                  </div>
                  <Switch id="cookie-keep" checked={cookieKeepEnabled} onCheckedChange={setCookieKeepEnabled} />
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="cookie-keep-hour">每天几点开始（0–23）</Label>
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
                    到点后入队，不会另开一套浏览器并发。
                  </p>
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
                      const cfg = await api.config()
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
                <div className="grid gap-2">
                  <Label htmlFor="provider-mode">响应里的 provider</Label>
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
                  保存运行环境
                </Button>
              </form>
            </CardContent>
          </Card>
        </TabsContent>
        <TabsContent value="cloak" className="mt-4">
          <Card>
            <CardHeader>
              <CardTitle>CloakBrowser</CardTitle>
              <CardDescription>登录用的浏览器版本和 License，可检测官方最新版并下载启用。</CardDescription>
            </CardHeader>
            <CardContent>
              <form onSubmit={saveCloak} className="grid gap-6">
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
                    保存 CloakBrowser
                  </Button>
                  <Button type="button" variant="outline" disabled={pending || checkingUpdate} onClick={checkCloakUpdate}>
                    {checkingUpdate ? "检测中…" : "检测更新"}
                  </Button>
                </div>
              </form>
            </CardContent>
          </Card>
        </TabsContent>
        <TabsContent value="account" className="mt-4">
          <Card>
            <CardHeader>
              <CardTitle>账号设置</CardTitle>
              <CardDescription>控制单账号转发速率、API 是否走代理、失败换号和用量刷新。</CardDescription>
            </CardHeader>
            <CardContent>
              <form onSubmit={saveAccount} className="grid gap-6">
                <div className="flex items-center justify-between rounded-lg border p-3">
                  <div>
                    <Label htmlFor="api-proxy">转发 API 走全局代理</Label>
                    <p className="text-xs text-muted-foreground">关闭后转发 Cline API 直连，提取支付链接仍走全局代理。</p>
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
                  <Label htmlFor="usage-refresh-sec">用量刷新间隔（秒）</Label>
                  <Input
                    id="usage-refresh-sec"
                    type="number"
                    min={15}
                    max={86400}
                    step={1}
                    value={usageRefreshSec}
                    onChange={(e) => setUsageRefreshSec(Number(e.target.value))}
                  />
                  <p className="text-xs text-muted-foreground">后台自动拉套餐用量的间隔，默认 60 秒。改完下一轮生效，账号页会跟着更新。</p>
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
                  保存账号设置
                </Button>
              </form>
            </CardContent>
          </Card>
        </TabsContent>
        <TabsContent value="herosms" className="mt-4">
          <Card>
            <CardHeader>
              <CardTitle>Hero SMS 接码</CardTitle>
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
        </TabsContent>
        <TabsContent value="amzkeys" className="mt-4">
          <Card>
            <CardHeader>
              <CardTitle>amzkeys卡台</CardTitle>
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
                <div className="flex items-center justify-between rounded-lg border p-3">
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
        </TabsContent>
      </Tabs>
    </div>
  )
}
