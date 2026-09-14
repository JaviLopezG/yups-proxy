#!/usr/bin/env python3
"""
Checks proxy availability by testing platform-specific Wikipedia profile/page
targets (and alternative DNS targets) for each proxy in the CSV.
Marks proxies as active (true) if they return 2xx or 3xx HTTP status, or inactive
(false) on 4xx, 5xx, timeouts, and connection errors. If all proxies in a category
are down or unresponsive, a fallback treats them all as active to prevent bot-blocking
updates from deactivating entire services.
"""

import argparse
import csv
import os
import re
import ssl
import sys
import urllib.error
import urllib.parse
import urllib.request
from concurrent.futures import ThreadPoolExecutor, as_completed

# ANSI color codes
GREEN = "\033[0;32m"
RED = "\033[0;31m"
YELLOW = "\033[0;33m"
CYAN = "\033[0;36m"
BOLD = "\033[1m"
RESET = "\033[0m"

USER_AGENT = (
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "
    "(KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
)

# Target URLs per service category. Wikipedia profiles are ubiquitous across platforms.
# For alternative DNS providers without Wikipedia, designated active services are used.
SERVICE_TARGETS = {
    "youtube": "https://www.youtube.com/wikipedia",
    "instagram": "https://www.instagram.com/wikipedia",
    "twitter": "https://x.com/wikipedia",
    "goodreads": "https://www.goodreads.com/author/show/16189742.Wikipedia",
    "imgur": "https://imgur.com/user/Wikipedia",
    "reddit": "https://www.reddit.com/user/wikipedia/",
    "medium": "https://ev.medium.com/welcome-to-medium-9e53ca408c48",
    "general": "https://wikipedia.org",
    "i2p": "https://stormycloud.i2p/",
    "ens": "https://zerolend.eth/",
}
DEFAULT_TARGET_URL = "https://wikipedia.org"


def build_test_url(service: str, proxy_type: str, proxy_url: str) -> str:
    """
    Transforms the proxy entry and service-specific Wikipedia/DNS target
    into the actual test destination URL, matching YUPS transformation rules.
    """
    target_url = SERVICE_TARGETS.get(service.strip().lower(), DEFAULT_TARGET_URL)
    proxy_url = proxy_url.strip()
    proxy_type = proxy_type.strip().lower()
    parsed_target = urllib.parse.urlparse(target_url)

    if proxy_type == "query_param":
        base = proxy_url
        if base.endswith("="):
            return base + urllib.parse.quote(target_url, safe="")
        if "?" in base:
            return base + "&url=" + urllib.parse.quote(target_url, safe="")
        return base + "?url=" + urllib.parse.quote(target_url, safe="")

    if proxy_type == "prepend":
        base = proxy_url
        if not base.endswith("/") and not target_url.startswith("/"):
            base += "/"
        return base + target_url

    if proxy_type == "append_ext":
        proxy_parsed = urllib.parse.urlparse(proxy_url)
        proxy_host = proxy_parsed.hostname or ""
        target_host = parsed_target.hostname or ""

        if proxy_host.startswith("eth.") and target_host.endswith(".eth"):
            ext = proxy_host[4:]
            new_host = f"{target_host}.{ext}"
        else:
            new_host = f"{target_host}.{proxy_host}"

        scheme = proxy_parsed.scheme or "https"
        return urllib.parse.urlunsplit((
            scheme,
            new_host,
            parsed_target.path,
            parsed_target.query,
            parsed_target.fragment,
        ))

    if proxy_type == "domain_replace":
        proxy_parsed = urllib.parse.urlparse(proxy_url)
        scheme = proxy_parsed.scheme or "https"
        host = proxy_parsed.netloc

        target_host = (parsed_target.hostname or "").lower()
        if target_host.startswith("www."):
            target_host = target_host[4:]

        path = parsed_target.path
        query = parsed_target.query

        if target_host == "youtu.be":
            video_id = path.lstrip("/")
            if video_id:
                path = "/watch"
                query = f"v={video_id}"
        else:
            proxy_path = proxy_parsed.path.rstrip("/")
            if proxy_path:
                path = proxy_path + "/" + path.lstrip("/")

        return urllib.parse.urlunsplit((
            scheme,
            host,
            path,
            query,
            parsed_target.fragment,
        ))

    return proxy_url


