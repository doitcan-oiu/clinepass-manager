import json
import sys


def configure_stdio() -> None:
    """Use UTF-8 for the Go/Python pipe protocol, including on Windows."""
    for name in ("stdin", "stdout", "stderr"):
        stream = getattr(sys, name)
        reconfigure = getattr(stream, "reconfigure", None)
        if callable(reconfigure):
            # Diagnostic output should remain printable even for invalid surrogates.
            errors = "backslashreplace" if name == "stderr" else "strict"
            reconfigure(encoding="utf-8", errors=errors)


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
