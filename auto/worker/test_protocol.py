import io
import json
import os
from pathlib import Path
import subprocess
import sys
import unittest
from unittest.mock import patch

from protocol import configure_stdio, log, result


class ProtocolEncodingTest(unittest.TestCase):
    def test_text_stream_replacements_are_supported(self):
        stdin = io.StringIO('{"message":"微软下一步成功 ✅"}')
        stdout = io.StringIO()
        stderr = io.StringIO()
        with patch.multiple(sys, stdin=stdin, stdout=stdout, stderr=stderr):
            configure_stdio()
            message = json.load(sys.stdin)["message"]
            log("%s", message)
            result(True, message=message)
            print(message, file=sys.stderr)
        self.assertEqual(
            [json.loads(line) for line in stdout.getvalue().splitlines()],
            [
                {"type": "log", "msg": message},
                {"type": "result", "ok": True, "message": message},
            ],
        )
        self.assertEqual(stderr.getvalue(), message + "\n")

    def test_login_entry_changes_gbk_pipes_to_utf8_before_reading_job(self):
        # Stub browser imports so this exercises the real worker entry point
        # without installing, downloading, or opening any browser.
        script = r'''
import sys
import types

assert all(getattr(sys, name).encoding.lower() == "gbk"
           for name in ("stdin", "stdout", "stderr"))

def unused(*args, **kwargs):
    raise AssertionError("No browser function may run in this test")

for name, functions in {
    "cloak": ("launch_ctx",),
    "flow": ("run_keepalive", "run_login", "run_refresh"),
    "pageutil": ("screenshot",),
}.items():
    module = types.ModuleType(name)
    for function in functions:
        setattr(module, function, unused)
    sys.modules[name] = module

import login

assert all(getattr(sys, name).encoding.lower() == "utf-8"
           for name in ("stdin", "stdout", "stderr"))
job = login.read_job()
login.log("%s", job["message"])
login.result(True, message=job["message"])
print(job["message"], file=sys.stderr)
'''
        message = "微软下一步 成功 ✅ 支付链接 https://example.invalid/支付"
        env = dict(os.environ, PYTHONIOENCODING="gbk", PYTHONUTF8="0")
        child = subprocess.run(
            [sys.executable, "-c", script],
            cwd=Path(__file__).resolve().parent,
            input=json.dumps({"message": message}, ensure_ascii=False).encode("utf-8"),
            capture_output=True,
            env=env,
            timeout=10,
        )
        self.assertEqual(child.returncode, 0, child.stderr.decode("utf-8", errors="replace"))
        self.assertEqual(
            [json.loads(line) for line in child.stdout.decode("utf-8").splitlines()],
            [
                {"type": "log", "msg": message},
                {"type": "result", "ok": True, "message": message},
            ],
        )
        self.assertEqual(child.stderr.decode("utf-8").strip(), message)


if __name__ == "__main__":
    unittest.main()
