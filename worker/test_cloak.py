import os
import tempfile
import unittest

from cloak import apply_geo_settings, chrome_args, geoip_db_ready


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


class GeoSettingsTest(unittest.TestCase):
    def test_no_proxy_skips_geoip_download(self):
        kwargs = apply_geo_settings({}, None, root="/tmp/missing-cloak-cache")
        self.assertFalse(kwargs["geoip"])
        self.assertEqual(kwargs["timezone"], "Asia/Shanghai")
        self.assertEqual(kwargs["locale"], "zh-CN")

    def test_proxy_without_db_skips_download(self):
        kwargs = apply_geo_settings({}, "http://127.0.0.1:1080", root="/tmp/missing-cloak-cache")
        self.assertFalse(kwargs["geoip"])
        self.assertEqual(kwargs["timezone"], "Asia/Shanghai")

    def test_proxy_with_local_db_uses_geoip(self):
        with tempfile.TemporaryDirectory() as tmp:
            geo = os.path.join(tmp, "geoip")
            os.makedirs(geo)
            db = os.path.join(geo, "GeoLite2-City.mmdb")
            with open(db, "wb") as f:
                f.write(b"x" * 1_000_001)
            self.assertTrue(geoip_db_ready(tmp))
            kwargs = apply_geo_settings({"timezone": "Asia/Shanghai"}, "http://127.0.0.1:1080", root=tmp)
            self.assertTrue(kwargs["geoip"])
            self.assertNotIn("timezone", kwargs)
            self.assertNotIn("locale", kwargs)


if __name__ == "__main__":
    unittest.main()
