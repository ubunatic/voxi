#!/usr/bin/env python3
"""WER / keyterm-recall scorer mirroring scripts/speech_context_bench/main.go's
wordErrorRate/containsTerm logic, for issue 066's cross-engine canary (Cohere
Transcribe / Nemotron 3.5 streaming) where the baseline harness's Go code
cannot be reused directly (non-whisper.cpp/non-voxtype engines).

Usage: python3 score.py <corpus.tsv> <results.jsonl>
results.jsonl: one JSON object per line with "id" (or "file" ending in
<id>.wav) and "transcript".
"""
import json
import re
import sys


def words(text: str):
    text = text.lower()
    raw = re.findall(r"[a-z0-9._+#@-]+", text)
    out = []
    for w in raw:
        w = w.strip("._-")
        if w:
            out.append(w)
    return out


def wer(expected: str, actual: str) -> float:
    want, got = words(expected), words(actual)
    if not want:
        return 0.0 if not got else 1.0
    prev = list(range(len(got) + 1))
    for i, w in enumerate(want):
        cur = [i + 1] + [0] * len(got)
        for j, g in enumerate(got):
            cost = 0 if w == g else 1
            cur[j + 1] = min(cur[j] + 1, prev[j + 1] + 1, prev[j] + cost)
        prev = cur
    return prev[len(got)] / len(want)


def contains_term(transcript: str, term: str) -> bool:
    t = " " + " ".join(words(transcript)) + " "
    return (" " + " ".join(words(term)) + " ") in t


def load_corpus(path):
    fixtures = {}
    with open(path) as f:
        for line in f:
            line = line.rstrip("\n")
            if not line or line.startswith("#"):
                continue
            parts = line.split("\t")
            if len(parts) != 4:
                continue
            fid, wavfile, expected, keyterms = parts
            fixtures[fid] = {
                "expected": expected,
                "keyterms": keyterms.split("|") if keyterms else [],
            }
    return fixtures


def main():
    corpus_path, results_path = sys.argv[1], sys.argv[2]
    fixtures = load_corpus(corpus_path)
    rows = []
    with open(results_path) as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            rows.append(json.loads(line))

    wers, found, total, lat = [], 0, 0, []
    print(f"{'id':32} {'wer':>6} {'keyterms':>10} {'latency_ms':>11} {'rtf':>6}  transcript")
    for r in rows:
        fid = r.get("id")
        if not fid:
            import os
            fid = os.path.basename(r["file"])[:-4]
        fx = fixtures.get(fid)
        if fx is None:
            print(f"WARNING: no fixture for {fid}", file=sys.stderr)
            continue
        transcript = r.get("transcript", "")
        w = wer(fx["expected"], transcript)
        kt = fx["keyterms"]
        hit = sum(1 for k in kt if contains_term(transcript, k))
        wers.append(w)
        found += hit
        total += len(kt)
        lat.append(r.get("latency_ms", 0))
        rtf = r.get("rtf", "")
        print(f"{fid:32} {w:6.3f} {hit:>4}/{len(kt):<5} {r.get('latency_ms',0):>11} {rtf!s:>6}  {transcript}")

    mean_wer = sum(wers) / len(wers) if wers else 0.0
    recall = found / total if total else float("nan")
    lat_sorted = sorted(lat)
    median_lat = lat_sorted[len(lat_sorted) // 2] if lat_sorted else 0
    print()
    print(f"mean WER: {mean_wer:.4f}  keyterm recall: {recall:.4f} ({found}/{total})  median latency: {median_lat}ms  n={len(rows)}")


if __name__ == "__main__":
    main()
