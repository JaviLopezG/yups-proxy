# YUPS (Your Unified Proxy Service)

YUPS is a lightweight, privacy-focused HTTP routing gateway that directs web
users to alternative, open-source, and privacy-preserving frontends for
mainstream platforms (such as Twitter/X, Reddit, YouTube, Instagram, Medium,
Imgur, Goodreads, ENS, and I2P), with general archive fallbacks.

The goal of YUPS is to make the accessible web resilient, private, and
censorship-resistant. It serves as a unified entry point: append
`https://yups.io/?url=` to any target URL to instantly load it through a privacy
proxy.

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
```

## Configuration

YUPS can be configured via environment variables or command-line flags:

| Variable          | Flag          | Default           | Description                                        |
| ----------------- | ------------- | ----------------- | -------------------------------------------------- |
| `PORT`            | `-port`       | `8080`            | Port for the HTTP server to listen on              |
| `HOST`            | `-host`       | `0.0.0.0`         | Host IP interface to bind                          |
| `BASE_URL`        | `-base-url`   | `https://yups.io` | Public URL prefix of the deployment                |
| `PROXIES_FILE`    | `-proxies`    | (embedded)        | Path to custom proxies CSV file                    |
| `YUPS_ACCESS_LOG` | `-access-log` | `true`            | Toggle structured access logging (easy to disable) |
| `CACHE_TTL`       | `-cache-ttl`  | `24h`             | Time-to-live for scraped smart card metadata       |

To disable access logging, pass `-access-log=false` or set
`YUPS_ACCESS_LOG=false`.

## Contributing Proxies

Anyone can add new proxies or supported platforms by editing `data/proxies.csv`
and opening a pull request.

The CSV structure:

```csv
service,tech,type,proxy_url,patterns,description
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
  - `prepend`: Prepends the proxy base URL directly to the target URL.
  - `append_ext`: Appends gateway domain (e.g., `.limo` for `.eth` ENS domains).
- `proxy_url`: Base URL of the mirror.
- `patterns`: Comma-separated domain patterns/wildcards to match (e.g.,
  `x.com,twitter.com,xcancel.com`).
- `description`: Short description of the instance.

We invite everyone to self-host their own instances, replicate the dataset, and
contribute improvements.

## Repositories & Contact

- GitHub:
  [https://github.com/javilopezg/yups-proxy](https://github.com/javilopezg/yups-proxy)
- Maintainer: Javi López ([mail@javilopezg.com](mailto:mail@javilopezg.com))

## License

BSD 3-Clause License. See [LICENSE](LICENSE) for details.
