#!/usr/bin/env python3
"""Generate transparent PNG graphs for README from NRDP benchmark CSV."""
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
    data = read_csv("bench/nrdp_results.csv")
    out = "assets/readme"
    os.makedirs(out, exist_ok=True)

    svcs = parse(data, "unique_services")
    color = '#0366d6'  # GitHub blue

    # --- 1. NRDP Ingestion Throughput ---
    fig, ax = plt.subplots(figsize=(8, 4.5))
    style_ax(ax)
    ax.plot(svcs, parse(data, "results_per_sec"), 's-',
            color=color, linewidth=2, markersize=7, label='NRDP ingestion rate')
    ax.set_xscale('log')
    ax.set_title('NRDP Passive Check Ingestion Throughput', fontsize=13, fontweight='bold')
    ax.set_xlabel('Unique Services (dynamic registration)')
    ax.set_ylabel('Results/sec')
    ax.legend(loc='best', framealpha=0.95, fontsize=9)
    ax.xaxis.set_major_formatter(ticker.FuncFormatter(lambda x, _: f'{x:,.0f}'))
    save(fig, os.path.join(out, "nrdp_throughput.png"))

    # --- 2. NRDP P95 Batch Latency ---
    fig, ax = plt.subplots(figsize=(8, 4.5))
    style_ax(ax)
    ax.plot(svcs, parse(data, "p95_batch_ms"), 's-',
            color='#d62728', linewidth=2, markersize=7, label='P95 batch latency')
    ax.set_xscale('log')
    ax.set_title('NRDP P95 Batch Latency', fontsize=13, fontweight='bold')
    ax.set_xlabel('Unique Services (dynamic registration)')
    ax.set_ylabel('P95 Latency (ms)')
    ax.legend(loc='best', framealpha=0.95, fontsize=9)
    ax.xaxis.set_major_formatter(ticker.FuncFormatter(lambda x, _: f'{x:,.0f}'))
    save(fig, os.path.join(out, "nrdp_latency.png"))

    # --- 3. Memory after NRDP ingestion ---
    fig, ax = plt.subplots(figsize=(8, 4.5))
    style_ax(ax)
    mem_mb = [float(r["mem_rss_kb"])/1024 for r in data]
    ax.plot(svcs, mem_mb, 's-',
            color='#2ca02c', linewidth=2, markersize=7, label='RSS after ingestion')
    ax.set_xscale('log')
    ax.set_title('Memory After NRDP Dynamic Registration', fontsize=13, fontweight='bold')
    ax.set_xlabel('Unique Services (dynamic registration)')
    ax.set_ylabel('MB')
    ax.legend(loc='best', framealpha=0.95, fontsize=9)
    ax.xaxis.set_major_formatter(ticker.FuncFormatter(lambda x, _: f'{x:,.0f}'))
    save(fig, os.path.join(out, "nrdp_memory.png"))

    print("\nDone! NRDP graphs in assets/readme/")

if __name__ == "__main__":
    main()