def is_healthy_response(status: int, headers) -> tuple[int | str, bool]:
    """
    Evaluates HTTP response status and headers:
    1. If status is 418 (I'm a teapot) -> True (anti-bot challenge like go-away).
    2. If cf-mitigated header is present -> True (Cloudflare challenge active).
    3. If respondent is Cloudflare (via server or cf-ray) -> True unless 5xx or 404.
    4. Otherwise -> True for 2xx and 3xx, False for 4xx/5xx.
    """
    if status == 418:
        return "418 (Teapot)", True

    if not headers:
        return status, (200 <= status < 400)

    cf_mitigated = headers.get("cf-mitigated")
    if cf_mitigated:
        detail = f"{status} (CF {cf_mitigated})" if status not in (200, 301, 302, 307, 308) else status
        return detail, True

    server_hdr = (headers.get("server") or "").lower()
    is_cloudflare = "cloudflare" in server_hdr or bool(headers.get("cf-ray"))
    if is_cloudflare:
        is_5xx = 500 <= status <= 599
        is_404 = status == 404
        if not is_5xx and not is_404:
            detail = f"{status} (Cloudflare)" if status not in (200, 301, 302, 307, 308) else status
            return detail, True
        detail = f"{status} (Cloudflare error)" if is_5xx else f"{status} (Not Found)"
        return detail, False

    return status, (200 <= status < 400)


ERROR_BODY_PATTERNS = (
    "error 4",
    "error 5",
    "<h1>error",
    "<h2>error",
    "<h3>error",
    "<h4>error",
    "<h5>error",
)


def find_error_in_body(body_lower: str) -> str | None:
    """
    Scans response body for known error strings even when HTTP status is 200.
    Case-insensitive search for:
      - 'error 4', 'error 5'
      - '<h1>error', '<h2>error', '<h3>error', '<h4>error', '<h5>error'
      (including spacing or attribute variations like <h1> Error)
    """
    for pattern in ERROR_BODY_PATTERNS:
        if pattern in body_lower:
            return pattern

    header_match = re.search(r"<h[1-5][^>]*>\s*error", body_lower)
    if header_match:
        return header_match.group(0).strip()

    return None


def check_proxy_health(target_url: str, timeout: float, ssl_ctx: ssl.SSLContext) -> tuple[int | str, bool]:
    """
    Sends an HTTP request with a browser User-Agent header.
    Returns (status_or_error, is_active).
    If status is 200 but page contains known error strings, marks as inactive.
    """
    req = urllib.request.Request(
        target_url,
        headers={
            "User-Agent": USER_AGENT,
            "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
            "Accept-Language": "en-US,en;q=0.9",
        },
    )

    try:
        with urllib.request.urlopen(req, timeout=timeout, context=ssl_ctx) as resp:
            status = resp.status
            if status == 200:
                body_bytes = resp.read(512 * 1024)
                body_lower = body_bytes.decode("utf-8", errors="replace").lower()
                matched_err = find_error_in_body(body_lower)
                if matched_err:
                    return f"200 (Error: {matched_err})", False
            return is_healthy_response(status, resp.headers)
    except urllib.error.HTTPError as e:
        return is_healthy_response(e.code, e.headers)
    except urllib.error.URLError as e:
        reason = getattr(e, "reason", None)
        err_msg = type(reason).__name__ if reason else type(e).__name__
        return (err_msg, False)
    except TimeoutError:
        return ("Timeout", False)
    except Exception as e:
        return (type(e).__name__, False)


def parse_bool(val: str, default: bool = True) -> bool:
    v = val.strip().lower()
    if v in ("true", "1", "yes", "y", "t", "active"):
        return True
    if v in ("false", "0", "no", "n", "f", "inactive"):
        return False
    return default


