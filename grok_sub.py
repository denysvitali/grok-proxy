#!/usr/bin/env python3
"""Standalone client for Grok subscription-backed API calls.

This program does not invoke the Grok binary. It speaks OIDC and HTTP directly.
"""

from __future__ import annotations

import argparse
import base64
import hashlib
import json
import os
import secrets
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
import webbrowser
from dataclasses import dataclass
from datetime import datetime, timezone
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path
from typing import Any, Iterator


ISSUER = "https://auth.x.ai"
CLIENT_ID = "b1a00492-073a-47ea-816f-4c329264a828"
DEFAULT_SCOPES = "openid profile email offline_access grok-cli:access api:access"
DEFAULT_API_BASE = "https://cli-chat-proxy.grok.com/v1"
CLIENT_VERSION = "0.2.99"
DEFAULT_AUTH_FILE = Path.home() / ".config" / "grok-subscription-client" / "auth.json"
GROK_AUTH_FILE = Path.home() / ".grok" / "auth.json"


class ClientError(RuntimeError):
    pass


def _b64url(raw: bytes) -> str:
    return base64.urlsafe_b64encode(raw).rstrip(b"=").decode()


def _parse_time(value: Any) -> float | None:
    if not value:
        return None
    if isinstance(value, (int, float)):
        return float(value)
    try:
        return datetime.fromisoformat(str(value).replace("Z", "+00:00")).timestamp()
    except ValueError:
        return None


