param(
    [Parameter(Mandatory)][string]$OldInstaller,
    [Parameter(Mandatory)][string]$NewInstaller
)

$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true

function Assert-NoDesktopRegistration {
    $uninstallKey = 'Software\Microsoft\Windows\CurrentVersion\Uninstall\Auto-Test.Desktop_is1'
    foreach ($hive in 'CurrentUser', 'LocalMachine') {
        foreach ($view in 'Registry32', 'Registry64') {
            $registry = [Microsoft.Win32.RegistryKey]::OpenBaseKey($hive, $view)
            try {
                $registration = $registry.OpenSubKey($uninstallKey)
                if ($registration) {
                    $registration.Dispose()
                    throw "Auto-Test is already registered ($hive, $view). Use a clean Windows test machine."
                }
            } finally {
                $registry.Dispose()
            }
        }
    }
}

function Install-Desktop([string]$installer, [string]$phase) {
    $log = Join-Path $scratch "$phase-setup.log"
    $setup = Start-Process -FilePath $installer -ArgumentList ($silent + @(
        '/NOICONS', '/NOCLOSEAPPLICATIONS', '/NORESTARTAPPLICATIONS',
        "/DIR=`"$install`"", "/GROUP=`"$group`"", "/LOG=`"$log`""
    )) -Environment $childEnvironment -WindowStyle Hidden -Wait -PassThru
    if ($setup.ExitCode -ne 0) {
        if (Test-Path -LiteralPath $log) { Get-Content -LiteralPath $log -Tail 25 | Write-Host }
        throw "$phase installer exited with code $($setup.ExitCode)"
    }
}

function Start-Desktop {
    Start-Process -FilePath $runtime -ArgumentList $options -Environment $childEnvironment -WindowStyle Hidden -PassThru
}

function Assert-ControlStopped {
    $connection = [Net.Sockets.TcpClient]::new()
    try {
        $connection.Connect([Net.IPAddress]::Loopback, $port)
        throw 'Old control plane still listens after the overlay'
    } catch [Net.Sockets.SocketException] {
        if ($_.Exception.SocketErrorCode -ne 'ConnectionRefused') { throw }
    } finally {
        $connection.Dispose()
    }
}

Assert-NoDesktopRegistration
$OldInstaller = (Resolve-Path -LiteralPath $OldInstaller).Path
$NewInstaller = (Resolve-Path -LiteralPath $NewInstaller).Path
if ((Get-FileHash -LiteralPath $OldInstaller).Hash -eq (Get-FileHash -LiteralPath $NewInstaller).Hash) {
    throw 'Supply distinct old and new Windows installers'
}

$temp = (Resolve-Path -LiteralPath ([IO.Path]::GetTempPath())).Path.TrimEnd('\')
if ($temp -like 'H:*') { throw 'Set TEMP to a scratch location outside H: before running the upgrade smoke' }
$group = "autotest-upgrade-smoke-$([guid]::NewGuid())"
$scratch = Join-Path $temp $group
$install = Join-Path $scratch 'installed files'
$profileData = Join-Path $scratch 'local app data'
$data = Join-Path $profileData 'Auto-Test'
$runtime = Join-Path $install 'python/pythonw.exe'
$python = Join-Path $install 'python/python.exe'
$launcher = Join-Path $install 'desktop.py'
$smoke = Join-Path $PSScriptRoot '../deploy/smoke.py'
$workerTokenFile = Join-Path $data 'worker-token.txt'
$stateFiles = (Join-Path $data 'platform.db.key'), $workerTokenFile
$silent = '/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART'
$childEnvironment = @{
    LOCALAPPDATA = $profileData
    PLATFORM_WORKER_TOKEN = ''
    PLATFORM_AUTH_USERNAME = ''
    PLATFORM_AUTH_PASSWORD = ''
}
$oldUsername = $env:PLATFORM_AUTH_USERNAME
$oldPassword = $env:PLATFORM_AUTH_PASSWORD
$desktop = $null
$null = New-Item -ItemType Directory -Path $scratch

try {
    $null = New-Item -ItemType Directory -Path $profileData
    $socket = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, 0)
    $socket.Start()
    $port = $socket.LocalEndpoint.Port
    $socket.Stop()
    $server = "http://127.0.0.1:$port"
    $options = "`"$launcher`" --port $port --no-browser"
    $env:PLATFORM_AUTH_USERNAME = 'upgrade-smoke'
    $env:PLATFORM_AUTH_PASSWORD = [Convert]::ToHexString([Security.Cryptography.RandomNumberGenerator]::GetBytes(20))

    Install-Desktop $OldInstaller 'Old'
    $desktop = Start-Desktop
    $fixture = & $python $smoke $server prepare --desktop-payload $install --worker-token-file $workerTokenFile | ConvertFrom-Json
    & $python $smoke $server check $fixture.tenant $fixture.run
    $before = @(Get-FileHash -LiteralPath $stateFiles)
    if ($desktop.HasExited) { throw 'Old desktop exited before the overlay' }

    Install-Desktop $NewInstaller 'Overlay'
    if (-not $desktop.WaitForExit(10000)) { throw 'Overlay did not stop the running old desktop' }
    Assert-ControlStopped

    $desktop = Start-Desktop
    $ready = $false
    for ($attempt = 0; $attempt -lt 60 -and -not $desktop.HasExited; $attempt++) {
        $response = Invoke-WebRequest "$server/api/tenants" -NoProxy -TimeoutSec 2 -SkipHttpErrorCheck -ErrorAction SilentlyContinue
        if ($response.StatusCode -eq 401) {
            $ready = $true
            break
        }
        Start-Sleep -Seconds 1
    }
    if (-not $ready) { throw 'Upgraded desktop did not become ready' }
    & $python $smoke $server check $fixture.tenant $fixture.run
    $after = @(Get-FileHash -LiteralPath $stateFiles)
    if (Compare-Object $before $after -Property Path, Hash) { throw 'Upgrade changed the database key or worker token' }
    'Windows overlay stopped the old desktop and preserved the owner, test run, database key, and worker token'
} catch {
    $desktopLog = Join-Path $data 'desktop.log'
    if (Test-Path -LiteralPath $desktopLog) { Get-Content -LiteralPath $desktopLog -Tail 30 }
    throw
} finally {
    $env:PLATFORM_AUTH_USERNAME = $oldUsername
    $env:PLATFORM_AUTH_PASSWORD = $oldPassword
    if ($desktop -and -not $desktop.HasExited) {
        $stop = Start-Process -FilePath $runtime -ArgumentList "$options --stop" -Environment $childEnvironment -WindowStyle Hidden -Wait -PassThru
        if ($stop.ExitCode -ne 0 -or -not $desktop.WaitForExit(10000)) { throw "Desktop cleanup failed in $scratch" }
    }
    $uninstaller = Join-Path $install 'unins000.exe'
    if (Test-Path -LiteralPath $uninstaller) {
        $remove = Start-Process -FilePath $uninstaller -ArgumentList $silent -Environment $childEnvironment -WindowStyle Hidden -Wait -PassThru
        if ($remove.ExitCode -ne 0 -or (Test-Path -LiteralPath $runtime)) { throw "Uninstall failed in $scratch" }
        Assert-NoDesktopRegistration
    }
    $resolvedScratch = (Resolve-Path -LiteralPath $scratch).Path
    if ($resolvedScratch.StartsWith("$temp\autotest-upgrade-smoke-", [StringComparison]::OrdinalIgnoreCase)) {
        Remove-Item -LiteralPath $resolvedScratch -Recurse -Force
    }
}
