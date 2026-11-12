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
        Path = "$repository/qt6_6120/qt6_6120_mingw/qt.qt6.6120.win64_mingw/6.12.0-0-202609280346qtbase-Windows-Windows_11_24H2-Mingw-Windows-Windows_11_24H2-X86_64.7z"
        SHA = '3356346505113242a4f7c0b09ba46188424ee7938232166655f794c28a3477fc'
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