def _json_request(
    url: str,
    *,
    method: str = "GET",
    headers: dict[str, str] | None = None,
    data: dict[str, Any] | None = None,
    form: dict[str, str] | None = None,
    timeout: float = 30,
) -> tuple[Any, dict[str, str]]:
    body = None
    request_headers = dict(headers or {})
    if data is not None:
        body = json.dumps(data).encode()
        request_headers["Content-Type"] = "application/json"
    elif form is not None:
        body = urllib.parse.urlencode(form).encode()
        request_headers["Content-Type"] = "application/x-www-form-urlencoded"
    req = urllib.request.Request(url, data=body, headers=request_headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as response:
            payload = response.read()
            parsed = json.loads(payload) if payload else {}
            return parsed, dict(response.headers.items())
    except urllib.error.HTTPError as exc:
        raw = exc.read().decode(errors="replace")
        try:
            detail = json.loads(raw)
        except json.JSONDecodeError:
            detail = raw
        raise ClientError(f"HTTP {exc.code} from {url}: {detail}") from None
    except urllib.error.URLError as exc:
        raise ClientError(f"request to {url} failed: {exc.reason}") from None


@dataclass
class Discovery:
    authorization_endpoint: str
    device_authorization_endpoint: str
    token_endpoint: str
    userinfo_endpoint: str


def discover(issuer: str = ISSUER) -> Discovery:
    doc, _ = _json_request(issuer.rstrip("/") + "/.well-known/openid-configuration")
    required = (
        "authorization_endpoint",
        "device_authorization_endpoint",
        "token_endpoint",
        "userinfo_endpoint",
    )
    missing = [key for key in required if not doc.get(key)]
    if missing:
        raise ClientError(f"OIDC discovery is missing: {', '.join(missing)}")
    return Discovery(**{key: doc[key] for key in required})


class TokenStore:
    def __init__(self, path: Path = DEFAULT_AUTH_FILE):
        self.path = path

    def load(self) -> dict[str, Any] | None:
        if not self.path.exists():
            return None
        try:
            data = json.loads(self.path.read_text())
        except (OSError, json.JSONDecodeError) as exc:
            raise ClientError(f"cannot read {self.path}: {exc}") from None
        return data if isinstance(data, dict) else None

    def save(self, token: dict[str, Any]) -> None:
        self.path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
        stored = dict(token)
        stored.setdefault("issuer", ISSUER)
        stored.setdefault("client_id", CLIENT_ID)
        if stored.get("expires_in") is not None:
            stored["expires_at"] = time.time() + float(stored["expires_in"])
        tmp = self.path.with_suffix(".tmp")
        fd = os.open(tmp, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
        try:
            with os.fdopen(fd, "w") as handle:
                json.dump(stored, handle, indent=2)
                handle.write("\n")
            os.replace(tmp, self.path)
            os.chmod(self.path, 0o600)
        finally:
            if tmp.exists():
                tmp.unlink()

    def clear(self) -> None:
        if self.path.exists():
            self.path.unlink()


def import_grok_token(path: Path = GROK_AUTH_FILE) -> dict[str, Any]:
    try:
        root = json.loads(path.read_text())
    except (OSError, json.JSONDecodeError) as exc:
        raise ClientError(f"cannot read {path}: {exc}") from None
    candidates = []
    for value in root.values() if isinstance(root, dict) else ():
        if not isinstance(value, dict) or not value.get("key"):
            continue
        candidates.append(value)
    if not candidates:
        raise ClientError(f"no session token found in {path}")
    selected = max(candidates, key=lambda item: _parse_time(item.get("expires_at")) or 0)
    return {
        "access_token": selected["key"],
        "refresh_token": selected.get("refresh_token"),
        "expires_at": _parse_time(selected.get("expires_at")),
        "issuer": selected.get("oidc_issuer", ISSUER),
        "client_id": selected.get("oidc_client_id", CLIENT_ID),
        "source": str(path),
    }


def _normalize_token(response: dict[str, Any]) -> dict[str, Any]:
    if not response.get("access_token"):
        raise ClientError("token response did not contain access_token")
    return response


def refresh_token(token: dict[str, Any], store: TokenStore) -> dict[str, Any]:
    refresh = token.get("refresh_token")
    if not refresh:
        raise ClientError("token expired and no refresh token is available; run login")
    issuer = token.get("issuer", ISSUER)
    client_id = token.get("client_id", CLIENT_ID)
    endpoints = discover(issuer)
    response, _ = _json_request(
        endpoints.token_endpoint,
        method="POST",
        form={"grant_type": "refresh_token", "refresh_token": refresh, "client_id": client_id},
    )
    response.setdefault("refresh_token", refresh)
    response.setdefault("issuer", issuer)
    response.setdefault("client_id", client_id)
    response = _normalize_token(response)
    store.save(response)
    return store.load() or response


def usable_token(store: TokenStore, *, import_existing: bool = True) -> dict[str, Any]:
    token = store.load()
    if token is None and import_existing and GROK_AUTH_FILE.exists():
        token = import_grok_token()
        store.save(token)
        token = store.load()
    if token is None:
        raise ClientError("not logged in; run `grok-sub login` or `grok-sub login --device`")
    expires_at = _parse_time(token.get("expires_at"))
    if expires_at is not None and expires_at <= time.time() + 300:
        token = refresh_token(token, store)
    return token


def login_device(store: TokenStore, *, issuer: str, client_id: str, scopes: str) -> None:
    endpoints = discover(issuer)
    device, _ = _json_request(
        endpoints.device_authorization_endpoint,
        method="POST",
        form={"client_id": client_id, "scope": scopes},
    )
    code = device.get("device_code")
    if not code:
        raise ClientError("device authorization response omitted device_code")
    verification = device.get("verification_uri_complete") or device.get("verification_uri")
    print(f"Open: {verification}", file=sys.stderr)
    if device.get("user_code"):
        print(f"Code: {device['user_code']}", file=sys.stderr)
    interval = max(float(device.get("interval", 5)), 1)
    deadline = time.monotonic() + float(device.get("expires_in", 600))
    while time.monotonic() < deadline:
        time.sleep(interval)
        try:
            response, _ = _json_request(
                endpoints.token_endpoint,
                method="POST",
                form={
                    "grant_type": "urn:ietf:params:oauth:grant-type:device_code",
                    "device_code": code,
                    "client_id": client_id,
                },
            )
        except ClientError as exc:
            message = str(exc)
            if "authorization_pending" in message:
                continue
            if "slow_down" in message:
                interval += 5
                continue
            raise
        response.setdefault("issuer", issuer)
        response.setdefault("client_id", client_id)
        store.save(_normalize_token(response))
        print("Login successful.", file=sys.stderr)
        return
    raise ClientError("device authorization expired")


def login_browser(store: TokenStore, *, issuer: str, client_id: str, scopes: str) -> None:
    endpoints = discover(issuer)
    verifier = _b64url(secrets.token_bytes(48))
    challenge = _b64url(hashlib.sha256(verifier.encode()).digest())
    state = secrets.token_urlsafe(24)
    result: dict[str, str] = {}

    class Callback(BaseHTTPRequestHandler):
        def do_GET(self) -> None:  # noqa: N802
            params = urllib.parse.parse_qs(urllib.parse.urlparse(self.path).query)
            result.update({key: values[0] for key, values in params.items() if values})
            body = b"Login received. You can close this window."
            self.send_response(200)
            self.send_header("Content-Type", "text/plain; charset=utf-8")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def log_message(self, _format: str, *args: Any) -> None:
            return

    server = HTTPServer(("127.0.0.1", 0), Callback)
    redirect_uri = f"http://127.0.0.1:{server.server_port}/callback"
    query = urllib.parse.urlencode(
        {
            "response_type": "code",
            "client_id": client_id,
            "redirect_uri": redirect_uri,
            "scope": scopes,
            "state": state,
            "code_challenge": challenge,
            "code_challenge_method": "S256",
        }
    )
    url = endpoints.authorization_endpoint + "?" + query
    print(f"Open: {url}", file=sys.stderr)
    webbrowser.open(url)
    server.timeout = 300
    server.handle_request()
    server.server_close()
    if result.get("state") != state:
        raise ClientError("OAuth callback state mismatch")
    if result.get("error"):
        raise ClientError(f"authorization failed: {result['error']}")
    if not result.get("code"):
        raise ClientError("OAuth callback did not contain an authorization code")
    response, _ = _json_request(
        endpoints.token_endpoint,
        method="POST",
        form={
            "grant_type": "authorization_code",
            "code": result["code"],
            "redirect_uri": redirect_uri,
            "client_id": client_id,
            "code_verifier": verifier,
        },
    )
    response.setdefault("issuer", issuer)
    response.setdefault("client_id", client_id)
    store.save(_normalize_token(response))
    print("Login successful.", file=sys.stderr)


class GrokClient:
    def __init__(self, token: str, *, base_url: str = DEFAULT_API_BASE):
        self.token = token
        self.base_url = base_url.rstrip("/")

    def headers(self, model: str | None = None) -> dict[str, str]:
        headers = {
            "Authorization": f"Bearer {self.token}",
            "X-XAI-Token-Auth": "xai-grok-cli",
            "x-grok-client-version": CLIENT_VERSION,
            "x-grok-client-mode": "cli",
            "User-Agent": f"grok-subscription-client/{CLIENT_VERSION}",
        }
        if model:
            headers["x-grok-model-override"] = model
        return headers

    def models(self) -> Any:
        payload, _ = _json_request(self.base_url + "/models", headers=self.headers())
        return payload

    def request(self, model: str, prompt: str, *, backend: str = "responses", stream: bool = True) -> Any:
        if backend == "responses":
            path = "/responses"
            body = {"model": model, "input": prompt, "stream": stream}
        elif backend == "chat_completions":
            path = "/chat/completions"
            body = {"model": model, "messages": [{"role": "user", "content": prompt}], "stream": stream}
        else:
            raise ClientError(f"unsupported backend: {backend}")
        if not stream:
            payload, _ = _json_request(
                self.base_url + path, method="POST", headers=self.headers(model), data=body, timeout=300
            )
            return payload
        return self._stream(self.base_url + path, body, model)

    def _stream(self, url: str, body: dict[str, Any], model: str) -> Iterator[dict[str, Any]]:
        headers = self.headers(model)
        headers["Content-Type"] = "application/json"
        headers["Accept"] = "text/event-stream"
        req = urllib.request.Request(url, data=json.dumps(body).encode(), headers=headers, method="POST")
        try:
            response = urllib.request.urlopen(req, timeout=300)
        except urllib.error.HTTPError as exc:
            detail = exc.read().decode(errors="replace")
            raise ClientError(f"HTTP {exc.code} from {url}: {detail}") from None
        except urllib.error.URLError as exc:
            raise ClientError(f"request to {url} failed: {exc.reason}") from None
        with response:
            for raw in response:
                line = raw.decode(errors="replace").strip()
                if not line or line.startswith(":") or line.startswith("event:"):
                    continue
                if line.startswith("data:"):
                    line = line[5:].strip()
                if line == "[DONE]":
                    return
                try:
                    yield json.loads(line)
                except json.JSONDecodeError:
                    continue


def _print_stream(events: Iterator[dict[str, Any]]) -> None:
    for event in events:
        event_type = event.get("type")
        if event_type in {"response.output_text.delta", "output_text.delta"}:
            print(event.get("delta", ""), end="", flush=True)
            continue
        choices = event.get("choices") or []
        if choices:
            print(choices[0].get("delta", {}).get("content", ""), end="", flush=True)
    print()


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="grok-sub")
    parser.add_argument("--auth-file", type=Path, default=DEFAULT_AUTH_FILE)
    parser.add_argument("--base-url", default=DEFAULT_API_BASE)
    sub = parser.add_subparsers(dest="command", required=True)
    login = sub.add_parser("login")
    login.add_argument("--device", action="store_true")
    login.add_argument("--issuer", default=ISSUER)
    login.add_argument("--client-id", default=CLIENT_ID)
    login.add_argument("--scopes", default=DEFAULT_SCOPES)
    sub.add_parser("import-grok")
    sub.add_parser("logout")
    sub.add_parser("models")
    chat = sub.add_parser("chat")
    chat.add_argument("prompt")
    chat.add_argument("--model", default="grok-4.5")
    chat.add_argument("--backend", choices=("responses", "chat_completions"), default="responses")
    chat.add_argument("--no-stream", action="store_true")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    store = TokenStore(args.auth_file)
    try:
        if args.command == "login":
            if args.device:
                login_device(store, issuer=args.issuer, client_id=args.client_id, scopes=args.scopes)
            else:
                login_browser(store, issuer=args.issuer, client_id=args.client_id, scopes=args.scopes)
            return 0
        if args.command == "import-grok":
            store.save(import_grok_token())
            print(f"Imported credentials into {store.path}", file=sys.stderr)
            return 0
        if args.command == "logout":
            store.clear()
            print("Local credentials removed.", file=sys.stderr)
            return 0
        token = usable_token(store)
        client = GrokClient(token["access_token"], base_url=args.base_url)
        if args.command == "models":
            print(json.dumps(client.models(), indent=2))
            return 0
        result = client.request(args.model, args.prompt, backend=args.backend, stream=not args.no_stream)
        if args.no_stream:
            print(json.dumps(result, indent=2))
        else:
            _print_stream(result)
        return 0
    except (ClientError, KeyboardInterrupt) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
