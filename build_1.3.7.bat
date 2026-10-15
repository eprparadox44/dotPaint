@echo off
setlocal
cd /d "%~dp0source"
set GOOS=windows
set CGO_ENABLED=0
if not exist "..\bin" mkdir "..\bin"
set GOARCH=amd64
go build -buildvcs=false -trimpath -ldflags="-H=windowsgui -s -w -buildid=" -o "..\bin\dotPaint_1.3.7_BILINGUAL_x64.exe" .
if errorlevel 1 exit /b 1
set GOARCH=386
go build -buildvcs=false -trimpath -ldflags="-H=windowsgui -s -w -buildid=" -o "..\bin\dotPaint_1.3.7_BILINGUAL_x86.exe" .
if errorlevel 1 exit /b 1
powershell -NoProfile -Command "Get-ChildItem '..\bin\*.exe' | ForEach-Object {if ($_.Length -gt 10485760) { Write-Error ('10MiB limit exceeded: ' + $_.Name); exit 2}}"
if errorlevel 1 exit /b 2
echo Build complete.
