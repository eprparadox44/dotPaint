#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
mkdir -p bin
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go -C source build -buildvcs=false -trimpath -ldflags='-H=windowsgui -s -w -buildid=' -o ../bin/dotPaint_2.0.1_x64.exe .
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go -C source build -buildvcs=false -tags devlog -trimpath -ldflags='-H=windowsgui -buildid=' -o ../bin/dotPaint_2.0.1_DEVELOPMENT_LOG_x64.exe .
size=$(wc -c < bin/dotPaint_2.0.1_x64.exe)
echo "release Windows x64: $size bytes (cap 10 MiB = 10,485,760 bytes)"
if ((size > 10485760)); then echo 'ERROR: release exe exceeds 10 MiB' >&2; exit 1; fi
echo "Development log build is exempt from the capacity limit."
