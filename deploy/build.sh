#!/usr/bin/env bash
# Build bản phát hành: web nhúng + binary server (linux/arm64) + runner cho 4 nền tảng kèm .sha256.
# Dùng: deploy/build.sh [thư-mục-ra]   (mặc định ./bin/release)
# Phiên bản = git describe (DEC-19: runner khác server_version sẽ tự cập nhật khi rảnh).
set -euo pipefail
cd "$(dirname "$0")/.."
OUT="${1:-bin/release}"
VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
LDFLAGS="-s -w -X github.com/namth/agenthub/internal/runner.Version=${VERSION}"
mkdir -p "$OUT/runners"
(cd web && { [ -d node_modules ] || npm ci --no-audit --no-fund; } && npx vite build >/dev/null)
for target in darwin/arm64 darwin/amd64 linux/arm64 linux/amd64; do
  os=${target%/*}; arch=${target#*/}
  f="$OUT/runners/agenthub-${os}-${arch}"
  GOOS=$os GOARCH=$arch CGO_ENABLED=0 go build -trimpath -ldflags "$LDFLAGS" -o "$f" ./cmd/server
  (cd "$OUT/runners" && shasum -a 256 "agenthub-${os}-${arch}" > "agenthub-${os}-${arch}.sha256")
done
cp "$OUT/runners/agenthub-${SERVER_TARGET:-linux-arm64}" "$OUT/agenthub"
echo "Phiên bản $VERSION → $OUT/agenthub (server) và $OUT/runners/ (chép vào RUNNERS_DIR trên server)"
