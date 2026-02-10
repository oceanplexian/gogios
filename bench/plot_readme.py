#!/usr/bin/env python3
"""Generate transparent PNG graphs for README from scale benchmark CSVs."""
import csv
import os

import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt
import matplotlib.ticker as ticker

def read_csv(path):
    with open(path) as f:
        return list(csv.DictReader(f))

def parse(rows, key):
    return [float(r[key]) for r in rows]

def style_ax(ax):
    """Style axes with semi-transparent white fill so text is readable on both
    GitHub light and dark themes."""
    ax.set_facecolor((1, 1, 1, 0.85))
    for spine in ax.spines.values():
        spine.set_color('#586069')
        spine.set_linewidth(0.8)
    ax.tick_params(colors='#24292e', labelsize=10)
    ax.xaxis.label.set_color('#24292e')
    ax.yaxis.label.set_color('#24292e')
    ax.title.set_color('#24292e')
    ax.grid(True, alpha=0.25, color='#586069', linewidth=0.5)

def save(fig, path):
    fig.patch.set_facecolor((1, 1, 1, 0.85))
    fig.savefig(path, dpi=150, bbox_inches='tight', pad_inches=0.2)
    plt.close(fig)
    print(f"  Saved {path}")

def main():
    v3 = read_csv("bench/scale_results_v3.csv")
    v4 = read_csv("bench/scale_results_v4.csv")
    out = "assets/readme"
    os.makedirs(out, exist_ok=True)

    svcs_v3 = parse(v3, "services")
    svcs_v4 = parse(v4, "services")

    colors = {
        'v3': '#d62728',   # red
        'v4': '#2ca02c',   # green
    }

    # --- 1. Check throughput (the big win) ---
    fig, ax = plt.subplots(figsize=(8, 4.5))
    style_ax(ax)
    ax.plot(svcs_v3, parse(v3, "checks_per_sec"), 'o-', label='Before (fork per check)',
            color=colors['v3'], linewidth=2, markersize=6)
    ax.plot(svcs_v4, parse(v4, "checks_per_sec"), 's-', label='After (fork server)',
            color=colors['v4'], linewidth=2, markersize=6)
    ax.axhline(y=1667, color='#0366d6', linestyle='--', linewidth=1, alpha=0.6, label='Target: 100k svcs in 60s')
    ax.set_xscale('log')
    ax.set_title('Check Throughput', fontsize=13, fontweight='bold')
    ax.set_xlabel('Number of Services')
    ax.set_ylabel('Checks/sec')
    ax.legend(loc='best', framealpha=0.95, fontsize=9)
    ax.xaxis.set_major_formatter(ticker.FuncFormatter(lambda x, _: f'{x:,.0f}'))
    save(fig, os.path.join(out, "check_throughput.png"))

    # --- 2. Memory usage ---
    fig, ax = plt.subplots(figsize=(8, 4.5))
    style_ax(ax)
    mem_v3 = [float(r["mem_rss_kb"])/1024 for r in v3]
    mem_v4 = [float(r["mem_rss_kb"])/1024 for r in v4]
    ax.plot(svcs_v3, mem_v3, 'o-', label='Before', color=colors['v3'], linewidth=2, markersize=6)
    ax.plot(svcs_v4, mem_v4, 's-', label='After', color=colors['v4'], linewidth=2, markersize=6)
    ax.set_xscale('log')
    ax.set_title('Memory Usage (RSS)', fontsize=13, fontweight='bold')
    ax.set_xlabel('Number of Services')
    ax.set_ylabel('MB')
    ax.legend(loc='best', framealpha=0.95, fontsize=9)
    ax.xaxis.set_major_formatter(ticker.FuncFormatter(lambda x, _: f'{x:,.0f}'))
    save(fig, os.path.join(out, "memory_usage.png"))

    # --- 3. Startup time ---
    fig, ax = plt.subplots(figsize=(8, 4.5))
    style_ax(ax)
    ax.plot(svcs_v3, parse(v3, "startup_ms"), 'o-', label='Before', color=colors['v3'], linewidth=2, markersize=6)
    ax.plot(svcs_v4, parse(v4, "startup_ms"), 's-', label='After', color=colors['v4'], linewidth=2, markersize=6)
    ax.set_xscale('log')
    ax.set_title('Startup Time', fontsize=13, fontweight='bold')
    ax.set_xlabel('Number of Services')
    ax.set_ylabel('ms')
    ax.legend(loc='best', framealpha=0.95, fontsize=9)
    ax.xaxis.set_major_formatter(ticker.FuncFormatter(lambda x, _: f'{x:,.0f}'))
    save(fig, os.path.join(out, "startup_time.png"))

    # --- 4. LQL throughput (3-panel) ---
    fig, axes = plt.subplots(1, 3, figsize=(18, 5))
    fig.suptitle('Livestatus Query Throughput', fontsize=14, fontweight='bold', color='#24292e')

    for ax, (key, title) in zip(axes, [
        ("lql_hosts_rps", "Hosts Query"),
        ("lql_services_rps", "Services Query"),
        ("lql_stats_rps", "Stats Query"),
    ]):
        style_ax(ax)
        ax.plot(svcs_v3, parse(v3, key), 'o-', label='Before', color=colors['v3'], linewidth=2, markersize=5)
        ax.plot(svcs_v4, parse(v4, key), 's-', label='After', color=colors['v4'], linewidth=2, markersize=5)
        ax.set_xscale('log')
        ax.set_yscale('log')
        ax.set_title(title, fontsize=12, fontweight='bold')
        ax.set_xlabel('Number of Services')
        ax.set_ylabel('Requests/sec')
        ax.legend(loc='best', framealpha=0.95, fontsize=9)
        ax.xaxis.set_major_formatter(ticker.FuncFormatter(lambda x, _: f'{x:,.0f}'))

    fig.tight_layout(rect=[0, 0, 1, 0.93])
    save(fig, os.path.join(out, "lql_throughput.png"))

    # --- 5. LQL latency (2-panel) ---
    fig, axes = plt.subplots(1, 2, figsize=(14, 5))
    fig.suptitle('Livestatus P95 Latency', fontsize=14, fontweight='bold', color='#24292e')

    for ax, (key, title) in zip(axes, [
        ("lql_hosts_p95_ms", "Hosts Query"),
        ("lql_services_p95_ms", "Services Query"),
    ]):
        style_ax(ax)
        ax.plot(svcs_v3, parse(v3, key), 'o-', label='Before', color=colors['v3'], linewidth=2, markersize=6)
        ax.plot(svcs_v4, parse(v4, key), 's-', label='After', color=colors['v4'], linewidth=2, markersize=6)
        ax.set_xscale('log')
        ax.set_yscale('log')
        ax.set_title(title, fontsize=12, fontweight='bold')
        ax.set_xlabel('Number of Services')
        ax.set_ylabel('P95 Latency (ms)')
        ax.legend(loc='best', framealpha=0.95, fontsize=9)
        ax.xaxis.set_major_formatter(ticker.FuncFormatter(lambda x, _: f'{x:,.0f}'))

    fig.tight_layout(rect=[0, 0, 1, 0.93])
    save(fig, os.path.join(out, "lql_latency.png"))

    print("\nDone! All graphs in assets/readme/")

if __name__ == "__main__":
    main()
