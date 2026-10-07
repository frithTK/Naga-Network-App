#Requires -Version 5.1
Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$ProjectDir = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$AppDir = Join-Path $ProjectDir "app"
$DistDir = Join-Path $ProjectDir "dist\windows\NagaNetwork"

function Find-Flutter {
    if ($env:FLUTTER_BIN -and (Test-Path $env:FLUTTER_BIN)) {
        return $env:FLUTTER_BIN
    }
    $cmd = Get-Command flutter -ErrorAction SilentlyContinue
    if ($cmd) {
        return $cmd.Source
    }
    $guesses = @(
        (Join-Path $env:USERPROFILE "flutter\bin\flutter.bat"),
        (Join-Path $env:LOCALAPPDATA "flutter\bin\flutter.bat"),
        "C:\src\flutter\bin\flutter.bat"
    )
    foreach ($path in $guesses) {
        if ($path -and (Test-Path $path)) {
            return $path
        }
    }
    throw "Flutter не найден. Установи Flutter stable 3.47+ и добавь его в PATH или задай FLUTTER_BIN."
}

function Assert-Command($Name) {
    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
        throw "$Name не найден в PATH."
    }
}

Assert-Command go
$Flutter = Find-Flutter

$CanonIco = Join-Path $ProjectDir "assets\brand\naga-network.ico"
if (-not (Test-Path $CanonIco)) {
    throw "Не найден канон иконки: $CanonIco"
}
$RunnerIco = Join-Path $AppDir "windows\runner\resources\app_icon.ico"
Copy-Item -Force $CanonIco $RunnerIco

$Pubspec = Get-Content (Join-Path $AppDir "pubspec.yaml") | Select-String '^version:' | Select-Object -First 1
$Version = "0.0.0"
if ($Pubspec) {
    $Version = ($Pubspec.ToString() -replace '^version:\s*', '' -replace '\+.*$', '').Trim()
}

Write-Host "Сборка naga-control.exe..."
$PreviousCgo = $env:CGO_ENABLED
$PreviousGoos = $env:GOOS
$PreviousGoarch = $env:GOARCH
$env:CGO_ENABLED = "0"
$env:GOOS = "windows"
$env:GOARCH = "amd64"
try {
    $ControlLdFlags = "-s -w -X naga.network/core/version.Version=$Version"
    go build -buildvcs=false -ldflags $ControlLdFlags -o (Join-Path $env:TEMP "naga-control.exe") (Join-Path $ProjectDir "cmd\naga-control")
    if ($LASTEXITCODE -ne 0) { throw "go build naga-control failed" }

    Write-Host "Сборка naga-update.exe..."
    go build -buildvcs=false -ldflags "-s -w" -o (Join-Path $env:TEMP "naga-update.exe") (Join-Path $ProjectDir "cmd\naga-update")
    if ($LASTEXITCODE -ne 0) { throw "go build naga-update failed" }

    Write-Host "Сборка лаунчера NagaNetwork.exe..."
    $LauncherDir = Join-Path $ProjectDir "cmd\naga-windows"
    $LauncherIco = Join-Path $LauncherDir "naga-network.ico"
    Copy-Item -Force $CanonIco $LauncherIco
    Push-Location $LauncherDir
    try {
        go run github.com/tc-hib/go-winres@latest simply --arch amd64 --manifest gui --icon naga-network.ico --product-name "Naga Network" --file-description "Naga Network"
        if ($LASTEXITCODE -ne 0) { throw "go-winres failed" }
        go build -buildvcs=false -ldflags "-s -w -H windowsgui" -o (Join-Path $env:TEMP "NagaNetwork.exe") .
        if ($LASTEXITCODE -ne 0) { throw "go build naga-windows failed" }
    } finally {
        Pop-Location
        Remove-Item -Force $LauncherIco -ErrorAction SilentlyContinue
        Get-ChildItem -Path $LauncherDir -Filter *.syso -ErrorAction SilentlyContinue | Remove-Item -Force -ErrorAction SilentlyContinue
    }
} finally {
    if ($null -eq $PreviousCgo) { Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue } else { $env:CGO_ENABLED = $PreviousCgo }
    if ($null -eq $PreviousGoos) { Remove-Item Env:GOOS -ErrorAction SilentlyContinue } else { $env:GOOS = $PreviousGoos }
    if ($null -eq $PreviousGoarch) { Remove-Item Env:GOARCH -ErrorAction SilentlyContinue } else { $env:GOARCH = $PreviousGoarch }
}

