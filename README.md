# YUPS (Your Unified Proxy Service)

YUPS is a lightweight, privacy-focused HTTP routing gateway that directs web
users to alternative, open-source, and privacy-preserving frontends for
mainstream platforms (such as Twitter/X, Reddit, YouTube, Instagram, Medium,
Imgur, Goodreads, ENS, and I2P), with general archive fallbacks.

The goal of YUPS is to make the accessible web resilient, private, and
censorship-resistant. It serves as a unified entry point: prepend
`https://yups.io/?url=` to any target URL to instantly load it through a privacy
proxy.

![Logo](./img/icons/logo-128.png)

## Key Features

- **Instant Temporary Redirection (HTTP 307)**: Normal browsers are immediately
  forwarded to a randomly chosen proxy mirror for the detected platform.
  Redirections strictly use HTTP 307 (never 301 or 308) to ensure client
  browsers never cache the redirect target permanently, allowing seamless
  failover when public mirrors change or go down.
- **Smart Card & Social Bot Previews**: When shared on social networks
  (Telegram, Twitter/X, Discord, WhatsApp, LinkedIn, Slack, Mastodon, Bluesky,
  etc.), YUPS serves an HTML preview populated with Open Graph and Twitter Card
  metadata extracted directly from the destination page, accompanied by a
  curated list of all available proxy alternatives.
- **SSRF Protection & First-Seen Tracking**: The metadata scraper validates
  destination IPs against private subnets, loopback interfaces, and cloud
  metadata endpoints (RFC 1918, RFC 3927, RFC 6598) with a 3-second timeout.
  Extracted smart card metadata is cached in-memory and tracks when a URL was
  first seen.
- **Community-Driven Proxy Registry**: All proxy definitions and routing
  patterns are stored in `data/proxies.csv`, making it straightforward for
  anyone to contribute new mirrors or platforms via pull requests.
- **Minimalist Interface**: Clean, responsive Google-style landing page styled
  with warm Capybara tones (`#EAB796` and `#9E5A2E`), featuring an *I'm feeling
  lucky* action and *Get proxy links* explorer.
- **Zero Heavy Runtime Dependencies**: Single static Go binary, embeddable
  assets, minimal footprint.

## Quick Start with Docker

The fastest way to deploy YUPS is using the provided Docker configuration.

### Using Docker Run

```bash
docker run -d \
  --name yups \
  -p 8080:8080 \
  -e BASE_URL="https://yups.io" \
  -e YUPS_ACCESS_LOG="true" \
  yups:latest
```

### Using Docker Compose

```bash
docker compose up -d
```

Check service health:

```bash
curl -i http://localhost:8080/healthz
```

## Running with Make

YUPS includes a thin `Makefile` to streamline local development tasks.

```bash
# Build the static executable
make build

# Run YUPS locally
make run

# Run test suite with readable, colored output
make test

# Verify proxy availability and update proxies.csv
make check-proxies

# Build Docker image
make docker-build
```

## Direct Go Commands

If you prefer invoking Go directly without Make, the underlying commands are:

```bash
# Compile binary
go build -trimpath -ldflags="-s -w" -o yups ./cmd/yups

# Run locally
go run ./cmd/yups

# Run tests
go test -v ./...

# Run battery test suite
go test -v ./test/...

# Verify proxy availability and update proxies.csv
go run ./cmd/yups -check -update-csv
```

## Configuration

YUPS can be configured via environment variables or command-line flags:

| Variable          | Flag              | Default           | Description                                        |
| ----------------- | ----------------- | ----------------- | -------------------------------------------------- |
| `PORT`            | `-port`           | `8080`            | Port for the HTTP server to listen on              |
| `HOST`            | `-host`           | `0.0.0.0`         | Host IP interface to bind                          |
| `BASE_URL`        | `-base-url`       | `https://yups.io` | Public URL prefix of the deployment                |
| `PROXIES_FILE`    | `-proxies`        | (embedded)        | Path to custom proxies CSV file                    |
| `YUPS_ACCESS_LOG` | `-access-log`     | `true`            | Toggle structured access logging (easy to disable) |
| `CACHE_TTL`       | `-cache-ttl`      | `24h`             | Time-to-live for scraped smart card metadata       |
| `CHECK_INTERVAL`  | `-check-interval` | `5m`              | Interval between background proxy health checks    |
| `CHECK_TIMEOUT`   | `-check-timeout`  | `20s`             | HTTP timeout for proxy health checks               |
| `CHECK_WORKERS`   | `-check-workers`  | `15`              | Concurrent worker pool size for proxy checks       |
| `DISABLE_CHECK`   | `-disable-check`  | `false`           | Disable background proxy health checking           |
|                   | `-check`          | `false`           | Run one-shot proxy check, print report, and exit   |
|                   | `-update-csv`     | `false`           | In `-check` mode, save updated states to CSV file  |

