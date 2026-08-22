import time

from errors import WorkerError
from pageutil import click_one_of, logged_in, on_radar_flow, page_title, sleep_ms, visible
from protocol import log
from urls import (
    AUTH_HOST,
    authkit_banned_after_wait,
    authkit_callback_error,
    authkit_session_id,
    on_cline,
    on_authkit_login,
    url_host,
)

GOOGLE_AUTH = [
    'a[data-method="google"]',
    'a[href*="provider=GoogleOAuth"]',
    'a[href*="GoogleOAuth"]',
]
MICROSOFT_AUTH = [
    'a[data-method="microsoft"]',
    'a[href*="provider=MicrosoftOAuth"]',
    'a[href*="MicrosoftOAuth"]',
]


def auth_selectors(provider: str) -> list[str]:
    return MICROSOFT_AUTH if provider == "microsoft" else GOOGLE_AUTH


def visible_auth_button(page, provider: str) -> bool:
    return any(visible(page, sel) for sel in auth_selectors(provider))


def raise_callback(code: str, raw: str) -> None:
    log("AuthKit 回调错误 error=%s，当前 URL=%s", code, raw)
    if code.lower() == "policy_denied":
        raise WorkerError("AuthKit Radar 拦截（policy_denied），已跳过", "radar_denied")
    raise WorkerError(f"AuthKit 回调失败：{code}", "authkit_stuck")


def wait_authkit_advance(page, timeout_ms: float) -> bool:
    deadline = time.time() + timeout_ms / 1000.0
    while time.time() < deadline:
        if on_cline(page.url) or on_radar_flow(page):
            return True
        if url_host(page.url) != AUTH_HOST:
            return True
        if authkit_callback_error(page.url):
            return False
        sleep_ms(400)
    return on_cline(page.url) or on_radar_flow(page) or url_host(page.url) != AUTH_HOST


def handle_authkit_wait(page, last_click: list, provider: str) -> bool:
    raw = page.url
    if on_radar_flow(page) or on_cline(raw):
        log("已进入手机验证或 Cline，当前 URL=%s", raw)
        return True
    code = authkit_callback_error(raw)
    if code:
        raise_callback(code, raw)
    sid = authkit_session_id(raw)
    auth_visible = visible_auth_button(page, provider)
    log(
        "到达 AuthKit，当前 URL=%s title=%s session=%s 登录按钮=%s 方式=%s",
        raw,
        page_title(page),
        sid,
        auth_visible,
        provider,
    )
    wait_ms = 10000 if sid else 8000
    if sid:
        log("OAuth 已回到 AuthKit（有 authorization_session_id），先确认是否跳到接码页")
    if wait_authkit_advance(page, wait_ms):
        return on_radar_flow(page) or on_cline(page.url)
    after = page.url
    code = authkit_callback_error(after)
    if code:
        raise_callback(code, after)
    log(
        "AuthKit 等待后仍未进入接码，当前 URL=%s title=%s 登录按钮=%s",
        after,
        page_title(page),
        visible_auth_button(page, provider),
    )
    if authkit_banned_after_wait(after):
        log("仍停在 AuthKit，账号已被封禁，跳过")
        raise WorkerError("账号已被封禁，已跳过", "banned")
    if on_authkit_login(after) and visible_auth_button(page, provider) and time.time() - last_click[0] > 12:
        label = "再次选择 Microsoft 登录" if provider == "microsoft" else "再次选择 Google 登录"
        log("AuthKit 仍是登录页，%s", label)
        try:
            click_one_of(page, auth_selectors(provider), 8000, label)
        except Exception:
            pass
        last_click[0] = time.time()
    sleep_ms(500)
    return False
