import unittest

from stripe_pay import (
    CURRENCY_TOGGLE,
    USD_TOGGLE,
    checkout_error,
    expiry_mm_yy,
    is_card_rejected,
    last4,
    payment_succeeded,
    select_usd_currency,
    still_on_unpaid_checkout,
)


class FakeLocator:
    def __init__(self, visible=False, cls="", text="", attrs=None):
        self._visible = visible
        self._cls = cls
        self._text = text
        self._attrs = attrs or {}

    @property
    def first(self):
        return self

    def is_visible(self):
        return self._visible

    def get_attribute(self, name):
        if name in self._attrs:
            return self._attrs[name]
        if name == "class":
            return self._cls
        return ""

    def click(self, timeout=0):
        self._clicked = True

    def count(self):
        return 1 if self._visible else 0

    def nth(self, _i):
        return self

    def inner_text(self):
        return self._text


class FakePage:
    def __init__(self, url="", html="", visible=None, locators=None):
        self.url = url
        self._html = html
        self._visible = visible or set()
        self._locators = locators or {}

    def content(self):
        return self._html

    def locator(self, selector):
        if selector in self._locators:
            return self._locators[selector]
        if selector in self._visible:
            return FakeLocator(visible=True)
        return FakeLocator(visible=False)


class StripePayTests(unittest.TestCase):
    def test_expiry(self):
        self.assertEqual(expiry_mm_yy("2028-12"), "12 / 28")
        self.assertEqual(expiry_mm_yy("28-6"), "06 / 28")

    def test_last4(self):
        self.assertEqual(last4("4111 1111 1111 9876"), "9876")

    def test_dashboard_alone_is_not_paid(self):
        self.assertFalse(payment_succeeded(FakePage(url="https://app.cline.bot/dashboard")))
        self.assertFalse(payment_succeeded(FakePage(url="https://checkout.stripe.com/c/pay/cs_test")))
        self.assertFalse(
            payment_succeeded(FakePage(url="https://app.cline.bot/onboarding/individual-plan", html="rollingUsage"))
        )
        self.assertTrue(still_on_unpaid_checkout("https://app.cline.bot/onboarding/individual-plan"))

    def test_success_needs_proof(self):
        self.assertTrue(
            payment_succeeded(FakePage(url="https://checkout.stripe.com/c/pay/cs_test/success"))
        )
        self.assertTrue(
            payment_succeeded(
                FakePage(
                    url="https://js.stripe.com/v3/embedded-checkout-inner.html",
                    locators={"body": FakeLocator(visible=True, text="支付成功")},
                )
            )
        )
        self.assertTrue(
            payment_succeeded(FakePage(url="https://app.cline.bot/dashboard", html='rollingUsage: {status: "ok"}'))
        )
        self.assertTrue(
            payment_succeeded(
                FakePage(
                    url="https://js.stripe.com/v3/embedded-checkout-inner.html",
                    visible={"#cardNumber", ".SubmitButton-CheckmarkIcon"},
                )
            )
        )

    def test_thank_you_in_html_is_not_paid_while_form_open(self):
        page = FakePage(
            url="https://js.stripe.com/v3/embedded-checkout-inner.html?checkoutSessionId=cs_live_x",
            html='<script>window.msg="Thank you"; "Payment successful"; "You\'re all set"</script><input id="cardNumber">',
            visible={"#cardNumber"},
        )
        self.assertFalse(payment_succeeded(page))

    def test_red_card_is_declined_not_success(self):
        card = FakeLocator(visible=True, cls="CheckoutInput CheckoutInput--invalid", attrs={"aria-invalid": "true"})
        err = FakeLocator(visible=True, text="Your card was declined.")
        page = FakePage(
            url="https://js.stripe.com/v3/embedded-checkout-inner.html",
            locators={
                "#cardNumber": card,
                '[data-qa="FormFieldGroup-cardForm"] .FormFieldGroup-errorPresence': err,
                ".FormFieldGroup-errorPresence": err,
            },
            visible={"#cardNumber"},
        )
        self.assertFalse(payment_succeeded(page))
        self.assertEqual(checkout_error(page), "Your card was declined.")
        self.assertTrue(is_card_rejected("Stripe 拒付: Your card was declined."))

    def test_select_usd_skips_when_absent(self):
        page = FakePage(url="https://checkout.stripe.com/c/pay/cs_test")
        select_usd_currency(page, wait_s=0)

    def test_select_usd_clicks_inactive_button(self):
        btn = FakeLocator(visible=True, cls="Button CurrencyOptionButton")
        page = FakePage(
            url="https://checkout.stripe.com/c/pay/cs_test",
            locators={
                CURRENCY_TOGGLE: FakeLocator(visible=True),
                USD_TOGGLE[0]: btn,
            },
        )
        select_usd_currency(page)
        self.assertTrue(getattr(btn, "_clicked", False))

    def test_card_rejected(self):
        self.assertTrue(is_card_rejected("Stripe 拒付: Your card was declined."))
        self.assertTrue(is_card_rejected("余额不足"))
        self.assertFalse(is_card_rejected("等待 Stripe 支付结果超时"))
        self.assertFalse(is_card_rejected("Your card number is incomplete."))
        self.assertFalse(is_card_rejected("订购已点，但支付未完成，仍在结账页"))
        self.assertTrue(is_card_rejected("Stripe 拒付: 卡号无效或卡片被拒绝"))
        self.assertFalse(is_card_rejected("Stripe 拒付: 您的卡号不完整。"))

    def test_field_error_alert_is_failure(self):
        err = FakeLocator(visible=True, text="您的卡号不完整。")
        page = FakePage(
            url="https://js.stripe.com/v3/embedded-checkout-inner.html",
            locators={
                "#cardNumber": FakeLocator(visible=True, attrs={"aria-invalid": "true"}),
                ".FieldError span[role='alert']": err,
                "span[role='alert']": err,
            },
            visible={"#cardNumber"},
        )
        self.assertEqual(checkout_error(page), "您的卡号不完整。")
        self.assertFalse(payment_succeeded(page))


if __name__ == "__main__":
    unittest.main()
