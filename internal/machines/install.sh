#!/bin/sh
set -eu

BASE_URL="__BASE_URL__"

AGENTHUB_HOME="${AGENTHUB_HOME:-$HOME/.agenthub}"
UNAME_S="${AGENTHUB_UNAME_S:-$(uname -s)}"
UNAME_M="${AGENTHUB_UNAME_M:-$(uname -m)}"

JOIN_CODE=""
WORKER_NAME=""
NO_SERVICE=0

while [ $# -gt 0 ]; do
    case "$1" in
        --join)
            JOIN_CODE="$2"
            shift 2
            ;;
        --join=*)
            JOIN_CODE="${1#*=}"
            shift 1
            ;;
        --name)
            WORKER_NAME="$2"
            shift 2
            ;;
        --name=*)
            WORKER_NAME="${1#*=}"
            shift 1
            ;;
        --no-service)
            NO_SERVICE=1
            shift 1
            ;;
        *)
            shift 1
            ;;
    esac
done

if [ -z "$JOIN_CODE" ]; then
    echo "Lỗi: Cần cung cấp --join <mã tham gia>" >&2
    exit 1
fi

if [ -z "$WORKER_NAME" ]; then
    echo "Lỗi: Cần cung cấp --name <tên máy>" >&2
    exit 1
fi

case "$UNAME_S" in
    Darwin|darwin)
        OS="darwin"
        ;;
    Linux|linux)
        OS="linux"
        ;;
    *)
        echo "Lỗi: Hệ điều hành không được hỗ trợ: $UNAME_S" >&2
        exit 1
        ;;
esac

case "$UNAME_M" in
    arm64|aarch64)
        ARCH="arm64"
        ;;
    x86_64|amd64)
        ARCH="amd64"
        ;;
    *)
        echo "Lỗi: Kiến trúc CPU không được hỗ trợ: $UNAME_M" >&2
        exit 1
        ;;
esac

mkdir -p "$AGENTHUB_HOME"

TARGET_BIN="$AGENTHUB_HOME/agenthub"
TARGET_SHA="$AGENTHUB_HOME/agenthub.sha256"

# Tải binary và checksum
curl -fsSL "${BASE_URL}/download/agenthub-${OS}-${ARCH}" -o "$TARGET_BIN"
curl -fsSL "${BASE_URL}/download/agenthub-${OS}-${ARCH}.sha256" -o "$TARGET_SHA"

# Kiểm tra SHA-256
EXPECTED_HASH=$(awk '{print $1}' "$TARGET_SHA")

