import unittest
from types import SimpleNamespace
from unittest.mock import patch

import microsoft
from errors import WorkerError


class Clock:
    def __init__(self):
        self.now = 0.0

    def sleep(self, ms):
        self.now += ms / 1000


class Locator:
    def __init__(self, page, selector):
        self.page, self.selector = page, selector
        self.first = self

    def count(self):
        return int(self.is_visible())

    def is_visible(self):
        state = self.page.current_state()
        if self.selector == "body":
            return True
        if self.selector in microsoft.EMAIL_SELS:
            return state in ("email", "both")
        if self.selector in microsoft.PASS_SELS:
            return state in ("password", "wrong_password", "both")
        if self.selector in microsoft.NEXT_SELS:
            return True  # Even unknown/challenge pages have a primary button.
        return state == "otp" and self.selector == microsoft.CHALLENGE_SELS[0]

    def is_enabled(self):
        return True

    def is_editable(self):
        return True

    def get_attribute(self, _name):
        return None

    def inner_text(self, **_kwargs):
        return self.page.texts.get(self.page.current_state(), "")

    def fill(self, value, **_kwargs):
        self.page.fills.append(self.selector)
        self.page.values[self.selector] = value

    def input_value(self):
        return self.page.values.get(self.selector, "")

    def click(self, **_kwargs):
        state = self.page.current_state()
        self.page.clicks.append(state)
        if self.page.click_error:
            raise RuntimeError("Timeout after dispatch; do not retry")
        self.page.state = self.page.transitions.get(state, state)


class Page:
    def __init__(self, state, transitions=None):
        self.state = state
        self.transitions = transitions or {}
        self.clicks, self.fills, self.values = [], [], {}
        self.click_error = False
        self.state_callback = None
        self.url_reads = 0
        self.texts = {
            "password": "输入密码 忘记密码？ 使用验证码登录",
            "stay_signed_in": "保持登录状态?",
            "consent": "Let this app access your info?",
            "wrong_password": "Your account or password is incorrect.",
            "locked": "Your account has been locked.",
            "manual": "Help us protect your account",
        }

    def current_state(self):
        return self.state_callback() if self.state_callback else self.state

    @property
    def url(self):
        self.url_reads += 1
        state = self.current_state()
        if state == "done":
            return "https://app.cline.bot/dashboard"
        if state == "callback":
            return "https://authkit.cline.bot/callback?authorization_session_id=secret"
        # Rotating query parameters must never reset the progress deadline.
        return f"https://login.live.com/oauth20_authorize.srf?state=secret-{self.url_reads}#private"

    def locator(self, selector):
        return Locator(self, selector)


