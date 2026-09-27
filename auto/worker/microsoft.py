import time
from urllib.parse import urlsplit

from authkit import MICROSOFT_AUTH, raise_callback
from errors import WorkerError
from pageutil import (
    click_one_of,
    first_ready_field,
    input_value,
    logged_in,
    sleep_ms,
    type_field,
    visible,
    wait_any_url,
)
from protocol import log
from urls import AUTH_HOST, authkit_callback_error, microsoft_step, on_cline, on_microsoft_url, url_host

EMAIL_SELS = ['input[name="loginfmt"]', "input#i0116"]
PASS_SELS = ["input#passwordEntry", 'input[name="passwd"]', "input#i0118"]
NEXT_SELS = ['button[data-testid="primaryButton"]', "input#idSIButton9", 'input[type="submit"]']
CARD_SETTLE = 1.0
NO_PROGRESS_TIMEOUT = 35.0
LOGIN_TIMEOUT = 120.0
CHALLENGE_SELS = (
    'input[name="otc"]', 'input[name="otcInput"]', "input#iOttText",
    'input[name="ProofConfirmation"]', "#idDiv_SAOTCC_Description",
    "#idRichContext_DisplaySign", 'iframe[src*="captcha"]', "#wlspispHIPSolutionContainer",
)


def safe_page_location(raw: str) -> str:
    """Keep OAuth state, authorization codes and login hints out of messages."""
    try:
        parsed = urlsplit(raw)
        return (parsed.hostname or "") + (parsed.path or "/")
    except ValueError:
        return "未知页面"


def microsoft_problem(text: str, challenge_visible: bool = False) -> tuple[str, str]:
    text = " ".join(text.lower().replace("’", "'").split())
    groups = (
        (("account has been locked", "account is locked", "account has been suspended", "too many times", "账号已锁定", "帐户已锁定", "账户已锁定", "尝试次数过多", "尝试登录次数过多", "暂时阻止"),
         "微软账号已锁定或尝试过于频繁，请先人工检查账号", "microsoft_locked"),
        (("account doesn't exist", "account does not exist", "couldn't find an account", "could not find an account", "该 microsoft 帐户不存在", "该 microsoft 账户不存在", "找不到该帐户", "账号不存在", "帐户不存在", "账户不存在"),
         "微软账号不存在，请检查邮箱地址", "microsoft_account_missing"),
        (("account or password is incorrect", "password is incorrect", "password isn't correct", "incorrect password", "密码不正确", "密码错误", "密码有误"),
         "微软账号或密码错误，请检查凭据后重试", "microsoft_credentials"),
        (("verify your identity", "help us protect your account", "approve sign in request", "approve the sign in", "enter the code we sent", "enter your security code", "help us beat the robots", "prove you're not a robot", "update your password", "change your password", "more information required", "验证你的身份", "验证您的身份", "验证你的标识", "帮助我们保护", "批准登录请求", "输入我们发送", "输入安全代码", "添加安全信息", "提供更多信息", "证明你不是机器人", "证明您不是机器人", "更新你的密码", "更改密码"),
         "微软要求验证码、身份验证或安全信息，请先人工完成验证后重试", "microsoft_manual_required"),
    )
    for phrases, message, code in groups:
        if any(phrase in text for phrase in phrases):
            return message, code
    if challenge_visible:
        return "微软要求验证码或身份验证，请先人工完成验证后重试", "microsoft_manual_required"
    return "", ""


def microsoft_confirmation(text: str) -> str:
    text = " ".join(text.lower().split())
    if any(s in text for s in ("stay signed in?", "保持登录状态", "是否保持登录")):
        return "stay_signed_in"
    if any(s in text for s in ("let this app access your info", "permissions requested", "允许此应用访问", "让此应用访问", "请求的权限")):
        return "consent"
    return ""


def microsoft_page_text(page) -> str:
    try:
        # inner_text reads rendered text, not hidden error templates or script data.
        return page.locator("body").inner_text(timeout=1000)[:12000]
    except Exception:
        return ""


def fill_microsoft(page, selector: str, value: str, label: str) -> None:
    type_field(page, selector, value)
    if input_value(page, selector).strip() != value.strip():
        type_field(page, selector, value)
    if input_value(page, selector).strip() != value.strip():
        raise WorkerError(f"微软{label}未能填入输入框，请检查页面后重试", "microsoft_input")


def click_microsoft_once(page, selectors, label: str) -> None:
    """Wait for a button, but never resubmit after an ambiguous click timeout."""
    deadline = time.monotonic() + 8
    while time.monotonic() < deadline:
        if logged_in(page):
            return
        for selector in selectors:
            loc = page.locator(selector).first
            try:
                ready = loc.is_visible() and loc.is_enabled()
            except Exception:
                continue
            if not ready:
                continue
            try:
                loc.click(timeout=8000, no_wait_after=True)
                return
            except Exception:
                if logged_in(page):
                    return
                raise WorkerError(f"微软{label}提交未完成，请检查页面后重试", "microsoft_submit") from None
        sleep_ms(200)
    raise WorkerError(f"微软{label}按钮未就绪，请检查页面后重试", "microsoft_submit")


