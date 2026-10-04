<# 
.SYNOPSIS
    OpenFood Development Script - Runs with embedded PostgreSQL (Windows)
    Usage: .\scripts\dev.ps1
    Supports hot reload with 'air' if installed, otherwise uses 'go run'
#>

param(
    [switch]$NoHotReload,
    [string]$DataDir = "$env:LOCALAPPDATA\OpenFood-Dev",
    [string]$Port = "18880"
)

$ErrorActionPreference = "Stop"

# Colors
$Green  = [ConsoleColor]::Green
$Yellow = [ConsoleColor]::Yellow
$Red    = [ConsoleColor]::Red
$Cyan   = [ConsoleColor]::Cyan

function Write-Color($msg, $color) {
    $orig = $Host.UI.RawUI.ForegroundColor
    $Host.UI.RawUI.ForegroundColor = $color
    Write-Host $msg
    $Host.UI.RawUI.ForegroundColor = $orig
}

function Check-Command($name, $installHint) {
    if (Get-Command $name -ErrorAction SilentlyContinue) {
        Write-Color "[OK] $name encontrado" $Green
        return $true
    } else {
        Write-Color "[AVISO] $name NAO encontrado - $installHint" $Yellow
        return $false
    }
}

function Check-PostgresBinaries {
    $pgBinDir = Join-Path (Get-Location) "tools\postgresql\bin"
    if (Test-Path "$pgBinDir\postgres.exe") {
        return $pgBinDir
    }
    return $null
}

function Setup-DevEnvironment {
    param($dataDir, $pgBinDir)
    
    Write-Color "Configurando ambiente de desenvolvimento..." $Cyan
    
    # Create data directory
    if (-not (Test-Path $dataDir)) {
        New-Item -ItemType Directory -Path $dataDir -Force | Out-Null
    }
    
    # Set environment variables
    $env:OPENFOOD_DATA_DIR = $dataDir
    $env:DATABASE_URL = "postgres://openfood:devpassword@localhost:5432/openfood?sslmode=disable"
    $env:SETUP_TOKEN = "dev-setup-token-" + [System.Guid]::NewGuid().ToString("N").Substring(0, 32)
    
    Write-Color "Data dir: $dataDir" $Cyan
    Write-Color "PostgreSQL bin: $pgBinDir" $Cyan
    Write-Color "SETUP_TOKEN: $env:SETUP_TOKEN" $Yellow
    Write-Color "" $Cyan
    Write-Color "========================================" $Yellow
    Write-Color "IMPORTANTE: Use este SETUP_TOKEN no primeiro acesso!" $Yellow
    Write-Color "URL: http://127.0.0.1:$Port/#setup=$env:SETUP_TOKEN" $Yellow
    Write-Color "========================================" $Yellow
    Write-Color "" $Cyan
    
    return $env:SETUP_TOKEN
}

function Run-App {
    param($pgBinDir, $noHotReload)
    
    # Ensure pg bin is in PATH for the runtime
    $env:PATH = "$pgBinDir;$env:PATH"
    
    $hasAir = Check-Command "air" "go install github.com/air-verse/air@latest"
    
    if ($noHotReload -or -not $hasAir) {
        Write-Color "Rodando com 'go run' (sem hot reload automatico)..." $Yellow
        Write-Color "Pressione Ctrl+C para parar" $Cyan
        go run ./cmd/openfood
    } else {
        Write-Color "Rodando com 'air' (hot reload ativo)..." $Green
        Write-Color "Pressione Ctrl+C para parar" $Cyan
        
        # Create air config if not exists
        $airConfig = ".air.toml"
        if (-not (Test-Path $airConfig)) {
            @"
root = "."
tmp_dir = "tmp"
build_args_bin = ["-o", "./tmp/openfood.exe", "./cmd/openfood"]
build_cmd = "go build -o ./tmp/openfood.exe ./cmd/openfood"
build_delay = 1000
build_exclude_dir = ["tmp", "dist", "tools", ".git"]
build_include_ext = ["go", "tpl", "tmpl", "html"]
build_log = "build-errors.log"
color_main = "cyan"
color_watcher = "yellow"
color_build = "green"
color_build_success = "green"
color_build_error = "red"
color_app = "blue"
delay = 1000
exclude_dir = ["tmp", "dist", "tools", ".git", "node_modules"]
include_ext = ["go", "tpl", "tmpl", "html"]
log_file = "air.log"
poll = true
poll_interval = 500
stop_on_error = false
"@ | Set-Content $airConfig
            Write-Color "Configuracao do air criada: $airConfig" $Cyan
        }
        
        air -c $airConfig
    }
}

# ===== MAIN =====
Write-Color "=== OpenFood Development Environment ===" $Cyan
Write-Color "" $Cyan

# Check prerequisites
$hasGo = Check-Command "go" "Instale Go 1.26+ de https://golang.org/dl/"
if (-not $hasGo) { exit 1 }

$hasGit = Check-Command "git" "Instale Git"

# Check PostgreSQL binaries
$pgBinDir = Check-PostgresBinaries
if (-not $pgBinDir) {
    Write-Color "" $Red
    Write-Color "========================================" $Red
    Write-Color "POSTGRESQL BINARIOS NAO ENCONTRADOS" $Red
    Write-Color "========================================" $Red
    Write-Color "" $Yellow
    Write-Color "Opcao 1 - Baixar e extrair manualmente:" $Cyan
    Write-Color "  1. Acesse: https://www.postgresql.org/download/windows/" $Cyan
    Write-Color "  2. Baixe 'PostgreSQL 17.x Windows x86-64 Binaries'" $Cyan
    Write-Color "  3. Extraia o ZIP" $Cyan
    Write-Color "  4. Copie a pasta 'pgsql/bin' para: .\tools\postgresql\bin\" $Cyan
    Write-Color "" $Yellow
    Write-Color "Opcao 2 - Usar winget (recomendado):" $Cyan
    Write-Color "  winget install PostgreSQL.PostgreSQL" $Cyan
    Write-Color "  Depois copie: C:\Program Files\PostgreSQL\17\bin -> .\tools\postgresql\bin" $Cyan
    Write-Color "" $Yellow
    Write-Color "Opcao 3 - Chocolatey:" $Cyan
    Write-Color "  choco install postgresql" $Cyan
    Write-Color "" $Red
    Write-Color "Estrutura esperada:" $Cyan
    Write-Color "  tools\" $Cyan
    Write-Color "  +-- postgresql\" $Cyan
    Write-Color "      +-- bin\" $Cyan
    Write-Color "          +-- postgres.exe" $Cyan
    Write-Color "          +-- initdb.exe" $Cyan
    Write-Color "          +-- pg_ctl.exe" $Cyan
    Write-Color "          +-- pg_dump.exe" $Cyan
    Write-Color "          +-- pg_restore.exe" $Cyan
    Write-Color "          +-- ... (outros .exe e .dll)" $Cyan
    Write-Color "" $Red
    exit 1
}

Write-Color "[OK] PostgreSQL binarios encontrados em: $pgBinDir" $Green

# Setup dev environment
$setupToken = Setup-DevEnvironment $DataDir $pgBinDir

# Build first to verify everything compiles
Write-Color "Compilando..." $Cyan
go build -o ./tmp/openfood.exe ./cmd/openfood
if ($LASTEXITCODE -ne 0) {
    Write-Color "Erro na compilacao!" $Red
    exit 1
}
Write-Color "Build OK!" $Green

# Run
Run-App -pgBinDir $pgBinDir -noHotReload $NoHotReload