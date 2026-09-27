@echo off
setlocal EnableExtensions DisableDelayedExpansion

if not exist "%~dp0..\bin\auto.exe" (
    call "%~dp0build.cmd"
    if errorlevel 1 exit /b 1
)

pushd "%~dp0.."
if errorlevel 1 exit /b 1

"bin\auto.exe" %*
set "auto_result=%errorlevel%"
popd
exit /b %auto_result%
