#!/usr/bin/env python3
"""Giao việc Claude (trưởng phòng) <-> Antigravity (kỹ sư thực thi).

Mỗi task là một thư mục handoff/tasks/TASK-NNN-<slug>/ gồm:
  brief.md      Claude viết — đặc tả, phạm vi file, tiêu chí nghiệm thu
  verify.sh     Claude viết — lệnh xác minh tự động (exit 0 = đạt)
  tests.lock    Script sinh — SHA-256 của verify.sh + file test Claude viết
  report.md     Antigravity viết — báo cáo khi nộp
  review.md     Claude viết — kết quả nghiệm thu / yêu cầu sửa
  state.json    Chỉ script này ghi — trạng thái và lịch sử

Trạng thái: assigned -> in_progress -> submitted -> accepted | changes_requested -> in_progress ...

Lệnh:
  new <slug> --title "..."            Claude: tạo task mới từ mẫu
  lock <id> <file_test>...            Claude: khoá verify.sh + file test
  dispatch <id> [--via auto|agy|agentapi|ide|clipboard] [--agy-model SLUG] [--model pro]
                                      Claude: giao task cho Antigravity. Với agy, lệnh chạy tới khi
                                      agent làm xong (Claude chạy nền) rồi in kết quả.
  start <id>                          Antigravity: nhận việc
  submit <id>                         Antigravity: tự kiểm rồi nộp
  verify <id> [--quiet]               kiểm khoá test + chạy verify.sh (--quiet: chỉ in phần đuôi)
  accept <id> / reject <id>           Claude: nghiệm thu / trả về (reject gửi review.md cho Antigravity)
  resume <id> [--note "..."] [--agy-model SLUG]
                                      Claude: nhắn agent làm tiếp trong cùng hội thoại agy khi nó dừng giữa chừng
  list                                Bảng trạng thái
  doctor                              Kiểm tra bộ phối hợp & Antigravity CLI đã sẵn sàng chưa

Cấu hình riêng của dự án: handoff/config.json
  protected_paths  thư mục/file agent thực thi không được sửa (hook .agents/hooks.json chặn)
  agy_model        slug model mặc định cho agy (xem `agy models`); null = mặc định của agy
  agy_fallback_models  model dự phòng, thử lần lượt khi model đang dùng hết hạn mức (429)
  agy_timeout      thời gian tối đa một lần chạy agy (vd "60m")
"""
from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import os
import re
import shutil
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
TASKS = ROOT / "handoff" / "tasks"
TEMPLATE = ROOT / "handoff" / "templates"
AGENTAPI = Path.home() / ".gemini" / "antigravity" / "bin" / "agentapi"
IDE_CLI = Path("/Applications/Antigravity IDE.app/Contents/Resources/app/bin/antigravity-ide")
AGY = shutil.which("agy") or str(Path.home() / ".local" / "bin" / "agy")
AGY_SETTINGS = Path.home() / ".gemini" / "antigravity-cli" / "settings.json"


def load_config() -> dict:
    path = ROOT / "handoff" / "config.json"
    cfg = {"protected_paths": [], "agy_model": None, "agy_fallback_models": [], "agy_timeout": "60m"}
    if path.exists():
        cfg.update(json.loads(path.read_text(encoding="utf-8")))
    return cfg


CONFIG = load_config()
AGY_TIMEOUT = os.environ.get("AGY_PRINT_TIMEOUT", CONFIG["agy_timeout"])

WORKER_CMDS = {"start": ("assigned", "changes_requested"), "submit": ("in_progress",)}


def now() -> str:
    return dt.datetime.now().isoformat(timespec="seconds")


def die(msg: str) -> None:
    print(f"LỖI: {msg}", file=sys.stderr)
    sys.exit(1)


def find_task(task_id: str) -> Path:
    task_id = task_id.upper() if task_id.lower().startswith("task-") else f"TASK-{int(task_id):03d}"
    matches = sorted(TASKS.glob(f"{task_id}-*")) + sorted(TASKS.glob(task_id))
    if not matches:
        die(f"không tìm thấy {task_id} trong {TASKS.relative_to(ROOT)}")
    return matches[0]


def load_state(task: Path) -> dict:
    return json.loads((task / "state.json").read_text(encoding="utf-8"))


