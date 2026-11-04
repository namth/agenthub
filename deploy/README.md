# Triển khai AgentHub

Server hiện tại: `192.168.1.36` (Debian 13, arm64), thư mục `~/agenthub`, domain `https://agent.oa.io.vn` qua Cloudflare Tunnel → `http://localhost:1407`.

## Build (trên máy dev)
```bash
deploy/build.sh            # ra bin/release/agenthub (server linux/arm64) + bin/release/runners/* (.sha256)
```
Phiên bản lấy từ `git describe`; server gửi nó trong `HANDSHAKE_OK.server_version`, runner khác phiên bản sẽ tự tải bản mới khi rảnh (DEC-19).

## Cài lần đầu (trên server)
1. `mkdir -p ~/agenthub/data ~/agenthub/backups`, chép `agenthub` và `backup.py` vào `~/agenthub`.
2. `cp .env.example ~/agenthub/.env && chmod 600 ~/agenthub/.env`, điền giá trị (JWT/webhook/registration key tự sinh bằng `openssl rand -hex 32`).
3. Dịch vụ: thay `__USER__`, `__DIR__` trong `agenthub.service`, chép vào `/etc/systemd/system/`, rồi `sudo systemctl enable --now agenthub`.
4. Sao lưu hằng ngày: `crontab -e` → `15 3 * * * /usr/bin/python3 ~/agenthub/backup.py ~/agenthub/data/agenthub.db ~/agenthub/backups`.

## Cập nhật
1. Chép `bin/release/agenthub` thành `~/agenthub/agenthub.new` và `bin/release/runners/*` vào `~/agenthub/runners/` (`RUNNERS_DIR`).
2. `mv agenthub.new agenthub && sudo systemctl restart agenthub`. Migration DB tự chạy khi khởi động (bảng `schema_migrations`). Nhật ký: `journalctl -u agenthub -f`.

## Thêm dự án (FEAT-04)
Đăng nhập bằng tài khoản `GITHUB_OWNER` → **Dự án** → chọn repo trong "Repo chưa thêm" → **Thêm**. Server tự tạo webhook (PAT cần quyền **Webhooks RW** trên repo đó); lỗi thì bấm **Tạo lại webhook** sau khi cấp quyền.

## Nối máy con (FEAT-05)
Ở **Máy của tôi** (hoặc **Cụm máy**) bấm **Thêm máy mới**, đặt tên, rồi trên máy đó:
- dán **lệnh cài một dòng** vào terminal, hoặc
- dán **hướng dẫn cho agent** vào Claude Code / Antigravity đang chạy trên máy đó để nó tự kiểm công cụ và cài.

Mã tham gia dùng một lần, hết hạn sau 15 phút. Runner cài vào `~/.agenthub` và chạy như dịch vụ của người dùng (LaunchAgent / systemd user). Kiểm: `~/.agenthub/agenthub runner --status`.

Cách cũ (khoá đăng ký, chỉ khi đặt `RUNNER_REGISTRATION_KEY`):
```bash
curl -X POST https://agent.oa.io.vn/api/fleet/register -H "X-Registration-Key: <RUNNER_REGISTRATION_KEY>" \
  -H "Content-Type: application/json" -d '{"name":"macbook","os":"darwin","agents":["CLAUDE","ANTIGRAVITY"]}'
```
