param([string]$Installer='dist/OpenFood-0.1.0-alpha.1-windows-x64-setup.exe')
$ErrorActionPreference='Stop'
$repoDir=Split-Path $PSScriptRoot -Parent
Set-Location -LiteralPath $repoDir
$fixtureId=[Guid]::NewGuid().ToString('N')
$installDir=Join-Path $repoDir ('runtime\Instalação QA com espaços '+$fixtureId)
$dataDir=Join-Path $repoDir ('runtime\Dados instalados ação '+$fixtureId)
$env:OPENFOOD_DATA_DIR=$dataDir
New-Item -ItemType Directory -Force runtime | Out-Null
function Install-Package {
    $p=Start-Process -FilePath (Resolve-Path -LiteralPath $Installer).Path -ArgumentList '/VERYSILENT','/SUPPRESSMSGBOXES','/NORESTART',('/DIR="'+$installDir+'"') -WindowStyle Hidden -PassThru
    if (!$p.WaitForExit(90000)) {throw 'Installer timeout'}
    if ($p.ExitCode -ne 0) {throw "Installer failed: $($p.ExitCode)"}
}
function Start-App {
    $p=Start-Process -FilePath (Join-Path $installDir 'OpenFood.exe') -ArgumentList '--background' -WindowStyle Hidden -PassThru
    for ($i=0;$i -lt 90;$i++) {
        if (Test-Path -LiteralPath (Join-Path $dataDir 'runtime.json')) {
            $state=Get-Content -LiteralPath (Join-Path $dataDir 'runtime.json') -Raw -Encoding UTF8 | ConvertFrom-Json
            try { $ready=Invoke-RestMethod -Uri ($state.URL+'/readyz') -TimeoutSec 2; if ($ready.status -eq 'ready') {return @{Process=$p;URL=$state.URL}} } catch {}
        }
        if ($p.HasExited) {throw 'Application exited before readiness'}
        Start-Sleep -Milliseconds 1000
    }
    throw 'Application readiness timeout'
}
function Login-App([string]$Url){
    $body=@{email='installed-qa@example.org';password=$script:testPassword}|ConvertTo-Json
    Invoke-RestMethod -Uri ($Url+'/api/login') -Method Post -Headers @{'X-OpenFood'='1'} -ContentType application/json -Body $body -SessionVariable qaSession | Out-Null
    $script:session=$qaSession
}
function Stop-App($State){
    Invoke-RestMethod -Uri ($State.URL+'/api/shutdown') -Method Post -Headers @{'X-OpenFood'='1'} -WebSession $script:session | Out-Null
    if (!$State.Process.WaitForExit(45000)) {throw 'Graceful shutdown timeout'}
}
Install-Package
$occupied=[System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback,18880)
try {$occupied.Start()} catch {throw 'Test requires port 18880 initially free'}
try {
    $state=Start-App
    if ($state.URL -eq 'http://127.0.0.1:18880') {throw 'Occupied port was reused'}
    $config=Get-Content -LiteralPath (Join-Path $dataDir 'config.json') -Raw -Encoding UTF8 | ConvertFrom-Json
    $script:testPassword=[Guid]::NewGuid().ToString('N')
    $body=@{token=$config.setup_token;email='installed-qa@example.org';password=$script:testPassword;store='Loja instalada QA'}|ConvertTo-Json
    Invoke-RestMethod -Uri ($state.URL+'/api/setup') -Method Post -Headers @{'X-OpenFood'='1'} -ContentType application/json -Body $body | Out-Null
    Login-App $state.URL
    $p=Start-Process -FilePath (Join-Path $installDir 'OpenFood.exe') -ArgumentList '--background' -WindowStyle Hidden -PassThru
    if (!$p.WaitForExit(5000)) {throw 'Second instance did not exit'}
    $busy=Start-Process -FilePath (Join-Path $installDir 'OpenFood.exe') -ArgumentList '--prepare-update' -WindowStyle Hidden -PassThru
    if (!$busy.WaitForExit(5000) -or $busy.ExitCode -ne 3) {throw 'Update with running app was not blocked'}
    $product=Invoke-RestMethod -Uri ($state.URL+'/api/products') -Method Post -Headers @{'X-OpenFood'='1'} -WebSession $session -ContentType application/json -Body (@{name='Produto instalado QA';price_cents=2500;stock=4}|ConvertTo-Json)
    $order=Invoke-RestMethod -Uri ($state.URL+'/api/orders') -Method Post -Headers @{'X-OpenFood'='1';'Idempotency-Key'='installed-qa-order'} -WebSession $session -ContentType application/json -Body (@{items=@(@{product_id=$product.id;quantity=2})}|ConvertTo-Json -Depth 5)
    # Kill only this isolated test application's process, never a user process.
    Stop-Process -Id $state.Process.Id -Force
    & (Join-Path $installDir 'postgresql\bin\pg_ctl.exe') stop -D (Join-Path $dataDir 'database') -m immediate -w | Out-Null
    if ($LASTEXITCODE -ne 0) {throw 'Test database immediate stop failed'}
    $state=Start-App
    Login-App $state.URL
    $orders=Invoke-RestMethod -Uri ($state.URL+'/api/orders') -WebSession $session
    if ($orders.Count -ne 1 -or $orders[0].total_cents -ne 5000) {throw 'Order lost after crash'}
    Stop-App $state
    Install-Package
    if (@(Get-ChildItem -LiteralPath (Join-Path $dataDir 'backups') -Filter '*.dump').Count -lt 1) {throw 'No pre-update backup'}
    $state=Start-App
    Login-App $state.URL
    $orders=Invoke-RestMethod -Uri ($state.URL+'/api/orders') -WebSession $session
    if ($orders.Count -ne 1 -or $orders[0].total_cents -ne 5000) {throw 'Reinstallation lost data'}
    Stop-App $state
    $uninstall=Start-Process -FilePath (Join-Path $installDir 'unins000.exe') -ArgumentList '/VERYSILENT','/SUPPRESSMSGBOXES','/NORESTART' -WindowStyle Hidden -PassThru
    if (!$uninstall.WaitForExit(60000) -or $uninstall.ExitCode -ne 0) {throw 'Uninstall failed'}
    if (!(Test-Path -LiteralPath (Join-Path $dataDir 'database\PG_VERSION'))) {throw 'Uninstall erased database'}
    $result=@{date='2026-10-02';status='PASS';environment='Windows com ferramentas; não VM limpa';passed=@('installer','accented paths','occupied HTTP port','second instance','update blocked while running','manual order','process crash','PostgreSQL immediate stop and WAL recovery','graceful shutdown','same-version reinstall with backup','data preserved','uninstall preserves data');pending=@('Windows 10/11 clean VM','standard non-admin account','physical reboot','upgrade across different schemas','tray visual inspection','Authenticode signing')}
    $result|ConvertTo-Json -Depth 5|Set-Content -Encoding UTF8 docs/evidence/windows-installer-results.json
    $result|ConvertTo-Json -Depth 5
} finally {$occupied.Stop()}
