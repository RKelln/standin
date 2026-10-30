# install.ps1: install standin on Windows.
#
#   irm https://raw.githubusercontent.com/RKelln/standin/main/install.ps1 | iex
#
# Options, set before running (they work with "irm | iex"):
#   $env:STANDIN_VERSION = 'v0.1.0'     install that release (default: the latest release)
#   $env:STANDIN_BIN_DIR = 'C:\path'    install into that folder
#                                       (default: %LOCALAPPDATA%\Programs\standin)
#
# It never needs an admin account. It checks the download against the release's
# checksums.txt and stops if they do not match. It adds the folder to your own
# (user) PATH, not the system PATH.

& {
    $ErrorActionPreference = 'Stop'
    $ProgressPreference = 'SilentlyContinue'

    # Windows PowerShell 5.1 may default to old TLS versions that GitHub refuses.
    try {
        [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
    } catch { }

    $Repo = 'RKelln/standin'
    $Version = if ($env:STANDIN_VERSION) { $env:STANDIN_VERSION } else { 'latest' }
    $BinDir = if ($env:STANDIN_BIN_DIR) { $env:STANDIN_BIN_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\standin' }

    # "throw", not "exit": exit would close the PowerShell window when run through iex.
    function Stop-Install([string]$Msg) { throw "Stopped: $Msg" }

    $Arch = switch ($env:PROCESSOR_ARCHITECTURE) {
        'AMD64' { 'x86_64' }
        'ARM64' { 'arm64' }
        default { Stop-Install "This computer's chip type ($env:PROCESSOR_ARCHITECTURE) is not supported yet." }
    }

    $Archive = "standin_Windows_$Arch.zip"
    if ($Version -eq 'latest') {
        # The /latest/download/ link avoids the GitHub API rate limit.
        $BaseUrl = "https://github.com/$Repo/releases/latest/download"
    } else {
        if ($Version -notlike 'v*') { $Version = "v$Version" }
        $BaseUrl = "https://github.com/$Repo/releases/download/$Version"
    }

    Write-Host ''
    Write-Host "Installing standin ($Version, Windows $Arch)"
    Write-Host "  into: $BinDir"
    Write-Host ''

    $Tmp = Join-Path ([IO.Path]::GetTempPath()) ("standin-" + [Guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $Tmp | Out-Null
    try {
        $ZipPath = Join-Path $Tmp $Archive
        $SumPath = Join-Path $Tmp 'checksums.txt'

        Write-Host "Downloading $Archive ..."
        try { Invoke-WebRequest -Uri "$BaseUrl/$Archive" -OutFile $ZipPath -UseBasicParsing }
        catch { Stop-Install "Could not download $BaseUrl/$Archive. Check your internet connection, or the version number." }
        try { Invoke-WebRequest -Uri "$BaseUrl/checksums.txt" -OutFile $SumPath -UseBasicParsing }
        catch { Stop-Install 'Could not download the checksum list for this release, so the download cannot be checked.' }

        Write-Host 'Checking the download ...'
        $Expected = $null
        foreach ($line in Get-Content $SumPath) {
            $parts = $line.Trim() -split '\s+'
            if ($parts.Count -ge 2 -and ($parts[1] -eq $Archive -or $parts[1] -eq "*$Archive")) {
                $Expected = $parts[0].ToLower()
                break
            }
        }
        if (-not $Expected) { Stop-Install "The checksum list does not name $Archive. Nothing was installed." }
        $Actual = (Get-FileHash -Algorithm SHA256 -Path $ZipPath).Hash.ToLower()
        if ($Expected -ne $Actual) {
            Stop-Install 'The download does not match its checksum (it may be damaged or changed). Nothing was installed.'
        }

        $Unpacked = Join-Path $Tmp 'unpacked'
        Expand-Archive -Path $ZipPath -DestinationPath $Unpacked -Force
        $Src = Join-Path $Unpacked 'standin.exe'
        if (-not (Test-Path $Src)) { Stop-Install 'The download did not contain standin.exe. Nothing was installed.' }

        if (-not (Test-Path $BinDir)) { New-Item -ItemType Directory -Path $BinDir -Force | Out-Null }
        $Dest = Join-Path $BinDir 'standin.exe'
        Copy-Item -Path $Src -Destination $Dest -Force
        Unblock-File -Path $Dest -ErrorAction SilentlyContinue

        $Installed = ''
        try { $Installed = (& $Dest version 2>$null | Out-String).Trim() } catch { }

        Write-Host ''
        Write-Host "Done. standin is installed at $Dest" -ForegroundColor Green
        if ($Installed) { Write-Host "  $Installed" }
        Write-Host ''
        Write-Host 'Next steps'

        $Step = 1
        $UserPath = [Environment]::GetEnvironmentVariable('Path', 'User')
        $Parts = @()
        if ($UserPath) { $Parts = $UserPath -split ';' | Where-Object { $_ } }
        if ($Parts -notcontains $BinDir) {
            $NewPath = (@($Parts) + $BinDir) -join ';'
            [Environment]::SetEnvironmentVariable('Path', $NewPath, 'User')
            Write-Host "  $Step. We added $BinDir to your PATH (the list of folders"
            Write-Host '     your terminal looks in). Close this window and open a new one.'
            $Step++
        }
        Write-Host "  $Step. Check it works:   standin version"
        $Step++
        Write-Host "  $Step. Ask your AI agent to help you set up standin in your project folder."
        Write-Host '     Your real files go in raw\. The AI only reads scrubbed\.'
        Write-Host ''
        Write-Host '  Keep your private table (the mapping that links stand-ins back to real values)'
        Write-Host '  to yourself. Standin stores it outside your project, in your .standin folder.'
        Write-Host ''
    } finally {
        Remove-Item -Recurse -Force $Tmp -ErrorAction SilentlyContinue
    }
}
