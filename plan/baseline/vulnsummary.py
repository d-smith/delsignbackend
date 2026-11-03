"""Summarize govulncheck -format json output into a markdown table.

Usage: python3 -I vulnsummary.py govulncheck.json > summary.md
"""
import json
import sys

LEVELS = {3: "called", 2: "imported", 1: "required"}

text = open(sys.argv[1]).read()
dec = json.JSONDecoder()
pos = 0
osv = {}
best = {}  # id -> (level, module, fixed)
while pos < len(text):
    while pos < len(text) and text[pos].isspace():
        pos += 1
    if pos >= len(text):
        break
    obj, pos = dec.raw_decode(text, pos)
    if "osv" in obj:
        osv[obj["osv"]["id"]] = obj["osv"].get("summary", "")
    if "finding" in obj:
        f = obj["finding"]
        frame = f["trace"][0]
        level = 3 if frame.get("function") else 2 if frame.get("package") else 1
        cur = best.get(f["osv"])
        if cur is None or level > cur[0]:
            best[f["osv"]] = (level, frame["module"], f.get("fixed_version", ""))

counts = {lvl: 0 for lvl in LEVELS}
for lvl, _, _ in best.values():
    counts[lvl] += 1

print("| Reachability | Count |")
print("|---|---|")
for lvl in (3, 2, 1):
    print(f"| {LEVELS[lvl]} | {counts[lvl]} |")
print()
print("| Reachability | Module | Fixed in | ID | Summary |")
print("|---|---|---|---|---|")
rows = sorted(best.items(), key=lambda kv: (-kv[1][0], kv[1][1], kv[0]))
for vid, (lvl, mod, fixed) in rows:
    summary = osv.get(vid, "").replace("|", "\\|")
    print(f"| {LEVELS[lvl]} | {mod} | {fixed or 'none'} | {vid} | {summary} |")
