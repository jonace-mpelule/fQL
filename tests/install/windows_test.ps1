$ErrorActionPreference = 'Stop'
$Root = (Resolve-Path "$PSScriptRoot/../..").Path
$Fixture = Join-Path ([IO.Path]::GetTempPath()) ('fql-windows-test-' + [Guid]::NewGuid())
$OldPath = $env:Path
try {
    New-Item -ItemType Directory -Path $Fixture | Out-Null
    $Version = '0.1.2-dev.1'
    $env:FQL_TEST_VERSION = $Version
    $env:FQL_TEST_FIXTURE = $Fixture
    $Binary = Join-Path $Fixture 'fql.exe'
    & go build -trimpath -ldflags "-X main.version=$Version" -o $Binary ./cmd/fql
    if ($LASTEXITCODE -ne 0) { throw 'Windows fixture build failed.' }
    $Archive = Join-Path $Fixture "fql_${Version}_windows_amd64.zip"
    Compress-Archive -Path $Binary -DestinationPath $Archive
    $Hash = (Get-FileHash $Archive -Algorithm SHA256).Hash.ToLowerInvariant()
    "$Hash  fql_${Version}_windows_amd64.zip" | Set-Content (Join-Path $Fixture 'SHA256SUMS') -Encoding ascii
    function global:Invoke-RestMethod { param($Uri, $Headers) return @{tag_name=('v' + $env:FQL_TEST_VERSION)} }
    function global:Invoke-WebRequest {
        param($Uri, $Headers, $OutFile, [switch]$UseBasicParsing)
        Copy-Item (Join-Path $env:FQL_TEST_FIXTURE ($Uri.Split('/')[-1])) $OutFile
    }
    $InstallDir = Join-Path $Fixture 'user tools'
    & "$Root/install.ps1" -InstallDir $InstallDir -NoModifyPath
    $Installed = Join-Path $InstallDir 'fql.exe'
    if ((& $Installed version) -ne "fql $Version") { throw 'Installer version mismatch.' }
    & "$Root/install.ps1" -Version $Version -InstallDir $InstallDir -NoModifyPath
    $Before = (Get-FileHash $Installed).Hash
    [IO.File]::AppendAllText($Archive, 'tampered')
    $Rejected = $false
    try { & "$Root/install.ps1" -Version $Version -InstallDir $InstallDir -NoModifyPath }
    catch { if ($_.Exception.Message -notmatch 'Checksum mismatch') { throw }; $Rejected = $true }
    if (-not $Rejected -or (Get-FileHash $Installed).Hash -ne $Before) { throw 'Failed download changed existing installation.' }
    Write-Host 'Windows install, upgrade and checksum-rejection checks passed.'
}
finally {
    $env:Path = $OldPath
    Remove-Item Function:Invoke-RestMethod -ErrorAction SilentlyContinue
    Remove-Item Function:Invoke-WebRequest -ErrorAction SilentlyContinue
    if (Test-Path $Fixture) { Remove-Item $Fixture -Recurse -Force }
}
