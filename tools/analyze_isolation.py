#!/usr/bin/env python3
"""Export public-safe descriptive statistics for run_isolation.py output.

Usage: python3 tools/analyze_isolation.py PRIVATE_RUN_ROOT --output summary.json
Only numeric telemetry and explicitly allowed metadata are exported. Private
paths, PIDs, URLs, tokens, raw logs and profile contents are never copied.
Partial/failed runs are retained as such; this tool never declares a leak fixed.
"""
import argparse
import datetime
import json
import math
import re
import statistics
from pathlib import Path

MODES = {
    "idle", "sqlite_fixed", "sqlite_compact", "collector_fixed",
    "json_only", "json_sse", "notification",
}
STATES = {
    "pass", "degraded", "failed", "running", "wall_timeout", "rss_limit",
    "sampler_stalled", "not_run", "unknown",
}
PHASES = {"baseline", "sample", "warmup_complete", "end", "post_stop_gc"}
MEMORY_KEYS = (
    "rss_bytes", "go_sys_bytes", "heap_alloc_bytes", "heap_inuse_bytes",
    "heap_idle_bytes", "heap_released_bytes", "stack_inuse_bytes",
    "stack_sys_bytes", "heap_objects", "next_gc_bytes",
    "allocator_live_allocations", "allocator_mapped_bytes", "allocator_mappings",
    "external_client_rss_bytes", "external_receiver_rss_bytes",
)
COUNT_KEYS = (
    "open_fds", "goroutines", "known_nodes", "monitor_subscriptions",
    "repository_incidents", "repository_topology_rows", "repository_topology_nodes",
    "repository_topology_incidents", "repository_history_rows", "repository_event_rows",
    "sqlite_topology_rows", "sqlite_history_rows", "sqlite_incident_rows",
    "cql_probe_failures", "tcp_probe_failures", "probe_health_observations",
)
CUMULATIVE_KEYS = (
    "operations", "codec_operations", "codec_bytes", "successful_cql_probes",
    "discarded_event_writes", "discarded_sample_writes", "topology_writes", "prune_calls",
)
PAYLOAD_KEYS = (
    "input_snapshot_json_bytes", "operation_payload_bytes_min", "operation_payload_bytes_max",
)
CONFIG_KEYS = (
    "warmup_seconds", "duration_seconds", "sample_interval_seconds", "configured_hz",
    "json_sse_get_hz", "json_sse_sse_clients", "nominal_snapshot_encodes_hz",
    "fixed_fixture_nodes", "fixed_fixture_incidents", "fixture_message_bytes",
)
BOOLEAN_KEYS = (
    "allocator_counters_enabled", "operation_payload_bytes_measured", "parent_process_only",
    "heap_alloc_is_live_heap", "sampler_allocations_included", "measurement_gc_forced",
    "probe_health_failed",
)
ENGINE_KEYS = ("tracks", "inflight", "attempts", "queued", "queue_capacity", "workers")
CLIENT_KEYS = ("snapshot_gets", "received_bytes", "errors", "payload_min", "payload_max")
RECEIVER_KEYS = ("requests", "accepted", "rejected", "bytes", "payload_min", "payload_max", "errors")
LIMITS = {
    "rss_bytes": 256 << 20, "allocator_mapped_bytes": 128 << 20,
    "open_fds": 128, "goroutines": 512,
    "engine.tracks": 512, "engine.inflight": 160,
    "engine.attempts": 2048, "engine.queued": 128,
}
BASE_PLAN = [
    ("idle", 300, 120), ("sqlite_fixed", 900, 180),
    ("collector_fixed", 900, 180), ("json_only", 600, 180),
    ("json_sse", 900, 180), ("notification", 900, 180),
]


def number(value):
    return value if type(value) in (int, float) and math.isfinite(value) else None


def add_numbers(target, source, keys, prefix=""):
    if not isinstance(source, dict):
        return
    for key in keys:
        value = number(source.get(key))
        if value is not None:
            target[prefix + key] = value


