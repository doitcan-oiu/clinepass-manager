import time

from authkit import MICROSOFT_AUTH, handle_authkit_wait
from errors import LoggedIn
from pageutil import (
    click_one_of,
    field_ready,
    first_ready_field,
    input_value,
    logged_in,
    sleep_ms,
    type_field,
    visible,
    wait_any_url,
)
from protocol import log
from urls import AUTH_HOST, microsoft_step, on_cline, on_microsoft_url, url_host

EMAIL_SELS = ['input[name="loginfmt"]', "input#i0116"]
PASS_SELS = ["input#passwordEntry", 'input[name="passwd"]', "input#i0118"]
NEXT_SELS = [
    'button[data-testid="primaryButton"]',
    "input#idSIButton9",
    'input[type="submit"]#idSIButton9',
    'input[type="submit"]',
]
CARD_SETTLE = 1.0


def wait_card(page, selectors, timeout_ms: float, name: str) -> str:
    deadline = time.time() + timeout_ms / 1000.0
    hold = ""
    since = 0.0
    last = 0.0
    while time.time() < deadline:
        if logged_in(page):
            raise LoggedIn()
        sel = first_ready_field(page, selectors)
        if sel:
            if sel != hold:
                hold = sel
                since = time.time()
            elif time.time() - since >= CARD_SETTLE:
                return sel
        else:
            hold = ""
            since = 0.0
        if time.time() - last > 6:
            log("微软%s卡片尚未就绪，当前 URL=%s", name, page.url)
            last = time.time()
        sleep_ms(200)
    raise RuntimeError(f"等待微软{name}输入卡片超时，当前 URL={page.url}")


def fill_microsoft(page, selector: str, value: str, label: str) -> None:
    type_field(page, selector, value)
    got = input_value(page, selector).strip()
    if got != value.strip():
        log("%s未落到输入框，再按真人节奏填一次", label)
        type_field(page, selector, value)
    sleep_ms(80)


def wait_leave_field(page, selector: str, timeout_ms: float) -> None:
    deadline = time.time() + timeout_ms / 1000.0
    while time.time() < deadline:
        if logged_in(page) or not field_ready(page, selector):
            return
        sleep_ms(150)
    raise RuntimeError("仍停在微软输入页")


def wait_email_accepted(page, email_sel: str, timeout_ms: float) -> None:
    deadline = time.time() + timeout_ms / 1000.0
    while time.time() < deadline:
        if logged_in(page):
            raise LoggedIn()
        if not field_ready(page, email_sel):
            remain = max((deadline - time.time()) * 1000, 8000)
            wait_card(page, PASS_SELS, remain, "密码")
            return
        sleep_ms(200)
    raise RuntimeError("提交邮箱后仍停在邮箱页")


def microsoft_login(page, acc: dict) -> None:
    deadline = time.time() + 180
    email_done = False
    pass_done = False
    last_click = [0.0]
    last_unknown = 0.0
    log("等待微软登录卡片加载")
    try:
        wait_card(page, EMAIL_SELS, 30000, "账号")
    except LoggedIn:
        return
    except Exception as exc:
        log("%s", exc)

    while time.time() < deadline:
        raw = page.url
        if logged_in(page):
            log("已离开微软登录，当前 URL=%s", raw)
            return
        if url_host(raw) == AUTH_HOST:
            if handle_authkit_wait(page, last_click, "microsoft"):
                return
            continue
        email_sel = first_ready_field(page, EMAIL_SELS)
        pass_sel = first_ready_field(page, PASS_SELS)
        step = microsoft_step(bool(email_sel), bool(pass_sel), email_done)
        if step == "email":
            try:
                sel = wait_card(page, EMAIL_SELS, 20000, "账号")
            except LoggedIn:
                return
            if not sel:
                sleep_ms(150)
                continue
            log("填写 Microsoft 账号")
            fill_microsoft(page, sel, acc.get("email") or "", "账号")
            try:
                click_one_of(page, ["input#idSIButton9", 'input[type="submit"]'], 15000, "账号下一步")
            except Exception:
                if logged_in(page):
                    return
                raise
            try:
                wait_email_accepted(page, sel, 25000)
            except LoggedIn:
                return
            except Exception as exc:
                log("邮箱卡片还没切走，再等异步加载：%s", exc)
                email_done = False
                continue
            email_done = True
            continue
        if step == "password":
            try:
                sel = wait_card(page, PASS_SELS, 20000, "密码")
            except LoggedIn:
                return
            if not sel or (first_ready_field(page, EMAIL_SELS) and not email_done):
                sleep_ms(150)
                continue
            log("填写 Microsoft 密码")
            fill_microsoft(page, sel, acc.get("password") or "", "密码")
            try:
                click_one_of(page, NEXT_SELS, 15000, "密码下一步")
            except Exception:
                if logged_in(page):
                    return
                raise
            pass_done = True
            try:
                wait_leave_field(page, sel, 20000)
            except Exception:
                pass
            continue
        if (
            pass_done
            and not email_sel
            and not pass_sel
            and on_microsoft_url(raw)
            and visible(page, 'input#idSIButton9, button[data-testid="primaryButton"]')
        ):
            log("点击微软页面下一步")
            try:
                click_one_of(page, NEXT_SELS, 8000, "微软下一步")
            except Exception:
                pass
            sleep_ms(200)
            continue
        if on_microsoft_url(raw) and time.time() - last_unknown > 8:
            log("微软页面未识别，当前 URL=%s", raw)
            last_unknown = time.time()
        sleep_ms(200)
    if logged_in(page):
        return
    raise RuntimeError(f"Microsoft 登录未完成，当前 URL={page.url}")


def start_microsoft(page, acc: dict, context=None):
    from pageutil import page_url
    from urls import microsoft_ready_url

    if not on_microsoft_url(page_url(page)):
        try:
            click_one_of(page, MICROSOFT_AUTH, 20000, "选择 Microsoft 登录")
        except Exception:
            if not on_microsoft_url(page_url(page)) and not on_cline(page_url(page)):
                raise
            log("已在授权相关页面，继续")
        log("微软点击后 URL=%s", page_url(page))
    page = _wait_microsoft_page(page, context, 10000)
    if not microsoft_ready_url(page_url(page)) and url_host(page_url(page)) == AUTH_HOST:
        log("仍停在 AuthKit，再点一次微软登录")
        try:
            click_one_of(page, MICROSOFT_AUTH, 12000, "再次选择 Microsoft 登录")
        except Exception as exc:
            log("再次点击失败: %s", exc)
    page = _wait_microsoft_page(page, context, 45000)
    u = page_url(page)
    if on_microsoft_url(u) or visible(page, 'input[name="loginfmt"], input#i0116, input[name="passwd"], input#passwordEntry'):
        microsoft_login(page, acc)
        return page
    if microsoft_ready_url(u):
        return page
    raise RuntimeError(f"未进入微软登录页，当前 URL={u}")


def _wait_microsoft_page(page, context, timeout_ms: float):
    from pageutil import follow_identity_page, page_url
    from urls import microsoft_ready_url

    try:
        return wait_any_url(page, [], timeout_ms, context=context, ready=microsoft_ready_url)
    except Exception:
        page = follow_identity_page(context, page, microsoft_ready_url)
        log("等待微软授权页超时，当前 URL=%s", page_url(page))
        return page
