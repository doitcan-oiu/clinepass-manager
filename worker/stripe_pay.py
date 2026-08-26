import json
import time
import urllib.error
import urllib.parse
import urllib.request

from pageutil import click_one_of, sleep_ms, type_field, visible
from payment import iframe_payment_src
from protocol import log
from urls import is_stripe

BILLING_NAME = "魏春苗"
BILLING_COUNTRY = "HK"
BILLING_ADDRESS = "香港荃湾区荃湾禾笛街799号"
SUCCESS_TEXT = ("支付成功", "付款成功", "已完成支付", "订阅成功", "Thank you", "Payment successful", "You're all set")
SUBMIT = [
    'button[data-testid="hosted-payment-submit-button"]',
    "button.SubmitButton",
    'form button[type="submit"]',
]
OTP = [
    'input[autocomplete="one-time-code"]',
    'input[name="code"]',
    'input[inputmode="numeric"][maxlength="6"]',
    "input#code",
]


def digits(s: str) -> str:
    return "".join(ch for ch in (s or "") if ch.isdigit())


def expiry_mm_yy(valid_date: str) -> str:
    s = (valid_date or "").strip().replace("/", "-")
    parts = [p for p in s.split("-") if p]
    if len(parts) >= 2:
        year = digits(parts[0])
        month = digits(parts[1])
        if len(year) == 4:
            year = year[2:]
        if len(month) == 1:
            month = "0" + month
        if len(year) >= 2 and len(month) >= 2:
            return f"{month} / {year[-2:]}"
    d = digits(s)
    if len(d) == 6:
        return f"{d[4:6]} / {d[2:4]}"
    if len(d) >= 4:
        return f"{d[:2]} / {d[2:4]}"
    return s


def last4(card_no: str) -> str:
    d = digits(card_no)
    return d[-4:] if len(d) >= 4 else d


def payment_succeeded(page) -> bool:
    url = ""
    try:
        url = page.url or ""
    except Exception:
        url = ""
    if "checkout.stripe.com" not in url and "js.stripe.com" not in url:
        if "app.cline.bot" in url or "cline.bot" in url:
            return True
    if "/success" in url or "payment_status=paid" in url:
        return True
    if visible(page, ".SubmitButton-CheckmarkIcon, .SubmitButton-Icon--success"):
        return True
    try:
        html = page.content() or ""
    except Exception:
        html = ""
    return any(n in html for n in SUCCESS_TEXT)


def checkout_error(page) -> str:
    sels = [
        ".FormFieldGroup-errorPresence",
        "[role='alert']",
        ".Error",
        ".CheckoutInput--invalid",
    ]
    for sel in sels:
        loc = page.locator(sel)
        try:
            n = loc.count()
        except Exception:
            continue
        for i in range(min(n, 6)):
            try:
                if not loc.nth(i).is_visible():
                    continue
                text = (loc.nth(i).inner_text() or "").strip()
            except Exception:
                continue
            if text:
                return text
    return ""


def open_checkout(page, payment_url: str):
    src = iframe_payment_src(page)
    url = src or (payment_url or "")
    if is_stripe(getattr(page, "url", "") or "") and visible(page, "#cardNumber"):
        return
    if url:
        log("打开 Stripe 支付页")
        page.goto(url, wait_until="domcontentloaded", timeout=60000)
        sleep_ms(800)
    deadline = time.time() + 30
    while time.time() < deadline:
        if visible(page, "#cardNumber"):
            return
        sleep_ms(400)
    raise RuntimeError("Stripe 支付页没有出现卡号框")


def fill_select(page, selector: str, value: str) -> None:
    loc = page.locator(selector).first
    loc.wait_for(state="visible", timeout=15000)
    loc.select_option(value=value)
    sleep_ms(400)


def fill_first_region(page) -> None:
    loc = page.locator("#billingAdministrativeArea").first
    try:
        loc.wait_for(state="visible", timeout=12000)
    except Exception:
        return
    try:
        options = loc.locator("option").all()
        for opt in options:
            val = (opt.get_attribute("value") or "").strip()
            if val:
                loc.select_option(value=val)
                log("账单地区已选第一个")
                sleep_ms(300)
                return
    except Exception as exc:
        log("选择账单地区失败: %s", exc)


def fill_if_present(page, selector: str, value: str) -> None:
    if not visible(page, selector):
        return
    type_field(page, selector, value)
    sleep_ms(200)


def click_terms(page) -> None:
    box = page.locator("#termsOfServiceConsentCheckbox").first
    try:
        box.wait_for(state="visible", timeout=8000)
        if not box.is_checked():
            box.check()
            log("已勾选服务条款")
        sleep_ms(200)
    except Exception as exc:
        log("勾选服务条款失败: %s", exc)


DECLINE_HINTS = (
    "declin",
    "拒",
    "insufficient",
    "do not honor",
    "not permitted",
    "stolen",
    "lost card",
    "pick up",
    "restricted",
    "your card",
    "余额",
    "无法处理",
    "交易失败",
    "expired card",
    "invalid account",
)
FORM_HINTS = ("required", "incomplete", "格式", "expiry date", "invalid expiry")


def is_card_rejected(err: str) -> bool:
    text = (err or "").strip().lower()
    if not text:
        return False
    if any(h in text for h in FORM_HINTS) and "declin" not in text and "拒" not in text:
        return False
    return any(h in text for h in DECLINE_HINTS)


