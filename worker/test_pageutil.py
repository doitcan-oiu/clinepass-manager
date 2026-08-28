import unittest

from pageutil import cookies_for_account, cookies_from_header


class CookieParseTests(unittest.TestCase):
    def test_header(self):
        got = cookies_from_header("cline_session_id=abc; oc_locale=zh")
        names = {c["name"]: c["value"] for c in got}
        self.assertEqual(names["cline_session_id"], "abc")
        self.assertEqual(names["oc_locale"], "zh")
        self.assertEqual(got[0]["domain"], ".cline.bot")

    def test_account_prefers_json(self):
        got = cookies_for_account(
            {
                "cookies_json": '[{"name":"cline_session_id","value":"from-json","domain":".cline.bot"}]',
                "cookie_header": "cline_session_id=from-header",
            }
        )
        self.assertEqual(got[0]["value"], "from-json")

    def test_account_falls_back_to_header(self):
        got = cookies_for_account({"cookie_header": "cline_session_id=only-header"})
        self.assertEqual(got[0]["value"], "only-header")


if __name__ == "__main__":
    unittest.main()
