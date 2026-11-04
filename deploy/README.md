# Triển khai AgentHub

Server hiện tại: `192.168.1.36` (Debian 13, arm64), thư mục `~/agenthub`, domain `https://agent.oa.io.vn` qua Cloudflare Tunnel → `http://localhost:1407`.

## Build (trên máy dev)
```bash
(cd web && npx vite build)                 # ra web/dist, được nhúng vào binary
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o agenthub ./cmd/server
```

## Cài lần đầu (trên server)
1. `mkdir -p ~/agenthub/data ~/agenthub/backups`, chép `agenthub` và `backup.py` vào `~/agenthub`.
2. `cp .env.example ~/agenthub/.env && chmod 600 ~/agenthub/.env`, điền giá trị (JWT/webhook/registration key tự sinh bằng `openssl rand -hex 32`).
3. Dịch vụ: thay `__USER__`, `__DIR__` trong `agenthub.service`, chép vào `/etc/systemd/system/`, rồi `sudo systemctl enable --now agenthub`.
4. Sao lưu hằng ngày: `crontab -e` → `15 3 * * * /usr/bin/python3 ~/agenthub/backup.py ~/agenthub/data/agenthub.db ~/agenthub/backups`.

## Cập nhật
Chép binary mới thành `~/agenthub/agenthub.new`, rồi `mv agenthub.new agenthub && sudo systemctl restart agenthub`. Nhật ký: `journalctl -u agenthub -f`.

## Nối máy con (runner)
```bash
# Đăng ký máy (một lần) — lấy runner_token:
curl -X POST https://agent.oa.io.vn/api/fleet/register -H "X-Registration-Key: <RUNNER_REGISTRATION_KEY>" \
  -H "Content-Type: application/json" -d '{"name":"macbook","os":"darwin","agents":["CLAUDE","ANTIGRAVITY"]}'
# Chạy runner trong repo dự án trên máy đó:
AGENTHUB_RUNNER_TOKEN=<runner_token> agenthub runner --server wss://agent.oa.io.vn/ws/agent \
  --agent claude='claude -p {prompt}' --agent antigravity='agy -p {prompt}'
```
