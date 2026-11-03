#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/source"
mkdir -p ../bin
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -buildvcs=false -trimpath -ldflags='-H=windowsgui -s -w -buildid=' -o ../bin/dotPaint_1.3.9_x64.exe .
CGO_ENABLED=0 GOOS=windows GOARCH=386 go build -buildvcs=false -trimpath -ldflags='-H=windowsgui -s -w -buildid=' -o ../bin/dotPaint_1.3.9_x86.exe .
for f in ../bin/dotPaint_1.3.9_x64.exe ../bin/dotPaint_1.3.9_x86.exe; do
    actual=$(stat -c%s "$f")
    if ((actual > 10485760)); then echo "ERROR: 10 MiB exceeded: $f ($actual)" >&2; exit 2; fi
    echo "$f: ${actual} bytes (<= 10485760)"
done
