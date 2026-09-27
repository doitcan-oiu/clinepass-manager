import json
import ssl
import time
import urllib.error
import urllib.parse
import urllib.request

HANDLER = "https://hero-sms.com/stubs/handler_api.php"
DEFAULT_SERVICE = "ot"
UA = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36"

ITU = {
    1, 7, 20, 27, 30, 31, 32, 33, 34, 36, 39, 40, 41, 43, 44, 45, 46, 47, 48, 49,
    51, 52, 53, 54, 55, 56, 57, 58, 60, 61, 62, 63, 64, 65, 66, 81, 82, 84, 86,
    90, 91, 92, 93, 94, 95, 98,
}


def digits(s: str) -> str:
    return "".join(ch for ch in (s or "") if ch.isdigit())


def _sms_error(text: str):
    text = (text or "").strip()
    mapping = {
        "BAD_KEY": "Hero SMS API Key 无效",
        "BAD_API_KEY": "Hero SMS API Key 无效",
        "NO_NUMBERS": "这个区域/报价没有号码",
        "NO_BALANCE": "Hero SMS 余额不足",
        "WRONG_SERVICE": "Hero SMS 服务代码无效",
        "WRONG_MAX_PRICE": "报价无效，请重新选择",
    }
    if text in mapping:
        raise RuntimeError(mapping[text])
    if text.startswith("BANNED"):
        raise RuntimeError("Hero SMS 账号被限制")


def _build_request(api_key: str, action: str, extra: dict | None = None) -> urllib.request.Request:
    q = {"api_key": api_key, "action": action}
    if extra:
        q.update({k: str(v) for k, v in extra.items() if v is not None and v != ""})
    url = HANDLER + "?" + urllib.parse.urlencode(q)
    return urllib.request.Request(
        url,
        headers={
            "User-Agent": UA,
            "Accept": "*/*",
            "Connection": "close",
        },
    )


def _handler(api_key: str, action: str, extra: dict | None = None) -> bytes:
    req = _build_request(api_key, action, extra)
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            body = resp.read()
    except urllib.error.HTTPError as exc:
        body = exc.read() if exc.fp else b""
        text = body.decode("utf-8", "replace").strip()
        _sms_error(text)
        raise RuntimeError(f"Hero SMS HTTP {exc.code}: {text[:200] or exc.reason}") from exc
    except (urllib.error.URLError, ssl.SSLError, TimeoutError, ConnectionError) as exc:
        reason = getattr(exc, "reason", exc)
        raise RuntimeError(f"Hero SMS 暂时不可用: {reason}") from exc
    text = body.decode("utf-8", "replace").strip()
    _sms_error(text)
    return body


def _transient(exc: Exception) -> bool:
    msg = str(exc).lower()
    needles = (
        "暂时不可用",
        "http 429",
        "http 502",
        "http 503",
        "http 504",
        "timed out",
        "timeout",
        "unexpected_eof",
        "eof occurred",
        "ssl",
        "urlopen error",
        "connection reset",
        "broken pipe",
        "remote end closed",
    )
    return any(n in msg for n in needles)


def _split_phone(phone: str, hint: int = 0) -> tuple[int, str]:
    phone = digits(phone)
    if phone.startswith("00"):
        phone = phone[2:]
    if hint > 0:
        prefix = str(hint)
        rest = phone
        if phone.startswith(prefix) and len(phone) > len(prefix) + 4:
            rest = phone[len(prefix) :]
        rest = rest.lstrip("0") or phone
        return hint, rest
    for n in (3, 2, 1):
        if len(phone) < n + 6:
            continue
        try:
            code = int(phone[:n])
        except ValueError:
            continue
        if code not in ITU and n != 1:
            continue
        local = phone[n:].lstrip("0")
        if local:
            return code, local
    return 0, phone


class Client:
    def __init__(self, api_key: str, service: str = ""):
        self.api_key = (api_key or "").strip()
        self.service = (service or "").strip() or DEFAULT_SERVICE

    def get_number(self, country: int, max_price: float = 0) -> dict:
        if country <= 0:
            raise RuntimeError("还没有选择 Hero SMS 区域")
        extra = {"service": self.service, "country": country}
        if max_price > 0:
            extra["maxPrice"] = ("%g" % max_price)
            extra["fixedPrice"] = "true"
        last = None
        for i in range(3):
            try:
                body = _handler(self.api_key, "getNumberV2", extra)
                last = None
                break
            except Exception as exc:
                last = exc
                if not _transient(exc) or i == 2:
                    raise
                time.sleep(8)
        if last is not None:
            raise last
        text = body.decode("utf-8", "replace").strip()
        if text.startswith("ACCESS_NUMBER:"):
            parts = text.split(":")
            phone = digits(parts[-1])
            code, local = _split_phone(phone)
            return {"id": parts[1], "phone": phone, "phone_code": code, "local_number": local}
        raw = json.loads(body)
        phone = digits(str(raw.get("phoneNumber") or raw.get("phone") or raw.get("number") or ""))
        hint = int(raw.get("countryPhoneCode") or raw.get("phoneCode") or 0)
        code, local = _split_phone(phone, hint)
        return {
            "id": str(raw.get("activationId") or raw.get("id") or ""),
            "phone": phone,
            "phone_code": code,
            "local_number": local,
        }

    def wait_code(self, num_id: str, timeout: float = 120) -> str:
        deadline = time.time() + timeout
        last = None
        while time.time() < deadline:
            try:
                body = _handler(self.api_key, "getStatus", {"id": num_id})
            except Exception as exc:
                if not _transient(exc):
                    raise
                last = exc
                from protocol import log

                log("Hero SMS 查询中断，继续等验证码: %s", exc)
                time.sleep(8)
                continue
            text = body.decode("utf-8", "replace").strip()
            if text in ("STATUS_WAIT_CODE", "STATUS_WAIT_RETRY") or text.startswith("STATUS_WAIT_RETRY:"):
                time.sleep(8)
                continue
            if text.startswith("STATUS_OK:"):
                return digits(text.split(":", 1)[1])
            if text in ("STATUS_CANCEL", "STATUS_CANCELLED"):
                raise RuntimeError("接码已取消或失败")
            time.sleep(8)
        if last is not None:
            raise TimeoutError("等待验证码超时") from last
        raise TimeoutError("等待验证码超时")

    def finish(self, num_id: str) -> None:
        try:
            _handler(self.api_key, "setStatus", {"id": num_id, "status": 6})
        except Exception:
            pass

    def cancel(self, num_id: str) -> None:
        try:
            _handler(self.api_key, "setStatus", {"id": num_id, "status": 8})
        except Exception:
            pass
