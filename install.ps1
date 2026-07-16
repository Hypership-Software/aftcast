# Aftcast installer for Windows.
#
#   irm https://raw.githubusercontent.com/Hypership-Software/aftcast/main/install.ps1 | iex
#
# Downloads the release binary for this machine, verifies its checksum, and
# runs `aftcast init` - which installs the binary to ~\.aftcast\bin, adds it
# to PATH, starts the daemon, and wires the Claude Code hooks.
#
#   $env:AFTCAST_VERSION   pin a release tag (default: latest)
#   $env:AFTCAST_BASE_URL  alternate release host for internal mirrors
#                          (default: https://github.com/Hypership-Software/aftcast/releases)

function Install-Aftcast {
    $ErrorActionPreference = 'Stop'
    [Net.ServicePointManager]::SecurityProtocol = `
        [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

    $archRaw = $env:PROCESSOR_ARCHITEW6432
    if (-not $archRaw) { $archRaw = $env:PROCESSOR_ARCHITECTURE }
    switch ($archRaw) {
        'AMD64' { $arch = 'amd64' }
        'ARM64' { $arch = 'arm64' }
        default { throw "install: unsupported architecture '$archRaw' - build from source instead: https://github.com/Hypership-Software/aftcast#install-from-source" }
    }
    $asset = "aftcast_windows_$arch.zip"

    $base = $env:AFTCAST_BASE_URL
    if (-not $base) { $base = 'https://github.com/Hypership-Software/aftcast/releases' }
    $version = $env:AFTCAST_VERSION
    if (-not $version) { $version = 'latest' }
    if ($version -eq 'latest') { $url = "$base/latest/download" } else { $url = "$base/download/$version" }

    $tmp = Join-Path $env:TEMP ('aftcast-install-' + [IO.Path]::GetRandomFileName())
    New-Item -ItemType Directory -Path $tmp | Out-Null
    try {
        Write-Host "downloading Aftcast ($version, windows/$arch)..."
        Invoke-WebRequest -UseBasicParsing -Uri "$url/$asset" -OutFile (Join-Path $tmp $asset)
        Invoke-WebRequest -UseBasicParsing -Uri "$url/checksums.txt" -OutFile (Join-Path $tmp 'checksums.txt')

        $expected = (Get-Content (Join-Path $tmp 'checksums.txt') |
            Where-Object { $_ -match [regex]::Escape($asset) } |
            ForEach-Object { ($_ -split '\s+')[0] } |
            Select-Object -First 1)
        if (-not $expected) { throw "install: checksums.txt has no entry for $asset" }
        $actual = (Get-FileHash -Algorithm SHA256 (Join-Path $tmp $asset)).Hash
        if ($actual -ne $expected.ToUpperInvariant()) {
            throw "install: checksum mismatch for $asset - the download may be corrupted; try again"
        }

        Expand-Archive -Path (Join-Path $tmp $asset) -DestinationPath $tmp
        $exe = Join-Path $tmp 'aftcast.exe'
        if (-not (Test-Path $exe)) { throw 'install: release archive did not contain aftcast.exe' }

        $claudeDir = Join-Path $HOME '.claude'
        if (-not (Test-Path $claudeDir) -and -not (Get-Command claude -ErrorAction SilentlyContinue)) {
            Write-Host 'note: Claude Code was not detected - Aftcast will be ready once it is installed'
        }

        & $exe init
        if ($LASTEXITCODE -ne 0) { throw "install: aftcast init failed (exit $LASTEXITCODE)" }

        Write-Host ''
        Write-Host 'done. open a new terminal (so PATH picks up ~\.aftcast\bin), then:'
        Write-Host '  aftcast status    # daemon running, hooks wired'
        Write-Host '  aftcast doctor    # detailed checks'
        Write-Host 'start a new Claude Code session and Aftcast observes it from there.'
    }
    finally {
        Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
    }
}

Install-Aftcast