def save_state(task: Path, state: dict, status: str | None = None, note: str = "") -> None:
    if status:
        state["status"] = status
        state["history"].append({"at": now(), "status": status, "note": note})
    (task / "state.json").write_text(json.dumps(state, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def check_lock(task: Path) -> list[str]:
    lock = task / "tests.lock"
    if not lock.exists():
        return ["chưa có tests.lock — Claude cần chạy `lock` trước khi giao việc"]
    problems = []
    for line in lock.read_text(encoding="utf-8").splitlines():
        if not line.strip():
            continue
        digest, rel = line.split("  ", 1)
        path = ROOT / rel
        if not path.exists():
            problems.append(f"file test bị xoá: {rel}")
        elif sha256(path) != digest:
            problems.append(f"file test bị sửa: {rel}")
    return problems


def run_verify(task: Path, quiet: bool = False) -> int:
    """quiet: chỉ in phần đuôi output (đạt: 6 dòng, trượt: 80 dòng) để trưởng phòng đọc ít mà đủ."""
    script = task / "verify.sh"
    print(f"$ bash {script.relative_to(ROOT)}", flush=True)
    if not quiet:
        return subprocess.run(["bash", str(script)], cwd=ROOT).returncode
    proc = subprocess.run(["bash", str(script)], cwd=ROOT, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    lines = proc.stdout.rstrip().splitlines()
    keep = 6 if proc.returncode == 0 else 80
    if len(lines) > keep:
        print(f"… (ẩn {len(lines) - keep} dòng đầu)")
    print("\n".join(lines[-keep:]))
    return proc.returncode


def agentapi(*args: str) -> str | None:
    """Gọi Antigravity CLI. Trả None nếu không gọi được (vd chạy ngoài terminal của Antigravity)."""
    if not AGENTAPI.exists() or not os.environ.get("ANTIGRAVITY_LS_ADDRESS"):
        return None
    proc = subprocess.run([str(AGENTAPI), *args], capture_output=True, text=True)
    out = (proc.stdout + proc.stderr).strip()
    if proc.returncode != 0 or '"error"' in out:
        print(f"agentapi lỗi: {out}", file=sys.stderr)
        return None
    return out


def agy_run(prompt: str, conversation_id: str | None = None, model: str | None = None) -> dict | None:
    """Chạy Antigravity CLI ở chế độ không tương tác, chờ agent làm xong, trả về phong bì JSON.
    Trả None nếu chưa cài agy. Sửa file được tự duyệt (--mode accept-edits); lệnh shell theo
    danh sách cho phép trong ~/.gemini/antigravity-cli/settings.json; hook .agents/hooks.json chặn sửa test."""
    if not Path(AGY).exists():
        return None
    args = [AGY, "-p", prompt, "--output-format", "json", "--mode", "accept-edits", "--print-timeout", AGY_TIMEOUT]
    if model:
        args += ["--model", model]
    if conversation_id:
        args += ["--conversation", conversation_id]
    print(f"Đang chạy agy (tối đa {AGY_TIMEOUT})…", flush=True)
    proc = subprocess.run(args, cwd=ROOT, capture_output=True, text=True)
    notices = [l for l in proc.stderr.splitlines() if re.search(r"deni|permission|quyền|error|lỗi", l, re.I)]
    try:
        result = json.loads(proc.stdout)
    except json.JSONDecodeError:
        result = {"status": "ERROR", "error": (proc.stdout + proc.stderr).strip()[-2000:]}
    result["exit_code"] = proc.returncode
    result["notices"] = notices
    return result


def last_tool_call(conversation_id: str | None) -> str | None:
    """Lời gọi công cụ cuối cùng trong bản ghi hội thoại agy — thường là thao tác vừa bị từ chối."""
    if not conversation_id:
        return None
    transcript = (Path.home() / ".gemini" / "antigravity-cli" / "brain" / conversation_id
                  / ".system_generated" / "logs" / "transcript.jsonl")
    if not transcript.exists():
        return None
    last = None
    for line in transcript.read_text(encoding="utf-8").splitlines():
        try:
            calls = json.loads(line).get("tool_calls") or []
        except json.JSONDecodeError:
            continue
        if calls:
            last = calls[-1]
    return None if last is None else f"{last.get('name')} {json.dumps(last.get('args'), ensure_ascii=False)[:600]}"


def is_quota(result: dict) -> bool:
    text = json.dumps(result, ensure_ascii=False)
    return "RESOURCE_EXHAUSTED" in text or "quota" in text.lower()


def agy_task(task: Path, prompt: str, model: str | None, conversation_id: str | None = None) -> None:
    """Chạy agy cho một task; model hết hạn mức mà task chưa nộp → tự chuyển sang model dự phòng
    (handoff/config.json → agy_fallback_models) trong cùng hội thoại, rồi in báo cáo."""
    models = list(dict.fromkeys([model, *CONFIG.get("agy_fallback_models", [])]))
    for i, m in enumerate(models):
        result = agy_run(prompt, conversation_id=conversation_id, model=m)
        state = load_state(task)
        conversation_id = result.get("conversation_id") or conversation_id
        state["conversation_id"], state["via"] = conversation_id, "agy"
        save_state(task, state)
        if not is_quota(result) or state["status"] == "submitted" or i == len(models) - 1:
            break
        nxt = models[i + 1]
        print(f"Model {m or 'mặc định'} hết hạn mức → chuyển sang {nxt or 'mặc định'}.", flush=True)
        state["history"].append({"at": now(), "status": state["status"], "note": f"đổi model agy: {m} → {nxt} (hết hạn mức)"})
        save_state(task, state)
        if conversation_id:
            prompt = (f"Tiếp tục {state['id']} theo yêu cầu ban đầu của hội thoại này, từ chỗ đang dừng, "
                      "theo skill /thuc-hien-task, tới khi `submit` thành công.")
    report_agy(task, result)


def report_agy(task: Path, result: dict) -> None:
    state = load_state(task)
    print(f"agy: status={result.get('status')} exit={result['exit_code']} "
          f"conversation={result.get('conversation_id')} turns={result.get('num_turns')} "
          f"thời gian={result.get('duration_seconds')}s")
    for n in result["notices"]:
        print(f"  [stderr] {n}")
    if result.get("error"):
        print(f"  [lỗi] {result['error']}")
    if is_quota(result):
        if state["status"] == "submitted":
            print("(Lỗi hạn mức xảy ra sau khi task đã nộp — không ảnh hưởng kết quả.)")
        else:
            print("HẾT HẠN MỨC MODEL (cả model dự phòng): không thử lại cùng model. Hạn mức tính theo từng model — "
                  "làm tiếp bằng model khác: resume/reject <ID> --agy-model <slug> (xem `agy models`), "
                  "hoặc thêm vào agy_fallback_models trong handoff/config.json.")
    print("\n--- Trả lời cuối của agent ---\n" + str(result.get("response", ""))[-1500:])
    print(f"\nTrạng thái task sau khi agent dừng: {state['status']}")
    if state["status"] != "submitted":
        print("CẢNH BÁO: agent dừng nhưng task chưa nộp — xem lỗi/thao tác bị từ chối ở trên.")
        if result["notices"] and (call := last_tool_call(result.get("conversation_id"))):
            print(f"Thao tác cuối của agent (nhiều khả năng bị từ chối):\n  {call}")
        print(f"Cho làm tiếp: python3 scripts/handoff.py resume {state['id']} --note \"<hướng dẫn>\"")


def ide_chat(prompt: str, attach: list[Path]) -> bool:
    """Gửi prompt vào khung agent của Antigravity IDE qua `antigravity-ide chat` (chạy được từ mọi terminal).
    Mở thư mục dự án trong cửa sổ IDE gần nhất trước, để agent làm việc đúng workspace."""
    if not IDE_CLI.exists():
        return False
    if subprocess.run([str(IDE_CLI), "-r", str(ROOT)]).returncode != 0:
        return False
    args = [str(IDE_CLI), "chat", "-m", "agent", "-r"]
    for f in attach:
        args += ["-a", str(f)]
    return subprocess.run([*args, prompt]).returncode == 0


def hand_over(prompt: str) -> None:
    """Phương án cuối: copy lời giao việc vào clipboard để dán vào Antigravity."""
    try:
        subprocess.run(["pbcopy"], input=prompt, text=True, check=True)
        where = "ĐÃ COPY vào clipboard — mở Antigravity (Agent Manager), tạo hội thoại mới và dán."
    except (OSError, subprocess.CalledProcessError):
        where = "Dán đoạn sau vào hội thoại Antigravity:"
    print(f"Không gọi được Antigravity CLI. {where}\n\n{prompt}")


# ---------- lệnh ----------

def cmd_new(a) -> None:
    TASKS.mkdir(parents=True, exist_ok=True)
    nums = [int(m.group(1)) for p in TASKS.iterdir() if (m := re.match(r"TASK-(\d+)", p.name))]
    task_id = f"TASK-{(max(nums) + 1) if nums else 1:03d}"
    slug = re.sub(r"[^a-z0-9]+", "-", a.slug.lower()).strip("-")
    task = TASKS / f"{task_id}-{slug}"
    task.mkdir()
    for name in ("brief.md", "verify.sh"):
        text = (TEMPLATE / name).read_text(encoding="utf-8")
        (task / name).write_text(text.replace("{{ID}}", task_id).replace("{{TITLE}}", a.title), encoding="utf-8")
    state = {"id": task_id, "title": a.title, "status": None, "conversation_id": None, "history": []}
    save_state(task, state, "draft", "Claude tạo task")
    print(task.relative_to(ROOT))


def cmd_lock(a) -> None:
    task = find_task(a.id)
    files = [task / "verify.sh", *[ROOT / f for f in a.files]]
    lines = []
    for f in files:
        if not f.exists():
            die(f"không có file {f}")
        lines.append(f"{sha256(f)}  {f.resolve().relative_to(ROOT)}")
    (task / "tests.lock").write_text("\n".join(lines) + "\n", encoding="utf-8")
    state = load_state(task)
    state["locked_files"] = [l.split("  ", 1)[1] for l in lines]
    save_state(task, state)
    print(f"Đã khoá {len(lines)} file.")


def cmd_dispatch(a) -> None:
    task = find_task(a.id)
    state = load_state(task)
    if problems := check_lock(task):
        die("; ".join(problems))
    rel = task.relative_to(ROOT)
    prompt = (
        f"Bạn là kỹ sư thực thi của dự án {ROOT.name} tại thư mục {ROOT}. "
        f"Trưởng phòng (Claude) vừa giao {state['id']}: {state['title']}. "
        f"Hãy dùng skill /thuc-hien-task (.agents/skills/thuc-hien-task/SKILL.md) với task {state['id']}: "
        f"đọc {rel}/brief.md, chạy `python3 scripts/handoff.py start {state['id']}`, thực hiện, "
        f"tự kiểm bằng `python3 scripts/handoff.py verify {state['id']}` (không chạy bash trực tiếp), "
        f"viết {rel}/report.md rồi chạy `python3 scripts/handoff.py submit {state['id']}`. "
        "Tuyệt đối không sửa các file test bị khoá trong tests.lock."
    )
    if a.via in ("auto", "agy") and Path(AGY).exists():
        save_state(task, state, "assigned", f"giao qua agy ({a.agy_model or 'model mặc định'})")
        agy_task(task, prompt, a.agy_model)
        return
    if a.via == "agy":
        die(f"chưa cài agy ({AGY}) — xem handoff/README.md")
    out = None
    if a.via in ("auto", "agentapi"):
        out = agentapi("new-conversation", f"--model={a.model}", f"--title={state['id']} {state['title']}", prompt)
    if out is None:
        if a.via in ("auto", "ide") and ide_chat(prompt, [task / "brief.md"]):
            save_state(task, state, "assigned", "giao qua antigravity-ide chat")
            print(f"Đã gửi {state['id']} vào khung agent của Antigravity IDE.")
            return
        save_state(task, state, "assigned", "giao thủ công (clipboard)")
        hand_over(prompt)
        return
    m = re.search(r"[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}", out)
    state["conversation_id"] = m.group(0) if m else None
    save_state(task, state, "assigned", f"giao qua agentapi ({a.model})")
    print(out)
    if not m:
        print("CẢNH BÁO: không đọc được conversation_id — `reject` sẽ phải giao thủ công.")


def cmd_start(a) -> None:
    task = find_task(a.id)
    state = load_state(task)
    if state["status"] not in WORKER_CMDS["start"]:
        die(f"{state['id']} đang ở trạng thái {state['status']}, không nhận việc được")
    save_state(task, state, "in_progress", "Antigravity nhận việc")
    print(f"{state['id']} -> in_progress. Đọc {task.relative_to(ROOT)}/brief.md" +
          (" và review.md" if (task / "review.md").exists() else ""))


def cmd_submit(a) -> None:
    task = find_task(a.id)
    state = load_state(task)
    if state["status"] not in WORKER_CMDS["submit"]:
        die(f"{state['id']} đang ở trạng thái {state['status']}, phải `start` trước")
    if problems := check_lock(task):
        die("vi phạm khoá test: " + "; ".join(problems) + " — khôi phục bằng `git checkout -- <file>`")
    report = task / "report.md"
    if not report.exists() or len(report.read_text(encoding="utf-8").strip()) < 50:
        die(f"thiếu {report.relative_to(ROOT)} (theo mẫu handoff/templates/report.md)")
    code = run_verify(task)
    if code != 0:
        die(f"verify.sh thất bại (exit {code}) — sửa code (không sửa test) rồi nộp lại")
    save_state(task, state, "submitted", "Antigravity nộp, verify.sh đạt")
    print(f"{state['id']} -> submitted. Báo trưởng phòng nghiệm thu.")


def cmd_verify(a) -> None:
    task = find_task(a.id)
    problems = check_lock(task)
    for p in problems:
        print(f"KHOÁ TEST: {p}")
    code = run_verify(task, quiet=a.quiet)
    print(f"\nKẾT QUẢ: khoá test {'OK' if not problems else 'VI PHẠM'}, verify.sh exit {code}")
    sys.exit(1 if problems or code else 0)


def cmd_accept(a) -> None:
    task = find_task(a.id)
    state = load_state(task)
    if check_lock(task) or run_verify(task, quiet=True) != 0:
        die("chưa đạt — chạy `verify` để xem chi tiết")
    save_state(task, state, "accepted", "Claude nghiệm thu")
    print(f"{state['id']} -> accepted")


def cmd_reject(a) -> None:
    task = find_task(a.id)
    state = load_state(task)
    if state["status"] != "submitted":
        die(f"{state['id']} đang ở trạng thái {state['status']}, chỉ trả về được task đã nộp")
    review = task / "review.md"
    if not review.exists():
        die(f"viết {review.relative_to(ROOT)} trước khi trả về")
    save_state(task, state, "changes_requested", "Claude yêu cầu sửa")
    msg = (f"{state['id']} chưa đạt nghiệm thu. Đọc {review.relative_to(ROOT)}, "
           f"chạy `python3 scripts/handoff.py start {state['id']}`, sửa theo từng mục, "
           "cập nhật report.md (thêm mục 'Vòng sửa') rồi `submit` lại.")
    if state.get("via") == "agy" and state.get("conversation_id") and Path(AGY).exists():
        agy_task(task, msg, a.agy_model, state["conversation_id"])
        return
    out = agentapi("send-message", state["conversation_id"], msg) if state.get("conversation_id") else None
    if out is None:
        if ide_chat(msg, [task / "brief.md", review]):
            print(f"Đã gửi yêu cầu sửa {state['id']} vào khung agent của Antigravity IDE.")
            return
        hand_over(msg)
    else:
        print(out)


def cmd_resume(a) -> None:
    task = find_task(a.id)
    state = load_state(task)
    if state["status"] not in ("assigned", "in_progress", "changes_requested"):
        die(f"{state['id']} đang ở trạng thái {state['status']}, không cần làm tiếp")
    if state.get("via") != "agy" or not state.get("conversation_id"):
        die("task không được giao qua agy — dùng dispatch để giao lại")
    msg = (f"Tiếp tục {state['id']} từ chỗ đang dừng, theo skill /thuc-hien-task, tới khi `submit` thành công. "
           + (a.note or ""))
    agy_task(task, msg, a.agy_model, state["conversation_id"])


def cmd_list(a) -> None:
    if not TASKS.exists() or not any(TASKS.iterdir()):
        print("Chưa có task nào.")
        return
    for task in sorted(p for p in TASKS.iterdir() if p.is_dir()):
        s = load_state(task)
        print(f"{s['id']:<9} {s['status']:<18} {s['title']}")


def cmd_doctor(a) -> None:
    checks: list[tuple[bool, str, str]] = []

    def check(ok: bool, label: str, fix: str = "") -> None:
        checks.append((ok, label, fix))

    for rel in (".agents/rules/vai-tro-ky-su-thuc-thi.md", ".agents/skills/thuc-hien-task/SKILL.md",
                ".agents/hooks.json", "scripts/agy_guard.py", "AGENTS.md", "handoff/config.json"):
        check((ROOT / rel).exists(), f"có {rel}", "chạy /khoi-tao-phoi-hop cập nhật")
    legacy = [p for p in (".agent/rules/vai-tro-ky-su-thuc-thi.md", ".agent/workflows/thuc-hien-task.md")
              if (ROOT / p).exists()]
    check(not legacy, "không còn file kiểu cũ trong .agent/", f"xoá {', '.join(legacy)} (đã chuyển sang .agents/)")
    guard = subprocess.run([sys.executable, str(ROOT / "scripts" / "agy_guard.py")], cwd=ROOT / ".agents",
                           input=json.dumps({"toolCall": {"name": "write_to_file",
                                                          "args": {"path": "scripts/handoff.py"}}}),
                           capture_output=True, text=True)
    check('"deny"' in guard.stdout, "hook chặn được sửa scripts/handoff.py", guard.stderr.strip()[-200:])
    has_agy = Path(AGY).exists()
    check(has_agy, f"đã cài Antigravity CLI ({AGY})",
          "curl -fsSL https://antigravity.google/cli/install.sh | bash   rồi chạy `agy` một lần để đăng nhập")
    if has_agy:
        ver = subprocess.run([AGY, "--version"], capture_output=True, text=True)
        check(ver.returncode == 0, f"agy chạy được: {(ver.stdout or ver.stderr).strip()[:60]}")
        allow: list[str] = []
        if AGY_SETTINGS.exists():
            try:
                allow = json.loads(AGY_SETTINGS.read_text(encoding="utf-8")).get("permissions", {}).get("allow", [])
            except json.JSONDecodeError:
                pass
        check(any("python3" in x for x in allow), "agy được phép chạy python3 (handoff.py, test)",
              f'thêm "command(python3)" vào permissions.allow trong {AGY_SETTINGS}')
    check(IDE_CLI.exists(), "có Antigravity IDE (đường dự phòng `antigravity-ide chat`)", "không bắt buộc")
    for ok, label, fix in checks:
        print(f"{'OK  ' if ok else 'THIẾU'} {label}" + ("" if ok or not fix else f"\n      → {fix}"))
    sys.exit(0 if all(ok for ok, label, _ in checks if "không bắt buộc" not in _) else 1)


def main() -> None:
    p = argparse.ArgumentParser(description="Giao việc Claude <-> Antigravity")
    sub = p.add_subparsers(dest="cmd", required=True)
    n = sub.add_parser("new"); n.add_argument("slug"); n.add_argument("--title", required=True)
    l = sub.add_parser("lock"); l.add_argument("id"); l.add_argument("files", nargs="*")
    d = sub.add_parser("dispatch"); d.add_argument("id")
    d.add_argument("--model", default="pro", choices=["pro", "flash", "flash_lite"])
    d.add_argument("--via", default="auto", choices=["auto", "agy", "agentapi", "ide", "clipboard"],
                   help="auto: agy → agentapi (trong terminal Antigravity) → antigravity-ide chat → clipboard")
    d.add_argument("--agy-model", default=os.environ.get("AGY_MODEL") or CONFIG["agy_model"],
                   help="slug model cho agy (xem `agy models`); mặc định: biến AGY_MODEL, rồi handoff/config.json")
    for name in ("start", "submit", "accept"):
        sub.add_parser(name).add_argument("id")
    v = sub.add_parser("verify"); v.add_argument("id")
    v.add_argument("--quiet", action="store_true", help="chỉ in phần đuôi output (cho trưởng phòng nghiệm thu)")
    default_model = os.environ.get("AGY_MODEL") or CONFIG["agy_model"]
    rj = sub.add_parser("reject"); rj.add_argument("id")
    rj.add_argument("--agy-model", default=default_model, help="đổi model khi model mặc định hết hạn mức")
    r = sub.add_parser("resume"); r.add_argument("id"); r.add_argument("--note", default="")
    r.add_argument("--agy-model", default=default_model, help="đổi model khi model mặc định hết hạn mức")
    sub.add_parser("list")
    sub.add_parser("doctor")
    a = p.parse_args()
    globals()[f"cmd_{a.cmd}"](a)


if __name__ == "__main__":
    main()
