"""Export selected issue content read-only, without credentials or human replies."""

import argparse
import json
from pathlib import Path
import re
import sqlite3

ISSUES = [
    "D-681052665ee9",  # idle memory trim
    "D-9bdb14b19c22",  # launcher UI technology
    "D-6e78ed251399",  # serial console resize
    "D-0d045d8cf3cd",  # pinned CA policy
    "A-822d57fae114",  # status overlay verification
]
FIELDS = (
    "kind", "title", "context", "options", "recommended_option",
    "recommendation_reason", "blocking_scope",
)


def scrub(value):
    if isinstance(value, str):
        value = re.sub(r"[A-Za-z]:\\[^\s，。；、\n]+", "〈範例影片檔案〉", value)
        return re.sub(r"\b(?:\d{1,3}\.){3}\d{1,3}\b", "〈範例位址〉", value)
    if isinstance(value, list):
        return [scrub(item) for item in value]
    if isinstance(value, dict):
        return {key: scrub(item) for key, item in value.items()}
    return value


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source-db", required=True, type=Path)
    parser.add_argument("--output", type=Path, default=Path("examples/spicesharp-demo.json"))
    args = parser.parse_args()
    with sqlite3.connect(args.source_db.resolve().as_uri() + "?mode=ro", uri=True) as db:
        placeholders = ",".join("?" for _ in ISSUES)
        rows = [json.loads(row[0]) for row in db.execute(
            f"SELECT body FROM issues WHERE id IN ({placeholders})", ISSUES
        )]
    lookup = {issue["issue_id"]: issue for issue in rows}
    issues = []
    for issue_id in ISSUES:
        if issue_id not in lookup:
            raise SystemExit(f"Required demo issue is missing: {issue_id}")
        source = lookup[issue_id]
        draft = scrub({key: source[key] for key in FIELDS if source.get(key) is not None})
        draft["context"] = "【示範重播】以下節錄自 SpiceSharp 的歷史議題，非目前待辦狀態。\n\n" + draft.get("context", "")
        draft["created_by"] = "SpiceSharp agent"
        draft["owner_coordinator"] = "coordinator"
        issues.append(draft)
    fixture = {
        "schema_version": 1,
        "project_name": "SpiceSharp · 示範",
        "description": "取自 SpiceSharp 的真實議題內容，去除私人環境資訊後重播為待回覆狀態；不包含原始回答、憑證或 session。",
        "issues": issues,
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(fixture, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"Exported {len(issues)} sanitized issue drafts to {args.output}")


if __name__ == "__main__":
    main()
