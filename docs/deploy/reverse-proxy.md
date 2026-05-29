# Running AegisFlow behind nginx or Caddy

AegisFlow exposes **three separate ports** out of the box:

| Port | Purpose | Notes |
|------|---------|-------|
| `8080` | Gateway (OpenAI-compatible API, MCP `/sse`, `/v1/ws`) | This is what coding agents talk to. |
| `8081` | Admin API (`/admin/v1/...`, `/health`, approvals, evidence) | Operator-only. Lock down. |
| `8082` | Metrics / dashboard JSON | Prometheus scrape + UI |

A real deployment puts a reverse proxy in front of these for TLS termination, access logging, and per-endpoint rate limiting. This page gives working nginx and Caddy snippets.

> **Most common gotcha:** SSE (`/sse`) and WebSocket (`/v1/ws`) need **`proxy_buffering off`** in nginx (or the equivalent flush settings in Caddy), or long-lived streams hang and editor connections look "stuck".

---

## TLS

Both snippets below assume you already have a certificate and key on disk:

- nginx: `/etc/letsencrypt/live/your-host/fullchain.pem` + `privkey.pem`
- Caddy: handled automatically via the `tls` directive (Let's Encrypt by default)

---

## nginx

```nginx
# /etc/nginx/sites-available/aegisflow.conf

# Upstreams for the three AegisFlow ports.
upstream aegisflow_gateway { server 127.0.0.1:8080; keepalive 32; }
upstream aegisflow_admin   { server 127.0.0.1:8081; keepalive 16; }
upstream aegisflow_metrics { server 127.0.0.1:8082; keepalive 8;  }

# Map for WebSocket upgrade.
map $http_upgrade $connection_upgrade {
    default upgrade;
    ''      close;
}

server {
    listen 443 ssl http2;
    server_name your-host;

    ssl_certificate     /etc/letsencrypt/live/your-host/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/your-host/privkey.pem;

    # Sane defaults for an AI/agent control plane.
    client_max_body_size 25m;
    proxy_read_timeout   3600s;   # SSE streams can be long.
    proxy_send_timeout   3600s;
    proxy_connect_timeout 10s;

    # ---- MCP Server-Sent Events ----
    # CRITICAL: turn off buffering so chunks flush to the agent in real time.
    location /sse {
        proxy_pass http://aegisflow_gateway;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;

        proxy_buffering off;
        proxy_cache off;
        chunked_transfer_encoding on;
        gzip off;
    }

    # ---- WebSocket (admin live charts, eval hooks) ----
    location /v1/ws {
        proxy_pass http://aegisflow_gateway;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
        proxy_set_header Host $host;
        proxy_read_timeout 3600s;
    }

    # ---- Gateway (OpenAI-compatible API) ----
    location / {
        proxy_pass http://aegisflow_gateway;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;
    }
}

# Admin API on a separate host (recommended) or path prefix.
server {
    listen 443 ssl http2;
    server_name admin.your-host;

    ssl_certificate     /etc/letsencrypt/live/your-host/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/your-host/privkey.pem;

    # Restrict to ops IPs in production.
    # allow 10.0.0.0/8;
    # deny  all;

    location / {
        proxy_pass http://aegisflow_admin;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;
    }

    # Prometheus scrape can read metrics through the admin host.
    location /metrics {
        proxy_pass http://aegisflow_metrics/metrics;
    }
}
```

Enable and reload:

```bash
sudo ln -s /etc/nginx/sites-available/aegisflow.conf /etc/nginx/sites-enabled/
sudo nginx -t && sudo systemctl reload nginx
```

### Verify

```bash
curl -fsS https://your-host/health                 # gateway health
curl -fsS https://admin.your-host/admin/v1/providers -H "X-API-Key: …"
curl -N    https://your-host/sse                   # should stream, not hang
```

If `/sse` hangs, you have buffering on somewhere. Re-check `proxy_buffering off`, `gzip off`, and any upstream Cloudflare / AWS ALB settings.

---

## Caddy

Caddy auto-manages TLS via Let's Encrypt and handles WebSocket upgrade with no extra config. SSE still needs flush hints.

```caddyfile
# /etc/caddy/Caddyfile

your-host {
    encode gzip

    # ---- MCP Server-Sent Events ----
    @sse path /sse
    handle @sse {
        reverse_proxy 127.0.0.1:8080 {
            flush_interval -1          # immediate flush, equivalent to no buffering
            transport http {
                read_timeout    1h
                write_timeout   1h
                response_header_timeout 1h
            }
        }
    }

    # ---- WebSocket ----
    @ws path /v1/ws
    handle @ws {
        reverse_proxy 127.0.0.1:8080
    }

    # ---- Gateway ----
    handle {
        reverse_proxy 127.0.0.1:8080
    }
}

admin.your-host {
    # Optional IP allowlist.
    # @ops remote_ip 10.0.0.0/8
    # handle @ops { reverse_proxy 127.0.0.1:8081 }
    # respond 403

    reverse_proxy 127.0.0.1:8081

    handle_path /metrics* {
        reverse_proxy 127.0.0.1:8082
    }
}
```

Reload:

```bash
sudo caddy reload --config /etc/caddy/Caddyfile
```

### Verify

```bash
curl -fsS https://your-host/health
curl -N    https://your-host/sse
curl -fsS https://admin.your-host/admin/v1/providers -H "X-API-Key: …"
```

---

## End-to-end check with Claude Code

After the proxy is up, point your editor at `https://your-host/sse` (the proxied MCP endpoint) and confirm an action round-trip from agent to AegisFlow goes through with no perceptible lag. If the agent times out, the SSE buffering is almost certainly the cause.

---

## Common mistakes

- **`proxy_buffering off` missing** → SSE hangs, agent reports "no response".
- **`gzip on` over SSE** → some clients buffer until end of stream.
- **Single-host setup** for gateway + admin → admin API accidentally exposed publicly. Use a separate vhost or strict allowlist.
- **Short `proxy_read_timeout`** (default 60s) → long approvals or evidence verifies drop. Bump to ≥ 1h.
- **TLS terminated at proxy but app expects HTTPS** → set `X-Forwarded-Proto https` so AegisFlow generates correct callback URLs (GitHub App, dashboards).
- **No rate limit at the edge** → put a `limit_req` zone in front of `/v1/chat/completions` for unauthenticated abuse protection.

---

## Where to file feedback

- Broken / outdated snippet → open a [bug report](https://github.com/saivedant169/AegisFlow/issues/new?template=bug_report.yml).
- Different reverse proxy (Traefik, Envoy, HAProxy) you would like covered → open a [feature request](https://github.com/saivedant169/AegisFlow/issues/new?template=feature_request.yml).
