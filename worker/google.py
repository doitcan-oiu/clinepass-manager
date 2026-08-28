import time

from authkit import GOOGLE_AUTH, handle_authkit_wait
from errors import LoggedIn
from pageutil import (
    click_one_of,
    human_scroll,
    logged_in,
    sleep_ms,
    type_field,
    visible,
    wait_any_url,
    wait_leave,
    wait_overlay_gone,
    wait_visible,
)
from protocol import log
from urls import AUTH_HOST, classify_google, google_continue_url, on_cline, url_host

CONSENT = [
    'div[jsname="uRHG6"] button',
    "#submit_approve_access",
    'button[data-idom-class*="P62QJc"]',
]
EMAIL_SEL = 'input#identifierId:not([aria-hidden="true"])'
PASS_SEL = 'input[name="Passwd"], #password input[type="password"]'
RECOVERY_SEL = 'input[name="knowledgePreregisteredEmailResponse"], input#knowledge-preregistered-email-response'


def check_tos_boxes(page) -> None:
    loc = page.locator('#gaplustos input[type="checkbox"], form input[type="checkbox"]')
    try:
        n = min(loc.count(), 4)
    except Exception:
        return
    for i in range(n):
        box = loc.nth(i)
        try:
            if box.is_visible() and not box.is_checked():
                box.check(timeout=2000)
        except Exception:
            pass


def click_tos_button(page) -> None:
    wait_overlay_gone(page)
    loc = page.locator("#gaplustosNext button").first
    if not loc.is_visible():
        raise RuntimeError("未找到同意按钮")
    try:
        loc.scroll_into_view_if_needed()
    except Exception:
        pass
    page.click("#gaplustosNext button", timeout=8000)


def accept_workspace_tos(page) -> None:
    try:
        wait_visible(page, "#gaplustosNext button", 15000)
    except LoggedIn:
        return
    log("检测到服务条款页，准备同意")
    for i in range(1, 4):
        if classify_google(page.url) != "tos" or logged_in(page):
            return
        human_scroll(page)
        check_tos_boxes(page)
        log("点击同意服务条款（第 %d 次）", i)
        try:
            click_tos_button(page)
            log("同意服务条款 成功")
        except Exception as exc:
            if classify_google(page.url) != "tos" or logged_in(page):
                return
            log("第 %d 次点击未生效: %s", i, exc)
        try:
            wait_leave(page, "workspacetermsofservice", 12000, "服务条款页")
            return
        except Exception:
            pass
    log("服务条款仍在，当前 URL=%s", page.url)


def recover_unknown_error(page) -> None:
    log("谷歌返回 unknownerror，尝试恢复")
    try:
        click_one_of(page, ["#next button", 'div[id$="Next"] button', 'button[type="submit"]'], 2500, "错误页下一步")
    except Exception:
        pass
    sleep_ms(200)
    if classify_google(page.url) != "unknownerror":
        return
    nxt = google_continue_url(page.url)
    if not nxt:
        raise RuntimeError("谷歌 unknownerror，没有 continue 可跳转")
    log("打开 continue 继续授权")
    page.goto(nxt, wait_until="domcontentloaded", timeout=30000)
    sleep_ms(200)
    if classify_google(page.url) == "unknownerror":
        raise RuntimeError(f"谷歌 unknownerror 恢复失败，当前 URL={page.url}")


