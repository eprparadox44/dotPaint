@echo off
setlocal
cd /d "%~dp0source"
set GOOS=windows
set CGO_ENABLED=0
if not exist "..\bin" mkdir "..\bin"
set GOARCH=amd64
go build -buildvcs=false -trimpath -ldflags="-H=windowsgui -s -w -buildid=" -o "..\bin\dotPaint_1.3.9_x64.exe" .
if errorlevel 1 exit /b 1
set GOARCH=386
go build -buildvcs=false -trimpath -ldflags="-H=windowsgui -s -w -buildid=" -o "..\bin\dotPaint_1.3.9_x86.exe" .
if errorlevel 1 exit /b 1
powershell -NoProfile -Command "$names = @('dotPaint_1.3.9_x64.exe', 'dotPaint_1.3.9_x86.exe'); foreach ($n in $names) {$p = Join-Path '..\bin' $n; if ((Get-Item -LiteralPath $p).Length -gt 10485760) {Write-Error ('10 MiB limit exceeded: ' + $n); exit 2}}"
if errorlevel 1 exit /b 2
echo dotPaint 1.3.9 build complete.
