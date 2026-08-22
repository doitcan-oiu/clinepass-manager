#!/usr/bin/env python3
import json
import os
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
    try:
        ctx = launch_ctx(settings, seed)
        pages = ctx.pages
        page = pages[0] if pages else ctx.new_page()
        if action == "refresh":
            out = run_refresh(page, ctx, acc)
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
        if ctx is not None:
            try:
                ctx.close()
            except Exception:
                pass


if __name__ == "__main__":
    sys.exit(main())
