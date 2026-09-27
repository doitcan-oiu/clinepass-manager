@echo off
setlocal EnableExtensions DisableDelayedExpansion
chcp 65001 >nul
set "PYTHONUTF8=1"
set "PYTHONIOENCODING=utf-8"
set "PYTHONUNBUFFERED=1"
set "PIP_DISABLE_PIP_VERSION_CHECK=1"

pushd "%~dp0.."
if errorlevel 1 exit /b 1
set "worker_python=%CD%\auto\worker\.venv\Scripts\python.exe"

if /i "%~1"=="--health-check" goto health_check
if /i "%~1"=="--setup-only" goto setup_only
if /i "%~1"=="--build-only" goto build_only

call :check_go
if errorlevel 1 goto failed
call :setup_worker
if errorlevel 1 goto failed
call :build_auto
if errorlevel 1 goto failed

rem Always launch this instance with the environment prepared above.
set "LOGIN_PYTHON=%worker_python%"
echo Starting ClinePass Auto. Press Ctrl+C to stop.
"bin\auto.exe" %*
set "auto_result=%errorlevel%"
goto finished

:setup_only
call :setup_worker
set "auto_result=%errorlevel%"
goto finished

:build_only
call :check_go
if errorlevel 1 goto failed
call :build_auto
set "auto_result=%errorlevel%"
goto finished

:health_check
rem A health check must never install dependencies, rebuild, or start a service.
if not exist "bin\auto.exe" (
    echo [ERROR] Auto is not built. Run build.cmd before checking its health.
    goto failed
)
if "%~2"=="" (
    echo [ERROR] Usage: start.cmd --health-check URL
    goto failed
)
if not "%~3"=="" (
    echo [ERROR] Usage: start.cmd --health-check URL
    goto failed
)
"bin\auto.exe" --health-check "%~2"
set "auto_result=%errorlevel%"
goto finished

:failed
set "auto_result=1"

:finished
popd
exit /b %auto_result%

:check_go
where go >nul 2>&1
if errorlevel 1 (
    echo [ERROR] Go was not found. Install Go 1.26 or newer from https://go.dev/dl/
    echo Reopen your terminal after installing Go, then run start.cmd again.
    exit /b 1
)
exit /b 0

:build_auto
if not exist "bin" mkdir "bin"
if not exist "bin" (
    echo [ERROR] Cannot create the bin directory.
    exit /b 1
)
echo Building the latest ClinePass Auto for Windows...
go build -trimpath -o "bin\auto.exe" ./auto
if errorlevel 1 (
    echo [ERROR] Build failed. Go 1.26 or newer is required.
    echo If Auto is running, stop it before rebuilding. Resolve the error above and retry.
    exit /b 1
)
echo Built "%CD%\bin\auto.exe"
exit /b 0

:setup_worker
set "base_python="
set "base_args="
if exist "%worker_python%" goto check_venv
if not defined LOGIN_PYTHON goto find_python
"%LOGIN_PYTHON%" -c "import sys; sys.exit(0 if sys.version_info >= (3, 10) else 1)" >nul 2>&1
if errorlevel 1 goto find_python
set "base_python=%LOGIN_PYTHON%"
goto create_venv

:find_python
py -3 -c "import sys; sys.exit(0 if sys.version_info >= (3, 10) else 1)" >nul 2>&1
if not errorlevel 1 (
    set "base_python=py"
    set "base_args=-3"
    goto create_venv
)
python -c "import sys; sys.exit(0 if sys.version_info >= (3, 10) else 1)" >nul 2>&1
if not errorlevel 1 (
    set "base_python=python"
    goto create_venv
)
python3 -c "import sys; sys.exit(0 if sys.version_info >= (3, 10) else 1)" >nul 2>&1
if not errorlevel 1 (
    set "base_python=python3"
    goto create_venv
)
echo [ERROR] Python 3.10 or newer was not found.
echo Install Python from https://www.python.org/downloads/windows/
echo Enable "Add python.exe to PATH", reopen your terminal, and run start.cmd again.
exit /b 1

:create_venv
echo Creating the Auto Python environment...
"%base_python%" %base_args% -m venv "auto\worker\.venv"
if errorlevel 1 (
    echo [ERROR] Cannot create the Auto Python environment. Resolve the error above and retry.
    exit /b 1
)

:check_venv
"%worker_python%" -c "import sys; sys.exit(0 if sys.version_info >= (3, 10) else 1)"
if errorlevel 1 (
    echo [ERROR] The existing worker\.venv does not contain a working Python 3.10 or newer.
    echo Rename worker\.venv as a backup, then run start.cmd again.
    exit /b 1
)
echo Installing Auto worker dependencies...
"%worker_python%" -m pip install --disable-pip-version-check -r "auto\worker\requirements.txt"
if errorlevel 1 (
    echo [ERROR] Python dependency installation failed. Resolve the error above and retry.
    exit /b 1
)
"%worker_python%" -c "import cloakbrowser; import playwright.sync_api; print('Auto Python dependencies are ready.')"
if errorlevel 1 (
    echo [ERROR] Python dependency verification failed. Resolve the error above and retry.
    exit /b 1
)
exit /b 0
