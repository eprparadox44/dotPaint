@echo off
setlocal
cd /d "%~dp0"
if not exist "..\x64" mkdir "..\x64"
set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=0
go build -buildvcs=false -trimpath -ldflags="-H=windowsgui -s -w -buildid=" -o "..\x64\dotPaint_1.2.38_RELEASE_NOLOG_x64.exe" .
if errorlevel 1 exit /b 1
go build -buildvcs=false -tags devlog -trimpath -ldflags="-H=windowsgui -s -w" -o "..\x64\dotPaint_1.2.38_DEVELOPMENT_LOG_x64.exe" .
if errorlevel 1 exit /b 1
echo Build complete.