def clean_sample(raw, expected_mode):
    """Flatten only approved telemetry; never propagate arbitrary input keys."""
    if not isinstance(raw, dict) or raw.get("mode") != expected_mode:
        return None
    elapsed = number(raw.get("elapsed_seconds"))
    phase = raw.get("phase")
    if elapsed is None or elapsed < 0 or not isinstance(phase, str) or phase not in PHASES:
        return None
    row = {"elapsed_seconds": elapsed, "phase": phase}
    go_version = raw.get("go_version")
    if isinstance(go_version, str) and re.fullmatch(r"go[0-9]+\.[0-9]+(?:\.[0-9]+)?", go_version):
        row["go_version"] = go_version
    add_numbers(row, raw, MEMORY_KEYS + COUNT_KEYS + CUMULATIVE_KEYS + PAYLOAD_KEYS + CONFIG_KEYS)
    add_numbers(row, raw, ("num_gc", "natural_gc_age_seconds"))
    for key in BOOLEAN_KEYS:
        if type(raw.get(key)) is bool:
            row[key] = raw[key]
    row["slope_eligible"] = (
        raw.get("slope_eligible") is True and phase == "sample"
        and raw.get("measurement_gc_forced") is not True
    )
    details=[]
    raw_details=raw.get("probe_health_last_failures")
    if isinstance(raw_details,list):
        for slot in raw_details[:6]:
            if not isinstance(slot,dict) or slot.get("protocol") not in ("cql","tcp") or type(slot.get("node_index")) is not int or not 0<=slot["node_index"]<3:
                continue
            item={"node_index":slot["node_index"],"protocol":slot["protocol"]}
            add_numbers(item,slot,("observations","failures"))
            last=slot.get("last_failure")
            if isinstance(last,dict) and last.get("phase") in ("dial","read","write","tls","protocol","unknown"):
                item["last_failure"]={"phase":last["phase"],"attempted_at_utc":safe_time(last.get("attempted_at_utc")),"checked_at_utc":safe_time(last.get("checked_at_utc"))}
                add_numbers(item["last_failure"],last,("latency_ns",))
            details.append(item)
    if details:row["probe_health_last_failures"]=details
    add_numbers(row, raw.get("engine"), ENGINE_KEYS, "engine.")
    client = raw.get("external_client")
    add_numbers(row, client, CLIENT_KEYS, "client.")
    if isinstance(client, dict):
        sse = client.get("sse_snapshots")
        if isinstance(sse, list) and len(sse) == 2:
            for index, value in enumerate(sse):
                if number(value) is not None:
                    row["client.sse_snapshots_" + str(index)] = value
    add_numbers(row, raw.get("notification_external_receiver"), RECEIVER_KEYS, "receiver.")
    return row


def read_samples(path, mode):
    rows, issues = [], []
    try:
        lines = path.read_text().splitlines()
    except (OSError, UnicodeError):
        return rows, [{"code": "samples_unavailable"}]
    for index, line in enumerate(lines, 1):
        if not line.strip():
            continue
        try:
            raw = json.loads(line)
        except (ValueError, TypeError):
            issues.append({"code": "invalid_sample_json", "line_number": index})
            continue
        row = clean_sample(raw, mode)
        if row is None:
            issues.append({"code": "invalid_sample_schema", "line_number": index})
            continue
        if rows and row["elapsed_seconds"] <= rows[-1]["elapsed_seconds"]:
            issues.append({"code": "nonincreasing_sample_time", "line_number": index})
            # Do not silently sort or use an invalid interval in a regression.
            continue
        rows.append(row)
    return rows, issues


def summary(values):
    values = [x for x in values if number(x) is not None]
    if not values:
        return None
    return {
        "n": len(values), "first": values[0], "last": values[-1],
        "min": min(values), "median": statistics.median(values), "max": max(values),
        "last_minus_first": values[-1] - values[0],
    }


