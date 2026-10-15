#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/source"
mkdir -p ../bin
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -buildvcs=false -trimpath -ldflags='-H=windowsgui -s -w -buildid=' -o ../bin/dotPaint_1.3.7_BILINGUAL_x64.exe .
CGO_ENABLED=0 GOOS=windows GOARCH=386 go build -buildvcs=false -trimpath -ldflags='-H=windowsgui -s -w -buildid=' -o ../bin/dotPaint_1.3.7_BILINGUAL_x86.exe .
for f in ../bin/dotPaint_1.3.7_BILINGUAL_x64.exe ../bin/dotPaint_1.3.7_BILINGUAL_x86.exe; do test "$(stat -c%s "$f")" -le 10485760; done
