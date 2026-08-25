import unittest

from cloak import chrome_args


class ChromeArgsTest(unittest.TestCase):
    def test_systemd_root_needs_no_sandbox_and_no_gpu(self):
        args = chrome_args({"virtual_display": True, "headless": False}, 12)
        self.assertIn("--no-sandbox", args)
        self.assertIn("--disable-setuid-sandbox", args)
        self.assertIn("--disable-gpu", args)
        self.assertIn("--fingerprint=12", args)
        self.assertIn("--fingerprint-windows-font-metrics", args)
        self.assertIn("--fingerprint-allow-3p-cookies", args)

    def test_headed_local_keeps_gpu(self):
        args = chrome_args({"virtual_display": False, "headless": False}, 0)
        self.assertIn("--no-sandbox", args)
        self.assertNotIn("--disable-gpu", args)


if __name__ == "__main__":
    unittest.main()
