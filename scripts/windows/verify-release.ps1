# Only install packages and create test adapters on a disposable CI runner.
$ErrorActionPreference = 'Stop'
if ($env:GITHUB_ACTIONS -ne 'true') { throw 'This check requires a disposable CI runner.' }
if ($env:RELEASE_TAG -notmatch '^v\d+\.\d+\.\d+$') { throw 'Expected a release tag' }
$version = $env:RELEASE_TAG.Substring(1)
$work = Join-Path $env:RUNNER_TEMP 'naga-release-check'
$install = Join-Path $work 'installed'
New-Item -ItemType Directory -Path $work -Force | Out-Null
$metadata = gh api "repos/$env:GITHUB_REPOSITORY/releases/tags/$env:RELEASE_TAG" | ConvertFrom-Json
if ($LASTEXITCODE -ne 0) { throw 'Cannot read release metadata' }
$names = @("NagaNetwork-$version-windows-x64-setup.exe", "NagaNetwork-$version-windows-x64.zip")
foreach ($name in $names) {
    gh release download $env:RELEASE_TAG --repo $env:GITHUB_REPOSITORY --pattern $name --dir $work
    if ($LASTEXITCODE -ne 0) { throw "Cannot download $name" }
    $asset = $metadata.assets | Where-Object name -eq $name
    $hash = 'sha256:' + (Get-FileHash -LiteralPath (Join-Path $work $name) -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($hash -ne $asset.digest) { throw "Incorrect asset digest: $name" }
}
$setup = Start-Process -FilePath (Join-Path $work $names[0]) -ArgumentList @('/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', "/DIR=$install", "/LOG=$work/install.log") -WindowStyle Hidden -Wait -PassThru
if ($setup.ExitCode -ne 0) { throw "Setup exit code: $($setup.ExitCode)" }
try {
    $portable = Join-Path $work 'portable'
    Expand-Archive -LiteralPath (Join-Path $work $names[1]) -DestinationPath $portable
    $files = @(Get-ChildItem -LiteralPath $portable -File -Recurse)
    foreach ($file in $files) {
        $relative = [IO.Path]::GetRelativePath($portable, $file.FullName)
        $target = Join-Path $install $relative
        if ((Get-FileHash -LiteralPath $target).Hash -ne (Get-FileHash -LiteralPath $file.FullName).Hash) { throw "Installed file differs from portable: $relative" }
    }
    $task = Get-ScheduledTask -TaskName NagaNetworkElevatedHelper
    if ($task.Principal.RunLevel -ne 'Highest' -or $task.Actions.Execute -ne (Join-Path $install 'naga-control.exe')) { throw 'Incorrect elevated helper registration' }
    function Invoke-Helper($request) {
        $pipe = [IO.Pipes.NamedPipeClientStream]::new('.', 'NagaNetworkElevated', [IO.Pipes.PipeDirection]::InOut)
        try {
            $pipe.Connect(20000)
            $writer = [IO.StreamWriter]::new($pipe, [Text.UTF8Encoding]::new($false), 1024, $true)
            $writer.AutoFlush = $true
            $reader = [IO.StreamReader]::new($pipe, [Text.Encoding]::UTF8, $false, 1024, $true)
            try { $writer.WriteLine(($request | ConvertTo-Json -Compress)); $reply = $reader.ReadLine() | ConvertFrom-Json }
            finally { $writer.Dispose(); $reader.Dispose() }
            if (-not $reply.ok) { throw "Elevated helper failed: $($reply.error)" }
        } finally { $pipe.Dispose() }
    }
    Invoke-Helper @{mode='ping'}
    $config = Join-Path $work 'tun-check.json'
    '{"inbounds":[{"type":"tun","interface_name":"naga-ci0","address":["172.29.254.1/30"],"auto_route":false,"stack":"gvisor"}],"outbounds":[{"type":"direct","tag":"direct"}]}' | Set-Content -LiteralPath $config -Encoding utf8NoBOM
    $pipeName = 'NagaTun-' + [guid]::NewGuid().ToString('n').Substring(0,16)
    $hostPipe = [IO.Pipes.NamedPipeServerStream]::new($pipeName, [IO.Pipes.PipeDirection]::InOut, 1, [IO.Pipes.PipeTransmissionMode]::Byte, [IO.Pipes.PipeOptions]::Asynchronous)
    try {
        $connected = $hostPipe.WaitForConnectionAsync()
        Invoke-Helper @{mode='tun-host';pipe=$pipeName}
        if (-not $connected.Wait(20000)) { throw 'TUN host did not connect' }
        $writer = [IO.StreamWriter]::new($hostPipe, [Text.UTF8Encoding]::new($false), 1024, $true)
        $writer.AutoFlush = $true
        $reader = [IO.StreamReader]::new($hostPipe, [Text.Encoding]::UTF8, $false, 1024, $true)
        try {
            $writer.WriteLine((@{config_path=$config;binary_path=(Join-Path $install 'sing-box.exe')} | ConvertTo-Json -Compress))
            $reply = $reader.ReadLine() | ConvertFrom-Json
            if (-not $reply.ok) { throw "TUN host start failed: $($reply.error)" }
            $adapter = $null
            for ($attempt=0; $attempt -lt 40; $attempt++) {
                $adapter = Get-NetAdapter -Name naga-ci0 -ErrorAction SilentlyContinue
                if ($adapter) { break }
                Start-Sleep -Milliseconds 250
            }
            if (-not $adapter) { throw 'Wintun adapter was not created' }
            Write-Output "Installed $version; $($files.Count) files match portable; elevated helper created Wintun adapter $($adapter.Name)."
        } finally { $writer.Dispose(); $reader.Dispose() }
    } finally { $hostPipe.Dispose() }
} finally {
    $uninstallPath = Join-Path $install 'unins000.exe'
    if (Test-Path -LiteralPath $uninstallPath) {
        $uninstall = Start-Process -FilePath $uninstallPath -ArgumentList @('/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART') -WindowStyle Hidden -Wait -PassThru
        if ($uninstall.ExitCode -ne 0) { throw "Uninstall exit code: $($uninstall.ExitCode)" }
    }
}
if (Get-ScheduledTask -TaskName NagaNetworkElevatedHelper -ErrorAction SilentlyContinue) { throw 'Uninstaller left the helper task installed' }
Write-Output 'Windows setup, helper, real TUN creation, file integrity and uninstall passed.'
