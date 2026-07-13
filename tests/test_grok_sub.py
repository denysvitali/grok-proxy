import json
import stat
import tempfile
import time
import unittest
from pathlib import Path
from unittest.mock import patch

import grok_sub


class GrokSubTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name)

    def tearDown(self):
        self.temp.cleanup()

    def test_imports_newest_grok_token(self):
        source = self.root / "auth.json"
        source.write_text(json.dumps({
            "old": {"key": "old-secret", "expires_at": "2024-01-01T00:00:00Z"},
            "new": {
                "key": "new-secret",
                "refresh_token": "refresh-secret",
                "expires_at": "2030-01-01T00:00:00Z",
                "oidc_issuer": "https://issuer.example",
                "oidc_client_id": "client",
            },
        }))
        token = grok_sub.import_grok_token(source)
        self.assertEqual(token["access_token"], "new-secret")
        self.assertEqual(token["refresh_token"], "refresh-secret")
        self.assertEqual(token["issuer"], "https://issuer.example")

    def test_token_store_permissions(self):
        path = self.root / "private" / "auth.json"
        store = grok_sub.TokenStore(path)
        store.save({"access_token": "secret", "expires_in": 3600})
        self.assertEqual(stat.S_IMODE(path.stat().st_mode), 0o600)
        self.assertEqual(store.load()["access_token"], "secret")

    def test_headers_match_cli_session_contract(self):
        headers = grok_sub.GrokClient("secret").headers("grok-4.5")
        self.assertEqual(headers["Authorization"], "Bearer secret")
        self.assertEqual(headers["X-XAI-Token-Auth"], "xai-grok-cli")
        self.assertEqual(headers["x-grok-model-override"], "grok-4.5")
        self.assertEqual(headers["x-grok-client-version"], grok_sub.CLIENT_VERSION)

    def test_responses_request_shape(self):
        client = grok_sub.GrokClient("secret", base_url="https://example.test/v1")
        with patch("grok_sub._json_request", return_value=({"ok": True}, {})) as request:
            result = client.request("grok-4.5", "hello", stream=False)
        self.assertEqual(result, {"ok": True})
        self.assertEqual(request.call_args.args[0], "https://example.test/v1/responses")
        self.assertEqual(request.call_args.kwargs["data"], {
            "model": "grok-4.5", "input": "hello", "stream": False
        })

    def test_refresh_preserves_refresh_token(self):
        store = grok_sub.TokenStore(self.root / "auth.json")
        old = {"access_token": "old", "refresh_token": "refresh", "issuer": "https://issuer"}
        discovery = grok_sub.Discovery("a", "d", "https://issuer/token", "u")
        with patch("grok_sub.discover", return_value=discovery), patch(
            "grok_sub._json_request", return_value=({"access_token": "new", "expires_in": 3600}, {})
        ):
            refreshed = grok_sub.refresh_token(old, store)
        self.assertEqual(refreshed["access_token"], "new")
        self.assertEqual(refreshed["refresh_token"], "refresh")
        self.assertGreater(refreshed["expires_at"], time.time())


if __name__ == "__main__":
    unittest.main()
