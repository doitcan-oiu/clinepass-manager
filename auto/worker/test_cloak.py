import os
import sys
import tempfile
import types
import unittest
from unittest.mock import Mock, patch

from cloak import apply_cloak_env, apply_geo_settings, chrome_args, geoip_db_ready, humanize_options, launch_ctx


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


class LaunchContextTest(unittest.TestCase):
    def test_launch_failure_is_preserved_without_retry_or_geoip_relabeling(self):
        for message in (
            "license invalid: exit code 77",
            "profile is already in use",
            "browser process crashed",
            "GeoIP resolution timed out after 20.0s",
        ):
            with self.subTest(message=message), tempfile.TemporaryDirectory() as profile:
                failure = RuntimeError(message)
                launch = Mock(side_effect=failure)
                sdk = types.ModuleType("cloakbrowser")
                sdk.launch_persistent_context = launch
                with patch.dict(sys.modules, cloakbrowser=sdk), patch.dict(os.environ), \
                     patch("cloak.geoip_db_ready", return_value=True), \
                     patch("cloak.package_version", return_value="test-sdk"), patch("cloak.log") as log:
                    with self.assertRaises(RuntimeError) as raised:
                        launch_ctx({"profile_dir": profile, "proxy": "http://127.0.0.1:1080"}, 0)
                self.assertIs(raised.exception, failure)
                launch.assert_called_once()
                self.assertTrue(launch.call_args.kwargs["geoip"])
                self.assertNotIn("timezone", launch.call_args.kwargs)
                self.assertFalse(any("geoip 启动失败" in str(call) for call in log.call_args_list))

    def test_environment_is_applied_before_sdk_import(self):
        import builtins

        original_import = builtins.__import__
        launch = Mock(return_value=object())
        sdk = types.ModuleType("cloakbrowser")
        sdk.launch_persistent_context = launch

        def checked_import(name, *args, **kwargs):
            if name == "cloakbrowser":
                self.assertEqual(os.environ.get("CLOAKBROWSER_CACHE_DIR"), "test-cache")
                self.assertEqual(os.environ.get("CLOAKBROWSER_LICENSE_KEY"), "test-license")
                return sdk
            return original_import(name, *args, **kwargs)

        with tempfile.TemporaryDirectory() as profile, patch.dict(os.environ), \
             patch("builtins.__import__", side_effect=checked_import), \
             patch("cloak.package_version", return_value="test-sdk"), patch("cloak.log") as log:
            ctx = launch_ctx({
                "profile_dir": profile, "cloak_cache_dir": "test-cache",
                "license_key": "test-license", "cloak_version": "152.0.test", "headless": False,
            }, 0)
        self.assertIs(ctx, launch.return_value)
        launch.assert_called_once()
        self.assertEqual(launch.call_args.kwargs["user_data_dir"], profile)
        self.assertEqual(launch.call_args.kwargs["browser_version"], "152.0.test")
        self.assertFalse(launch.call_args.kwargs["headless"])
        output = "\n".join(call.args[0] % call.args[1:] if len(call.args) > 1 else call.args[0] for call in log.call_args_list)
        self.assertIn("sdk=test-sdk humanize=default+custom", output)
        self.assertNotIn("test-license", output)


if __name__ == "__main__":
    unittest.main()
