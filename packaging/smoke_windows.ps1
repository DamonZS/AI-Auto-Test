param([Parameter(Mandatory)][string]$Installer)

$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true
$Installer = (Resolve-Path -LiteralPath $Installer).Path
$existing = Get-ItemProperty -LiteralPath 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\Auto-Test.Desktop_is1' -ErrorAction SilentlyContinue
if ($existing) { throw 'Use a clean Windows user without an Auto-Test installation' }
$scratch = Join-Path ([IO.Path]::GetTempPath()) "autotest-smoke-$([guid]::NewGuid())"
$null = New-Item -ItemType Directory -Path $scratch
$scratch = (Resolve-Path -LiteralPath $scratch).Path
$install = Join-Path $scratch 'installed files'
$data = Join-Path $scratch 'user data'
$runtime = Join-Path $install 'python/pythonw.exe'
$stopwatch = [Diagnostics.Stopwatch]::StartNew()
$oldUsername = $env:PLATFORM_AUTH_USERNAME
$oldPassword = $env:PLATFORM_AUTH_PASSWORD

try {
    $setup = Start-Process -FilePath $Installer -ArgumentList @('/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', '/NOICONS', "/DIR=`"$install`"") -WindowStyle Hidden -Wait -PassThru
    if ($setup.ExitCode -ne 0) { throw "Installer failed with exit code $($setup.ExitCode)" }
    $forbidden = Get-ChildItem -LiteralPath $install -Recurse -File | Where-Object {
        $_.Name -match '^\.env($|\.)|\.(db(-.*)?|sqlite3?|key|xlsx|har|pem|pfx)$' -and
            $_.FullName -notmatch '[\\/]certifi[\\/]cacert\.pem$'
    }
    if ($forbidden) { throw "Unexpected installed files: $($forbidden.Name -join ', ')" }
    $null = New-Item -ItemType Directory -Path $data
    $database = Join-Path $data 'platform.db'
    $env:PLATFORM_AUTH_USERNAME = 'deployment-smoke'
    $env:PLATFORM_AUTH_PASSWORD = [Convert]::ToHexString([Security.Cryptography.RandomNumberGenerator]::GetBytes(20))
    $provisionLog = Join-Path $scratch 'provision.log'
    $provision = Start-Process -FilePath (Join-Path $install 'autotest-server.exe') -ArgumentList "-db `"$database`" -provision-user -auth-tenant deployment-smoke -auth-tenant-name `"Deployment smoke`"" -RedirectStandardError $provisionLog -WindowStyle Hidden -Wait -PassThru
    if ($provision.ExitCode -ne 0) {
        Get-Content -LiteralPath $provisionLog
        throw "Owner provisioning failed with exit code $($provision.ExitCode)"
    }

    $socket = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, 0)
    $socket.Start()
    $port = $socket.LocalEndpoint.Port
    $socket.Stop()
    $server = "http://127.0.0.1:$port"
    $launcher = Join-Path $install 'desktop.py'
    $options = "`"$launcher`" --data-dir `"$data`" --port $port --no-browser"
    $desktop = Start-Process -FilePath $runtime -ArgumentList $options -Environment @{ PLATFORM_WORKER_TOKEN = ''; PLATFORM_AUTH_PASSWORD = ''; PLATFORM_AUTH_USERNAME = '' } -WindowStyle Hidden -PassThru
    $smoke = Join-Path $PSScriptRoot '../deploy/smoke.py'
    $fixture = python $smoke $server prepare --worker-token-file (Join-Path $data 'worker-token.txt') | ConvertFrom-Json
    python $smoke $server check $fixture.tenant $fixture.run
    foreach ($file in 'platform.db', 'platform.db.key') {
        if (-not (Test-Path -LiteralPath (Join-Path $data $file))) { throw "Missing runtime file $file" }
    }
    "Windows installed smoke passed in $([int]$stopwatch.Elapsed.TotalSeconds)s"
} catch {
    $desktopLog = Join-Path $data 'desktop.log'
    if (Test-Path -LiteralPath $desktopLog) { Get-Content -LiteralPath $desktopLog }
    throw
} finally {
    $env:PLATFORM_AUTH_USERNAME = $oldUsername
    $env:PLATFORM_AUTH_PASSWORD = $oldPassword
    if ($desktop) {
        $stop = Start-Process -FilePath $runtime -ArgumentList "$options --stop" -WindowStyle Hidden -Wait -PassThru
        if ($stop.ExitCode -ne 0 -or -not $desktop.WaitForExit(10000)) { throw 'Desktop stop failed' }
        $connection = [Net.Sockets.TcpClient]::new()
        try {
            $connection.Connect([Net.IPAddress]::Loopback, $port)
            throw 'Control plane still listens after stop'
        } catch [Net.Sockets.SocketException] {
            if ($_.Exception.SocketErrorCode -ne 'ConnectionRefused') { throw }
        } finally {
            $connection.Dispose()
        }
    }
    $uninstaller = Join-Path $install 'unins000.exe'
    if (Test-Path -LiteralPath $uninstaller) {
        $remove = Start-Process -FilePath $uninstaller -ArgumentList '/VERYSILENT /SUPPRESSMSGBOXES /NORESTART' -Environment @{ LOCALAPPDATA = $scratch } -WindowStyle Hidden -Wait -PassThru
        if ($remove.ExitCode -ne 0 -or (Test-Path -LiteralPath $runtime)) { throw 'Uninstall failed' }
        'Windows stop and uninstall passed'
    }
    $temp = (Resolve-Path -LiteralPath ([IO.Path]::GetTempPath())).Path.TrimEnd('\')
    if ($scratch.StartsWith("$temp\autotest-smoke-", [StringComparison]::OrdinalIgnoreCase)) {
        Remove-Item -LiteralPath $scratch -Recurse -Force
    }
}
