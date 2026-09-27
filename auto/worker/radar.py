import time

from errors import WorkerError
from herosms import Client
from pageutil import click_one_of, field_ready, input_value, sleep_ms, visible, wait_any_url
from protocol import log
from urls import on_radar_url, radar_send_url


def digits_only(s: str) -> str:
    return "".join(ch for ch in (s or "") if ch.isdigit())


def authkit_input_match(page, selector: str, want: str) -> bool:
    return digits_only(input_value(page, selector)) == digits_only(want)


def type_authkit_field(page, selector: str, value: str) -> None:
    loc = page.locator(selector).first
    try:
        loc.fill(value, timeout=12000)
        if authkit_input_match(page, selector, value):
            return
    except Exception:
        pass
    loc.click(timeout=8000)
    page.keyboard.press("Control+a")
    page.keyboard.press("Backspace")
    loc.type(value, delay=18, timeout=12000)


def fill_authkit_field(page, selector: str, value: str) -> None:
    from pageutil import wait_visible

    wait_visible(page, selector, 20000, stop_if_done=False)
    type_authkit_field(page, selector, value)
    if authkit_input_match(page, selector, value):
        return
    typed = value
    current = input_value(page, selector)
    if current.strip().startswith("+") and value.startswith("+"):
        typed = value[1:]
    type_authkit_field(page, selector, typed)
    if not authkit_input_match(page, selector, value):
        raise RuntimeError(f'页面是 "{input_value(page, selector)}"，期望 "{value}"')


def radar_form_ready(page) -> bool:
    return field_ready(page, 'input[name="country_code"]') and field_ready(page, 'input[name="local_number"]')


def wait_radar_form(page, timeout_ms: float = 15000) -> None:
    deadline = time.time() + timeout_ms / 1000.0
    while time.time() < deadline:
        if radar_form_ready(page):
            return
        sleep_ms(200)
    raise RuntimeError(f"接码页没有可填的手机号输入框，当前 URL={page.url}")


def fill_otp(page, code: str) -> None:
    code = digits_only(code)
    if len(code) < 4:
        raise RuntimeError("验证码无效")
    code = code[:6]
    log("填写验证码")
    first = 'input[data-test="otp-input"], input[data-index], .ak-Otp input, input[name="code"], input[autocomplete="one-time-code"]'
    page.click(first, timeout=12000)
    page.keyboard.press("Control+a")
    page.keyboard.press("Backspace")
    page.keyboard.type(code, delay=18)
    sleep_ms(120)
    try:
        click_one_of(
            page,
            ['button[data-hak-cta][type="submit"]', 'button.ak-PrimaryButton[type="submit"]', 'button[type="submit"]'],
            4000,
            "提交验证码",
        )
    except Exception:
        pass


def on_radar_send(u: str) -> bool:
    return "radar-challenge/send" in u


def goto_radar_send(page, send_url: str) -> None:
    if on_radar_send(page.url) and visible(page, 'input[name="local_number"]'):
        return
    target = (send_url or "").strip()
    if not target or "radar-challenge/verify" in target:
        target = radar_send_url(page.url)
    if target:
        log("打开手机号输入页")
        try:
            page.goto(target, wait_until="domcontentloaded", timeout=30000)
        except Exception as exc:
            log("打开手机号页失败: %s，尝试后退", exc)
            try:
                page.go_back()
            except Exception:
                pass
    else:
        try:
            page.go_back()
        except Exception:
            pass
    sleep_ms(300)
    if on_radar_send(page.url) or visible(page, 'input[name="local_number"]'):
        return
    raise RuntimeError(f"无法回到手机号输入页，当前 URL={page.url}")