def ols(rows, key):
    points = [(r["elapsed_seconds"] / 3600, r[key]) for r in rows if number(r.get(key)) is not None]
    if len(points) < 2:
        return None
    xs, ys = zip(*points)
    xm, ym = statistics.mean(xs), statistics.mean(ys)
    denominator = sum((x - xm) ** 2 for x in xs)
    if denominator == 0:
        return None
    return sum((x - xm) * (y - ym) for x, y in points) / denominator


def metric_stats(rows, key):
    values = [r[key] for r in rows if key in r]
    result = summary(values)
    if result is None:
        return None
    result["ols_per_hour"] = ols(rows, key)
    result["unit"] = "bytes" if key.endswith("_bytes") else "count"
    return result


def throughput(rows, keys):
    result = {}
    for key in keys:
        points = [(r["elapsed_seconds"], r[key]) for r in rows if number(r.get(key)) is not None]
        if not points:
            continue
        first_time, first = points[0]
        last_time, last = points[-1]
        seconds = last_time - first_time
        monotonic = all(b[1] >= a[1] for a, b in zip(points, points[1:]))
        result[key] = {
            "first": first, "last": last, "observed_interval_seconds": seconds,
            "delta": last - first, "counter_monotonic": monotonic,
            "per_second": (last - first) / seconds if seconds > 0 and monotonic else None,
        }
    return result


def safe_time(raw):
    if not isinstance(raw, str) or not re.fullmatch(r"[0-9T:.+Z-]{20,40}", raw):
        return None
    try:
        value = datetime.datetime.fromisoformat(raw.replace("Z", "+00:00"))
        return value.isoformat() if value.tzinfo is not None else None
    except ValueError:
        return None


def numeric_meta(raw, key):
    return number(raw.get(key)) if isinstance(raw, dict) else None


