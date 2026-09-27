import json
import sys


def log(msg: str, *args) -> None:
    if args:
        msg = msg % args
    sys.stdout.write(json.dumps({"type": "log", "msg": msg}, ensure_ascii=False) + "\n")
    sys.stdout.flush()


def result(ok: bool, **fields) -> None:
    payload = {"type": "result", "ok": ok}
    payload.update(fields)
    sys.stdout.write(json.dumps(payload, ensure_ascii=False) + "\n")
    sys.stdout.flush()
