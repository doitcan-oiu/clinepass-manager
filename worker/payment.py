import time

from pageutil import click_one_of, sleep_ms
from protocol import log
from urls import cookie_expired, is_stripe

APP_BASE = "https://app.cline.bot"


def iframe_payment_src(page) -> str:
    loc = page.locator('iframe[src*="stripe"], iframe[src*="checkout"], iframe').first
    try:
        if not loc.is_visible():
            return ""
        src = (loc.get_attribute("src") or "").strip()
    except Exception:
        return ""
    if not src:
        return ""
    if "stripe" in src or "checkout" in src or src.startswith("https://"):
        return src
    return ""


def capture_payment(page) -> str:
    u = page.url
    if "/onboarding/" not in u and "/checkout" not in u:
        page.goto(APP_BASE + "/onboarding/individual-plan", wait_until="domcontentloaded", timeout=60000)
        sleep_ms(1000)
    if cookie_expired(page.url):
        raise RuntimeError("Cookie 已失效，需要重新登录")
    if "/checkout" not in page.url:
        try:
            click_one_of(
                page,
                ["button:has(svg.lucide-chevron-right)", "button.h-12", 'button[type="button"]:has(svg)'],
                20000,
                "进入结账",
            )
        except Exception:
            pass
    deadline = time.time() + 60
    while time.time() < deadline:
        src = iframe_payment_src(page)
        if src:
            return src
        if is_stripe(page.url):
            return page.url
        sleep_ms(500)
    raise RuntimeError("等待支付 iframe 超时")
