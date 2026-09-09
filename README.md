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

## Contact

- Maintainer: ([mail@javilopezg.com](mailto:mail@javilopezg.com))

## License

BSD 3-Clause License. See [LICENSE](LICENSE) for details.

## FAQ

### Why the little dog?

It's a capybara, an angry capybara. She is angry about the current Internet. She
doesn't like bans, blocks or prohibitions at all.

Also, capybaras fight their enemies smashing them with their ass, and this
capybara has a lot of enemies to smash like corporations or the ICANN.

### Why is the site brown?

Capybaras are brown so the site is brown. It also represents the enshitification
of Internet.

You can clone it and publish an alternative site. I'll help you whit that. The
more clones, more resilient will be Internet.

### Sometimes the page is not loading

Yes, sometimes the proxies are down. I should check what proxies are alive to
exclude them but this is a functionality to future versions.

### With the same url my friend and me has different results

Yes, it will use a random valid proxy to redirect any request so two requests
that are exactly the same can get different redirections.

### What services are managed?

Currently, we have detected proxies for Twitter (X), Youtube, Instagram, Reddit,
Medium, Imgur, and Goodreas. Other urls are managed as "general" (news media,
probably) and redirected to archive proxies.

You can see (and improve) the whole list in the
[proxies.csv](./data/proxies.csv).

### What platforms are not managed yet?

There are a lot of services that require an user, a subscription, or specialized
software/configuration to access the information like Facebook, TikTok,
Linkedin, Pinterest, Quora, Tor, Lokinet, GNS, IPNS... We have to do something
with them. [Ideas?](mailto:mail@javilopezg.com).

### Can I contribute to yups?

Sure. More hands are always usefull :)

### Can I clone/deploy it?

Of course, more yups-like sites would improve Internet.

### I want my own proxy to access an specific server

There are some softwares that we know are working like
[Nitter](https://github.com/zedeus/nitter),
[Kittygram](https://codeberg.org/irelephant/kittygram). You can check the
[proxies.csv](./data/proxies.csv) file or write me a line.

### I want to give you money

I really apreciate it, but no, thanks. I can't legally or morally accept it. The
people doing the real work are the ones building and maintaining the proxies and
some of them are accepting donations.

### I want to help but I don't want to collaborate, I'm a free solo

I know that feeling. Big things need a lot of people, but small changes can make
huge improvements. There are
[a lot of things](https://code.javilopezg.com/javilopezg/quijote/src/branch/main/REQUIREMENTS.md#14-nice-to-have)
that you can do to improve Internet. Let me know If I can help you to choose a
line of work.
