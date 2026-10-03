param([string]$PostgresArchive = "tools/postgresql.zip", [string]$ISCC = "")
$ErrorActionPreference = 'Stop'
$repoDir = Split-Path $PSScriptRoot -Parent
Set-Location -LiteralPath $repoDir
$pgUrl = 'https://get.enterprisedb.com/postgresql/postgresql-17.11-3-windows-x64-binaries.zip'
$pgHash = '4B8DB0930C38F6EF845DB919551DEDDA3B6B845AEB0927B3D79A6E8E9E4537CF'
New-Item -ItemType Directory -Force tools,dist/windows | Out-Null
if (!(Test-Path -LiteralPath $PostgresArchive)) { Invoke-WebRequest -Uri $pgUrl -OutFile $PostgresArchive }
if ((Get-FileHash -LiteralPath $PostgresArchive -Algorithm SHA256).Hash -ne $pgHash) { throw 'PostgreSQL archive checksum mismatch' }
if (!(Test-Path tools/pg/pgsql/bin/postgres.exe)) { Expand-Archive -LiteralPath $PostgresArchive -DestinationPath tools/pg -Force }
New-Item -ItemType Directory -Force dist/windows/postgresql | Out-Null
foreach ($part in @('bin','lib','share')) { New-Item -ItemType Directory -Force "dist/windows/postgresql/$part" | Out-Null; Copy-Item -Path "tools/pg/pgsql/$part/*" -Destination "dist/windows/postgresql/$part" -Recurse -Force }
foreach ($license in @('server_license.txt','commandlinetools_3rd_party_licenses.txt')) {Copy-Item -LiteralPath "tools/pg/pgsql/$license" -Destination dist/windows/postgresql }
python scripts/bundle-vc-runtime.py
if ($LASTEXITCODE -ne 0) { throw 'VC runtime bundle failed' }
New-Item -ItemType Directory -Force dist/windows/licenses | Out-Null
$modules = go list -m -json all | Out-String
$entries = @($modules -split '(?m)^\}(?=\s*\{)' | ForEach-Object { ($_.TrimEnd() + $(if (!$_.TrimEnd().EndsWith('}')) {'}'} else {''})) | ConvertFrom-Json })
foreach ($module in $entries) { if ($module.Dir) { foreach ($name in @('LICENSE','LICENSE.txt','COPYING')) { $source=Join-Path $module.Dir $name; if (Test-Path -LiteralPath $source) { $fileName=($module.Path -replace '[/\\]','_')+'-'+$name; Copy-Item -LiteralPath $source -Destination (Join-Path 'dist/windows/licenses' $fileName) -Force } } } }
$env:CGO_ENABLED = '0'
go build -trimpath -ldflags '-H windowsgui -s -w -buildid=' -o dist/windows/OpenFood.exe ./cmd/openfood
if ($LASTEXITCODE -ne 0) { throw 'Go build failed' }
if (!$ISCC) { $found = Get-Command iscc.exe -ErrorAction SilentlyContinue; if ($found) { $ISCC=$found.Source } else { $ISCC='C:\Program Files (x86)\Inno Setup 6\ISCC.exe' } }
& $ISCC /Q packaging/windows/openfood.iss
if ($LASTEXITCODE -ne 0) { throw 'Installer build failed' }
Compress-Archive -Path dist/windows/* -DestinationPath dist/OpenFood-0.2.0-alpha.1-windows-x64.zip -Force
Get-ChildItem dist -File | Where-Object { $_.Extension -in '.exe','.zip' } | ForEach-Object { $hash=(Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLower(); "$hash  $($_.Name)" } | Set-Content -Encoding ASCII dist/SHA256SUMS