class MicrosoftLoginTests(unittest.TestCase):
    def setUp(self):
        self.clock = Clock()
        self.logs = []
        self.patches = [
            patch.object(microsoft.time, "monotonic", side_effect=lambda: self.clock.now),
            patch.object(microsoft, "sleep_ms", side_effect=self.clock.sleep),
            patch.object(microsoft, "log", side_effect=lambda fmt, *args: self.logs.append(fmt % args if args else fmt)),
        ]
        for item in self.patches:
            item.start()
            self.addCleanup(item.stop)

    def run_login(self, page):
        microsoft.microsoft_login(page, {"email": "test@example.test", "password": "example"})

    def assert_stalled(self, page, expected_clicks):
        with self.assertRaises(WorkerError) as raised:
            self.run_login(page)
        self.assertEqual(raised.exception.code, "microsoft_stalled")
        self.assertEqual(page.clicks, expected_clicks)
        self.assertLess(self.clock.now, 40)
        self.assertLessEqual(len(self.logs), 3)
        self.assertNotIn("secret", " ".join(self.logs) + str(raised.exception))

    def test_password_is_submitted_once_then_stops_after_no_progress(self):
        self.assert_stalled(Page("password"), ["password"])

    def test_email_is_submitted_once_then_stops_after_no_progress(self):
        self.assert_stalled(Page("email"), ["email"])

    def test_simultaneous_cards_do_not_submit_password_before_email_accepted(self):
        page = Page("both")
        self.assert_stalled(page, ["both"])
        self.assertTrue(all(selector in microsoft.EMAIL_SELS for selector in page.fills))

    def test_unknown_primary_button_is_never_clicked(self):
        self.assert_stalled(Page("unknown"), [])

    def test_known_confirmation_is_never_repeated(self):
        self.assert_stalled(Page("stay_signed_in"), ["stay_signed_in"])

    def test_flickering_password_field_does_not_restart_timeout(self):
        page = Page("password")
        page.state_callback = lambda: "password" if self.clock.now < 2 or int(self.clock.now) % 2 else "unknown"
        self.assert_stalled(page, ["password"])

    def test_normal_flow_finishes_without_waiting_for_missing_email_card(self):
        page = Page("email", {"email": "password", "password": "stay_signed_in", "stay_signed_in": "consent", "consent": "done"})
        self.run_login(page)
        self.assertEqual(page.clicks, ["email", "password", "stay_signed_in", "consent"])
        self.assertLess(self.clock.now, 5)

    def test_existing_password_card_finishes_promptly(self):
        page = Page("password", {"password": "done"})
        self.run_login(page)
        self.assertEqual(page.clicks, ["password"])
        self.assertLess(self.clock.now, 2)

    def test_already_logged_in_skips_all_controls(self):
        page = Page("done")
        self.run_login(page)
        self.assertEqual(page.clicks, [])
        self.assertEqual(self.clock.now, 0)

    def test_password_error_is_reported_immediately_without_retry(self):
        page = Page("password", {"password": "wrong_password"})
        with self.assertRaises(WorkerError) as raised:
            self.run_login(page)
        self.assertEqual(raised.exception.code, "microsoft_credentials")
        self.assertEqual(page.clicks, ["password"])
        self.assertLess(self.clock.now, 2)

    def test_locked_account_and_security_challenges_stop_without_clicking(self):
        for state, code in (("locked", "microsoft_locked"), ("manual", "microsoft_manual_required"), ("otp", "microsoft_manual_required")):
            with self.subTest(state=state):
                page = Page(state)
                with self.assertRaises(WorkerError) as raised:
                    self.run_login(page)
                self.assertEqual(raised.exception.code, code)
                self.assertEqual(page.clicks, [])

    def test_ambiguous_click_failure_is_never_resubmitted(self):
        page = Page("password")
        page.click_error = True
        with self.assertRaises(WorkerError) as raised:
            self.run_login(page)
        self.assertEqual(raised.exception.code, "microsoft_submit")
        self.assertEqual(page.clicks, ["password"])

    def test_callback_wait_does_not_repeat_oauth_login(self):
        self.assert_stalled(Page("callback"), [])

    def test_total_timeout_has_its_own_reason_after_real_stage_changes(self):
        page = Page("email")
        stages = ["email", "password", "stay_signed_in", "consent", "callback"]
        page.state_callback = lambda: stages[min(int(self.clock.now // 30), 4)]
        with self.assertRaises(WorkerError) as raised:
            self.run_login(page)
        self.assertIn("总耗时超过 120 秒", str(raised.exception))
        self.assertNotIn("35 秒无进展", str(raised.exception))
        self.assertEqual(page.clicks, stages[:4])
        self.assertLess(self.clock.now, 121)

    def test_identity_popup_is_still_followed(self):
        entry = SimpleNamespace(url="https://authkit.cline.bot/sign-in")
        popup = Page("password")
        context = SimpleNamespace(pages=[entry, popup])
        with patch("protocol.log"):
            self.assertIs(microsoft._wait_microsoft_page(entry, context, 1000), popup)

    def test_start_with_existing_session_never_opens_login_again(self):
        page = Page("done")
        self.assertIs(microsoft.start_microsoft(page, {}), page)
        self.assertEqual(page.clicks, [])

    def test_security_messages_and_known_confirmation_detection(self):
        for message in ("验证你的身份", "Enter the code we sent", "More information required", "添加安全信息"):
            self.assertEqual(microsoft.microsoft_problem(message)[1], "microsoft_manual_required")
        self.assertEqual(microsoft.microsoft_problem("That Microsoft account doesn’t exist.")[1], "microsoft_account_missing")
        self.assertEqual(microsoft.microsoft_problem("Sign in 输入密码 忘记密码？ 使用验证码登录"), ("", ""))
        self.assertEqual(microsoft.microsoft_confirmation("Next / Continue / 下一步"), "")
        self.assertEqual(microsoft.safe_page_location("https://login.live.com/path?code=secret#secret"), "login.live.com/path")


if __name__ == "__main__":
    unittest.main()
