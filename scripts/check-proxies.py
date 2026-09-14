#!/usr/bin/env python3
"""
Checks proxy availability by requesting the homepage of each proxy in the CSV.
Marks proxies as active (true) if they return 2xx or 3xx HTTP status, or inactive
(false) on 4xx, 5xx, timeouts, and connection errors.
"""

import argparse
import csv
import os
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


def get_homepage_url(proxy_url: str) -> str:
    """Extracts the base scheme and host root URL for testing proxy availability."""
    parsed = urllib.parse.urlparse(proxy_url.strip())
    scheme = parsed.scheme if parsed.scheme else "https"
    netloc = parsed.netloc
    return f"{scheme}://{netloc}/"


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


def check_proxy_health(target_url: str, timeout: float, ssl_ctx: ssl.SSLContext) -> tuple[int | str, bool]:
    """
    Sends an HTTP request with a browser User-Agent header.
    Returns (status_or_error, is_active).
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
            return is_healthy_response(resp.status, resp.headers)
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
        default=5.0,
        help="HTTP request timeout in seconds (default: 5.0)",
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

    def probe_row(index: int, row: list[str]):
        proxy_url = row[3].strip() if len(row) > 3 else ""
        test_url = get_homepage_url(proxy_url)
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

        proxy_url = r[3].strip() if len(r) > 3 else ""
        test_url = get_homepage_url(proxy_url)

        if not should_check:
            # Skip automatic probing; retain current active state
            results[idx] = (test_url, "manual check", curr_active, True)
        else:
            rows_to_probe.append((idx, r, test_url))

    with ThreadPoolExecutor(max_workers=args.concurrency) as executor:
        futures = {
            executor.submit(probe_row, idx, r): idx
            for idx, r, _ in rows_to_probe
        }
        for future in as_completed(futures):
            idx, test_url, status, is_active = future.result()
            results[idx] = (test_url, status, is_active, False)

    # Process in original order for clean terminal output and CSV writing
    active_count = 0
    inactive_count = 0
    skipped_count = 0
    by_service: dict[str, dict[str, int]] = {}

    updated_rows = []
    for idx, row in enumerate(rows):
        test_url, status, is_active, is_skipped = results[idx]
        service = row[0].strip() if row else "unknown"
        tech = row[1].strip() if len(row) > 1 else ""

        by_service.setdefault(service, {"active": 0, "inactive": 0, "skipped": 0})

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
    print(
        f"Total: {len(rows)} | "
        f"{GREEN}Active: {active_count}{RESET} | "
        f"{RED}Inactive: {inactive_count}{RESET} | "
        f"{YELLOW}Manual/Skipped: {skipped_count}{RESET}"
    )
    print(f"{BOLD}Breakdown by service:{RESET}")
    for svc, counts in sorted(by_service.items()):
        act = counts["active"]
        inact = counts["inactive"]
        skp = counts["skipped"]
        color = GREEN if act > 0 else RED
        skp_str = f", {YELLOW}{skp} manual{RESET}" if skp > 0 else ""
        print(f"  - {svc:<14}: {color}{act} active{RESET}, {inact} inactive{skp_str}")

    print(f"{BOLD}══════════════════════════════════════════════════════════{RESET}")
    print(f"Updated {csv_path} successfully.")


if __name__ == "__main__":
    main()