Write-Host "Сборка Flutter Windows release (version $Version)..."
Push-Location $AppDir
try {
    & $Flutter config --enable-windows-desktop | Out-Host
    if ($LASTEXITCODE -ne 0) { throw "flutter config failed" }
    & $Flutter pub get | Out-Host
    if ($LASTEXITCODE -ne 0) { throw "flutter pub get failed" }
    & $Flutter build windows --release --build-name $Version --dart-define="NAGA_VERSION=$Version" | Out-Host
    if ($LASTEXITCODE -ne 0) { throw "flutter build windows failed" }
} finally {
    Pop-Location
}

$Bundle = Join-Path $AppDir "build\windows\x64\runner\Release"
$UiExe = Join-Path $Bundle "naga_network.exe"
if (-not (Test-Path $UiExe)) {
    throw "Не найден бандл: $UiExe"
}

Write-Host "Упаковка $DistDir ..."
if (Test-Path $DistDir) {
    Remove-Item -Recurse -Force $DistDir
}
New-Item -ItemType Directory -Path $DistDir | Out-Null
Copy-Item -Recurse -Force (Join-Path $Bundle "*") $DistDir
Copy-Item -Force (Join-Path $env:TEMP "naga-control.exe") (Join-Path $DistDir "naga-control.exe")
Copy-Item -Force (Join-Path $env:TEMP "NagaNetwork.exe") (Join-Path $DistDir "NagaNetwork.exe")
Copy-Item -Force (Join-Path $env:TEMP "naga-update.exe") (Join-Path $DistDir "naga-update.exe")

function Find-VCRedistCRT {
    if (-not ${env:ProgramFiles(x86)}) {
        ${env:ProgramFiles(x86)} = "C:\Program Files (x86)"
    }
    $roots = @(
        "${env:ProgramFiles(x86)}\Microsoft Visual Studio\2022",
        "${env:ProgramFiles}\Microsoft Visual Studio\2022",
        "${env:ProgramFiles(x86)}\Microsoft Visual Studio\2025",
        "${env:ProgramFiles}\Microsoft Visual Studio\2025"
    )
    $vswhere = "${env:ProgramFiles(x86)}\Microsoft Visual Studio\Installer\vswhere.exe"
    if (Test-Path $vswhere) {
        $install = & $vswhere -latest -products * -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath
        if ($install) { $roots += $install }
    }
    $dirs = @()
    foreach ($root in ($roots | Select-Object -Unique)) {
        if (-not (Test-Path $root)) { continue }
        $found = @(Get-ChildItem -Path $root -Directory -Filter "Microsoft.VC14*.CRT" -Recurse -ErrorAction SilentlyContinue |
            Where-Object { $_.FullName -match '\\x64\\Microsoft\.VC14\d\.CRT$' -and $_.FullName -notmatch '\\onecore\\' })
        if ($found.Count -gt 0) {
            $dirs += $found
        }
    }
    if ($dirs.Count -eq 0) {
        return $null
    }
    return ($dirs | Sort-Object FullName -Descending | Select-Object -First 1).FullName
}

$CrtDir = Find-VCRedistCRT
if ($CrtDir) {
    Write-Host "Копирование VC++ runtime из $CrtDir ..."
    Copy-Item -Force (Join-Path $CrtDir "*.dll") $DistDir
}
if (-not (Test-Path (Join-Path $DistDir "vcruntime140.dll"))) {
    throw "vcruntime140.dll не попал в бандл. Поставь Visual Studio 2022/2025 Build Tools — C++."
}

