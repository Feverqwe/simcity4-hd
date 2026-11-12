[CmdletBinding()]
param([Parameter(Mandatory)][string]$Destination)

$ErrorActionPreference = 'Stop'
$destination = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($Destination)
New-Item -ItemType Directory -Path $destination -Force | Out-Null

$sevenZip = Get-Command 7z.exe -ErrorAction SilentlyContinue
$extractor = if ($sevenZip) { $sevenZip.Source } else { Join-Path $env:ProgramFiles '7-Zip/7z.exe' }
$repository = 'online/qtsdkrepository/windows_x86/desktop'
$archives = @(
    @{
        Name = 'qtbase.7z'
        Path = "$repository/qt6_683/qt6_683/qt.qt6.683.win64_mingw/6.8.3-0-202503201308qtbase-Windows-Windows_10_22H2-Mingw-Windows-Windows_10_22H2-X86_64.7z"
        SHA = 'a2cd09053e907b161d84696ec5f1c316cfc089482c5b04de9141b949fa19acd1'
    },
    @{
        Name = 'mingw.7z'
        Path = "$repository/tools_mingw1310/qt.tools.win64_mingw1310/13.1.0-202407240918mingw1310.7z"
        SHA = 'a7b502294a903b64fccd4a41ba39f48d3f6c9a5ece167f0cfaeac920d15b344f'
    }
)

foreach ($archive in $archives) {
    $file = Join-Path $destination $archive.Name
    $verified = $false
    foreach ($hostUrl in @('https://download.qt.io', 'https://mirrors.dotsrc.org/qtproject')) {
        & curl.exe -fsSL --connect-timeout 10 --max-time 180 "$hostUrl/$($archive.Path)" -o $file
        if ($LASTEXITCODE -eq 0 -and (Get-FileHash -Algorithm SHA256 $file).Hash -ieq $archive.SHA) {
            $verified = $true
            break
        }
    }
    if (-not $verified) { throw "Could not download and verify $($archive.Name)." }
    & $extractor x -y "-o$destination" $file | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Could not extract $($archive.Name)." }
    Remove-Item -LiteralPath $file
}
