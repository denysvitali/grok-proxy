# grok-subscription-client

A standalone client that uses Grok subscription session authentication without
executing the installed `grok` binary.

## Usage

```bash
python3 grok_sub.py import-grok
python3 grok_sub.py models
python3 grok_sub.py chat "Hello"
```

Log in directly through xAI OAuth with browser + PKCE:

```bash
python3 grok_sub.py login
```

For a headless machine, use device authorization:

```bash
python3 grok_sub.py login --device
```

Credentials are stored in
`~/.config/grok-subscription-client/auth.json` with mode `0600`. The client can
also import an existing session from `~/.grok/auth.json`; token values are never
printed. Expiring OAuth tokens are refreshed automatically when a refresh token
is available.

The default model backend is the Responses API. The older Chat Completions
shape is available with `--backend chat_completions`.

## Security properties

- The Grok executable is never invoked.
- OAuth uses a public client with PKCE or the device-code grant.
- OAuth callback state is checked.
- Credential files are written atomically with mode `0600`.
- Tokens are excluded from normal output and errors.

