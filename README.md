# Cerebro

API proxy and multi-account manager for [Cerebras Cloud](https://cloud.cerebras.ai/). Pools multiple Cerebras API keys behind a single endpoint with automatic round-robin rotation and rate-limit-aware cooldowns.

## Features

- **Drop-in replacement** — OpenAI-compatible API surface (`/v1/chat/completions`, `/v1/completions`, `/v1/models`, etc.)
- **Key rotation** — Round-robin across multiple Cerebras API keys, automatically skipping rate-limited keys
- **Streaming support** — Full SSE streaming passthrough for chat completions
- **Multi-tenant** — Each tenant gets their own bearer token with isolated usage tracking
- **Rate limit awareness** — Parses Cerebras `x-ratelimit-*` headers to set intelligent cooldown durations
- **Stats endpoint** — `/stats` shows per-key and per-tenant usage in JSON
- **Docker-ready** — Multi-stage Dockerfile, deployable on Render.com in minutes

## Quick Start

### 1. Configure

Copy the example config and add your Cerebras API keys:

```bash
cp config.example.yaml config.yaml
```

Edit `config.yaml`:

```yaml
cerebras_keys:
  - "csk-your-first-key"
  - "csk-your-second-key"

tenants:
  - name: "personal"
    api_key: "your-secret-bearer-token"
```

### 2. Run

```bash
go build -o cerebro ./cmd/cerebro
./cerebro
```

Or with environment variables:

```bash
CEREBRAS_API_KEYS=csk-key1,csk-key2 CEREBRO_CONFIG=config.yaml ./cerebro
```

### 3. Use

Point your OpenAI-compatible client to Cerebro:

```python
import openai

client = openai.OpenAI(
    base_url="http://localhost:8080/v1",
    api_key="your-secret-bearer-token"  # Your Cerebro tenant token
)

response = client.chat.completions.create(
    model="gpt-oss-120b",
    messages=[{"role": "user", "content": "Hello!"}],
    stream=True
)

for chunk in response:
    if chunk.choices[0].delta.content:
        print(chunk.choices[0].delta.content, end="", flush=True)
```

### Running the Demo

A pre-built demonstration script `demo.py` is included to quickly test streaming completions and verify statistics tracking.

To run it:
1. Ensure Cerebro is running (e.g. at `http://localhost:8080`).
2. Run the script:
   ```bash
   python3 demo.py
   ```

The script will:
1. Auto-discover available models via the Cerebro proxy.
2. Fire a streaming chat completion request.
3. Stream the output token-by-token.
4. Fetch and print the updated usage statistics showing tenant and key routing performance.

## Endpoints

| Path | Auth | Description |
|---|---|---|
| `/v1/*` | Bearer token | Reverse proxy to Cerebras API |
| `/health` | None | Health check for load balancers |
| `/stats` | Bearer token | Per-key and per-tenant usage stats |
| `/` | None | Service info |

## Configuration

### Config File (`config.yaml`)

```yaml
cerebras_keys:
  - "csk-key1"
  - "csk-key2"

server:
  port: 8080
  upstream: "https://api.cerebras.ai"

default_cooldown_seconds: 60

tenants:
  - name: "personal"
    api_key: "cbr-my-token"
```

### Environment Variables

| Variable | Description | Default |
|---|---|---|
| `CEREBRAS_API_KEYS` | Comma-separated Cerebras API keys | _(from config)_ |
| `CEREBRO_PORT` | Server port | `8080` |
| `CEREBRO_CONFIG` | Config file path | `config.yaml` |
| `CEREBRO_UPSTREAM` | Upstream URL | `https://api.cerebras.ai` |

Environment variables override config file values.

## Deploy to Render

1. Push to GitHub
2. Create a new **Web Service** on Render
3. Select **Docker** runtime
4. Set environment variables:
   - `CEREBRAS_API_KEYS` = your comma-separated keys (as a secret)
5. Set health check path to `/health`

A `render.yaml` is included for [Blueprint](https://render.com/docs/blueprint-spec) deployment.

## How It Works

```
Client → Cerebro (auth) → Round-Robin Key Selection → Cerebras API
                                    ↓ (on 429)
                              Skip key, try next
                                    ↓ (all exhausted)
                              Return 429 + Retry-After
```

Keys that receive a 429 are put on cooldown based on the `x-ratelimit-reset-tokens-minute` header from Cerebras (or a configurable default). Available keys continue serving requests normally.

## License

MIT