def checkout_card(api_base: str, replace: bool = False) -> dict:
    data = api_json(api_base, "/api/amzkeys/cards", method="POST", body={"replace": replace})
    card_no = digits(data.get("card_no") or "")
    cvv = (data.get("cvv") or "").strip()
    if not card_no or not cvv:
        raise RuntimeError("AmzKeys 开卡没有返回卡号或 CVV")
    return {
        "card_no": card_no,
        "cvv": cvv,
        "valid_date": data.get("valid_date") or "",
        "expiry": data.get("expiry") or expiry_mm_yy(data.get("valid_date") or ""),
        "last4": data.get("last4") or last4(card_no),
        "reused": bool(data.get("reused")),
    }


def fetch_auth_code(api_base: str, card_last4: str) -> str:
    q = urllib.parse.urlencode({"last4": card_last4} if card_last4 else {})
    path = "/api/amzkeys/auth-codes"
    if q:
        path += "?" + q
    data = api_json(api_base, path)
    items = data.get("item") or []
    if not items:
        return ""
    return str(items[0].get("auth_code") or "").strip()


def api_json(api_base: str, path: str, method: str = "GET", body: dict | None = None) -> dict:
    base = (api_base or "").rstrip("/")
    if not base:
        raise RuntimeError("没有本机 API 地址，无法开卡")
    data = None
    if method in ("POST", "PUT", "PATCH"):
        data = json.dumps(body or {}).encode("utf-8")
    req = urllib.request.Request(
        base + path,
        method=method,
        headers={"Content-Type": "application/json", "Accept": "application/json"},
        data=data,
    )
    timeout = 200 if "cards" in path else 30
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read()
    except urllib.error.HTTPError as exc:
        body = exc.read() if exc.fp else b""
        text = body.decode("utf-8", "replace")
        try:
            msg = json.loads(text).get("error") or text
        except Exception:
            msg = text or exc.reason
        raise RuntimeError(str(msg).strip() or f"HTTP {exc.code}") from exc
    text = raw.decode("utf-8", "replace")
    if not text.strip():
        return {}
    return json.loads(text)


def fill_card(page, card: dict) -> None:
    type_field(page, "#cardNumber", card["card_no"])
    sleep_ms(250)
    type_field(page, "#cardExpiry", card["expiry"] or expiry_mm_yy(card.get("valid_date") or ""))
    sleep_ms(200)
    type_field(page, "#cardCvc", card["cvv"])
    sleep_ms(200)
    type_field(page, "#billingName", BILLING_NAME)
    sleep_ms(200)
    fill_select(page, "#billingCountry", BILLING_COUNTRY)
    fill_first_region(page)
    fill_if_present(page, "#billingLocality", BILLING_ADDRESS)
    fill_if_present(page, "#billingAddressLine1", BILLING_ADDRESS)
    fill_if_present(page, "#billingAddressLine2", BILLING_ADDRESS)
    click_terms(page)


def handle_3ds(page, api_base: str, card_last4: str) -> None:
    deadline = time.time() + 90
    last = ""
    while time.time() < deadline:
        if payment_succeeded(page):
            return
        otp = ""
        for sel in OTP:
            if visible(page, sel):
                otp = sel
                break
        if otp:
            code = fetch_auth_code(api_base, card_last4)
            if code and code != last:
                log("填写 3DS 验证码")
                type_field(page, otp, code)
                last = code
                try:
                    click_one_of(page, ['button[type="submit"]', "button:has-text('提交')", "button:has-text('Verify')"], 5000, "提交 3DS")
                except Exception:
                    pass
        sleep_ms(2000)


def wait_pay_result(page, api_base: str, card: dict) -> tuple[bool, str]:
    deadline = time.time() + 90
    while time.time() < deadline:
        if payment_succeeded(page):
            log("Stripe 支付成功")
            return True, ""
        err = checkout_error(page)
        if err:
            return False, "Stripe 拒付: " + err
        if visible(page, ",".join(OTP)) or "three-ds" in (page.url or "") or "3ds" in (page.url or "").lower():
            handle_3ds(page, api_base, card["last4"])
            if payment_succeeded(page):
                return True, ""
        sleep_ms(800)
    if payment_succeeded(page):
        return True, ""
    return False, "等待 Stripe 支付结果超时"


def pay_with_card(page, payment_url: str, card: dict, api_base: str) -> tuple[bool, str]:
    open_checkout(page, payment_url)
    fill_card(page, card)
    click_one_of(page, SUBMIT, 15000, "提交订购")
    return wait_pay_result(page, api_base, card)


def autopay(page, payment_url: str, settings: dict) -> tuple[bool, str]:
    api_base = (settings.get("manager_api") or "").strip()
    card = checkout_card(api_base, replace=False)
    if card.get("reused"):
        log("复用当前虚拟卡，后四位 %s", card["last4"])
    else:
        log("没有可用卡，新开一张，后四位 %s", card["last4"])
    paid, err = pay_with_card(page, payment_url, card, api_base)
    if paid:
        return True, ""
    if not is_card_rejected(err):
        return False, err
    log("当前卡被拒绝（%s），换一张再付", err)
    card = checkout_card(api_base, replace=True)
    log("已开新卡，后四位 %s", card["last4"])
    paid, err = pay_with_card(page, payment_url, card, api_base)
    if paid:
        return True, ""
    return False, err