def main():
    parser = argparse.ArgumentParser(
        description="Verify proxy availability and update data/proxies.csv."
    )
    parser.add_argument(
        "--file",
        "-f",
        default="data/proxies.csv",
        help="Path to the proxies CSV file (default: data/proxies.csv)",
    )
    parser.add_argument(
        "--timeout",
        "-t",
        type=float,
        default=20.0,
        help="HTTP request timeout in seconds (default: 20.0)",
    )
    parser.add_argument(
        "--concurrency",
        "-c",
        type=int,
        default=15,
        help="Maximum concurrent health checks (default: 15)",
    )
    args = parser.parse_args()

    csv_path = args.file
    if not os.path.exists(csv_path):
        print(f"{RED}Error: CSV file not found at {csv_path}{RESET}", file=sys.stderr)
        sys.exit(1)

    with open(csv_path, "r", encoding="utf-8") as f:
        reader = csv.reader(f)
        try:
            header = next(reader)
        except StopIteration:
            print(f"{RED}Error: CSV file {csv_path} is empty{RESET}", file=sys.stderr)
            sys.exit(1)
        rows = [row for row in reader if row]

    # Find or append the active and auto-check columns
    active_col_idx = -1
    auto_check_col_idx = -1
    for idx, col in enumerate(header):
        normalized = col.strip().lower().replace("_", "-")
        if normalized == "active":
            active_col_idx = idx
        elif normalized in ("auto-check", "autocheck", "check"):
            auto_check_col_idx = idx

    if active_col_idx == -1:
        header.append("active")
        active_col_idx = len(header) - 1

    if auto_check_col_idx == -1:
        header.append("auto-check")
        auto_check_col_idx = len(header) - 1

    print(f"{BOLD}Checking proxy availability ({len(rows)} proxies)...{RESET}\n")

    ssl_ctx = ssl.create_default_context()
    ssl_ctx.check_hostname = False
    ssl_ctx.verify_mode = ssl.CERT_NONE

    def probe_row(index: int, row: list[str], test_url: str):
        status, is_active = check_proxy_health(test_url, args.timeout, ssl_ctx)
        return index, test_url, status, is_active

    results = {}
    rows_to_probe = []

    for idx, r in enumerate(rows):
        # Determine existing active state
        curr_active_val = r[active_col_idx].strip() if len(r) > active_col_idx else ""
        curr_active = parse_bool(curr_active_val, default=True)

        # Determine auto-check flag
        auto_check_val = r[auto_check_col_idx].strip() if len(r) > auto_check_col_idx else ""
        should_check = parse_bool(auto_check_val, default=True)

        service = r[0].strip() if len(r) > 0 else ""
        proxy_type = r[2].strip() if len(r) > 2 else ""
        proxy_url = r[3].strip() if len(r) > 3 else ""
        test_url = build_test_url(service, proxy_type, proxy_url)

        if not should_check:
            # Skip automatic probing; retain current active state
            results[idx] = (test_url, "manual check", curr_active, True, False)
        else:
            rows_to_probe.append((idx, r, test_url))

    with ThreadPoolExecutor(max_workers=args.concurrency) as executor:
        futures = {
            executor.submit(probe_row, idx, r, test_url): idx
            for idx, r, test_url in rows_to_probe
        }
        for future in as_completed(futures):
            idx, test_url, status, is_active = future.result()
            results[idx] = (test_url, status, is_active, False, False)

    # Evaluate category/service health: if all probed proxies in a service are down
    # (unresponsive/failing), assume possible software update or bot challenge and
    # mark them all active as fallback.
    service_to_indices: dict[str, list[int]] = {}
    for idx, row in enumerate(rows):
        service = row[0].strip().lower() if row else "unknown"
        service_to_indices.setdefault(service, []).append(idx)

    fallback_services: set[str] = set()
    for service, indices in service_to_indices.items():
        probed_indices = [i for i in indices if not results[i][3]]  # not is_skipped
        if probed_indices:
            healthy_probed = [i for i in probed_indices if results[i][2]]  # is_active
            if len(healthy_probed) == 0:
                fallback_services.add(service)
                for i in probed_indices:
                    test_url, status, _, is_skipped, _ = results[i]
                    results[i] = (test_url, status, True, is_skipped, True)

    # Process in original order for clean terminal output and CSV writing
    active_count = 0
    inactive_count = 0
    skipped_count = 0
    fallback_count = 0
    by_service: dict[str, dict[str, int]] = {}

    updated_rows = []
    for idx, row in enumerate(rows):
        test_url, status, is_active, is_skipped, is_fallback = results[idx]
        service = row[0].strip() if row else "unknown"
        tech = row[1].strip() if len(row) > 1 else ""

        by_service.setdefault(service, {"active": 0, "inactive": 0, "skipped": 0, "fallback": 0})

        if is_skipped:
            skipped_count += 1
            by_service[service]["skipped"] += 1
            if is_active:
                active_count += 1
                by_service[service]["active"] += 1
            else:
                inactive_count += 1
                by_service[service]["inactive"] += 1

            tag = f"[{YELLOW}SKIP{RESET}]"
            state_str = "active" if is_active else "inactive"
            print(f"  {tag} {YELLOW}{service:<12} {tech:<16} {test_url:<40} -> manual review (kept {state_str}){RESET}")
        elif is_fallback:
            fallback_count += 1
            active_count += 1
            by_service[service]["active"] += 1
            by_service[service]["fallback"] += 1
            tag = f"[{YELLOW}PASS*{RESET}]"
            print(f"  {tag} {service:<12} {tech:<16} {test_url:<40} -> {status} {YELLOW}(all category down -> kept active){RESET}")
        else:
            if is_active:
                active_count += 1
                by_service[service]["active"] += 1
                tag = f"[{GREEN}PASS{RESET}]"
            else:
                inactive_count += 1
                by_service[service]["inactive"] += 1
                tag = f"[{RED}FAIL{RESET}]"

            print(f"  {tag} {service:<12} {tech:<16} {test_url:<40} -> {status}")

        # Update row active value
        active_val = "true" if is_active else "false"
        while len(row) <= max(active_col_idx, auto_check_col_idx):
            row.append("")
        row[active_col_idx] = active_val

        # Preserve auto-check value
        if not row[auto_check_col_idx].strip():
            row[auto_check_col_idx] = "false" if is_skipped else "true"

        updated_rows.append(row)

    # Write updated CSV back
    with open(csv_path, "w", encoding="utf-8", newline="") as f:
        writer = csv.writer(f)
        writer.writerow(header)
        writer.writerows(updated_rows)

    print(f"\n{BOLD}══════════════════ Proxy Health Summary ══════════════════{RESET}")
    fb_summary = f" | {YELLOW}Category Fallback: {fallback_count}{RESET}" if fallback_count > 0 else ""
    print(
        f"Total: {len(rows)} | "
        f"{GREEN}Active: {active_count}{RESET} | "
        f"{RED}Inactive: {inactive_count}{RESET} | "
        f"{YELLOW}Manual/Skipped: {skipped_count}{RESET}"
        f"{fb_summary}"
    )
    if fallback_services:
        print(f"\n{YELLOW}{BOLD}Notice:{RESET} All probed proxies were down for category: {', '.join(sorted(fallback_services))}.")
        print("They were kept active in case an upstream bot challenge or software update expelled bots.\n")

    print(f"{BOLD}Breakdown by service:{RESET}")
    for svc, counts in sorted(by_service.items()):
        act = counts["active"]
        inact = counts["inactive"]
        skp = counts["skipped"]
        fb = counts.get("fallback", 0)
        color = GREEN if act > 0 else RED
        skp_str = f", {YELLOW}{skp} manual{RESET}" if skp > 0 else ""
        fb_str = f", {YELLOW}{fb} fallback{RESET}" if fb > 0 else ""
        print(f"  - {svc:<14}: {color}{act} active{RESET}, {inact} inactive{skp_str}{fb_str}")

    print(f"{BOLD}══════════════════════════════════════════════════════════{RESET}")
    print(f"Updated {csv_path} successfully.")


if __name__ == "__main__":
    main()

