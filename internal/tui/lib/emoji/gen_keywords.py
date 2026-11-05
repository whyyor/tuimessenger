#!/usr/bin/env python3
"""Regenerate keywords.json: extra search words per emoji ("awkward" for 😬).

Merges two MIT-licensed datasets:
  emojilib    https://github.com/muan/emojilib
  emojibase   https://github.com/milesj/emojibase
Words the shortcode or title already covers are dropped. Run from this dir:
  python3 gen_keywords.py
"""
import json
import re
import urllib.request

EMOJILIB = "https://raw.githubusercontent.com/muan/emojilib/main/dist/emoji-en-US.json"
EMOJIBASE = "https://cdn.jsdelivr.net/npm/emojibase-data@latest/en/data.json"


def fetch(url):
    with urllib.request.urlopen(url) as r:
        return json.load(r)


def norm(e):
    return e.replace("\ufe0f", "")


def clean(word):
    word = word.lower().replace("_", " ").replace("\u2019", "'").strip()
    return word if re.fullmatch(r"[a-z0-9][a-z0-9 '&-]+", word) else None


data = json.load(open("data.json"))["e"]
covered = {}
for e in data:
    names = {e["n"], *e["s"]}
    words = {clean(n) for n in names} | {clean(e["t"])}
    covered[norm(e["u"])] = {w for w in words if w}

extra = {k: set() for k in covered}
lib = fetch(EMOJILIB)
for e, words in lib.items():
    for w in words[1:]:  # the first entry is just the name
        if norm(e) in extra and (w := clean(w)):
            extra[norm(e)].add(w)
for item in fetch(EMOJIBASE):
    for w in item.get("tags") or []:
        if norm(item["emoji"]) in extra and (w := clean(w)):
            extra[norm(item["emoji"])].add(w)

# Chat slang neither dataset has; extend freely.
SLANG = {
    "😬": ["cringe", "yikes", "oops"],
    "🫠": ["cringe", "dying inside"],
    "😖": ["cringe"],
    "💀": ["dying", "dead lol", "im dead"],
    "😭": ["dying", "crying laughing"],
    "😂": ["lol", "lmao", "haha"],
    "🤣": ["lol", "lmao", "rofl"],
    "🙄": ["bruh", "eyeroll"],
    "😑": ["bruh", "meh"],
    "🤦": ["facepalm", "bruh", "smh"],
    "🫡": ["ok boss", "yes sir"],
    "🥹": ["aww", "cute"],
    "🙃": ["fine", "ok sure"],
    "😮‍💨": ["phew", "sigh", "relief"],
    "🤝": ["deal", "agreed"],
    "👀": ["looking", "sus", "tea"],
    "💯": ["facts", "true", "exactly"],
    "🫶": ["love", "thanks"],
}
for e, words in SLANG.items():
    if norm(e) in extra:
        extra[norm(e)].update(words)

out = {}
for e, words in extra.items():
    names = covered[e]
    keep = sorted(w for w in words if not any(w in n.replace(" ", "") or w in n for n in names))
    if keep:
        out[e] = keep
with open("keywords.json", "w") as f:
    json.dump(out, f, ensure_ascii=False, separators=(",", ":"), sort_keys=True)
print(f"{len(out)} emoji, {sum(map(len, out.values()))} words")
