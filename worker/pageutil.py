import random
import time

from errors import LoggedIn
from urls import left_identity_url, on_radar_url


def sleep_ms(ms: int) -> None:
    time.sleep(max(ms, 0) / 1000.0)


def visible(page, selector: str) -> bool:
    try:
        return bool(page.locator(selector).first.is_visible())
    except Exception:
        return False


def page_title(page) -> str:
    try:
        return (page.title() or "").strip()
    except Exception:
        return ""


def on_radar_flow(page) -> bool:
    if on_radar_url(page.url):
        return True
    return (
        visible(page, 'input[name="local_number"]')
        or visible(page, 'input[name="country_code"]')
        or visible(page, 'input[data-test="otp-input"]')
        or visible(page, ".ak-Otp")
    )


def logged_in(page) -> bool:
    return left_identity_url(page.url) or on_radar_flow(page)


def wait_overlay_gone(page) -> None:
    loc = page.locator('div[jsname="OQ2Y6"]')
    try:
        if loc.count() == 0:
            return
        loc.first.wait_for(state="hidden", timeout=8000)
    except Exception:
        pass


def click_one_of(page, selectors, timeout_ms: float, step: str) -> None:
    wait_overlay_gone(page)
    deadline = time.time() + timeout_ms / 1000.0
    last_err = None
    while time.time() < deadline:
        for sel in selectors:
            loc = page.locator(sel).first
            try:
                if not loc.is_visible():
                    continue
            except Exception:
                continue
            try:
                try:
                    page.hover(sel, timeout=4000)
                    sleep_ms(180 + random.randint(0, 220))
                except Exception:
                    pass
                page.click(sel, timeout=8000)
                from protocol import log

                log("%s 成功", step)
                sleep_ms(900 + random.randint(0, 700))
                return
            except Exception as exc:
                last_err = exc
        sleep_ms(250)
    if last_err is not None:
        raise RuntimeError(f"{step} 失败: {last_err}")
    raise RuntimeError(f"{step} 失败: 未找到可点击按钮")


def type_field(page, selector: str, value: str) -> None:
    wait_overlay_gone(page)
    try:
        page.hover(selector, timeout=4000)
        sleep_ms(160 + random.randint(0, 200))
    except Exception:
        pass
    page.click(selector, timeout=12000)
    sleep_ms(120 + random.randint(0, 180))
    page.keyboard.press("Control+a")
    sleep_ms(40 + random.randint(0, 50))
    page.keyboard.press("Backspace")
    sleep_ms(80 + random.randint(0, 80))
    page.type(selector, value)


def input_value(page, selector: str) -> str:
    try:
        return page.locator(selector).first.input_value() or ""
    except Exception:
        return ""


def wait_visible(page, selector: str, timeout_ms: float, stop_if_done: bool = True) -> None:
    deadline = time.time() + timeout_ms / 1000.0
    while time.time() < deadline:
        if stop_if_done and left_identity_url(page.url):
            raise LoggedIn()
        if visible(page, selector):
            return
        sleep_ms(200)
    if stop_if_done and left_identity_url(page.url):
        raise LoggedIn()
    raise RuntimeError("等待元素超时")


def page_url(page) -> str:
    try:
        return page.url or ""
    except Exception:
        return ""


def follow_identity_page(context, page, ready):
    if context is None:
        return page
    pages = list(getattr(context, "pages", None) or [])
    for p in pages:
        try:
            u = p.url or ""
        except Exception:
            continue
        if ready(u):
            if p is not page:
                from protocol import log

                log("已切到身份页 URL=%s", u)
            return p
    return page


def wait_any_url(page, parts, timeout_ms: float, context=None, ready=None):
    from protocol import log
    from urls import url_has_any

    if ready is None:
        ready = lambda u, _parts=parts: url_has_any(u, _parts)
    deadline = time.time() + timeout_ms / 1000.0
    last = 0.0
    while time.time() < deadline:
        page = follow_identity_page(context, page, ready)
        if ready(page_url(page)):
            return page
        remain = max(500, min(4000, (deadline - time.time()) * 1000))
        try:
            page.wait_for_url(lambda u: ready(u), timeout=remain)
            return page
        except Exception:
            pass
        u = page_url(page)
        if ready(u):
            return page
        if time.time() - last > 5:
            log("等待跳转，当前 URL=%s", u or "<空>")
            last = time.time()
    raise RuntimeError(f"等待 URL 超时，当前 URL={page_url(page)}")


