import random
import time

from authkit import handle_authkit_wait
from errors import WorkerError
from google import CONSENT, accept_workspace_tos, recover_unknown_error, start_google
from microsoft import start_microsoft
from pageutil import click_one_of, human_idle_authkit, on_radar_flow, serialize_cookies, sleep_ms, visible
from payment import capture_payment
from protocol import log
from radar import handle_radar, handle_terms
from urls import (
    AUTH_HOST,
    authkit_callback_error,
    classify_google,
    cookie_expired,
    on_cline,
    on_cline_app,
    on_radar_url,
    url_host,
)

AUTH_URL = "https://authkit.cline.bot"
APP_DASHBOARD = "https://app.cline.bot/dashboard"


def wait_cline(page, timeout_ms: float, provider: str) -> None:
    deadline = time.time() + timeout_ms / 1000.0
    last = 0.0
    last_click = [0.0]
    while time.time() < deadline:
        if on_cline(page.url) or on_radar_flow(page):
            return
        if classify_google(page.url) == "tos" and visible(page, "#gaplustosNext button"):
            accept_workspace_tos(page)
        if classify_google(page.url) == "unknownerror":
            recover_unknown_error(page)
        if classify_google(page.url) == "consent" and (
            visible(page, 'div[jsname="uRHG6"] button') or visible(page, "#submit_approve_access")
        ):
            try:
                click_one_of(page, CONSENT, 8000, "同意授权")
            except Exception:
                pass
        if url_host(page.url) == AUTH_HOST and not on_radar_flow(page):
            if handle_authkit_wait(page, last_click, provider):
                return
            continue
        if time.time() - last > 8:
            log("仍在等待进入 Cline，当前 URL=%s", page.url)
            last = time.time()
        sleep_ms(500)
    wrap_authkit(RuntimeError(f"等待进入 Cline 超时，当前 URL={page.url}"), page.url)


def wrap_authkit(err: Exception, page_url: str) -> None:
    if isinstance(err, WorkerError):
        raise err
    if authkit_callback_error(page_url).lower() == "policy_denied":
        raise WorkerError("AuthKit Radar 拦截（policy_denied），已跳过", "radar_denied")
    host = url_host(page_url)
    if page_url.startswith("chrome-error") or page_url.startswith("chrome://") or (
        host == AUTH_HOST and not on_radar_url(page_url)
    ):
        raise WorkerError(str(err), "authkit_stuck") from err
    raise err


def run_login(page, context, acc: dict, settings: dict) -> dict:
    provider = (acc.get("login_provider") or "google").strip().lower()
    if provider in ("outlook", "hotmail", "live", "ms", "microsoft"):
        provider = "microsoft"
    else:
        provider = "google"
    invite = (settings.get("invite_url") or "").strip() or AUTH_URL
    log("无头模式=%s", bool(settings.get("headless")))
    if settings.get("proxy"):
        log("使用全局代理")
    log("登录方式=%s", provider)
    log("打开邀请链接 %s", invite)
    page.goto(invite, wait_until="domcontentloaded", timeout=60000)
    sleep_ms(1400 + random.randint(0, 900))
    if url_host(page.url) == AUTH_HOST and not on_radar_flow(page):
        human_idle_authkit(page)
    if on_cline_app(page.url) and "radar-challenge" not in page.url:
        log("当前已在 Cline，跳过身份登录")
    else:
        try:
            if provider == "microsoft":
                page = start_microsoft(page, acc, context) or page
            else:
                page = start_google(page, acc, context) or page
        except WorkerError:
            raise
        except Exception as exc:
            wrap_authkit(exc, page.url)
        log("等待进入 Cline")
        wait_cline(page, 180000, provider)
        handle_radar(page, settings)
        handle_terms(page)
        wait_cline(page, 60000, provider)
    try:
        page.goto(APP_DASHBOARD, wait_until="domcontentloaded", timeout=60000)
    except Exception as exc:
        log("打开 dashboard 失败: %s", exc)
    sleep_ms(800)
    if cookie_expired(page.url):
        wrap_authkit(RuntimeError(f"登录后没有进入 Cline，当前 URL={page.url}"), page.url)
    cookies_json, cookie_header = serialize_cookies(context.cookies())
    log("已保存 %d 条 Cookie", cookies_json.count('"name"'))
    payment = ""
    try:
        payment = capture_payment(page)
        log("支付链接: %s", payment)
    except Exception as exc:
        log("获取支付链接失败: %s", exc)
    return {"cookies_json": cookies_json, "cookie_header": cookie_header, "payment_url": payment}


def run_refresh(page, context, acc: dict) -> dict:
    from pageutil import cookies_for_context

    raw = (acc.get("cookies_json") or "").strip()
    if not raw and not (acc.get("cookie_header") or "").strip():
        raise RuntimeError("没有可用 Cookie，需要先完整登录")
    cookies = cookies_for_context(raw)
    context.add_cookies(cookies)
    log("已注入 %d 条 Cookie，跳过身份登录", len(cookies))
    payment = capture_payment(page)
    log("支付链接: %s", payment)
    fresh = context.cookies()
    if fresh:
        cookies_json, cookie_header = serialize_cookies(fresh)
        log("已更新 %d 条 Cookie", len(fresh))
    else:
        cookies_json = acc.get("cookies_json") or ""
        cookie_header = acc.get("cookie_header") or ""
    return {"cookies_json": cookies_json, "cookie_header": cookie_header, "payment_url": payment}
