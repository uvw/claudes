# Installs claudes from its GitHub release on Windows, or upgrades it.
#
#   irm https://github.com/uvw/claudes/releases/latest/download/install.ps1 | iex
#
# It downloads the archive for this computer and checks it against the
# release's checksums.txt. When the GitHub CLI is installed and signed in, it
# also checks the archive's build provenance. It installs into
# %LOCALAPPDATA%\Programs\claudes and adds that folder to the user's PATH.
# CLAUDES_VERSION and CLAUDES_INSTALL_DIR choose another version or folder;
# so do -Version and -Dir, when the script runs as a file.
#
# The script runs in its own scope, so iex leaves the caller's variables and
# preferences as they were.

& {
    param(
        [string]$Version = $(if ($env:CLAUDES_VERSION) { $env:CLAUDES_VERSION } else { 'latest' }),
        [string]$Dir = $(if ($env:CLAUDES_INSTALL_DIR) { $env:CLAUDES_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\claudes' })
    )

    $ErrorActionPreference = 'Stop'
    $ProgressPreference = 'SilentlyContinue'
    $repo = 'uvw/claudes'

    function Fail([string]$message) {
        Write-Host "claudes install: $message" -ForegroundColor Red
        # exit would close the window of a user who ran the one-liner with iex.
        throw "claudes install: $message"
    }

    # Windows PowerShell 5.1 offers older TLS versions unless told otherwise.
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

    $arch = switch ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture) {
        'X64' { 'amd64' }
        'Arm64' { 'arm64' }
        default { Fail "$_ Windows is not supported: releases cover amd64 and arm64" }
    }

    # CLAUDES_DOWNLOAD_URL, a URL or a folder, replaces the release, so the
    # release workflow can try this script on the archives it just built.
    if ($env:CLAUDES_DOWNLOAD_URL) {
        $base = $env:CLAUDES_DOWNLOAD_URL
    } elseif ($Version -eq 'latest') {
        $base = "https://github.com/$repo/releases/latest/download"
    } else {
        if (-not $Version.StartsWith('v')) { $Version = "v$Version" }
        if ($Version -notmatch '^v\d+\.\d+\.\d+$') { Fail "$Version is not a version such as v0.2.0" }
        $base = "https://github.com/$repo/releases/download/$Version"
    }

    $tmp = Join-Path ([IO.Path]::GetTempPath()) ("claudes-install-" + [Guid]::NewGuid())
    New-Item -ItemType Directory -Path $tmp | Out-Null

    function Fetch([string]$name) {
        $out = Join-Path $tmp $name
        if ($base -match '^https://') {
            try { Invoke-WebRequest -UseBasicParsing -Uri "$base/$name" -OutFile $out }
            catch { Fail "cannot download $base/$name" }
        } elseif ($base -match '^http://') {
            Fail "$base is not https"
        } else {
            Copy-Item (Join-Path $base $name) $out
        }
        return $out
    }

    try {
        $sums = Fetch 'checksums.txt'
        $pattern = "^([0-9a-f]{64})  (claudes_\d+\.\d+\.\d+_windows_$arch\.zip)$"
        $line = Get-Content $sums | Where-Object { $_ -match $pattern } | Select-Object -First 1
        if (-not $line) { Fail "the release has no archive for windows_$arch" }
        $null = $line -match $pattern
        $want, $archive = $Matches[1], $Matches[2]
        Write-Host "Downloading $archive"
        $zip = Fetch $archive

        $got = (Get-FileHash -Algorithm SHA256 $zip).Hash.ToLowerInvariant()
        if ($got -ne $want) { Fail "$archive does not match checksums.txt: the download is damaged or was changed" }
        Write-Host 'Checksum matches checksums.txt'

        if ($base -notmatch '^https://github\.com/') {
            Write-Host 'Build provenance not checked: the archive did not come from GitHub'
        } elseif ((Get-Command gh -ErrorAction SilentlyContinue) -and (& { gh auth status *> $null; $LASTEXITCODE -eq 0 })) {
            & gh attestation verify $zip --repo $repo *> $null
            if ($LASTEXITCODE -ne 0) { Fail "$archive has no valid build provenance from $repo" }
            Write-Host 'Build provenance verified with gh'
        } else {
            Write-Host 'Build provenance not checked: it needs the GitHub CLI (gh), signed in'
        }

        Expand-Archive -Path $zip -DestinationPath (Join-Path $tmp 'unpacked')
        New-Item -ItemType Directory -Force -Path $Dir | Out-Null
        $exe = Join-Path $Dir 'claudes.exe'
        # A running claudes.exe cannot be overwritten, but it can be renamed.
        if (Test-Path $exe) { Move-Item -Force $exe "$exe.old" }
        Copy-Item (Join-Path $tmp 'unpacked\claudes.exe') $exe
        Remove-Item -Force "$exe.old" -ErrorAction SilentlyContinue
        $installed = & $exe --version
        if ($LASTEXITCODE -ne 0) { Fail "the installed $exe does not run" }
        Write-Host "Installed $installed in $Dir"
    } finally {
        Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
    }

    $userPath = @([Environment]::GetEnvironmentVariable('Path', 'User') -split ';' | Where-Object { $_ })
    if ($userPath -notcontains $Dir) {
        [Environment]::SetEnvironmentVariable('Path', (($userPath + $Dir) -join ';'), 'User')
        Write-Host "Added $Dir to your PATH. Open a new terminal to use claudes."
    }
    if (-not (($env:Path -split ';') -contains $Dir)) { $env:Path = "$env:Path;$Dir" }

    Write-Host ''
    Write-Host 'Next steps, each optional:'
    Write-Host "  claudes cd:   add  claudes init pwsh | Out-String | Invoke-Expression  to `$PROFILE"
    Write-Host "  completion:   see https://github.com/$repo#shell-setup"
    Write-Host '  agent skill:  claudes skill --install'
    Write-Host '  check:        claudes doctor'
} @args