def microsoft_login(page, acc: dict) -> None:
    started = last_progress = time.monotonic()
    email_done = pass_done = False
    email_accepted = False
    confirmations = set()
    seen_states = set()
    held_state = ""
    held_since = started
    waiting = "登录页面加载"
    log("等待微软登录页面")
    while True:
        if logged_in(page):
            log("微软登录完成")
            return
        now = time.monotonic()
        if now - started >= LOGIN_TIMEOUT:
            raise WorkerError("微软登录总耗时超过 120 秒，已停止本次登录，请检查页面后重试", "microsoft_stalled")
        if now - last_progress >= NO_PROGRESS_TIMEOUT:
            raise WorkerError(f"微软{waiting}超过 35 秒无进展，已停止本次登录，请检查账号或网络后重试", "microsoft_stalled")
        raw = page.url
        if url_host(raw) == AUTH_HOST:
            code = authkit_callback_error(raw)
            if code:
                raise_callback(code, safe_page_location(raw))
            state = "callback"
            if state not in seen_states:
                log("微软授权已返回，等待登录完成")
                waiting = "授权回调"
        else:
            text = microsoft_page_text(page)
            message, code = microsoft_problem(text, any(visible(page, sel) for sel in CHALLENGE_SELS))
            if message:
                raise WorkerError(message, code)
            email_sel = first_ready_field(page, EMAIL_SELS)
            pass_sel = first_ready_field(page, PASS_SELS)
            if email_done and not email_sel:
                email_accepted = True
            state = microsoft_step(bool(email_sel), bool(pass_sel), email_accepted)
            if state == "other" and on_microsoft_url(raw):
                state = microsoft_confirmation(text) or "other"
            if state != held_state:
                held_state, held_since = state, now
            if state in ("email", "password") and now - held_since >= CARD_SETTLE:
                if state == "email" and not email_done:
                    fill_microsoft(page, email_sel, acc.get("email") or "", "账号")
                    email_done = True
                    click_microsoft_once(page, NEXT_SELS, "账号")
                    log("已提交微软账号，等待密码页面")
                    waiting = "账号提交后等待"
                    last_progress = time.monotonic()
                elif state == "password" and not pass_done:
                    fill_microsoft(page, pass_sel, acc.get("password") or "", "密码")
                    pass_done = True
                    click_microsoft_once(page, NEXT_SELS, "密码")
                    log("已提交微软密码，等待验证结果")
                    waiting = "密码验证"
                    last_progress = time.monotonic()
            elif state in ("stay_signed_in", "consent") and state not in confirmations:
                confirmations.add(state)
                label = "保持登录" if state == "stay_signed_in" else "授权确认"
                click_microsoft_once(page, NEXT_SELS, label)
                log("已确认微软%s，等待跳转", label)
                waiting = label
                last_progress = time.monotonic()
        # Only a new recognized phase counts as progress. DOM flicker, URL tokens,
        # and a credential field reappearing must not extend the stall deadline.
        if state != "other" and state not in seen_states:
            seen_states.add(state)
            last_progress = time.monotonic()
        sleep_ms(200)


def start_microsoft(page, acc: dict, context=None):
    from pageutil import page_url
    from urls import microsoft_ready_url

    if logged_in(page):
        return page
    if not on_microsoft_url(page_url(page)):
        try:
            click_one_of(page, MICROSOFT_AUTH, 20000, "选择 Microsoft 登录")
        except Exception:
            if not on_microsoft_url(page_url(page)) and not on_cline(page_url(page)):
                raise
            log("已进入微软授权流程")
    page = _wait_microsoft_page(page, context, 10000)
    if not microsoft_ready_url(page_url(page)) and url_host(page_url(page)) == AUTH_HOST:
        log("等待微软授权页，重试一次登录入口")
        click_microsoft_once(page, MICROSOFT_AUTH, "登录入口")
    page = _wait_microsoft_page(page, context, 35000)
    u = page_url(page)
    if on_microsoft_url(u) or visible(page, 'input[name="loginfmt"], input#i0116, input[name="passwd"], input#passwordEntry'):
        microsoft_login(page, acc)
        return page
    if microsoft_ready_url(u):
        return page
    raise WorkerError("未进入微软登录页，请检查网络或登录入口后重试", "microsoft_stalled")


def _wait_microsoft_page(page, context, timeout_ms: float):
    from pageutil import follow_identity_page
    from urls import microsoft_ready_url

    try:
        return wait_any_url(page, [], timeout_ms, context=context, ready=microsoft_ready_url)
    except Exception:
        return follow_identity_page(context, page, microsoft_ready_url)
