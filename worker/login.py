#!/usr/bin/env python3
import atexit
import json
import os
import signal
import sys
import traceback

HERE = os.path.dirname(os.path.abspath(__file__))
if HERE not in sys.path:
    sys.path.insert(0, HERE)

from cloak import launch_ctx  # noqa: E402
from errors import WorkerError  # noqa: E402
from flow import run_login, run_refresh  # noqa: E402
from pageutil import screenshot  # noqa: E402
from protocol import log, result  # noqa: E402


def read_job() -> dict:
    raw = sys.stdin.read()
    if not raw.strip():
        raise RuntimeError("没有收到登录任务")
    return json.loads(raw)


def main() -> int:
    job = read_job()
    action = (job.get("action") or "login").strip().lower()
    acc = job.get("account") or {}
    settings = job.get("settings") or {}
    seed = int(acc.get("fingerprint_seed") or 0)
    shot = (settings.get("screenshot_path") or "").strip()
    ctx = None
    page = None
    closed = False

    def close_browser() -> None:
        nonlocal closed
        if ctx is None or closed:
            return
        closed = True
        log("正在正常关闭浏览器，释放 Cloak 会话")
        try:
            ctx.close()
        except Exception as exc:
            log("关闭浏览器失败: %s", exc)

    def on_signal(signum, _frame):
        log("收到信号 %s，正常关闭浏览器", signum)
        close_browser()
        raise SystemExit(128 + signum)

    atexit.register(close_browser)
    signal.signal(signal.SIGTERM, on_signal)
    signal.signal(signal.SIGINT, on_signal)
    try:
        ctx = launch_ctx(settings, seed)
        pages = ctx.pages
        page = pages[0] if pages else ctx.new_page()
        if action == "refresh":
            out = run_refresh(page, ctx, acc, settings)
        else:
            out = run_login(page, ctx, acc, settings)
        result(True, **out)
        return 0
    except WorkerError as exc:
        if shot:
            screenshot(page, shot)
        log("%s", exc.message)
        result(False, error=exc.message, code=exc.code)
        return 2
    except Exception as exc:
        if shot:
            screenshot(page, shot)
        msg = str(exc).strip() or exc.__class__.__name__
        log("%s", msg)
        if os.environ.get("LOGIN_WORKER_DEBUG"):
            traceback.print_exc(file=sys.stderr)
        result(False, error=msg, code="")
        return 1
    finally:
        close_browser()


if __name__ == "__main__":
    sys.exit(main())
