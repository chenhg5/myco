#!/usr/bin/env python3
"""Download counts of myco's GitHub releases.

GitHub only keeps a running total per release file. This script groups the
totals by what was downloaded, and can append a dated snapshot to a CSV so
the daily workflow (.github/workflows/downloads.yml) builds up a history
that shows the trend.

    python3 scripts/downloads.py                      # totals now, by kind and by version
    python3 scripts/downloads.py --snapshot FILE      # append today's counts to FILE (CSV)
    python3 scripts/downloads.py --trend FILE [DAYS]  # downloads per day from a snapshot CSV

Counts include updates and our own machines (CI, servers, test installs):
the .dmg is the only file that is mostly first installs.

Uses GITHUB_TOKEN when set (higher rate limit); works without it.
"""
import csv
import datetime as dt
import json
import os
import sys
import urllib.request
from collections import defaultdict

REPO = os.environ.get("MYCO_REPO", "chenhg5/myco")

# What a release file is, in the order the report lists them.
KINDS = [
    ("mac_install", "Mac App 新安装（.dmg）"),
    ("mac_update", "Mac App 更新包（App 内更新）"),
    ("windows_app", "Windows App（下载和 App 内更新）"),
    ("cli", "命令行（安装、升级、runner 自动升级）"),
]


def kind(name):
    if name == "SHA256SUMS":
        return None
    if name.endswith(".dmg"):
        return "mac_install"
    if name.startswith("myco-app-darwin"):
        return "mac_update"
    if name.startswith("myco-app-windows"):
        return "windows_app"
    if name.startswith("myco-"):
        return "cli"
    return None


def platform(name):
    """darwin-arm64, linux-amd64, windows-amd64 ... for command-line files."""
    p = name.removeprefix("myco-").removesuffix(".exe")
    return p


def fetch():
    """Every release with its files: [(tag, published_at, [(name, count)])]."""
    out, page = [], 1
    headers = {"Accept": "application/vnd.github+json", "User-Agent": "myco-downloads"}
    if os.environ.get("GITHUB_TOKEN"):
        headers["Authorization"] = "Bearer " + os.environ["GITHUB_TOKEN"]
    while True:
        url = f"https://api.github.com/repos/{REPO}/releases?per_page=100&page={page}"
        req = urllib.request.Request(url, headers=headers)
        with urllib.request.urlopen(req, timeout=30) as r:
            batch = json.load(r)
        if not batch:
            return out
        for rel in batch:
            files = [(a["name"], a["download_count"]) for a in rel.get("assets", [])]
            out.append((rel["tag_name"], rel.get("published_at") or "", files))
        page += 1


def report(releases):
    by_kind, by_cli = defaultdict(int), defaultdict(int)
    for _, _, files in releases:
        for name, n in files:
            k = kind(name)
            if k:
                by_kind[k] += n
            if k == "cli":
                by_cli[platform(name)] += n
    print(f"{REPO}: {len(releases)} releases\n")
    print("累计下载")
    for k, label in KINDS:
        print(f"  {by_kind[k]:>7}  {label}")
    print("\n命令行按平台")
    for p, n in sorted(by_cli.items(), key=lambda x: -x[1]):
        print(f"  {n:>7}  {p}")
    print("\n最近 10 个版本（Mac 新安装 / Mac 更新 / Windows / 命令行）")
    for tag, published, files in releases[:10]:
        row = defaultdict(int)
        for name, n in files:
            k = kind(name)
            if k:
                row[k] += n
        print(f"  {tag:<10} {published[:10]}  " + "  ".join(f"{row[k]:>5}" for k, _ in KINDS))


def snapshot(releases, path):
    today = dt.datetime.now(dt.timezone.utc).date().isoformat()
    new = not os.path.exists(path) or os.path.getsize(path) == 0
    rows = []
    if not new:
        with open(path, newline="") as f:
            rows = [r for r in csv.DictReader(f) if r["date"] != today]  # re-running a day replaces it
    for tag, _, files in releases:
        for name, n in files:
            if kind(name):
                rows.append({"date": today, "tag": tag, "file": name, "downloads": n})
    with open(path, "w", newline="") as f:
        w = csv.DictWriter(f, fieldnames=["date", "tag", "file", "downloads"])
        w.writeheader()
        w.writerows(rows)
    print(f"{today}: {sum(1 for r in rows if r['date'] == today)} files → {path}")


def trend(path, days):
    totals = defaultdict(lambda: defaultdict(int))  # date -> kind -> running total
    with open(path, newline="") as f:
        for r in csv.DictReader(f):
            k = kind(r["file"])
            if k:
                totals[r["date"]][k] += int(r["downloads"])
    dates = sorted(totals)
    if len(dates) < 2:
        print("need at least two daily snapshots for a trend")
        return
    print("每天新增下载（Mac 新安装 / Mac 更新 / Windows / 命令行）")
    for prev, cur in list(zip(dates, dates[1:]))[-days:]:
        print(f"  {cur}  " + "  ".join(f"{totals[cur][k] - totals[prev][k]:>5}" for k, _ in KINDS))


def main(argv):
    if len(argv) >= 2 and argv[0] == "--trend":
        trend(argv[1], int(argv[2]) if len(argv) > 2 else 30)
        return
    releases = fetch()
    if len(argv) == 2 and argv[0] == "--snapshot":
        snapshot(releases, argv[1])
        return
    if argv:
        sys.exit(__doc__)
    report(releases)


if __name__ == "__main__":
    main(sys.argv[1:])
