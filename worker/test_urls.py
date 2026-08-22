import unittest

from urls import (
    authkit_banned_after_wait,
    authkit_callback_error,
    classify_google,
    google_ready_url,
    microsoft_ready_url,
    microsoft_step,
    on_microsoft_url,
    on_workos_url,
    radar_send_url,
)


class URLTests(unittest.TestCase):
    def test_policy_denied(self):
        u = "https://authkit.cline.bot/?error=policy_denied&authorization_session_id=01ABC"
        self.assertEqual(authkit_callback_error(u), "policy_denied")
        self.assertEqual(authkit_callback_error("https://authkit.cline.bot/?authorization_session_id=01ABC"), "")

    def test_banned_after_wait(self):
        login = "https://authkit.cline.bot/?redirect_uri=https%3A%2F%2Fapi.cline.bot%2Fapi%2Fv1%2Fauth%2Fcallback&authorization_session_id=01ABC"
        self.assertTrue(authkit_banned_after_wait(login))
        self.assertFalse(authkit_banned_after_wait("https://authkit.cline.bot/radar-challenge/send?authorization_session_id=01ABC"))
        self.assertFalse(authkit_banned_after_wait("https://authkit.cline.bot/sign-up"))
        self.assertFalse(authkit_banned_after_wait("https://app.cline.bot/dashboard"))

    def test_microsoft_host(self):
        self.assertTrue(on_microsoft_url("https://login.live.com/oauth20_authorize.srf"))
        self.assertFalse(on_microsoft_url("https://authkit.cline.bot/sign-up"))

    def test_google_classify(self):
        self.assertEqual(classify_google("https://accounts.google.com/signin/oauth"), "consent")
        self.assertEqual(classify_google("https://accounts.google.com/challenge/pwd"), "password")

    def test_microsoft_step(self):
        self.assertEqual(microsoft_step(True, True, False), "email")
        self.assertEqual(microsoft_step(True, True, True), "password")
        self.assertEqual(microsoft_step(False, True, False), "password")

    def test_microsoft_ready_ignores_workos(self):
        workos = "https://auth.workos.com/sso/oauth/microsoft/abc/callback"
        self.assertTrue(on_workos_url(workos))
        self.assertFalse(microsoft_ready_url(workos))
        self.assertTrue(microsoft_ready_url("https://login.live.com/oauth20_authorize.srf"))
        self.assertTrue(google_ready_url("https://accounts.google.com/signin"))

    def test_radar_send_url(self):
        self.assertIn(
            "radar-challenge/send",
            radar_send_url("https://authkit.cline.bot/radar-challenge/verify?x=1"),
        )


if __name__ == "__main__":
    unittest.main()
