# Install FQL for the current Windows user; no Go or administrator privileges required.
[CmdletBinding()]
param(
    [string]$Version = $env:FQL_VERSION,
    [string]$InstallDir = $(if ($env:FQL_INSTALL_DIR) { $env:FQL_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'FQL\bin' }),
    [switch]$NoModifyPath
)
$ErrorActionPreference = 'Stop'
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
if ($env:OS -ne 'Windows_NT') { throw 'This installer supports Windows; macOS/Linux should use install.sh.' }
if ([Runtime.InteropServices.RuntimeInformation]::OSArchitecture -ne 'X64') { throw 'The Windows release supports amd64 only.' }
if (-not [IO.Path]::IsPathRooted($InstallDir) -or $InstallDir.Contains("`n") -or $InstallDir.Contains("`r")) { throw 'InstallDir must be an absolute path without newlines.' }
$Repo = 'jonace-mpelule/fQL'
$Headers = @{ 'User-Agent' = 'fql-installer'; Accept = 'application/vnd.github+json' }
if (-not $Version) {
    try { $Release = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest" -Headers $Headers }
    catch {
        $Releases = @(Invoke-RestMethod "https://api.github.com/repos/$Repo/releases?per_page=1" -Headers $Headers)
        if (-not $Releases.Count) { throw 'No release published; supply -Version.' }
        $Release = $Releases[0]
    }
    $Version = $Release.tag_name
}
$Version = $Version -replace '^v', ''
if ($Version -notmatch '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z][0-9A-Za-z.-]*)?$') { throw 'Invalid release version.' }
$Temp = Join-Path ([IO.Path]::GetTempPath()) ('fql-install-' + [Guid]::NewGuid())
$Stage = $null
try {
    New-Item -ItemType Directory -Path $Temp | Out-Null
    $Asset = "fql_${Version}_windows_amd64.zip"
    $Base = "https://github.com/$Repo/releases/download/v$Version"
    Invoke-WebRequest "$Base/SHA256SUMS" -Headers $Headers -OutFile (Join-Path $Temp 'SHA256SUMS') -UseBasicParsing
    $Archive = Join-Path $Temp $Asset
    Invoke-WebRequest "$Base/$Asset" -Headers $Headers -OutFile $Archive -UseBasicParsing
    $MatchesFound = @(Get-Content (Join-Path $Temp 'SHA256SUMS') | Where-Object { $_ -match ('^([0-9a-f]{64})  ' + [Regex]::Escape($Asset) + '$') })
    if ($MatchesFound.Count -ne 1) { throw 'Missing or duplicate archive checksum.' }
    $Expected = ($MatchesFound[0] -split '  ')[0]
    if ((Get-FileHash $Archive -Algorithm SHA256).Hash.ToLowerInvariant() -ne $Expected) { throw 'Checksum mismatch; installation was not changed.' }
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $Zip = [IO.Compression.ZipFile]::OpenRead($Archive)
    try { if ($Zip.Entries.Count -ne 1 -or $Zip.Entries[0].FullName -ne 'fql.exe') { throw 'Unexpected archive contents.' } }
    finally { $Zip.Dispose() }
    Expand-Archive -Path $Archive -DestinationPath (Join-Path $Temp 'extracted')
    $Binary = Join-Path $Temp 'extracted\fql.exe'
    if ((& $Binary version) -ne "fql $Version" -or $LASTEXITCODE -ne 0) { throw 'Executable failed version verification.' }
    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    $Destination = Join-Path $InstallDir 'fql.exe'
    if (Test-Path $Destination -PathType Container) { throw 'Destination fql.exe is a directory.' }
    $Stage = Join-Path $InstallDir ('.fql-' + [Guid]::NewGuid() + '.exe')
    Copy-Item $Binary $Stage
    if (Test-Path $Destination) { [IO.File]::Replace($Stage, $Destination, $null) }
    else { [IO.File]::Move($Stage, $Destination) }
    $Stage = $null
    if (-not $NoModifyPath -and $env:FQL_MODIFY_PATH -ne '0') {
        [string]$UserPath = [Environment]::GetEnvironmentVariable('Path', 'User')
        if (@($UserPath -split ';') -notcontains $InstallDir) {
            [Environment]::SetEnvironmentVariable('Path', (($UserPath.TrimEnd(';') + ';' + $InstallDir).TrimStart(';')), 'User')
        }
    }
    if (@($env:Path -split ';') -notcontains $InstallDir) { $env:Path += ';' + $InstallDir }
    Write-Host "Installed FQL $Version to $Destination. Run: fql version"
}
finally {
    if ($Stage -and (Test-Path $Stage)) { Remove-Item $Stage -Force }
    if (Test-Path $Temp) { Remove-Item $Temp -Recurse -Force }
}
