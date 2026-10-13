@echo off
setlocal
cd /d "%~dp0source"
set CGO_ENABLED=0
set GOOS=windows
set GOARCH=amd64
go build -buildvcs=false -trimpath -ldflags="-H=windowsgui -s -w" -o "..\dotPaint_1.3.3_x64.exe" .
if errorlevel 1 exit /b 1
set GOARCH=386
go build -buildvcs=false -trimpath -ldflags="-H=windowsgui -s -w" -o "..\dotPaint_1.3.3_x86.exe" .
if errorlevel 1 exit /b 1
set GOARCH=amd64
go build -buildvcs=false -tags devlog -trimpath -ldflags="-H=windowsgui" -o "..\dotPaint_1.3.3_DEVELOPMENT_LOG_x64.exe" .
if errorlevel 1 exit /b 1
powershell -NoProfile -Command "$p = '%~dp0'; @('dotPaint_1.3.3_x64.exe','dotPaint_1.3.3_x86.exe') | %% { $f=Join-Path $p $_; if ((Get-Item $f).Length -gt 10485760) { throw ('Oversize: '+$_) } }"
if errorlevel 1 exit /b 1
echo dotPaint 1.3.3 all builds completed.