def request_radar_code(page, settings: dict, sms: Client, attempt: int) -> None:
    wait_radar_form(page)
    country = int(settings.get("hero_sms_country") or 0)
    price = float(settings.get("hero_sms_max_price") or 0)
    log("Hero SMS 取号 country=%d price=%s（第 %d 次）", country, ("%g" % price) if price > 0 else "auto", attempt)
    num = sms.get_number(country, price)
    finished = False
    try:
        log("已取号 +%s %s id=%s", num["phone_code"], num["local_number"], num["id"])
        sleep_ms(200)
        if int(num.get("phone_code") or 0) <= 0 or not num.get("local_number"):
            raise RuntimeError(f'取到的号码无法拆分区号: {num.get("phone")}')
        cc = f'+{num["phone_code"]}'
        fill_authkit_field(page, 'input[name="country_code"]', cc)
        sleep_ms(150)
        fill_authkit_field(page, 'input[name="local_number"]', num["local_number"])
        sleep_ms(200)
        click_one_of(
            page,
            ['button[data-hak-cta][type="submit"]', 'button.ak-PrimaryButton[type="submit"]', 'button[type="submit"]'],
            15000,
            "发送验证码",
        )
        try:
            wait_any_url(page, ["radar-challenge/verify"], 30000)
        except Exception as exc:
            if on_radar_send(page.url) or visible(page, 'input[name="local_number"]'):
                raise RuntimeError("发送验证码失败，号码可能已被使用") from exc
            raise
        sleep_ms(200)
        log("等待短信验证码")
        code = sms.wait_code(num["id"], 120)
        log("收到验证码")
        sleep_ms(120)
        fill_otp(page, code)
        log("验证码已填，等待离开验证页")
        deadline = time.time() + 45
        last = 0.0
        while time.time() < deadline:
            u = page.url
            if "radar-challenge" not in u:
                sms.finish(num["id"])
                finished = True
                log("已离开接码页，当前 URL=%s", u)
                return
            if time.time() - last > 6:
                log("仍在验证页，当前 URL=%s", u)
                last = time.time()
            sleep_ms(200)
        raise RuntimeError(f"提交验证码后仍停在验证页，当前 URL={page.url}")
    finally:
        if not finished:
            sms.cancel(num["id"])


def is_no_sms(exc: Exception) -> bool:
    msg = str(exc)
    return isinstance(exc, TimeoutError) or "等待验证码超时" in msg or "接码已取消" in msg or "Hero SMS 暂时不可用" in msg


def should_retry_phone(exc: Exception) -> bool:
    if is_no_sms(exc):
        return True
    msg = str(exc)
    if "号码可能已被使用" in msg:
        return True
    return "等待 URL 超时" in msg and "radar-challenge/send" in msg


def handle_radar(page, settings: dict) -> None:
    if not on_radar_url(page.url):
        return
    if not (settings.get("hero_sms_api_key") or "").strip():
        raise RuntimeError("遇到手机验证，请先在设置里填写 Hero SMS API Key")
    if int(settings.get("hero_sms_country") or 0) <= 0:
        raise RuntimeError("遇到手机验证，请先在设置里选择接码区域和报价")
    wait_radar_form(page)
    send_url = page.url
    if "radar-challenge/verify" in send_url:
        send_url = radar_send_url(send_url)
    goto_radar_send(page, send_url)
    send_url = page.url
    sms = Client(settings.get("hero_sms_api_key") or "", settings.get("hero_sms_service") or "")
    last = None
    attempts = 3
    for i in range(1, attempts + 1):
        try:
            request_radar_code(page, settings, sms, i)
            return
        except Exception as exc:
            last = exc
            if not should_retry_phone(exc):
                raise
            log("第 %d/%d 次接码失败: %s", i, attempts, exc)
            if i < attempts:
                log("返回手机号输入页，换一个 Hero SMS 号码重试")
                sleep_ms(300)
                goto_radar_send(page, send_url)
    raise WorkerError("多次接码失败，需要重新登录", "sms_relogin") from last


def handle_terms(page) -> None:
    if not visible(page, "label#terms, #terms"):
        return
    log("勾选条款")
    click_one_of(page, ["label#terms", "#terms"], 8000, "勾选条款")
    sleep_ms(400)
    click_one_of(
        page,
        ['button[type="button"].w-full', "button.bg-black[type=button]", 'button[type="button"]'],
        15000,
        "确认注册",
    )
