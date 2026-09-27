import unittest

from herosms import UA, _build_request, _transient


class HeroSMSRequestTests(unittest.TestCase):
    def test_request_uses_browser_ua(self):
        req = _build_request("key", "getNumberV2", {"service": "ot", "country": 4})
        self.assertEqual(req.get_header("User-agent"), UA)
        self.assertNotIn("Python-urllib", req.get_header("User-agent"))
        self.assertIn("action=getNumberV2", req.full_url)

    def test_403_is_not_transient(self):
        self.assertFalse(_transient(RuntimeError("Hero SMS HTTP 403: Forbidden")))
        self.assertTrue(_transient(RuntimeError("Hero SMS HTTP 503")))

    def test_ssl_eof_is_transient(self):
        self.assertTrue(
            _transient(RuntimeError("Hero SMS 暂时不可用: [SSL: UNEXPECTED_EOF_WHILE_READING] EOF occurred"))
        )
        self.assertTrue(_transient(RuntimeError("<urlopen error [SSL: UNEXPECTED_EOF_WHILE_READING]>")))


if __name__ == "__main__":
    unittest.main()