ACTUAL_HASH=""
if command -v sha256sum >/dev/null 2>&1; then
    ACTUAL_HASH=$(sha256sum "$TARGET_BIN" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
    ACTUAL_HASH=$(shasum -a 256 "$TARGET_BIN" | awk '{print $1}')
else
    echo "Lỗi: Không tìm thấy công cụ tính SHA-256 (sha256sum hoặc shasum)" >&2
    exit 1
fi

if [ "$ACTUAL_HASH" != "$EXPECTED_HASH" ]; then
    echo "Lỗi: Checksum SHA-256 không khớp!" >&2
    echo "Mong đợi: $EXPECTED_HASH" >&2
    echo "Thực tế:  $ACTUAL_HASH" >&2
    rm -f "$TARGET_BIN" "$TARGET_SHA"
    exit 1
fi

chmod +x "$TARGET_BIN"

# Phát hiện agent và kiểm tra đăng nhập
run_probe() {
    if command -v timeout >/dev/null 2>&1; then
        timeout 60 "$@" >/dev/null 2>&1
    elif command -v gtimeout >/dev/null 2>&1; then
        gtimeout 60 "$@" >/dev/null 2>&1
    else
        "$@" >/dev/null 2>&1
    fi
}

CLAUDE_OK=0
AGY_OK=0

if command -v claude >/dev/null 2>&1; then
    if run_probe claude -p "Trả lời đúng một chữ: OK"; then
        CLAUDE_OK=1
    else
        echo "claude chưa đăng nhập — sau khi đăng nhập chạy: agenthub account add claude"
    fi
fi

if command -v agy >/dev/null 2>&1; then
    if run_probe agy -p "Trả lời đúng một chữ: OK"; then
        AGY_OK=1
    else
        echo "agy chưa đăng nhập — sau khi đăng nhập chạy: agenthub account add antigravity"
    fi
fi

AGENTS=""
AGENTS_JSON_ITEMS=""

if [ "$CLAUDE_OK" = "1" ]; then
    AGENTS='"CLAUDE"'
    AGENTS_JSON_ITEMS='{"name":"claude","kind":"CLAUDE","command":["claude","-p","{prompt}"],"config_dir":"","ever_ok":true}'
fi

if [ "$AGY_OK" = "1" ]; then
    if [ -n "$AGENTS" ]; then
        AGENTS="$AGENTS, \"ANTIGRAVITY\""
    else
        AGENTS='"ANTIGRAVITY"'
    fi
    if [ -n "$AGENTS_JSON_ITEMS" ]; then
        AGENTS_JSON_ITEMS="$AGENTS_JSON_ITEMS,{\"name\":\"antigravity\",\"kind\":\"ANTIGRAVITY\",\"command\":[\"agy\",\"-p\",\"{prompt}\"],\"config_dir\":\"\",\"ever_ok\":true}"
    else
        AGENTS_JSON_ITEMS='{"name":"antigravity","kind":"ANTIGRAVITY","command":["agy","-p","{prompt}"],"config_dir":"","ever_ok":true}'
    fi
fi

AGENTS_FILE="$AGENTHUB_HOME/agents.json"
OLD_UMASK=$(umask)
umask 077
printf '{"agents":[%s]}' "$AGENTS_JSON_ITEMS" > "$AGENTS_FILE"
chmod 600 "$AGENTS_FILE"
umask "$OLD_UMASK"

JOIN_PAYLOAD=$(printf '{"code":"%s","name":"%s","os":"%s","arch":"%s","agents":[%s]}' "$JOIN_CODE" "$WORKER_NAME" "$OS" "$ARCH" "$AGENTS")

if ! JOIN_RESP=$(curl -fsSL -X POST "${BASE_URL}/api/fleet/join" \
    -H "Content-Type: application/json" \
    -d "$JOIN_PAYLOAD"); then
    echo "Lỗi: Gọi /api/fleet/join thất bại" >&2
    exit 1
fi

TOKEN=$(printf '%s' "$JOIN_RESP" | sed -n 's/.*"runner_token"[ ]*:[ ]*"\([^"]*\)".*/\1/p')

if [ -z "$TOKEN" ]; then
    echo "Lỗi: Không nhận được runner_token từ máy chủ" >&2
    exit 1
fi

# Ghi token với quyền 600, không in token
TOKEN_FILE="$AGENTHUB_HOME/token"
OLD_UMASK=$(umask)
umask 077
printf '%s' "$TOKEN" > "$TOKEN_FILE"
chmod 600 "$TOKEN_FILE"
umask "$OLD_UMASK"

# Cài đặt dịch vụ tự chạy (trừ khi --no-service)
if [ "$NO_SERVICE" -ne 1 ]; then
    WS_URL=$(echo "$BASE_URL" | sed -e 's/^https:/wss:/' -e 's/^http:/ws:/')
    # Dịch vụ (launchd/systemd) không có PATH của shell: ghi lại PATH lúc cài để runner tìm được claude/gh/agy.
    RUNNER_PATH="$PATH"
    # Cờ --agent dạng kind=lệnh; kind khớp đuôi id agent trên server (…/claude, …/antigravity).
    AGENT_ARGS=""
    PLIST_AGENTS=""
    if command -v claude >/dev/null 2>&1; then
        AGENT_ARGS="$AGENT_ARGS --agent \"claude=claude -p {prompt}\""
        PLIST_AGENTS="$PLIST_AGENTS        <string>--agent</string>
        <string>claude=claude -p {prompt}</string>
"
    fi
    if command -v agy >/dev/null 2>&1; then
        AGENT_ARGS="$AGENT_ARGS --agent \"antigravity=agy -p {prompt}\""
        PLIST_AGENTS="$PLIST_AGENTS        <string>--agent</string>
        <string>antigravity=agy -p {prompt}</string>
"
    fi

    if [ "$OS" = "darwin" ]; then
        PLIST_DIR="$HOME/Library/LaunchAgents"
        mkdir -p "$PLIST_DIR"
        PLIST="$PLIST_DIR/io.agenthub.runner.plist"
        cat <<EOF > "$PLIST"
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>io.agenthub.runner</string>
    <key>ProgramArguments</key>
    <array>
        <string>caffeinate</string>
        <string>-ims</string>
        <string>$TARGET_BIN</string>
        <string>runner</string>
        <string>--server</string>
        <string>$WS_URL/ws/agent</string>
${PLIST_AGENTS}    </array>
    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>$RUNNER_PATH</string>
    </dict>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>$AGENTHUB_HOME/runner.log</string>
    <key>StandardErrorPath</key>
    <string>$AGENTHUB_HOME/runner.err</string>
</dict>
</plist>
EOF
        launchctl unload "$PLIST" 2>/dev/null || true
        launchctl load "$PLIST" 2>/dev/null || true
    elif [ "$OS" = "linux" ]; then
        SVC_DIR="$HOME/.config/systemd/user"
        mkdir -p "$SVC_DIR"
        SVC="$SVC_DIR/agenthub-runner.service"
        cat <<EOF > "$SVC"
[Unit]
Description=AgentHub Runner
After=network.target

[Service]
Type=simple
Environment="PATH=$RUNNER_PATH"
ExecStart=$TARGET_BIN runner --server $WS_URL/ws/agent$AGENT_ARGS
Restart=always
RestartSec=5

[Install]
WantedBy=default.target
EOF
        if command -v systemctl >/dev/null 2>&1; then
            systemctl --user daemon-reload 2>/dev/null || true
            systemctl --user enable --now agenthub-runner 2>/dev/null || true
        fi
    fi
fi

# Bảng kiểm công cụ
echo ""
echo "=========================================================="
echo "  BẢNG KIỂM CÔNG CỤ HỆ THỐNG"
echo "=========================================================="
if command -v git >/dev/null 2>&1; then
    echo "  [✓] git: Đã cài đặt"
else
    echo "  [✗] git: Chưa có (Gợi ý: cài git qua package manager hệ điều hành)"
fi

if command -v gh >/dev/null 2>&1; then
    if gh auth status >/dev/null 2>&1; then
        echo "  [✓] gh: Đã cài đặt & đã đăng nhập"
    else
        echo "  [!] gh: Đã cài đặt nhưng chưa đăng nhập (Chạy 'gh auth login' để đăng nhập)"
    fi
else
    echo "  [✗] gh: Chưa có (Gợi ý: brew install gh hoặc apt install gh)"
fi

if command -v claude >/dev/null 2>&1; then
    echo "  [✓] claude: Đã cài đặt"
else
    echo "  [✗] claude: Chưa có (Gợi ý: npm install -g @anthropic-ai/claude-code)"
fi

if command -v agy >/dev/null 2>&1; then
    echo "  [✓] agy: Đã cài đặt"
else
    echo "  [✗] agy: Chưa có (Gợi ý: pipx install antigravity-cli)"
fi

if command -v go >/dev/null 2>&1; then
    echo "  [✓] go: Đã cài đặt"
else
    echo "  [✗] go: Chưa có (Gợi ý: brew install go hoặc tải từ golang.org)"
fi

if command -v node >/dev/null 2>&1; then
    echo "  [✓] node: Đã cài đặt"
else
    echo "  [✗] node: Chưa có (Gợi ý: brew install node hoặc tải từ nodejs.org)"
fi
echo "=========================================================="
echo ""
