import os
import tempfile
import unittest

from cloak import apply_cloak_env, apply_geo_settings, chrome_args, geoip_db_ready, humanize_options


class ApplyCloakEnvTest(unittest.TestCase):
    def test_empty_license_clears_paid_env(self):
        os.environ["CLOAKBROWSER_LICENSE_KEY"] = "ck_parent"
        os.environ["CLOAKBROWSER_VERSION"] = "151.0"
        apply_cloak_env({"license_key": "", "cloak_version": "151.0.7922.108.2"})
        self.assertNotIn("CLOAKBROWSER_LICENSE_KEY", os.environ)
        self.assertNotIn("CLOAKBROWSER_VERSION", os.environ)

    def test_license_sets_paid_env(self):
        os.environ.pop("CLOAKBROWSER_LICENSE_KEY", None)
        apply_cloak_env({"license_key": "ck_paid", "cloak_version": "151.0.7922.108.2"})
        self.assertEqual(os.environ.get("CLOAKBROWSER_LICENSE_KEY"), "ck_paid")
        self.assertEqual(os.environ.get("CLOAKBROWSER_VERSION"), "151.0.7922.108.2")
        os.environ.pop("CLOAKBROWSER_LICENSE_KEY", None)
        os.environ.pop("CLOAKBROWSER_VERSION", None)


class HumanizeOptionsTest(unittest.TestCase):
    def test_not_careful_and_no_idle(self):
        opts = humanize_options()
        self.assertTrue(opts["humanize"])
        self.assertEqual(opts["human_preset"], "default")
        cfg = opts["human_config"]
        self.assertFalse(cfg["idle_between_actions"])
        self.assertEqual(cfg["mistype_chance"], 0)
        self.assertLessEqual(cfg["typing_delay"], 30)


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
