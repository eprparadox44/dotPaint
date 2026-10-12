@echo off
setlocal
cd /d "%~dp0"
if not exist "..\x64" mkdir "..\x64"
if not exist "..\x86" mkdir "..\x86"
set GOOS=windows
set CGO_ENABLED=0
set GOARCH=amd64
go build -buildvcs=false -trimpath -ldflags="-H=windowsgui -s -w" -o "..\x64\dotPaint_1.2.45_CUSTOM_COLOR_EDITOR_x64.exe" .
if errorlevel 1 exit /b 1
go build -buildvcs=false -tags devlog -trimpath -ldflags="-H=windowsgui -s -w" -o "..\x64\dotPaint_1.2.45_DEVELOPMENT_LOG_x64.exe" .
if errorlevel 1 exit /b 1
set GOARCH=386
go build -buildvcs=false -trimpath -ldflags="-H=windowsgui -s -w" -o "..\x86\dotPaint_1.2.45_CUSTOM_COLOR_EDITOR_x86.exe" .
if errorlevel 1 exit /b 1
echo Build complete.
