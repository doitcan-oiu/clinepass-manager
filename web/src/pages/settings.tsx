import { useEffect, useMemo, useState, type FormEvent } from "react"
import { toast } from "sonner"
import { api, type HeroSMSCatalog, type HeroSMSCountry } from "@/lib/api"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"

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
  const [apiKey, setApiKey] = useState("")
  const [configured, setConfigured] = useState(false)
  const [service, setService] = useState("ot")
  const [country, setCountry] = useState(0)
  const [maxPrice, setMaxPrice] = useState(0)
  const [catalog, setCatalog] = useState<HeroSMSCatalog | null>(null)
  const [pending, setPending] = useState(false)
  const [loadingCatalog, setLoadingCatalog] = useState(false)

  useEffect(() => {
    api
      .config()
      .then((cfg) => {
        setProxy(cfg.proxy || "")
        setHeadless(cfg.headless !== false)
        setMaxConcurrent(cfg.max_concurrent >= 1 ? cfg.max_concurrent : 1)
        setApiKey(cfg.hero_sms_api_key || "")
        setConfigured(!!cfg.hero_sms_configured)
        setService(cfg.hero_sms_service || "ot")
        setCountry(cfg.hero_sms_country || 0)
        setMaxPrice(cfg.hero_sms_max_price || 0)
        if (cfg.hero_sms_configured) {
          loadCatalog(cfg.hero_sms_service || "ot").catch(() => {})
        }
      })
      .catch((e) => toast.error(e.message))
  }, [])

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
      await api.saveConfig({ proxy, headless, max_concurrent: n })
      setMaxConcurrent(n)
      toast.success("运行环境已保存")
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "保存失败")
    } finally {
      setPending(false)
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
        <TabsList className="grid w-full grid-cols-2">
          <TabsTrigger value="runtime">运行环境</TabsTrigger>
          <TabsTrigger value="herosms">Hero SMS</TabsTrigger>
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
                  <p className="text-xs text-muted-foreground">留空则直连。</p>
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
                  <p className="text-xs text-muted-foreground">同时打开多少个浏览器去提取链接，最少 1，没有上限。改完立即生效。</p>
                </div>
                <Button type="submit" disabled={pending} className="w-fit">
                  保存运行环境
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
      </Tabs>
    </div>
  )
}
