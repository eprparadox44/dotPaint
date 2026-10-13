#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/source"
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags='-H=windowsgui -s -w' -o ../dotPaint_1.3.3_x64.exe .
GOOS=windows GOARCH=386 CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags='-H=windowsgui -s -w' -o ../dotPaint_1.3.3_x86.exe .
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -buildvcs=false -tags devlog -trimpath -ldflags='-H=windowsgui' -o ../dotPaint_1.3.3_DEVELOPMENT_LOG_x64.exe .
for file in ../dotPaint_1.3.3_x64.exe ../dotPaint_1.3.3_x86.exe; do
  bytes=$(wc -c < "$file")
  test "$bytes" -le 10485760 || { echo "Normal release oversize: $file ($bytes bytes)" >&2; exit 1; }
done