function Get-FileSha256([string]$Path) {
    return (Get-FileHash -Algorithm SHA256 -Path $Path).Hash.ToLowerInvariant()
}

function Fetch-VerifiedZip([string]$Url, [string]$Sha256, [string]$OutFile) {
    Write-Host "Загрузка $Url ..."
    Invoke-WebRequest -Uri $Url -OutFile $OutFile -UseBasicParsing
    $got = Get-FileSha256 $OutFile
    if ($got -ne $Sha256.ToLowerInvariant()) {
        throw "SHA256 mismatch for $OutFile`: got $got want $($Sha256.ToLowerInvariant())"
    }
}

function Install-NagaRuntime([string]$DistDir) {
    $SingBoxVersion = "1.13.20"
    $SingBoxUrl = "https://github.com/SagerNet/sing-box/releases/download/v$SingBoxVersion/sing-box-$SingBoxVersion-windows-amd64.zip"
    $SingBoxSha = "f2eb3d17bd183d143ef764c8f08c5625a93e628ec4eced24276ed6e9210fb296"
    $WintunUrl = "https://www.wintun.net/builds/wintun-0.14.1.zip"
    $WintunSha = "07c256185d6ee3652e09fa55c0b673e2624b565e02c4b9091c79ca7d2f24ef51"

    $work = Join-Path $env:TEMP ("naga-runtime-" + [guid]::NewGuid().ToString("n"))
    New-Item -ItemType Directory -Path $work | Out-Null
    try {
        $sbZip = Join-Path $work "sing-box.zip"
        $wtZip = Join-Path $work "wintun.zip"
        Fetch-VerifiedZip $SingBoxUrl $SingBoxSha $sbZip
        Fetch-VerifiedZip $WintunUrl $WintunSha $wtZip

        $sbDir = Join-Path $work "sing-box"
        $wtDir = Join-Path $work "wintun"
        Expand-Archive -LiteralPath $sbZip -DestinationPath $sbDir
        Expand-Archive -LiteralPath $wtZip -DestinationPath $wtDir

        $sbExe = Get-ChildItem $sbDir -Recurse -Filter "sing-box.exe" | Select-Object -First 1
        if (-not $sbExe) { throw "sing-box.exe не найден в архиве" }
        $wtDll = Get-ChildItem $wtDir -Recurse -Filter "wintun.dll" |
            Where-Object { $_.FullName -match '\\bin\\amd64\\wintun\.dll$' } |
            Select-Object -First 1
        if (-not $wtDll) { throw "wintun.dll amd64 не найден в архиве" }

        Copy-Item -Force $sbExe.FullName (Join-Path $DistDir "sing-box.exe")
        Copy-Item -Force $wtDll.FullName (Join-Path $DistDir "wintun.dll")

        $sbLicense = Get-ChildItem $sbDir -Recurse -Filter "LICENSE" | Select-Object -First 1
        if ($sbLicense) {
            Copy-Item -Force $sbLicense.FullName (Join-Path $DistDir "LICENSE.sing-box")
        }
        $wtLicense = Get-ChildItem $wtDir -Recurse -Filter "LICENSE.txt" | Select-Object -First 1
        if ($wtLicense) {
            Copy-Item -Force $wtLicense.FullName (Join-Path $DistDir "LICENSE.wintun.txt")
        }

        Write-Host "Проверка sing-box.exe ..."
        $exe = Join-Path $DistDir "sing-box.exe"
        & $exe version | Out-Host
        if ($LASTEXITCODE -ne 0) { throw "sing-box version failed" }
        $cfg = Join-Path $work "tun-check.json"
        @'
{
  "inbounds": [
    {
      "type": "tun",
      "tag": "tun-in",
      "interface_name": "naga-tun0",
      "address": ["172.19.0.1/30"],
      "auto_route": true,
      "strict_route": true,
      "stack": "gvisor"
    }
  ],
  "outbounds": [{"type": "direct", "tag": "direct"}]
}
'@ | Set-Content -Encoding ascii $cfg
        & $exe check -c $cfg | Out-Host
        if ($LASTEXITCODE -ne 0) {
            throw "sing-box check failed for TUN config with gvisor/strict_route"
        }
        if (-not (Test-Path (Join-Path $DistDir "wintun.dll"))) {
            throw "wintun.dll не попал в бандл"
        }
    } finally {
        Remove-Item -Recurse -Force $work -ErrorAction SilentlyContinue
    }
}

