import unittest

from radar import is_no_sms, should_retry_phone


class RadarRetryTests(unittest.TestCase):
    def test_used_number_should_retry(self):
        self.assertTrue(should_retry_phone(RuntimeError("发送验证码失败，号码可能已被使用")))
        self.assertTrue(
            should_retry_phone(
                RuntimeError(
                    "等待 URL 超时，当前 URL=https://authkit.cline.bot/radar-challenge/send?user_id=user_01ABC"
                )
            )
        )

    def test_sms_timeout_should_retry(self):
        self.assertTrue(is_no_sms(RuntimeError("等待验证码超时")))
        self.assertTrue(should_retry_phone(RuntimeError("等待验证码超时")))

    def test_other_errors_do_not_retry(self):
        self.assertFalse(should_retry_phone(RuntimeError("填写区号失败")))
        self.assertFalse(
            should_retry_phone(RuntimeError("等待 URL 超时，当前 URL=https://app.cline.bot/dashboard"))
        )


if __name__ == "__main__":
    unittest.main()
