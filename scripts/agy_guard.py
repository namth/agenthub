#!/usr/bin/env python3
"""Hook PreToolUse của Antigravity (.agents/hooks.json): chặn agent thực thi sửa file do trưởng phòng khoá.

Đọc JSON từ stdin ({"toolCall": {"name", "args"}, ...}), in JSON ra stdout:
  {"decision": "deny", "reason": ...}  khi lời gọi ghi/xoá/di chuyển chạm file được bảo vệ
  {"decision": "ask"}                   mọi trường hợp khác — giữ nguyên luồng duyệt quyền mặc định

File được bảo vệ:
  - mọi file liệt kê trong handoff/tasks/*/tests.lock (verify.sh + test của trưởng phòng)
  - brief.md, tests.lock, state.json, review.md của mọi task
  - công cụ & luật giao việc: scripts/handoff.py, scripts/agy_guard.py, .agents/, handoff/config.json
  - mọi file .env*
  - các đường dẫn riêng của dự án trong handoff/config.json → "protected_paths" (vd mã cũ, vendor)

Tên tham số của từng công cụ Antigravity không cố định, nên script quét mọi chuỗi trong `args`.
Đây là lớp chặn sớm; lớp chặn cuối vẫn là kiểm SHA-256 khi `handoff.py submit`.
"""
from __future__ import annotations

import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
TASKS = ROOT / "handoff" / "tasks"
TASK_FILES = ("brief.md", "tests.lock", "state.json", "review.md")
BASE_PROTECTED = ("scripts/handoff.py", "scripts/agy_guard.py", ".agents/", "handoff/config.json")


def config_protected() -> tuple[str, ...]:
    path = ROOT / "handoff" / "config.json"
    if not path.exists():
        return ()
    try:
        return tuple(str(p).lstrip("./") for p in json.loads(path.read_text(encoding="utf-8")).get("protected_paths", []))
    except json.JSONDecodeError:
        return ()


PROTECTED_PREFIXES = BASE_PROTECTED + config_protected()
# Lệnh shell có thể ghi/xoá/di chuyển file
MUTATING = re.compile(
    r"(>>?|\btee\b|\bsed\s+(-[a-zA-Z]*i|--in-place)|\bperl\s+-[a-zA-Z]*i|\brm\b|\bmv\b|\bcp\b|\btruncate\b"
    r"|\bchmod\b|\bln\b|\bgit\s+(checkout|restore|rm|mv|stash|reset|apply)\b|\bpatch\b|\bdd\b|write_text|open\()"
)


def protected_paths() -> set[str]:
    paths: set[str] = set()
    if TASKS.exists():
        for task in TASKS.iterdir():
            if not task.is_dir():
                continue
            for name in TASK_FILES:
                paths.add(f"handoff/tasks/{task.name}/{name}")
            lock = task / "tests.lock"
            if lock.exists():
                for line in lock.read_text(encoding="utf-8").splitlines():
                    if "  " in line:
                        paths.add(line.split("  ", 1)[1].strip())
    return paths


def strings(value) -> list[str]:
    if isinstance(value, str):
        return [value]
    if isinstance(value, dict):
        return [s for v in value.values() for s in strings(v)]
    if isinstance(value, list):
        return [s for v in value for s in strings(v)]
    return []


def rel(raw: str) -> str | None:
    """Chuẩn hoá một chuỗi có dạng đường dẫn về đường dẫn tương đối với ROOT; None nếu không phải."""
    raw = raw.strip().removeprefix("file://")
    if not raw or "\n" in raw or len(raw) > 1024:
        return None
    p = Path(raw)
    p = p if p.is_absolute() else ROOT / p
    try:
        return p.resolve().relative_to(ROOT).as_posix()
    except (ValueError, OSError):
        return None


def hit(path: str, protected: set[str]) -> bool:
    name = path.rsplit("/", 1)[-1]
    return path in protected or path.startswith(PROTECTED_PREFIXES) or name.startswith(".env")


def decide(payload: dict) -> dict:
    call = payload.get("toolCall") or {}
    tool = str(call.get("name", ""))
    texts = strings(call.get("args", {}))
    protected = protected_paths()

    if tool == "run_command":
        cmd = " ".join(texts)
        if MUTATING.search(cmd):
            for p in sorted(protected) + list(PROTECTED_PREFIXES) + [".env"]:
                if p in cmd:
                    return {"decision": "deny",
                            "reason": f"Lệnh có thể sửa file được trưởng phòng bảo vệ: {p}. "
                                      "Không sửa test/brief/công cụ giao việc; ghi lý do vào report.md nếu cần."}
        return {"decision": "ask"}

    for text in texts:
        path = rel(text)
        if path and hit(path, protected):
            return {"decision": "deny",
                    "reason": f"{path} được trưởng phòng bảo vệ (test khoá / tài liệu giao việc / đường dẫn chỉ-đọc của dự án). "
                              "Không sửa; ghi lý do vào report.md nếu cần."}
    return {"decision": "ask"}


def main() -> None:
    try:
        payload = json.load(sys.stdin)
    except json.JSONDecodeError:
        payload = {}
    print(json.dumps(decide(payload), ensure_ascii=False))


if __name__ == "__main__":
    main()