function Install-OlcRuntime([string]$DistDir) {
    $HevVersion = "2.17.1"
    $HevUrl = "https://github.com/heiher/hev-socks5-tunnel/releases/download/$HevVersion/hev-socks5-tunnel-win64.zip"
    $HevSha = "08c85eb47eea60fad4a2ace9f85577b05056be08118924f8305f2f03b7dd9994"
    $work = Join-Path $env:TEMP ("naga-hev-" + [guid]::NewGuid().ToString("n"))
    New-Item -ItemType Directory -Path $work | Out-Null
    try {
        $zip = Join-Path $work "hev.zip"
        Fetch-VerifiedZip $HevUrl $HevSha $zip
        Expand-Archive -LiteralPath $zip -DestinationPath (Join-Path $work "hev")
        $hevExe = Get-ChildItem (Join-Path $work "hev") -Recurse -Filter "hev-socks5-tunnel.exe" | Select-Object -First 1
        if (-not $hevExe) { throw "hev-socks5-tunnel.exe не найден в архиве" }
        Copy-Item -Force $hevExe.FullName (Join-Path $DistDir "hev-socks5-tunnel.exe")
        $msys = Get-ChildItem (Join-Path $work "hev") -Recurse -Filter "msys-2.0.dll" | Select-Object -First 1
        if (-not $msys) { throw "msys-2.0.dll не найден рядом с hev-socks5-tunnel" }
        Copy-Item -Force $msys.FullName (Join-Path $DistDir "msys-2.0.dll")
        $hevLicense = Get-ChildItem (Join-Path $work "hev") -Recurse -Include "LICENSE","LICENSE.txt","COPYING" | Select-Object -First 1
        if ($hevLicense) {
            Copy-Item -Force $hevLicense.FullName (Join-Path $DistDir "LICENSE.hev-socks5-tunnel")
        }
    } finally {
        Remove-Item -Recurse -Force $work -ErrorAction SilentlyContinue
    }

    $OlcCommit = "7f849e08022ac4ce6dcdb1a49ad5560446daca1f"
    $OlcShort = "7f849e08"
    $olcSrc = $env:NAGA_OLCRTC_PATH
    if ($olcSrc -and (Test-Path $olcSrc)) {
        Copy-Item -Force $olcSrc (Join-Path $DistDir "olcrtc.exe")
        $verSrc = Join-Path (Split-Path -Parent $olcSrc) "olcrtc.version"
        if (Test-Path $verSrc) {
            Copy-Item -Force $verSrc (Join-Path $DistDir "olcrtc.version")
        } else {
            Set-Content -Path (Join-Path $DistDir "olcrtc.version") -Value $OlcShort -Encoding ascii
        }
        Write-Host "olcrtc.exe скопирован из NAGA_OLCRTC_PATH"
        return
    }

    $olcWork = Join-Path $env:TEMP ("naga-olcrtc-src-" + [guid]::NewGuid().ToString("n"))
    New-Item -ItemType Directory -Path $olcWork | Out-Null
    $prevCgo = $env:CGO_ENABLED
    $prevGoos = $env:GOOS
    $prevGoarch = $env:GOARCH
    try {
        Write-Host "Сборка olcrtc.exe ($OlcShort) из ghostlane-project/olcrtc ..."
        Push-Location $olcWork
        $env:GIT_TERMINAL_PROMPT = "0"
        git -c advice.detachedHead=false init | Out-Null
        git remote add origin https://github.com/ghostlane-project/olcrtc.git
        git fetch --depth 1 origin $OlcCommit
        if ($LASTEXITCODE -ne 0) { throw "git fetch olcrtc $OlcCommit failed" }
        git -c advice.detachedHead=false checkout --detach FETCH_HEAD | Out-Null
        if ($LASTEXITCODE -ne 0) { throw "git checkout olcrtc failed" }
        $got = (git rev-parse HEAD).Trim()
        if (-not $got.StartsWith($OlcShort)) {
            throw "olcrtc HEAD $got is not pinned $OlcShort"
        }
        $env:CGO_ENABLED = "0"
        $env:GOOS = "windows"
        $env:GOARCH = "amd64"
        go build -trimpath -ldflags "-s -w" -o (Join-Path $olcWork "olcrtc.exe") .\cmd\olcrtc
        if ($LASTEXITCODE -ne 0) { throw "go build olcrtc failed" }
        $exe = Join-Path $olcWork "olcrtc.exe"
        if (-not (Test-Path $exe)) { throw "olcrtc.exe не собран" }
        Copy-Item -Force $exe (Join-Path $DistDir "olcrtc.exe")
        Set-Content -Path (Join-Path $DistDir "olcrtc.version") -Value $OlcShort -Encoding ascii
        if (Test-Path (Join-Path $olcWork "LICENSE")) {
            Copy-Item -Force (Join-Path $olcWork "LICENSE") (Join-Path $DistDir "LICENSE.olcrtc")
        }
        Write-Host "olcrtc.exe ($OlcShort) попал в бандл"
    } finally {
        Pop-Location
        if ($null -eq $prevCgo) { Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue } else { $env:CGO_ENABLED = $prevCgo }
        if ($null -eq $prevGoos) { Remove-Item Env:GOOS -ErrorAction SilentlyContinue } else { $env:GOOS = $prevGoos }
        if ($null -eq $prevGoarch) { Remove-Item Env:GOARCH -ErrorAction SilentlyContinue } else { $env:GOARCH = $prevGoarch }
        Remove-Item -Recurse -Force $olcWork -ErrorAction SilentlyContinue
    }
}

