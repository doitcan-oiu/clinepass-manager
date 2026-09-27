@echo off
setlocal EnableExtensions DisableDelayedExpansion
call "%~dp0start.cmd" --setup-only
exit /b %errorlevel%
