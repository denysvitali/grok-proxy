# Grok Proxy

`grok-proxy` is a Go CLI and local HTTP proxy that uses the session from a
Grok subscription without executing the installed `grok` binary. It can be
used directly as a chat client or as a model provider for Claude Code and
Codex.

> [!IMPORTANT]
> The subscription endpoint is not a stable public API. Its URL, headers, and
> behavior may change with Grok CLI releases. Check that using a subscription
> this way is permitted for your account and use case.

## Install

```bash
go install github.com/denysvitali/grok-proxy@latest
```

To build from a checkout:

```bash
go build -o grok-proxy .
```

## Authentication and CLI usage

Import an existing Grok CLI session:

```bash
grok-proxy import-grok
```

Or sign in through xAI OAuth using browser + PKCE or device authorization:

```bash
grok-proxy login
grok-proxy login --device
```

Credentials are stored atomically in
`~/.config/grok-proxy/auth.json` with mode `0600`. An existing credential file
from the former Python client or `~/.grok/auth.json` is imported automatically
when needed. Token values are never logged.

Other commands:

```bash
grok-proxy models
grok-proxy chat "Explain this repository"
grok-proxy chat --no-stream --model grok-4.5 "Hello"
grok-proxy logout
```

## Start the proxy

```bash
grok-proxy serve
```

The default listener is `127.0.0.1:8080`. The server exposes:

- `GET /` for account, subscription, usage, and proxy status
- `GET /login` for interactive xAI login from a browser
- `POST /v1/responses` for Codex and other Responses API clients
- `POST /v1/messages` for Claude Code and Anthropic Messages clients
- `POST /v1/messages/count_tokens` for a conservative local token estimate
- `GET /v1/models`
- `GET /healthz`

When running the proxy on a remote host, open `/login` in a browser and select
**Sign in with xAI**. The page uses xAI device authorization and stores the
result in the configured `auth_file`; keep the page open until it reports that
login succeeded. Return to `/` to view account and usage information fetched
from the same account services as Grok Build.

The first release supports text, system/developer instructions, function
tools, tool results, reasoning settings, usage, and SSE streaming. Image,
audio, file, batch, and WebSocket requests are rejected explicitly.

### Claude Code

With the default unauthenticated loopback listener, any non-empty placeholder
token is sufficient on the client side:

```bash
ANTHROPIC_BASE_URL=http://127.0.0.1:8080 \
ANTHROPIC_AUTH_TOKEN=local \
claude
```

Claude model names are mapped to the configured default Grok model.

### Codex

Add a custom provider to `~/.codex/config.toml`:

```toml
model = "grok-4.5"
model_provider = "grok_proxy"

[model_providers.grok_proxy]
name = "Grok Proxy"
base_url = "http://127.0.0.1:8080/v1"
wire_api = "responses"
requires_openai_auth = false
```

Then run `codex` normally. A requested model beginning with `grok-` is passed
through to the subscription endpoint.

## Configuration

Configuration is loaded in this order: command-line flags, `GROK_PROXY_*`
environment variables, `~/.config/grok-proxy/config.yaml`, then defaults.

```yaml
base_url: https://cli-chat-proxy.grok.com/v1
log_level: info
log_format: text

server:
  listen: 127.0.0.1:8080
  # api_key: replace-with-a-long-random-value
  max_body_bytes: 16777216

proxy:
  default_model: grok-4.5
  model_map:
    claude-special: grok-composer-2.5-fast
```

Exact entries in `proxy.model_map` take priority. Other `grok-*` names pass
through, while foreign Claude/OpenAI model names use `proxy.default_model`.

### Client authentication and network exposure

Set a shared key with the dedicated environment variable:

```bash
GROK_PROXY_API_KEY="$(openssl rand -hex 32)" grok-proxy serve
```

OpenAI clients send the value as `Authorization: Bearer ...`; Anthropic clients
may use either that header or `x-api-key`. The proxy refuses to bind to a
non-loopback address without a key unless `--allow-insecure` is explicitly
provided.

```bash
GROK_PROXY_API_KEY=secret grok-proxy serve --listen 0.0.0.0:8080
```

## Development

```bash
go test ./...
go test -race ./...
go vet ./...
```
