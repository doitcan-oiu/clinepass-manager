import unittest
from types import SimpleNamespace
from unittest.mock import patch

import authkit
from errors import WorkerError


class AuthkitWaitTests(unittest.TestCase):
    def setUp(self):
        self.now = 0.0
        self.page = SimpleNamespace(url="https://authkit.cline.bot/?authorization_session_id=example")
        self.transition_at = None
        self.transition_url = ""
        patches = [
            patch.object(authkit.time, "monotonic", side_effect=lambda: self.now),
            patch.object(authkit, "sleep_ms", side_effect=self.sleep),
            patch.object(authkit, "on_radar_flow", return_value=False),
            patch.object(authkit, "visible_auth_button", return_value=True),
            patch.object(authkit, "page_title", return_value="Sign in"),
            patch.object(authkit, "log"),
        ]
        for mocked in patches:
            mocked.start()
            self.addCleanup(mocked.stop)
        click_patch = patch.object(authkit, "click_one_of")
        self.click = click_patch.start()
        self.addCleanup(click_patch.stop)

    def sleep(self, ms):
        self.now += ms / 1000
        if self.transition_at is not None and self.now >= self.transition_at:
            self.page.url = self.transition_url

    def wait(self):
        return authkit.handle_authkit_wait(self.page, [0.0], "google")

    def test_pending_callback_waits_then_stops_without_ban_or_login_click(self):
        with self.assertRaises(WorkerError) as raised:
            self.wait()
        self.assertEqual(raised.exception.code, "authkit_callback_pending")
        self.assertNotIn("封禁", str(raised.exception))
        self.assertGreaterEqual(self.now, 30)
        self.assertLess(self.now, 30.2)
        self.click.assert_not_called()

    def test_slow_successful_callback_is_allowed_to_finish(self):
        self.transition_at = 10
        self.transition_url = "https://app.cline.bot/dashboard"
        self.assertTrue(self.wait())
        self.assertGreaterEqual(self.now, 10)
        self.assertLess(self.now, 10.2)
        self.click.assert_not_called()

    def test_explicit_policy_denied_stops_immediately(self):
        self.page.url += "&error=policy_denied"
        with self.assertRaises(WorkerError) as raised:
            self.wait()
        self.assertEqual(raised.exception.code, "radar_denied")
        self.assertIn("服务端拒绝", str(raised.exception))
        self.assertEqual(self.now, 0)
        self.click.assert_not_called()

    def test_policy_denied_during_wait_stops_without_waiting_out_timeout(self):
        self.transition_at = 0.45
        self.transition_url = self.page.url + "&error=policy_denied"
        with self.assertRaises(WorkerError) as raised:
            self.wait()
        self.assertEqual(raised.exception.code, "radar_denied")
        self.assertLess(self.now, 1)
        self.click.assert_not_called()

    def test_callback_arriving_during_initial_wait_gets_full_callback_wait(self):
        self.page.url = "https://authkit.cline.bot/sign-in"
        self.transition_at = 0.45
        self.transition_url = "https://authkit.cline.bot/?authorization_session_id=example"
        self.assertFalse(self.wait())
        initial_wait = self.now
        self.assertLess(initial_wait, 2)
        with self.assertRaises(WorkerError) as raised:
            self.wait()
        self.assertEqual(raised.exception.code, "authkit_callback_pending")
        self.assertGreaterEqual(self.now - initial_wait, 30)
        self.click.assert_not_called()


if __name__ == "__main__":
    unittest.main()
