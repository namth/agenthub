# Giao việc Claude ↔ Antigravity

| Vai trò | Ai | Trách nhiệm |
| :--- | :--- | :--- |
| Trưởng phòng / Giám đốc sản phẩm | **Claude Code** | Phân tích nghiệp vụ, thiết kế kiến trúc, viết đặc tả (`brief.md`), viết test + `verify.sh`, giao việc, nghiệm thu, cập nhật `docs/` |
| Kỹ sư thực thi | **Antigravity (Gemini)** | Đọc brief, viết code trong phạm vi được giao, tự kiểm, viết `report.md`, nộp |

## Vòng đời task

```
draft ──dispatch──▶ assigned ──start──▶ in_progress ──submit──▶ submitted
                                            ▲                      │
                                            │                 verify (Claude)
                                          start                    │
                                            │            ┌─────────┴─────────┐
                                    changes_requested ◀─reject           accept──▶ accepted
```

`state.json` chỉ do `scripts/handoff.py` ghi — không ai sửa tay.

## Quy tắc cứng

1. **Test là của trưởng phòng.** `verify.sh` và các file test liệt kê trong `tests.lock` không được sửa/xoá. Hai lớp bảo vệ: hook `.agents/hooks.json` chặn ngay khi agent định sửa; `submit`/`verify` kiểm SHA-256 lúc nộp.
2. **Không làm ngoài phạm vi.** Chỉ chạm file trong `brief.md` mục 3. Cần thêm → ghi vào mục "câu hỏi" của `report.md`, không tự ý làm.
3. **Báo cáo trung thực.** Output verify dán nguyên văn; việc chưa xong ghi rõ.
4. **Docs do trưởng phòng giữ.** Antigravity không sửa `docs/` trừ khi brief cho phép.

## Cấu hình riêng của dự án — `handoff/config.json`

| Khoá | Ý nghĩa |
| :--- | :--- |
| `protected_paths` | Thư mục/file agent thực thi **không bao giờ** được sửa (mã cũ, vendor, dữ liệu gốc…). Hook chặn theo tiền tố đường dẫn, vd `"legacy/"`. |
| `agy_model` | Slug model mặc định cho `agy` (xem `agy models`); `null` = mặc định của agy. |
| `agy_fallback_models` | Model dự phòng, vd `["gemini-3.8-flash-high", "gemini-3.8-flash-low"]`. Model đang dùng hết hạn mức (429) mà task chưa nộp → script tự chuyển sang model kế tiếp trong cùng hội thoại. |
| `agy_timeout` | Thời gian tối đa một lần chạy `agy`, vd `"60m"`. |

Luôn được bảo vệ, không cần khai báo: file trong `tests.lock`, `brief.md`/`state.json`/`review.md`/`tests.lock` của mọi task, `scripts/handoff.py`, `scripts/agy_guard.py`, `.agents/`, `handoff/config.json`, mọi `.env*`.

## Cách Claude gửi việc sang Antigravity

`dispatch` (và `reject`) dùng đường đầu tiên chạy được:

| Thứ tự | Đường | Điều kiện | Đặc điểm |
| :---: | :--- | :--- | :--- |
| 1 | **`agy -p`** (Antigravity CLI) | Đã cài và đăng nhập `agy` | Claude chờ agent làm xong rồi nghiệm thu luôn; `reject` nhắn tiếp đúng hội thoại cũ (`--conversation`) |
| 2 | `agentapi` | Claude Code chạy trong terminal tích hợp của Antigravity | Hội thoại riêng cho từng task trong Antigravity |
| 3 | `antigravity-ide chat -m agent` | Đã cài Antigravity IDE | Mở dự án trong IDE và gửi prompt; người dùng báo lại khi xong |
| 4 | Clipboard | Luôn có | Dán thủ công |

Ép một đường: `python3 scripts/handoff.py dispatch TASK-001 --via ide`.

### Cài Antigravity CLI (một lần cho cả máy)

```bash
curl -fsSL https://antigravity.google/cli/install.sh | bash
```

```bash
agy
```

Lần chạy `agy` đầu tiên để đăng nhập, rồi thoát (`/exit`). Sau đó cho phép agent chạy `python3` mà không hỏi (cần để `handoff.py` và test chạy được ở chế độ ngầm) — thêm vào `~/.gemini/antigravity-cli/settings.json`:

```json
{ "permissions": { "allow": ["command(python3)"] } }
```

Thêm lệnh riêng của dự án nếu task cần (vd `"command(npm)"`, `"command(pytest)"`). **Không** dùng `--dangerously-skip-permissions`.

Kiểm tra mọi thứ đã sẵn sàng:

```bash
python3 scripts/handoff.py doctor
```

## Lệnh nhanh

```bash
python3 scripts/handoff.py list                 # bảng trạng thái
python3 scripts/handoff.py verify TASK-001      # kiểm khoá test + chạy verify.sh
python3 scripts/handoff.py verify TASK-001 --quiet  # chỉ in phần đuôi (trưởng phòng nghiệm thu)
python3 scripts/handoff.py doctor               # kiểm tra bộ phối hợp & agy
```