To disable access logging, pass `-access-log=false` or set
`YUPS_ACCESS_LOG=false`.

## Contributing Proxies

Anyone can add new proxies or supported platforms by editing `data/proxies.csv`
and opening a pull request.

The CSV structure:

```csv
service,tech,type,proxy_url,patterns,description,active,auto-check
```

- `service`: Identifier for the platform (e.g., `twitter`, `reddit`, `youtube`,
  `general`).
- `tech`: Name of the software frontend (e.g., `xcancel`, `nitter`, `redlib`,
  `invidious`).
- `type`: Transformation rule:
  - `domain_replace`: Swaps original host with the proxy mirror host while
    keeping path and query.
  - `query_param`: Appends the full target URL to the proxy base URL query
    string.
  - `prepend`: Prepends the proxy base URL directly to the target URL (without
    scheme, e.g. for I2P web gateways).
  - `append_ext`: Appends gateway domain (e.g., `.limo` for `.eth` ENS domains).
- `proxy_url`: Base URL of the mirror.
- `patterns`: Comma-separated domain patterns/wildcards to match (e.g.,
  `x.com,twitter.com`). Any proxy URL defined in the dataset automatically acts
  as a valid pattern for its service without needing to be listed here. When a
  user submits a URL belonging to any proxy (active or inactive), YUPS
  automatically reverts it to the original domain (using the first pattern as
  the canonical host) and redirects to an alternative active mirror, or renders
  the results page if no distinct mirror exists.
- `description`: Short description of the instance.
- `active`: Boolean flag (`true` or `false`) indicating whether the proxy is
  currently online and verified. Inactive proxies are ignored by the routing
  service.
- `auto-check`: Boolean flag (`true` or `false`) indicating whether the proxy
  should be probed automatically by background/CLI health checks. Proxies with
  `auto-check=false` are skipped during automated checks, preserving their
  manual `active` status and highlighted in yellow in terminal logs.

We invite everyone to self-host their own instances, replicate the dataset, and
contribute improvements.

## Contact

- Maintainer: ([mail@javilopezg.com](mailto:mail@javilopezg.com))

## License

BSD 3-Clause License. See [LICENSE](LICENSE) for details.

## FAQ

### Why the little dog?

It's a capybara—an angry capybara. She is angry about the current state of the
Internet. She doesn't like bans, blocks, or restrictions at all.

Also, capybaras fight their enemies by smashing them with their butt, and this
capybara has plenty of enemies to smash, like big corporations and ICANN.

### Why is the site brown?

Capybaras are brown, so the site is brown. It also represents the
enshittification of the Internet.

You can clone it and host an alternative instance. I'll gladly help you with
that. The more clones out there, the more resilient the Internet becomes.

### Sometimes a page doesn't load

Public proxies go down periodically or introduce aggressive bot protections.
YUPS allows you to probe all proxies and automatically update their status in
`proxies.csv` before starting the service:

```bash
make check-proxies
```

Any proxy returning HTTP 4xx, 5xx, or network timeouts will be marked
`active=false`, and YUPS will automatically skip it during routing.

### Why do my friend and I get different results for the same URL?

YUPS picks a random valid proxy for each request, so two identical requests can
be routed to different mirrors.

### What services are supported?

Currently, we support proxies for Twitter (X), YouTube, Instagram, Reddit,
Medium, Imgur, and Goodreads. Other URLs are treated as "general" (news media,
blogs, etc.) and routed to web archive proxies.

You can see (and help improve) the full list in
[proxies.csv](./data/proxies.csv).

### What platforms are not supported yet?

Many services require a user account, a paid subscription, or specialized
software/configuration to view content—such as Facebook, TikTok, LinkedIn,
Pinterest, Quora, Tor, Lokinet, GNS, and IPNS. We want to find workarounds for
them. Have [ideas?](mailto:mail@javilopezg.com).

### Can I contribute to YUPS?

Sure! Extra hands are always useful.

### Can I clone or self-host it?

Of course! The more YUPS-like sites out there, the better and more resilient the
Internet will be.

### What if I want my own proxy to access a specific service?

There are several open-source frontends actively maintained, such as
[Nitter](https://github.com/zedeus/nitter) or
[Kittygram](https://codeberg.org/irelephant/kittygram). Check the
[proxies.csv](./data/proxies.csv) file for more examples, or drop me a line.

### What if I want to give you money?

I really appreciate it, but no, thanks! I can't legally or morally accept it.
The people doing the heavy lifting are the ones building and hosting the
proxies, and many of them accept donations directly.

### What if I want to help, but I prefer working solo?

I know that feeling. Big projects need communities, but independent work can
make huge improvements too. There are
[a lot of things](https://code.javilopezg.com/javilopezg/quijote/src/branch/main/REQUIREMENTS.md#14-nice-to-have)
you can do to improve the decentralized web. Let me know if you'd like
suggestions on areas to explore.
