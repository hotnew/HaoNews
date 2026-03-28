#!/usr/bin/env python3
"""Minimal Hao.News latency and error-rate benchmark.

Defaults cover the main browser and feed surfaces:
  /, /topics, /topics/futures, /topics/futures/rss, /api/feed

Usage examples:
  python3 scripts/bench_hao_news.py
  python3 scripts/bench_hao_news.py --base-url http://127.0.0.1:51818 --concurrency 20 --requests-per-path 100
  python3 scripts/bench_hao_news.py --json > bench.json
"""

from __future__ import annotations

import argparse
import concurrent.futures as futures
import json
import math
import random
import statistics
import sys
import time
from collections import Counter, defaultdict
from dataclasses import dataclass
from typing import Dict, List, Sequence
from urllib import error, request


DEFAULT_PATHS = [
    "/",
    "/topics",
    "/topics/futures",
    "/topics/futures/rss",
    "/api/feed",
]


@dataclass(slots=True)
class Sample:
    path: str
    ok: bool
    status: int
    latency_ms: float
    error: str = ""


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--base-url",
        default="http://127.0.0.1:51818",
        help="Base URL for the benchmark targets.",
    )
    parser.add_argument(
        "--paths",
        nargs="*",
        default=DEFAULT_PATHS,
        help="Paths to test. Defaults to the main browser/API/RSS paths.",
    )
    parser.add_argument(
        "--concurrency",
        type=int,
        default=20,
        help="Number of concurrent workers.",
    )
    parser.add_argument(
        "--requests-per-path",
        type=int,
        default=50,
        help="Measured requests per path.",
    )
    parser.add_argument(
        "--warmup-per-path",
        type=int,
        default=0,
        help="Unmeasured warmup requests per path.",
    )
    parser.add_argument(
        "--timeout",
        type=float,
        default=10.0,
        help="Per-request timeout in seconds.",
    )
    parser.add_argument(
        "--seed",
        type=int,
        default=7,
        help="Shuffle seed for task ordering.",
    )
    parser.add_argument(
        "--json",
        action="store_true",
        help="Emit JSON instead of a text table.",
    )
    return parser.parse_args()


def normalize_base_url(value: str) -> str:
    value = value.strip()
    if not value:
        return "http://127.0.0.1:51818"
    return value.rstrip("/")


def build_url(base_url: str, path: str) -> str:
    path = path.strip()
    if not path.startswith("/"):
        path = "/" + path
    return base_url + path


def fetch_once(base_url: str, path: str, timeout: float) -> Sample:
    target = build_url(base_url, path)
    req = request.Request(
        target,
        method="GET",
        headers={
            "User-Agent": "hao.news-bench/1.0",
            "Accept": "text/html,application/rss+xml,application/json;q=0.9,*/*;q=0.8",
        },
    )
    start = time.perf_counter()
    try:
        with request.urlopen(req, timeout=timeout) as resp:
            resp.read()
            latency_ms = (time.perf_counter() - start) * 1000.0
            return Sample(path=path, ok=200 <= resp.status < 400, status=resp.status, latency_ms=latency_ms)
    except error.HTTPError as exc:
        try:
            exc.read()
        except Exception:
            pass
        latency_ms = (time.perf_counter() - start) * 1000.0
        return Sample(path=path, ok=False, status=getattr(exc, "code", 0) or 0, latency_ms=latency_ms, error=str(exc))
    except Exception as exc:
        latency_ms = (time.perf_counter() - start) * 1000.0
        return Sample(path=path, ok=False, status=0, latency_ms=latency_ms, error=str(exc))


def run_warmup(base_url: str, paths: Sequence[str], warmup_per_path: int, timeout: float) -> None:
    if warmup_per_path <= 0:
        return
    for _ in range(warmup_per_path):
        for path in paths:
            fetch_once(base_url, path, timeout)


def run_benchmark(base_url: str, paths: Sequence[str], concurrency: int, requests_per_path: int, timeout: float, seed: int) -> List[Sample]:
    tasks: List[str] = []
    for path in paths:
        tasks.extend([path] * requests_per_path)
    rng = random.Random(seed)
    rng.shuffle(tasks)
    results: List[Sample] = []
    with futures.ThreadPoolExecutor(max_workers=max(1, concurrency)) as executor:
        future_map = [executor.submit(fetch_once, base_url, path, timeout) for path in tasks]
        for future in futures.as_completed(future_map):
            results.append(future.result())
    return results


def nearest_rank_percentile(values: Sequence[float], percentile: float) -> float:
    if not values:
        return 0.0
    ordered = sorted(values)
    if percentile <= 0:
        return ordered[0]
    if percentile >= 1:
        return ordered[-1]
    rank = max(1, math.ceil(percentile * len(ordered)))
    return ordered[rank - 1]


