from urllib.parse import urlparse, parse_qs


AUTH_HOST = "authkit.cline.bot"
APP_HOST = "app.cline.bot"
API_HOST = "api.cline.bot"


def url_host(raw: str) -> str:
    try:
        return (urlparse(raw).hostname or "").lower()
    except Exception:
        return ""


def url_path(raw: str) -> str:
    try:
        return urlparse(raw).path or raw
    except Exception:
        return raw


def authkit_query(raw: str, key: str) -> str:
    try:
        vals = parse_qs(urlparse(raw).query).get(key) or []
        return (vals[0] or "").strip()
    except Exception:
        return ""


def authkit_session_id(raw: str) -> str:
    return authkit_query(raw, "authorization_session_id")


def authkit_callback_error(raw: str) -> str:
    return authkit_query(raw, "error")


def on_radar_url(raw: str) -> bool:
    path = url_path(raw)
    return "radar-challenge" in path or "/radar" in path


def on_cline_app(raw: str) -> bool:
    return url_host(raw) == APP_HOST


def on_cline(raw: str) -> bool:
    return on_cline_app(raw) or on_radar_url(raw)


def url_has_any(raw: str, parts) -> bool:
    u = raw or ""
    return any(p in u for p in parts)


def on_workos_url(raw: str) -> bool:
    host = url_host(raw)
    return "workos.com" in host


def on_microsoft_url(raw: str) -> bool:
    host = url_host(raw)
    if not host:
        return False
    return (
        "microsoftonline.com" in host
        or "login.live.com" in host
        or "account.live.com" in host
        or host == "login.microsoft.com"
    )


def on_authkit_login(raw: str) -> bool:
    if url_host(raw) != AUTH_HOST:
        return False
    if on_radar_url(raw):
        return False
    if "/api/" in url_path(raw):
        return False
    return True


def authkit_banned_after_wait(raw: str) -> bool:
    return (
        bool(authkit_session_id(raw))
        and url_host(raw) == AUTH_HOST
        and not on_radar_url(raw)
        and not on_cline(raw)
    )


def cookie_expired(raw: str) -> bool:
    host = url_host(raw)
    if host.endswith("google.com") or host.endswith("google.com.hk") or on_microsoft_url(raw):
        return True
    return host == AUTH_HOST and not on_radar_url(raw)


def left_identity_url(raw: str) -> bool:
    if not raw or raw == "about:blank" or raw.startswith("chrome-error") or raw.startswith("chrome://"):
        return False
    host = url_host(raw)
    if not host or host.endswith("google.com") or host.endswith("google.com.hk") or on_microsoft_url(raw):
        return False
    if on_authkit_login(raw):
        return False
    return on_cline(raw) or host == API_HOST


def classify_google(raw: str) -> str:
    path = url_path(raw)
    if "workspacetermsofservice" in path:
        return "tos"
    if "/challenge/pwd" in path:
        return "password"
    if "/signin/identifier" in path or "/v3/signin/identifier" in path:
        return "email"
    if "/signin/oauth" in path:
        return "consent"
    if "accountchooser" in path:
        return "chooser"
    if "unknownerror" in path:
        return "unknownerror"
    return ""


def google_continue_url(raw: str) -> str:
    return authkit_query(raw, "continue")


def microsoft_step(email_visible: bool, pass_visible: bool, email_done: bool) -> str:
    if email_visible and not email_done:
        return "email"
    if pass_visible and (email_done or not email_visible):
        return "password"
    return "other"


def radar_send_url(raw: str) -> str:
    return raw.replace("radar-challenge/verify", "radar-challenge/send", 1)


def is_stripe(raw: str) -> bool:
    return "stripe.com" in raw or "checkout.stripe.com" in raw


def microsoft_ready_url(raw: str) -> bool:
    return on_microsoft_url(raw) or on_radar_url(raw) or on_cline(raw)


def google_ready_url(raw: str) -> bool:
    return "accounts.google.com" in (raw or "") or on_radar_url(raw) or on_cline(raw)