def wait_path_contains(page, part: str, timeout_ms: float) -> None:
    from urls import url_path

    deadline = time.time() + timeout_ms / 1000.0
    while time.time() < deadline:
        if part in url_path(page.url):
            return
        sleep_ms(200)
    raise RuntimeError(f"等待 path 包含 {part} 超时")


def wait_leave(page, part: str, timeout_ms: float, name: str = "") -> None:
    from urls import url_path
    from protocol import log

    deadline = time.time() + timeout_ms / 1000.0
    last = 0.0
    while time.time() < deadline:
        u = page.url
        if left_identity_url(u):
            if name:
                log("已离开登录页，当前 URL=%s", u)
            return
        if part not in url_path(u):
            return
        if name and time.time() - last > 8:
            log("仍在等待离开%s，当前 URL=%s", name, u)
            last = time.time()
        sleep_ms(200)
    if left_identity_url(page.url):
        return
    raise RuntimeError(f"等待离开 {part} 超时，当前 URL={page.url}")


def human_idle_authkit(page) -> None:
    from protocol import log

    log("在 AuthKit 停留，让 Radar 采集页面信号")
    for _ in range(4 + random.randint(0, 2)):
        page.mouse.move(72 + random.random() * 520, 64 + random.random() * 280)
        sleep_ms(280 + random.randint(0, 420))
    try:
        human_scroll(page)
    except Exception:
        pass
    sleep_ms(1400 + random.randint(0, 900))


def human_scroll(page) -> None:
    for _ in range(4 + random.randint(0, 3)):
        page.mouse.wheel(0, 280 + random.random() * 220)
        sleep_ms(90 + random.randint(0, 110))


def screenshot(page, path: str) -> None:
    if not page or not path:
        return
    try:
        page.screenshot(path=path, full_page=True)
    except Exception:
        pass


def serialize_cookies(cookies) -> tuple[str, str]:
    import json

    parts = []
    clean = []
    for c in cookies or []:
        name = (c.get("name") or "").strip()
        if not name:
            continue
        clean.append(c)
        parts.append(f"{name}={c.get('value', '')}")
    return json.dumps(clean, ensure_ascii=False), "; ".join(parts)


def cookies_for_context(raw: str) -> list:
    import json

    items = json.loads(raw)
    out = []
    for c in items:
        name = (c.get("name") or "").strip()
        if not name:
            continue
        item = {
            "name": name,
            "value": c.get("value") or "",
            "path": c.get("path") or "/",
        }
        if c.get("domain"):
            item["domain"] = c["domain"]
        if c.get("expires"):
            item["expires"] = c["expires"]
        if "httpOnly" in c:
            item["httpOnly"] = bool(c["httpOnly"])
        if "secure" in c:
            item["secure"] = bool(c["secure"])
        if c.get("sameSite"):
            item["sameSite"] = c["sameSite"]
        out.append(item)
    if not out:
        raise RuntimeError("Cookie 为空")
    return out


def cookies_from_header(raw: str) -> list:
    out = []
    for part in (raw or "").split(";"):
        part = part.strip()
        if not part or "=" not in part:
            continue
        name, value = part.split("=", 1)
        name = name.strip()
        if not name:
            continue
        out.append({"name": name, "value": value.strip(), "domain": ".cline.bot", "path": "/"})
    if not out:
        raise RuntimeError("Cookie 为空")
    return out


def cookies_for_account(acc: dict) -> list:
    raw = (acc.get("cookies_json") or "").strip()
    if raw:
        try:
            return cookies_for_context(raw)
        except Exception:
            pass
    return cookies_from_header(acc.get("cookie_header") or "")


def field_ready(page, selector: str) -> bool:
    loc = page.locator(selector).first
    try:
        if not loc.is_visible():
            return False
        hidden = loc.get_attribute("aria-hidden")
        if hidden and hidden.strip().lower() == "true":
            return False
        if not loc.is_enabled() or not loc.is_editable():
            return False
        return True
    except Exception:
        return False


def first_ready_field(page, selectors) -> str:
    for sel in selectors:
        if field_ready(page, sel):
            return sel
    return ""
