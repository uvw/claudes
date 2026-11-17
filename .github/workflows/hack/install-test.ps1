# Tries install/install.ps1 on a release directory: one made from a built binary,
# or the release workflow's dist/. It installs twice, as a fresh install and
# an upgrade, then checks that a damaged archive is refused.
#
# Usage: .github/workflows/hack/install-test.ps1 -Binary claudes.exe
#        .github/workflows/hack/install-test.ps1 -Release dist [-Version v0.2.0]
param([string]$Binary, [string]$Release, [string]$Version)

$ErrorActionPreference = 'Stop'
$root = Resolve-Path (Join-Path $PSScriptRoot '../../..')
$script = Get-Content (Join-Path $root 'install/install.ps1') -Raw
$work = Join-Path ([IO.Path]::GetTempPath()) ("install-test-" + [Guid]::NewGuid())
New-Item -ItemType Directory -Path $work | Out-Null
$arch = if ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq 'Arm64') { 'arm64' } else { 'amd64' }

try {
    if (-not $Release) {
        $Release = Join-Path $work 'release'
        $pack = Join-Path $work 'pack'
        New-Item -ItemType Directory -Path $Release, $pack | Out-Null
        Copy-Item $Binary (Join-Path $pack 'claudes.exe')
        $archive = "claudes_0.0.0_windows_$arch.zip"
        Compress-Archive -Path (Join-Path $pack 'claudes.exe') -DestinationPath (Join-Path $Release $archive)
        $sum = (Get-FileHash -Algorithm SHA256 (Join-Path $Release $archive)).Hash.ToLowerInvariant()
        Set-Content -NoNewline -Path (Join-Path $Release 'checksums.txt') -Value "$sum  $archive`n"
    }
    $bin = Join-Path $work 'bin'
    foreach ($run in 'fresh', 'upgrade') {
        $env:CLAUDES_DOWNLOAD_URL, $env:CLAUDES_INSTALL_DIR = $Release, $bin
        $out = Invoke-Expression $script 6>&1 | Out-String
        if ($out -notmatch 'Checksum matches checksums.txt') { throw "${run}: no checksum line:`n$out" }
        $installed = & (Join-Path $bin 'claudes.exe') --version
        if ($Version -and $installed -notlike "*$($Version.TrimStart('v'))*") { throw "${run}: installed $installed, want $Version" }
        Write-Host "$run install: $installed"
    }

    $damaged = Join-Path $work 'damaged'
    Copy-Item -Recurse $Release $damaged
    Get-ChildItem $damaged -Filter "claudes_*_windows_$arch.zip" | ForEach-Object { Add-Content -NoNewline -Path $_.FullName -Value 'x' }
    $env:CLAUDES_DOWNLOAD_URL, $env:CLAUDES_INSTALL_DIR = $damaged, (Join-Path $work 'bad')
    $refused = $false
    try { Invoke-Expression $script 6>$null } catch { $refused = $_.Exception.Message -match 'does not match checksums.txt' }
    if (-not $refused) { throw 'a damaged archive was not refused' }
    if (Test-Path (Join-Path $work 'bad/claudes.exe')) { throw 'a damaged archive left a binary behind' }
    Write-Host 'damaged archive refused'
} finally {
    Remove-Item Env:CLAUDES_DOWNLOAD_URL, Env:CLAUDES_INSTALL_DIR -ErrorAction SilentlyContinue
    Remove-Item -Recurse -Force $work -ErrorAction SilentlyContinue
}
