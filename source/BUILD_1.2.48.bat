@echo off
setlocal
cd /d "%~dp0"
if not exist "..\x64" mkdir "..\x64"
if not exist "..\x86" mkdir "..\x86"
set GOOS=windows
set CGO_ENABLED=0
set GOARCH=amd64
go build -buildvcs=false -trimpath -ldflags="-H=windowsgui -s -w" -o "..\x64\dotPaint_1.2.48_BUGFIX_x64.exe" .
if errorlevel 1 exit /b 1
go build -buildvcs=false -tags devlog -trimpath -ldflags="-H=windowsgui" -o "..\x64\dotPaint_1.2.48_DEVELOPMENT_LOG_x64.exe" .
if errorlevel 1 exit /b 1
set GOARCH=386
go build -buildvcs=false -trimpath -ldflags="-H=windowsgui -s -w" -o "..\x86\dotPaint_1.2.48_BUGFIX_x86.exe" .
if errorlevel 1 exit /b 1
for %%F in ("..\x64\dotPaint_1.2.48_BUGFIX_x64.exe" "..\x86\dotPaint_1.2.48_BUGFIX_x86.exe") do (
  if %%~zF GTR 10000000 echo ERROR: %%F exceeds 10000000 bytes && exit /b 1
)
echo dotPaint 1.2.48 build complete.
endlocal
