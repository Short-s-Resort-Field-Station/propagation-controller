# Serving the Web UI at shortsfieldstation.org

The Piconium serves everything itself — one binary, one database, both
surfaces. Remote access is just getting HTTPS traffic to it.

## Recommended layout

- `shortsfieldstation.org` — Austin's main site (separate side project)
- **`unit01.shortsfieldstation.org` — this appliance** (leaves the apex free,
  and Unit 02 / the greenhouse / the water trailer each get their own
  subdomain later)

## Recommended transport: Cloudflare Tunnel

No port forwarding, no public IP exposure, automatic TLS. The Piconium makes
an outbound connection to Cloudflare; visitors reach it through that.

On the Piconium (once, after OS install):

```sh
# 1. Install cloudflared (Debian)
curl -fsSL https://pkg.cloudflare.com/cloudflare-main.gpg | sudo tee /usr/share/keyrings/cloudflare-main.gpg >/dev/null
echo "deb [signed-by=/usr/share/keyrings/cloudflare-main.gpg] https://pkg.cloudflare.com/cloudflared any main" | sudo tee /etc/apt/sources.list.d/cloudflared.list
sudo apt update && sudo apt install cloudflared

# 2. Authenticate against the Cloudflare account that holds the domain
cloudflared tunnel login

# 3. Create the tunnel and route the subdomain to it
cloudflared tunnel create propagation-unit-01
cloudflared tunnel route dns propagation-unit-01 unit01.shortsfieldstation.org

# 4. /etc/cloudflared/config.yml
#    tunnel: propagation-unit-01
#    credentials-file: /root/.cloudflared/<tunnel-id>.json
#    ingress:
#      - hostname: unit01.shortsfieldstation.org
#        service: http://localhost:80
#      - service: http_status:404

# 5. Run as a service
sudo cloudflared service install
```

Prerequisite: the domain's DNS must be managed by Cloudflare (free plan is
fine — point the registrar's nameservers at Cloudflare once).

Alternative (no Cloudflare): port-forward 443 → Piconium and run Caddy as a
reverse proxy (`caddy reverse-proxy --from unit01.shortsfieldstation.org
--to localhost:80`) — Caddy handles Let's Encrypt automatically. Requires a
stable public IP or dynamic DNS.

## What visitors get (auth model)

| Who | Sees | Can do |
| --- | --- | --- |
| Anyone with the URL | The full live dashboard — shelves, charts, events, reservoir | Nothing. Every control endpoint requires sign-in. |
| Viewer account | Same | Nothing (same as public; exists for future private data) |
| Grower (operator) | Same | Water shelves, lights/fans, pump, e-stop, diagnostics |
| Steward (admin) | Same | Everything: settings, schedules, zone config, safety resets, operator accounts |
| The rack touchscreen | — | Full control, no sign-in (loopback = physical access) |

Security notes:

- Sessions are HttpOnly cookies (30-day), tokens stored hashed; passwords
  bcrypt-hashed. Sign-outs, disables, and password changes kill sessions.
- **First-boot setup** (creating the steward account) only works from the
  unit's own network — a freshly imaged unit whose URL is already public
  cannot be claimed remotely.
- The rack-console trust is loopback *without proxy headers*: cloudflared and
  Caddy always add `X-Forwarded-For`, so tunnel traffic is never mistaken for
  the touchscreen.
- The emergency stop requires a Grower sign-in (a public e-stop button would
  be a denial-of-service invitation).

## 2026-11-10 — chosen layout: `shortsfieldstation.org/propagate` (unlisted)

Austin wants the dashboard on the main domain at an unlisted path rather than
a subdomain. Pieces, all in `deploy/`:

1. **App served under `/propagate/`** — web bundle built with Vite
   `base: "/propagate/"`, API at `/propagate/api/v1`, and the Go router mounts
   the whole app under `PublicPrefix` (`internal/api/router.go`) as well as the
   bare root; `/` and `/ui/touch` redirect to the prefixed pages. Kiosk opens
   `http://localhost/propagate/ui/touch`.
2. **Tunnel** (`deploy/cloudflared/config.yml`) — `unit01.shortsfieldstation.org`
   → `http://localhost:80` on the Piconium. That hostname exists only as the
   Worker's origin; nothing links to it.
3. **Worker** (`deploy/cloudflare/propagate-worker/`) — route
   `shortsfieldstation.org/propagate*` on the main zone proxies to the tunnel,
   streams SSE, forwards cookies, sets `X-Robots-Tag: noindex`. Worker routes
   take precedence over the Cloudflare Pages site for that path only.

One-time setup (needs Austin's Cloudflare login, browser-based):

```sh
# on the Piconium (as root)
curl -fsSL -o /root/cloudflared.deb https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64.deb
dpkg -i /root/cloudflared.deb
cloudflared tunnel login                      # opens a browser URL — authorize shortsfieldstation.org
cloudflared tunnel create propagation-unit-01
cloudflared tunnel route dns propagation-unit-01 unit01.shortsfieldstation.org
mkdir -p /etc/cloudflared && cp /root/.cloudflared/<tunnel-id>.json /etc/cloudflared/propagation-unit-01.json
cp /opt/propagation-controller/deploy/cloudflared/config.yml /etc/cloudflared/config.yml
cloudflared service install && systemctl enable --now cloudflared

# on the Mac, in deploy/cloudflare/propagate-worker
npx wrangler login                            # browser
npx wrangler deploy
```

Verify: `curl -sI https://shortsfieldstation.org/propagate/ | grep -i -E "x-robots|HTTP/"` → 200 + noindex; the rest of the site untouched.