def google_login(page, acc: dict) -> None:
    deadline = time.time() + 180
    chooser = f'div[data-identifier="{acc.get("email") or ""}"]'
    email_done = False
    pass_done = False
    last_unknown = 0.0
    last_click = [0.0]
    last_consent = 0.0
    email_at = 0.0
    pass_at = 0.0
    while time.time() < deadline:
        raw = page.url
        if logged_in(page):
            log("已离开谷歌登录，当前 URL=%s", raw)
            return
        step = classify_google(raw)
        if step == "password":
            email_done = True
        if url_host(raw) == AUTH_HOST:
            if handle_authkit_wait(page, last_click, "google"):
                return
            continue
        if step == "tos":
            accept_workspace_tos(page)
            continue
        if step == "unknownerror":
            recover_unknown_error(page)
            continue
        if step == "consent":
            if time.time() - last_consent > 6:
                log("检测到授权页，点击同意授权")
                click_one_of(page, CONSENT, 15000, "同意授权")
                last_consent = time.time()
            sleep_ms(200)
            continue
        if visible(page, RECOVERY_SEL) and (acc.get("recovery_email") or "").strip():
            log("填写辅助邮箱")
            type_field(page, RECOVERY_SEL, acc["recovery_email"])
            click_one_of(page, ['#idvPreregisteredEmailNext button', 'div[id$="Next"] button'], 15000, "辅助邮箱下一步")
            sleep_ms(200)
            continue
        if step == "chooser" or visible(page, chooser):
            if visible(page, chooser):
                log("选择已列出的账号")
                try:
                    click_one_of(page, [chooser], 8000, "点击账号卡片")
                except Exception:
                    pass
            try:
                wait_leave(page, "accountchooser", 15000, "账号选择页")
            except Exception:
                pass
            continue
        if step == "password" or (email_done and visible(page, PASS_SEL)):
            if pass_done and time.time() - pass_at < 8:
                sleep_ms(200)
                continue
            if not visible(page, PASS_SEL):
                log("已到密码页，等待密码框出现")
                try:
                    wait_visible(page, PASS_SEL, 12000)
                except LoggedIn:
                    return
                except Exception:
                    sleep_ms(200)
                    continue
            log("填写 Google 密码")
            wait_overlay_gone(page)
            type_field(page, PASS_SEL, acc.get("password") or "")
            click_one_of(page, ["#passwordNext button"], 15000, "密码下一步")
            pass_done = True
            pass_at = time.time()
            if logged_in(page):
                return
            continue
        if email_done and step == "email" and not visible(page, PASS_SEL):
            if time.time() - email_at > 8:
                email_done = False
            else:
                sleep_ms(200)
            continue
        if not email_done and (step == "email" or (not step and visible(page, EMAIL_SEL) and not visible(page, PASS_SEL))):
            log("填写 Google 账号")
            wait_overlay_gone(page)
            type_field(page, EMAIL_SEL, acc.get("email") or "")
            click_one_of(page, ["#identifierNext button"], 15000, "账号下一步")
            email_done = True
            email_at = time.time()
            try:
                wait_visible(page, PASS_SEL, 12000)
            except LoggedIn:
                return
            except Exception:
                pass
            continue
        if visible(page, 'iframe[src*="recaptcha"], iframe[src*="challenge"], #captcha, div[id*="captcha"]'):
            log("检测到验证码/安全检查，请在打开的浏览器中手动完成后等待")
            sleep_ms(1500)
            continue
        if "accounts.google.com" in raw and time.time() - last_unknown > 8:
            log("谷歌页面未识别，当前 URL=%s", raw)
            last_unknown = time.time()
        sleep_ms(200)
    if logged_in(page):
        return
    raise RuntimeError(f"Google 登录未完成，当前 URL={page.url}")


def start_google(page, acc: dict, context=None):
    from pageutil import follow_identity_page, page_url
    from urls import google_ready_url

    if "accounts.google.com" not in page_url(page):
        try:
            click_one_of(page, GOOGLE_AUTH, 20000, "选择 Google 登录")
        except Exception:
            if "accounts.google.com" not in page_url(page) and not on_cline(page_url(page)):
                raise
            log("已在授权相关页面，继续")
        log("谷歌点击后 URL=%s", page_url(page))
    try:
        page = wait_any_url(page, [], 45000, context=context, ready=google_ready_url)
    except Exception:
        page = follow_identity_page(context, page, google_ready_url)
        log("等待授权页超时，当前 URL=%s", page_url(page))
    if "accounts.google.com" in page_url(page) or visible(page, 'input#identifierId, input[name="identifier"], input[name="Passwd"]'):
        google_login(page, acc)
    return page
