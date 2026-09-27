@echo off
setlocal EnableExtensions DisableDelayedExpansion

where go >nul 2>&1
if errorlevel 1 (
    echo [ERROR] Go was not found. Install Go 1.26 or newer and reopen this terminal.
    exit /b 1
)

pushd "%~dp0.."
if errorlevel 1 exit /b 1

if not exist "bin" mkdir "bin"
if not exist "bin" (
    echo [ERROR] Cannot create the bin directory.
    popd
    exit /b 1
)

echo Building ClinePass Auto for Windows...
go build -trimpath -o "bin\auto.exe" ./auto
set "build_result=%errorlevel%"
popd
if not "%build_result%"=="0" exit /b %build_result%

echo Built "%~dp0..\bin\auto.exe"
exit /b 0
