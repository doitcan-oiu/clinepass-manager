import unittest

from stripe_pay import expiry_mm_yy, is_card_rejected, last4, payment_succeeded


class FakePage:
    def __init__(self, url="", html="", visible=None):
        self.url = url
        self._html = html
        self._visible = visible or set()

    def content(self):
        return self._html

    def locator(self, selector):
        return self

    def first(self):
        return self

    def is_visible(self):
        return False


class StripePayTests(unittest.TestCase):
    def test_expiry(self):
        self.assertEqual(expiry_mm_yy("2028-12"), "12 / 28")
        self.assertEqual(expiry_mm_yy("28-6"), "06 / 28")

    def test_last4(self):
        self.assertEqual(last4("4111 1111 1111 9876"), "9876")

    def test_success_redirect(self):
        self.assertTrue(payment_succeeded(FakePage(url="https://app.cline.bot/dashboard")))
        self.assertFalse(payment_succeeded(FakePage(url="https://checkout.stripe.com/c/pay/cs_test")))

    def test_success_text(self):
        self.assertTrue(payment_succeeded(FakePage(url="https://checkout.stripe.com/c/pay/cs_test", html="支付成功")))

    def test_card_rejected(self):
        self.assertTrue(is_card_rejected("Stripe 拒付: Your card was declined."))
        self.assertTrue(is_card_rejected("余额不足"))
        self.assertFalse(is_card_rejected("等待 Stripe 支付结果超时"))
        self.assertFalse(is_card_rejected("Your card number is incomplete."))


if __name__ == "__main__":
    unittest.main()
