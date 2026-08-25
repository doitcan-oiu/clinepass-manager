import os

from protocol import log


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
    log(
        "Cloak 官方包装启动 humanize=careful geoip=True headless=%s seed=%s version=%s cache=%s runtime=%s",
        headless,
        seed,
        version or "latest",
        os.environ.get("CLOAKBROWSER_CACHE_DIR") or "",
        os.environ.get("XDG_RUNTIME_DIR") or "",
    )
    kwargs = {
        "user_data_dir": profile,
        "headless": headless,
        "humanize": True,
        "human_preset": "careful",
        "human_config": {
            "idle_between_actions": True,
            "idle_between_duration": [0.35, 0.9],
            "mistype_chance": 0.04,
        },
        "geoip": True,
        "args": args,
    }
    if proxy:
        kwargs["proxy"] = proxy
        log("使用全局代理，geoip 跟出口 IP")
    else:
        log("未设代理，geoip 跟本机出口，避免默认 UTC/en-US 被当成机器人")
    if license_key:
        kwargs["license_key"] = license_key
    if version and (license_key or os.environ.get("CLOAKBROWSER_LICENSE_KEY")):
        kwargs["browser_version"] = version
    log("正在启动浏览器")
    try:
        ctx = launch_persistent_context(**kwargs)
    except Exception as exc:
        if kwargs.get("geoip"):
            log("geoip 启动失败，改为不跟出口时区: %s", exc)
            kwargs["geoip"] = False
            ctx = launch_persistent_context(**kwargs)
        else:
            raise
    log("浏览器已启动")
    return ctx
