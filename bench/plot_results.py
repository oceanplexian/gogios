#!/usr/bin/env python3
"""Generate comparison plots from scale benchmark CSV files."""
import csv
import sys
import os

import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt
import matplotlib.ticker as ticker

def read_csv(path):
    with open(path) as f:
        reader = csv.DictReader(f)
        rows = list(reader)
    return rows

def parse(rows, key):
    return [float(r[key]) for r in rows]

def main():
    v2_path = "bench/scale_results_v2.csv"
    v3_path = "bench/scale_results_v3.csv"
    out_dir = "bench/graphs"
    os.makedirs(out_dir, exist_ok=True)

    v2 = read_csv(v2_path)
    v3 = read_csv(v3_path)

    svcs_v2 = parse(v2, "services")
    svcs_v3 = parse(v3, "services")

    fig, axes = plt.subplots(2, 3, figsize=(18, 10))
    fig.suptitle("Gogios Scale Benchmark: v2 (before) vs v3 (after optimizations)", fontsize=14, fontweight='bold')

    # 1. Check throughput
    ax = axes[0][0]
    ax.plot(svcs_v2, parse(v2, "checks_per_sec"), 'o-', label='v2', color='#d62728', linewidth=2)
    ax.plot(svcs_v3, parse(v3, "checks_per_sec"), 's-', label='v3', color='#2ca02c', linewidth=2)
    ax.set_xscale('log')
    ax.set_title('Check Throughput')
    ax.set_xlabel('Services')
    ax.set_ylabel('Checks/sec')
    ax.legend()
    ax.grid(True, alpha=0.3)
    ax.xaxis.set_major_formatter(ticker.FuncFormatter(lambda x, _: f'{x:,.0f}'))

    # 2. Memory RSS
    ax = axes[0][1]
    mem_v2 = [float(r["mem_rss_kb"])/1024 for r in v2]
    mem_v3 = [float(r["mem_rss_kb"])/1024 for r in v3]
    ax.plot(svcs_v2, mem_v2, 'o-', label='v2', color='#d62728', linewidth=2)
    ax.plot(svcs_v3, mem_v3, 's-', label='v3', color='#2ca02c', linewidth=2)
    ax.set_xscale('log')
    ax.set_title('Memory Usage (RSS)')
    ax.set_xlabel('Services')
    ax.set_ylabel('MB')
    ax.legend()
    ax.grid(True, alpha=0.3)
    ax.xaxis.set_major_formatter(ticker.FuncFormatter(lambda x, _: f'{x:,.0f}'))

    # 3. Startup time
    ax = axes[0][2]
    ax.plot(svcs_v2, parse(v2, "startup_ms"), 'o-', label='v2', color='#d62728', linewidth=2)
    ax.plot(svcs_v3, parse(v3, "startup_ms"), 's-', label='v3', color='#2ca02c', linewidth=2)
    ax.set_xscale('log')
    ax.set_title('Startup Time')
    ax.set_xlabel('Services')
    ax.set_ylabel('ms')
    ax.legend()
    ax.grid(True, alpha=0.3)
    ax.xaxis.set_major_formatter(ticker.FuncFormatter(lambda x, _: f'{x:,.0f}'))

    # 4. LQL Services RPS
    ax = axes[1][0]
    ax.plot(svcs_v2, parse(v2, "lql_services_rps"), 'o-', label='v2', color='#d62728', linewidth=2)
    ax.plot(svcs_v3, parse(v3, "lql_services_rps"), 's-', label='v3', color='#2ca02c', linewidth=2)
    ax.set_xscale('log')
    ax.set_yscale('log')
    ax.set_title('LQL Services Query RPS')
    ax.set_xlabel('Services')
    ax.set_ylabel('Requests/sec')
    ax.legend()
    ax.grid(True, alpha=0.3)
    ax.xaxis.set_major_formatter(ticker.FuncFormatter(lambda x, _: f'{x:,.0f}'))

    # 5. LQL Hosts RPS
    ax = axes[1][1]
    ax.plot(svcs_v2, parse(v2, "lql_hosts_rps"), 'o-', label='v2', color='#d62728', linewidth=2)
    ax.plot(svcs_v3, parse(v3, "lql_hosts_rps"), 's-', label='v3', color='#2ca02c', linewidth=2)
    ax.set_xscale('log')
    ax.set_yscale('log')
    ax.set_title('LQL Hosts Query RPS')
    ax.set_xlabel('Services')
    ax.set_ylabel('Requests/sec')
    ax.legend()
    ax.grid(True, alpha=0.3)
    ax.xaxis.set_major_formatter(ticker.FuncFormatter(lambda x, _: f'{x:,.0f}'))

    # 6. LQL Stats RPS
    ax = axes[1][2]
    ax.plot(svcs_v2, parse(v2, "lql_stats_rps"), 'o-', label='v2', color='#d62728', linewidth=2)
    ax.plot(svcs_v3, parse(v3, "lql_stats_rps"), 's-', label='v3', color='#2ca02c', linewidth=2)
    ax.set_xscale('log')
    ax.set_yscale('log')
    ax.set_title('LQL Stats Query RPS')
    ax.set_xlabel('Services')
    ax.set_ylabel('Requests/sec')
    ax.legend()
    ax.grid(True, alpha=0.3)
    ax.xaxis.set_major_formatter(ticker.FuncFormatter(lambda x, _: f'{x:,.0f}'))

    plt.tight_layout()
    plt.savefig(os.path.join(out_dir, "scale_comparison.png"), dpi=150)
    print(f"Saved {out_dir}/scale_comparison.png")

    # Second figure: latency
    fig2, axes2 = plt.subplots(1, 2, figsize=(14, 5))
    fig2.suptitle("LQL P95 Latency: v2 vs v3", fontsize=14, fontweight='bold')

    ax = axes2[0]
    ax.plot(svcs_v2, parse(v2, "lql_hosts_p95_ms"), 'o-', label='v2', color='#d62728', linewidth=2)
    ax.plot(svcs_v3, parse(v3, "lql_hosts_p95_ms"), 's-', label='v3', color='#2ca02c', linewidth=2)
    ax.set_xscale('log')
    ax.set_yscale('log')
    ax.set_title('Hosts Query P95 Latency')
    ax.set_xlabel('Services')
    ax.set_ylabel('ms')
    ax.legend()
    ax.grid(True, alpha=0.3)
    ax.xaxis.set_major_formatter(ticker.FuncFormatter(lambda x, _: f'{x:,.0f}'))

    ax = axes2[1]
    ax.plot(svcs_v2, parse(v2, "lql_services_p95_ms"), 'o-', label='v2', color='#d62728', linewidth=2)
    ax.plot(svcs_v3, parse(v3, "lql_services_p95_ms"), 's-', label='v3', color='#2ca02c', linewidth=2)
    ax.set_xscale('log')
    ax.set_yscale('log')
    ax.set_title('Services Query P95 Latency')
    ax.set_xlabel('Services')
    ax.set_ylabel('ms')
    ax.legend()
    ax.grid(True, alpha=0.3)
    ax.xaxis.set_major_formatter(ticker.FuncFormatter(lambda x, _: f'{x:,.0f}'))

    plt.tight_layout()
    plt.savefig(os.path.join(out_dir, "latency_comparison.png"), dpi=150)
    print(f"Saved {out_dir}/latency_comparison.png")

if __name__ == "__main__":
    main()