Write-Host "Загрузка sing-box и wintun ..."
Install-NagaRuntime $DistDir
Write-Host "Загрузка hev-socks5-tunnel ..."
Install-OlcRuntime $DistDir

function Find-SignTool {
    if ($env:NAGA_SIGNTOOL -and (Test-Path $env:NAGA_SIGNTOOL)) {
        return $env:NAGA_SIGNTOOL
    }
    if (-not ${env:ProgramFiles(x86)}) {
        ${env:ProgramFiles(x86)} = "C:\Program Files (x86)"
    }
    $found = Get-ChildItem "${env:ProgramFiles(x86)}\Windows Kits\10\bin" -Recurse -Filter signtool.exe -ErrorAction SilentlyContinue |
        Where-Object { $_.DirectoryName -match '\\x64$' } |
        Sort-Object FullName -Descending |
        Select-Object -First 1
    if ($found) {
        return $found.FullName
    }
    $cmd = Get-Command signtool.exe -ErrorAction SilentlyContinue
    if ($cmd) {
        return $cmd.Source
    }
    return $null
}

function Sign-NagaFile([string]$Path) {
    if (-not (Test-Path $Path)) {
        throw "нечего подписывать: $Path"
    }
    $signtool = Find-SignTool
    if (-not $signtool) {
        throw "signtool.exe не найден. Поставь Windows SDK или задай NAGA_SIGNTOOL."
    }
    $timestamp = $env:NAGA_SIGN_TIMESTAMP
    if (-not $timestamp) {
        $timestamp = "http://timestamp.digicert.com"
    }
    $args = @("sign", "/fd", "SHA256", "/td", "SHA256", "/tr", $timestamp)
    if ($env:NAGA_SIGN_PFX) {
        $args += @("/f", $env:NAGA_SIGN_PFX)
        if ($env:NAGA_SIGN_PFX_PASSWORD) {
            $args += @("/p", $env:NAGA_SIGN_PFX_PASSWORD)
        }
    } elseif ($env:NAGA_SIGN_SHA1) {
        $args += @("/sha1", $env:NAGA_SIGN_SHA1)
    } else {
        throw "задай NAGA_SIGN_PFX или NAGA_SIGN_SHA1"
    }
    $args += $Path
    & $signtool @args
    if ($LASTEXITCODE -ne 0) {
        throw "signtool failed for $Path"
    }
}