def summarize(samples: Sequence[Sample]) -> Dict[str, object]:
    by_path: Dict[str, List[Sample]] = defaultdict(list)
    for sample in samples:
        by_path[sample.path].append(sample)

    def summarize_group(items: Sequence[Sample]) -> Dict[str, object]:
        success_latencies = [sample.latency_ms for sample in items if sample.ok]
        all_latencies = [sample.latency_ms for sample in items]
        status_counts = Counter(sample.status for sample in items)
        ok_count = sum(1 for sample in items if sample.ok)
        err_count = len(items) - ok_count
        return {
            "requests": len(items),
            "ok": ok_count,
            "errors": err_count,
            "error_rate": (err_count / len(items)) if items else 0.0,
            "status_counts": dict(sorted(status_counts.items())),
            "latency_ms": {
                "p50": nearest_rank_percentile(success_latencies, 0.50),
                "p95": nearest_rank_percentile(success_latencies, 0.95),
                "p99": nearest_rank_percentile(success_latencies, 0.99),
                "avg": statistics.fmean(success_latencies) if success_latencies else 0.0,
                "min": min(success_latencies) if success_latencies else 0.0,
                "max": max(success_latencies) if success_latencies else 0.0,
            },
            "all_latency_ms": {
                "p50": nearest_rank_percentile(all_latencies, 0.50),
                "p95": nearest_rank_percentile(all_latencies, 0.95),
                "p99": nearest_rank_percentile(all_latencies, 0.99),
            },
        }

    overall = summarize_group(samples)
    overall["by_path"] = {path: summarize_group(items) for path, items in sorted(by_path.items())}
    return overall


def fmt_ms(value: float) -> str:
    return f"{value:7.1f}"


def print_report(summary: Dict[str, object], elapsed_s: float, base_url: str, paths: Sequence[str], concurrency: int, requests_per_path: int, warmup_per_path: int) -> None:
    total_requests = sum(section["requests"] for section in summary["by_path"].values()) if summary.get("by_path") else 0
    overall = summary
    print(f"Base URL: {base_url}")
    print(f"Paths: {', '.join(paths)}")
    print(f"Concurrency: {concurrency} | requests/path: {requests_per_path} | warmup/path: {warmup_per_path}")
    print(f"Elapsed: {elapsed_s:.2f}s | Throughput: {total_requests / elapsed_s:.1f} req/s")
    print()
    header = f"{'PATH':30} {'REQ':>6} {'OK':>6} {'ERR%':>7} {'P50(ms)':>9} {'P95(ms)':>9} {'P99(ms)':>9} {'AVG(ms)':>9}"
    print(header)
    print("-" * len(header))
    for path, section in overall["by_path"].items():
        print(
            f"{path:30} "
            f"{section['requests']:6d} {section['ok']:6d} "
            f"{section['error_rate'] * 100:6.2f}% "
            f"{fmt_ms(section['latency_ms']['p50'])} "
            f"{fmt_ms(section['latency_ms']['p95'])} "
            f"{fmt_ms(section['latency_ms']['p99'])} "
            f"{fmt_ms(section['latency_ms']['avg'])}"
        )
    print("-" * len(header))
    print(
        f"{'OVERALL':30} "
        f"{overall['requests']:6d} {overall['ok']:6d} "
        f"{overall['error_rate'] * 100:6.2f}% "
        f"{fmt_ms(overall['latency_ms']['p50'])} "
        f"{fmt_ms(overall['latency_ms']['p95'])} "
        f"{fmt_ms(overall['latency_ms']['p99'])} "
        f"{fmt_ms(overall['latency_ms']['avg'])}"
    )
    print()
    print("Status counts by path:")
    for path, section in overall["by_path"].items():
        print(f"  {path}: {section['status_counts']}")


def main() -> int:
    args = parse_args()
    base_url = normalize_base_url(args.base_url)
    paths = [p.strip() for p in args.paths if p.strip()]
    if not paths:
        paths = list(DEFAULT_PATHS)
    run_warmup(base_url, paths, max(0, args.warmup_per_path), args.timeout)
    started = time.perf_counter()
    samples = run_benchmark(base_url, paths, max(1, args.concurrency), max(1, args.requests_per_path), args.timeout, args.seed)
    elapsed_s = time.perf_counter() - started
    summary = summarize(samples)
    if args.json:
        payload = {
            "base_url": base_url,
            "paths": paths,
            "concurrency": args.concurrency,
            "requests_per_path": args.requests_per_path,
            "warmup_per_path": args.warmup_per_path,
            "elapsed_seconds": elapsed_s,
            "throughput_rps": (len(samples) / elapsed_s) if elapsed_s > 0 else 0.0,
            "summary": summary,
        }
        json.dump(payload, sys.stdout, indent=2, sort_keys=True)
        sys.stdout.write("\n")
        return 0
    print_report(summary, elapsed_s, base_url, paths, args.concurrency, args.requests_per_path, args.warmup_per_path)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
