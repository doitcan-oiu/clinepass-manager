import os

from protocol import log

DEFAULT_TIMEZONE = "Asia/Shanghai"
DEFAULT_LOCALE = "zh-CN"
GEOIP_MIN_BYTES = 1_000_000


def apply_cloak_env(settings: dict) -> None:
    mapping = {
        "cloak_cache_dir": "CLOAKBROWSER_CACHE_DIR",
        "cloak_binary_path": "CLOAKBROWSER_BINARY_PATH",
        "license_key": "CLOAKBROWSER_LICENSE_KEY",
    }
    for key, env in mapping.items():
        val = (settings.get(key) or "").strip()
        if val:
            os.environ[env] = val
    version = (settings.get("cloak_version") or "").strip()
    if version and (settings.get("license_key") or os.environ.get("CLOAKBROWSER_LICENSE_KEY")):
        os.environ["CLOAKBROWSER_VERSION"] = version


def chrome_args(settings: dict, seed: int) -> list[str]:
    args = [
        "--no-sandbox",
        "--disable-setuid-sandbox",
        "--disable-dev-shm-usage",
        "--fingerprint-storage-quota=5000",
        "--ignore-gpu-blocklist",
        "--fingerprint-windows-font-metrics",
        "--fingerprint-allow-3p-cookies",
    ]
    if seed > 0:
        args.append(f"--fingerprint={seed}")
    if settings.get("virtual_display") or settings.get("headless"):
        args.append("--disable-gpu")
    return args


def cloak_cache_dir() -> str:
    return os.environ.get("CLOAKBROWSER_CACHE_DIR") or os.path.expanduser("~/.cloakbrowser")


def geoip_db_path(root: str | None = None) -> str:
    return os.path.join(root or cloak_cache_dir(), "geoip", "GeoLite2-City.mmdb")


def geoip_db_ready(root: str | None = None) -> bool:
    path = geoip_db_path(root)
    try:
        return os.path.isfile(path) and os.path.getsize(path) >= GEOIP_MIN_BYTES
    except OSError:
        return False


def humanize_options() -> dict:
    """Keep Cloak mouse curves, but do not use the slow careful preset.

    careful + idle_between_actions made every click/key wait 350–900ms, so a
    single Google email took ~20s. Paid Cloak fingerprints no longer need that.
    """
    return {
        "humanize": True,
        "human_preset": "default",
        "human_config": {
            "typing_delay": 22,
            "typing_delay_spread": 10,
            "typing_pause_chance": 0.02,
            "mistype_chance": 0,
            "idle_between_actions": False,
            "idle_between_duration": [0.05, 0.12],
        },
    }


def apply_geo_settings(kwargs: dict, proxy: str | None, root: str | None = None) -> dict:
    """Only follow Cloak geoip when the 70MB City DB is already on disk.

    First launch otherwise downloads GeoLite2 from GitHub (httpx timeout 300s)
    and that download is not bounded by CLOAKBROWSER_GEOIP_TIMEOUT_SECONDS.
    A stalled GitHub fetch looks like a hang at「正在启动浏览器」.
    """
    if proxy and geoip_db_ready(root):
        kwargs["geoip"] = True
        kwargs.pop("timezone", None)
        kwargs.pop("locale", None)
        return kwargs
    kwargs["geoip"] = False
    kwargs["timezone"] = DEFAULT_TIMEZONE
    kwargs["locale"] = DEFAULT_LOCALE
    return kwargs


def launch_ctx(settings: dict, seed: int):
    from cloakbrowser import launch_persistent_context

    apply_cloak_env(settings)
    profile = (settings.get("profile_dir") or "").strip()
    if not profile:
        raise RuntimeError("缺少浏览器配置目录")
    os.makedirs(profile, exist_ok=True)
    proxy = (settings.get("proxy") or "").strip() or None
    headless = bool(settings.get("headless"))
    license_key = (settings.get("license_key") or "").strip()
    version = (settings.get("cloak_version") or "").strip()
    args = chrome_args(settings, seed)
    if not license_key and not os.environ.get("CLOAKBROWSER_LICENSE_KEY"):
        log("没有 Cloak license，官方包装只会下免费 146；151 需要 cloakbrowser.dev/free 的 key，否则 AuthKit Radar 更容易拦截")
    kwargs = {
        "user_data_dir": profile,
        "headless": headless,
        **humanize_options(),
        "args": args,
    }
    apply_geo_settings(kwargs, proxy)
    if proxy:
        kwargs["proxy"] = proxy
        if kwargs.get("geoip"):
            log("使用全局代理，geoip 跟已有本地库解析出口")
        else:
            log("使用全局代理，但没有本地 GeoIP 库，跳过 70MB 下载，时区固定 %s", DEFAULT_TIMEZONE)
    else:
        log("未设代理，不下载 GeoIP，时区固定 %s / %s，避免默认 UTC/en-US", DEFAULT_TIMEZONE, DEFAULT_LOCALE)
    log(
        "Cloak 官方包装启动 humanize=default geoip=%s tz=%s locale=%s headless=%s seed=%s version=%s cache=%s runtime=%s",
        kwargs.get("geoip"),
        kwargs.get("timezone") or "",
        kwargs.get("locale") or "",
        headless,
        seed,
        version or "latest",
        os.environ.get("CLOAKBROWSER_CACHE_DIR") or "",
        os.environ.get("XDG_RUNTIME_DIR") or "",
    )
    if license_key:
        kwargs["license_key"] = license_key
    if version and (license_key or os.environ.get("CLOAKBROWSER_LICENSE_KEY")):
        kwargs["browser_version"] = version
    log("正在启动浏览器")
    try:
        ctx = launch_persistent_context(**kwargs)
    except Exception as exc:
        if kwargs.get("geoip"):
            log("geoip 启动失败，改为固定时区 %s: %s", DEFAULT_TIMEZONE, exc)
            kwargs["geoip"] = False
            kwargs["timezone"] = DEFAULT_TIMEZONE
            kwargs["locale"] = DEFAULT_LOCALE
            ctx = launch_persistent_context(**kwargs)
        else:
            raise
    log("浏览器已启动")
    return ctx
