@echo off
setlocal EnableExtensions
cd /d "%~dp0"
if not exist bin mkdir bin
set "GOOS=windows"
set "GOARCH=amd64"
set "CGO_ENABLED=0"
go -C source build -buildvcs=false -trimpath -ldflags="-H=windowsgui -s -w -buildid=" -o ../bin/dotPaint_2.0.1_x64.exe .
if errorlevel 1 exit /b 1
go -C source build -buildvcs=false -tags devlog -trimpath -ldflags="-H=windowsgui -buildid=" -o ../bin/dotPaint_2.0.1_DEVELOPMENT_LOG_x64.exe .
if errorlevel 1 exit /b 1
for %%I in (bin\dotPaint_2.0.1_x64.exe) do if %%~zI GTR 10485760 (
  echo ERROR: standard EXE exceeds 10 MiB: %%~zI
  exit /b 1
)
echo Build OK: Windows x64 standard ^(10 MiB max^) and developer log ^(unrestricted^).
