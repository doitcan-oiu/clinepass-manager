import os

from protocol import log


def apply_cloak_env(settings: dict) -> None:
    mapping = {
        "cloak_version": "CLOAKBROWSER_VERSION",
        "cloak_cache_dir": "CLOAKBROWSER_CACHE_DIR",
        "cloak_binary_path": "CLOAKBROWSER_BINARY_PATH",
        "license_key": "CLOAKBROWSER_LICENSE_KEY",
    }
    for key, env in mapping.items():
        val = (settings.get(key) or "").strip()
        if val:
            os.environ[env] = val


def chrome_args(settings: dict, seed: int) -> list[str]:
    args = [
        "--no-sandbox",
        "--disable-setuid-sandbox",
        "--disable-dev-shm-usage",
        "--fingerprint-storage-quota=5000",
        "--ignore-gpu-blocklist",
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
    args = chrome_args(settings, seed)
    log(
        "Cloak 官方包装启动 humanize=True geoip=%s headless=%s seed=%s cache=%s runtime=%s",
        bool(proxy),
        headless,
        seed,
        os.environ.get("CLOAKBROWSER_CACHE_DIR") or "",
        os.environ.get("XDG_RUNTIME_DIR") or "",
    )
    kwargs = {
        "user_data_dir": profile,
        "headless": headless,
        "humanize": True,
        "geoip": bool(proxy),
        "args": args,
    }
    if proxy:
        kwargs["proxy"] = proxy
        log("使用全局代理，geoip 跟出口 IP")
    if (settings.get("license_key") or "").strip():
        kwargs["license_key"] = settings["license_key"].strip()
    if (settings.get("cloak_version") or "").strip():
        kwargs["browser_version"] = settings["cloak_version"].strip()
    log("正在启动浏览器")
    ctx = launch_persistent_context(**kwargs)
    log("浏览器已启动")
    return ctx