$ShouldSign = [bool]($env:NAGA_SIGN_PFX -or $env:NAGA_SIGN_SHA1)
if ($ShouldSign) {
    Write-Host "Подпись Authenticode..."
    foreach ($name in @("naga_network.exe", "NagaNetwork.exe", "naga-control.exe", "flutter_windows.dll")) {
        Sign-NagaFile (Join-Path $DistDir $name)
    }
} else {
    Write-Host "Authenticode пропущен (нет NAGA_SIGN_PFX / NAGA_SIGN_SHA1). SmartScreen у чужого ПК останется."
}

$Zip = Join-Path $ProjectDir "dist\windows\NagaNetwork-$Version-windows-x64.zip"
if (Test-Path $Zip) {
    Remove-Item -Force $Zip
}
Compress-Archive -Path (Join-Path $DistDir "*") -DestinationPath $Zip

function Find-ISCC {
    if (-not ${env:ProgramFiles(x86)}) {
        ${env:ProgramFiles(x86)} = "C:\Program Files (x86)"
    }
    if ($env:INNO_SETUP_ISCC -and (Test-Path $env:INNO_SETUP_ISCC)) {
        return $env:INNO_SETUP_ISCC
    }
    $candidates = @(
        "${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe",
        "$env:LOCALAPPDATA\Programs\Inno Setup 6\ISCC.exe",
        "${env:ProgramFiles}\Inno Setup 6\ISCC.exe"
    )
    foreach ($path in $candidates) {
        if ($path -and (Test-Path $path)) {
            return $path
        }
    }
    $cmd = Get-Command ISCC.exe -ErrorAction SilentlyContinue
    if ($cmd) {
        return $cmd.Source
    }
    return $null
}

$IssPath = Join-Path $ProjectDir "packaging\windows\naga-network.iss"
if (-not (Test-Path $IssPath)) {
    throw "Не найден скрипт установщика: $IssPath"
}
$ISCC = Find-ISCC
if (-not $ISCC) {
    throw @"
ISCC.exe не найден. Поставь Inno Setup 6 и повтори сборку.
  https://jrsoftware.org/isinfo.php
Либо задай INNO_SETUP_ISCC. Portable zip уже собран: $Zip
В CI Inno Setup ставит chocolatey.
"@
}

Write-Host "Сборка установщика ($ISCC)..."
$Setup = Join-Path $ProjectDir "dist\windows\NagaNetwork-$Version-windows-x64-setup.exe"
if (Test-Path $Setup) {
    Remove-Item -Force $Setup
}
& $ISCC "/DMyAppVersion=$Version" "/DRepoRoot=$ProjectDir" $IssPath
if ($LASTEXITCODE -ne 0) { throw "iscc failed" }
if (-not (Test-Path $Setup)) {
    throw "Не найден установщик: $Setup"
}
if ($ShouldSign) {
    Write-Host "Подпись установщика..."
    Sign-NagaFile $Setup
}

Write-Host ""
Write-Host "Готово."
Write-Host "  Папка: $DistDir"
Write-Host "  Zip:   $Zip"
Write-Host "  Setup: $Setup"
if ($ShouldSign) {
    Write-Host "Бинарники и setup подписаны Authenticode."
} else {
    Write-Host "Setup без подписи: SmartScreen может показать «неизвестный издатель», пока нет OV/EV-сертификата."
}
Write-Host "Запускай NagaNetwork.exe или setup.exe. Setup один раз просит UAC и ставит elevated-helper; portable без helper — UAC на «Подключить». В бандле sing-box.exe, wintun.dll, hev-socks5-tunnel.exe, msys-2.0.dll, olcrtc.exe и olcrtc.version=7f849e08."
