@echo off
REM OpenFood Development Script - Windows Batch version
REM Usage: scripts\dev.bat

setlocal enabledelayedexpansion

set PROJECT_ROOT=%~dp0..
set TOOLS_DIR=%PROJECT_ROOT%\tools
set PG_BIN_DIR=%TOOLS_DIR%\postgresql\bin
set DATA_DIR=%LOCALAPPDATA%\OpenFood-Dev
set PORT=18880

echo === OpenFood Development Environment ===
echo.

REM Check for Go
where go >nul 2>nul
if errorlevel 1 (
    echo [ERRO] Go nao encontrado. Instale em https://golang.org/dl/
    pause
    exit /b 1
)

echo [OK] Go encontrado

REM Download PostgreSQL if needed
if not exist "%PG_BIN_DIR%\postgres.exe" (
    echo [INFO] Baixando PostgreSQL binarios...
    powershell -ExecutionPolicy Bypass -File "%PROJECT_ROOT%\scripts\dev.ps1" -NoHotReload
    goto :run
)

echo [OK] PostgreSQL binarios ja disponiveis

:run
REM Setup environment
set OPENFOOD_DATA_DIR=%DATA_DIR%
set DATABASE_URL=postgres://openfood:devpassword@localhost:5432/openfood?sslmode=disable
set SETUP_TOKEN=dev-setup-token-%random%%random%%random%%random%
set SETUP_TOKEN=!SETUP_TOKEN:~0,32!

echo.
echo [INFO] Data dir: %DATA_DIR%
echo [INFO] SETUP_TOKEN: %SETUP_TOKEN%
echo [INFO] URL: http://127.0.0.1:%PORT%/#setup=%SETUP_TOKEN%
echo.

REM Build
echo [INFO] Compilando...
go build -o .\tmp\openfood.exe .\cmd\openfood
if errorlevel 1 (
    echo [ERRO] Falha na compilacao
    pause
    exit /b 1
)
echo [OK] Build concluido

REM Run
echo [INFO] Iniciando OpenFood (Ctrl+C para parar)...
set PATH=%PG_BIN_DIR%;%PATH%
go run .\cmd\openfood