def analyze_case(root, mode, planned_seconds, planned_warmup, entry):
    raw_state = entry.get("state") if isinstance(entry, dict) else None
    state = raw_state if isinstance(raw_state, str) and raw_state in STATES else ("not_run" if entry is None else "unknown")
    rows, issues = read_samples(root / mode / "samples.jsonl", mode)
    operating = [r for r in rows if r["phase"] != "post_stop_gc"]
    eligible = [r for r in rows if r["slope_eligible"]]
    if eligible:
        start, end = eligible[0]["elapsed_seconds"], eligible[-1]["elapsed_seconds"]
        tail_start = start + (end - start) * 2 / 3
        tail = [r for r in eligible if r["elapsed_seconds"] >= tail_start]
    else:
        start = end = tail_start = None
        tail = []
    metrics = {}
    for key in MEMORY_KEYS:
        full = metric_stats(eligible, key)
        if full is not None:
            metrics[key] = {"all_eligible": full, "last_third": metric_stats(tail, key)}
    bounds = {}
    for key, limit in LIMITS.items():
        observed = summary([r[key] for r in operating if key in r])
        if observed:
            bounds[key] = {"observed": observed, "configured_upper_limit": limit,
                           "observed_limit_exceeded": observed["max"] > limit}
    counts = {}
    for key in COUNT_KEYS + tuple("engine." + x for x in ENGINE_KEYS):
        observed = summary([r[key] for r in operating if key in r])
        if observed is not None:
            counts[key] = observed
    gc = summary([r["num_gc"] for r in eligible if "num_gc" in r])
    gc_pairs = [(a["num_gc"], b["num_gc"]) for a, b in zip(eligible, eligible[1:])
                if "num_gc" in a and "num_gc" in b]
    gc_changes = sum(b > a for a, b in gc_pairs)
    if any(b < a for a, b in gc_pairs):
        issues.append({"code": "nonmonotonic_gc_counter"})
    cumulative = CUMULATIVE_KEYS + (
        "client.snapshot_gets", "client.received_bytes", "client.sse_snapshots_0",
        "client.sse_snapshots_1", "client.errors", "receiver.requests", "receiver.accepted",
        "receiver.rejected", "receiver.bytes", "receiver.errors",
    )
    payload_keys = PAYLOAD_KEYS + (
        "client.payload_min", "client.payload_max", "receiver.payload_min", "receiver.payload_max",
    )
    # Helper progress uses zero before its first transfer, not a zero-byte JSON
    # payload. A nonmeasured operation payload is an input-size placeholder.
    payload = {key: summary([
        r[key] for r in operating if key in r and r[key] > 0
        and (not key.startswith("operation_payload_")
             or r.get("operation_payload_bytes_measured") is True)
    ]) for key in payload_keys}
    payload = {key: value for key, value in payload.items() if value is not None}
    if len(eligible) < 2:
        issues.append({"code": "insufficient_slope_eligible_samples"})
    if eligible and any(r.get("allocator_counters_enabled") is not True for r in eligible):
        issues.append({"code": "native_allocator_counters_not_confirmed_enabled"})
    if any(r.get("natural_gc_age_seconds", 0) < 0 for r in eligible):
        issues.append({"code": "negative_gc_age_observed"})
    if state == "pass" and not any(r["phase"] == "end" for r in rows):
        issues.append({"code": "pass_manifest_without_end_sample"})
    if state == "pass" and not any(r["phase"] == "post_stop_gc" for r in rows):
        issues.append({"code": "post_stop_diagnostic_missing"})
    result = {
        "mode": mode, "manifest_state": state,
        "started_utc": safe_time(entry.get("started_utc")) if isinstance(entry, dict) else None,
        "finished_utc": safe_time(entry.get("finished_utc")) if isinstance(entry, dict) else None,
        "manifest_exit_code": numeric_meta(entry, "exit_code"),
        "planned_duration_seconds": planned_seconds, "planned_warmup_seconds": planned_warmup,
        "observed_manifest_wall_seconds": numeric_meta(entry, "wall_seconds"),
        "case_completed_successfully": state == "pass" and numeric_meta(entry, "exit_code") == 0,
        "resource_workload_completed": state in ("pass", "degraded") and numeric_meta(entry, "exit_code") == 0,
        "probe_health_failed": any(r.get("probe_health_failed") is True for r in rows),
        "analysis_evidence_usable": bool(rows) and len(eligible) >= 2 and not issues,
        "samples_total": len(rows), "slope_eligible_samples": len(eligible),
        "eligible_start_seconds": start, "eligible_end_seconds": end,
        "last_third_start_seconds": tail_start, "last_third_samples": len(tail),
        "memory_statistics": metrics,
        "natural_gc": {
            "num_gc": gc, "num_gc_delta": gc["last_minus_first"] if gc else None,
            "observed_sample_intervals_with_gc_progress": gc_changes,
            "age_seconds": summary([r["natural_gc_age_seconds"] for r in eligible if "natural_gc_age_seconds" in r]),
            "heap_alloc_is_live_heap": False,
            "note": "Natural-GC HeapAlloc and HeapObjects depend on GC phase; no forced GC occurs in eligible samples.",
        },
        "observed_operating_bounds": bounds, "observed_operating_resource_counts": counts,
        "throughput_operating_interval": throughput(operating, cumulative),
        "throughput_eligible_interval": throughput(eligible, cumulative),
        "payload_byte_observations": payload,
        "post_stop_gc_diagnostic": [r for r in rows if r["phase"] == "post_stop_gc"],
        "sample_series": rows, "analysis_issues": issues,
        "inference": "Descriptive finite case only; no memory-leak resolution verdict is made.",
    }
    return result


def safe_sha(raw, length):
    return raw if isinstance(raw, str) and re.fullmatch("[0-9a-f]{" + str(length) + "}", raw) else None


