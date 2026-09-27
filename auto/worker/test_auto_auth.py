import os
import unittest
from unittest.mock import MagicMock, patch

from stripe_pay import api_json


class AutoCallbackAuthTest(unittest.TestCase):
    def test_payment_callback_sends_auto_token(self):
        response = MagicMock()
        response.__enter__.return_value.read.return_value = b'{"ok":true}'
        with patch.dict(os.environ, {"AUTO_API_TOKEN": "worker-token"}):
            with patch("urllib.request.urlopen", return_value=response) as urlopen:
                self.assertEqual(api_json("http://127.0.0.1:9998", "/api/amzkeys/cards", "POST", {}), {"ok": True})
                req = urlopen.call_args.args[0]
                self.assertEqual(req.get_header("Authorization"), "Bearer worker-token")
                self.assertEqual(req.full_url, "http://127.0.0.1:9998/api/amzkeys/cards")


if __name__ == "__main__":
    unittest.main()