def analyze(root):
    issues = []
    try:
        manifest = json.loads((root / "manifest.json").read_text())
        if not isinstance(manifest, dict):
            raise ValueError()
    except (OSError, UnicodeError, ValueError, TypeError):
        manifest = {}
        issues.append({"code": "manifest_unavailable_or_invalid"})
    plan = []
    raw_plan = manifest.get("plan")
    if isinstance(raw_plan, list):
        for item in raw_plan:
            if (isinstance(item, list) and len(item) == 3 and isinstance(item[0], str) and item[0] in MODES
                    and number(item[1]) is not None and number(item[2]) is not None):
                if item[0] not in [p[0] for p in plan]:
                    plan.append(tuple(item))
                else:
                    issues.append({"code": "duplicate_plan_mode"})
            else:
                issues.append({"code": "invalid_plan_entry"})
    if not plan:
        plan = BASE_PLAN
        issues.append({"code": "using_default_plan_for_incomplete_metadata"})
    entries = {}
    if isinstance(manifest.get("cases"), list):
        for entry in manifest["cases"]:
            if isinstance(entry, dict) and isinstance(entry.get("mode"), str) and entry.get("mode") in MODES:
                if entry["mode"] in entries:
                    issues.append({"code": "duplicate_case_mode"})
                else:
                    entries[entry["mode"]] = entry
    cases = [analyze_case(root, mode, seconds, warmup, entries.get(mode)) for mode, seconds, warmup in plan]
    manifest_complete = manifest.get("complete") is True
    all_complete = manifest_complete and bool(cases) and not issues and all(c["case_completed_successfully"] for c in cases)
    return {
        "schema_version": 1,
        "started_utc": safe_time(manifest.get("started_utc")),
        "finished_utc": safe_time(manifest.get("finished_utc")),
        "source_commit": safe_sha(manifest.get("source_commit"), 40),
        "binary_sha256": safe_sha(manifest.get("binary_sha256"), 64),
        "manifest_complete": manifest_complete,
        "all_planned_cases_completed_successfully": all_complete,
        "all_case_analysis_evidence_usable": bool(cases) and all(c["analysis_evidence_usable"] for c in cases),
        "sequential": manifest.get("sequential") is True,
        "fresh_process_per_case": manifest.get("fresh_process_per_case") is True,
        "runtime_configuration": {
            key: manifest[key] if manifest.get(key) == value else None
            for key, value in {"GOMEMLIMIT": "128MiB", "GOGC": "100", "GOMAXPROCS": "2"}.items()
        },
        "measured_seconds_limit": numeric_meta(manifest, "measured_seconds_limit"),
        "cases": cases, "analysis_issues": issues,
        "conclusion": "Completed workload checks are not a week-scale memory acceptance verdict.",
        "limitations": [
            "Only allowlisted numeric telemetry, case/state metadata and hashes are exported; no raw URLs/PIDs/paths/tokens/logs/profiles.",
            "Only explicit slope_eligible sample rows without forced GC enter OLS; post-stop GC is a separate diagnostic.",
            "Last-third means the final third of each observed eligible interval; partial cases have shorter intervals and remain incomplete.",
            "OLS is descriptive with correlated samples and GC phase; no confidence interval, causal attribution or automatic leak verdict.",
            "HeapAlloc/HeapObjects under natural GC are not an exact retained/live-object measurement.",
            "Native mapped bytes include allocator pools and are not RSS; allocation count is not requested live-byte count.",
            "RSS minus GoSys is not a native allocator measurement; per-metric trends must be interpreted jointly.",
            "External client/receiver RSS is separate and is never added to measured server RSS.",
            "Observed sampled bounds can miss transient peaks; failed, interrupted and unrun cases remain visible.",
            "Different payload bytes, operation rates and case combinations can confound comparisons; generic full/compact SQLite modes have different stored semantics.",
            "json_sse has no direct server encode counter; successful external SSE/GET counters describe received work, not all server encoding attempts.",
            "Sampler/runtime/fixture allocations are included; no finite isolation result establishes resolution of a weeks-long production issue.",
        ],
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("run_root", help="Private runner output directory containing manifest.json")
    parser.add_argument("--output", help="Write public-safe JSON summary instead of stdout")
    args = parser.parse_args()
    result = analyze(Path(args.run_root))
    body = json.dumps(result, ensure_ascii=False, indent=2, allow_nan=False) + "\n"
    if args.output:
        try:
            Path(args.output).write_text(body)
        except OSError:
            raise SystemExit("Unable to write analysis output (path omitted).") from None
    else:
        print(body, end="")


if __name__ == "__main__":
    main()
