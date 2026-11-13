[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [string]$Output,
    [string]$QtRoot = $env:QT_ROOT_DIR,
    [string]$Generator = 'MinGW Makefiles',
    [Parameter(Mandatory)]
    [string]$Backend,
    [string]$RussianSource
)

$ErrorActionPreference = 'Stop'
function Invoke-Checked([string]$Program, [string[]]$Arguments) {
    & $Program @Arguments
    if ($LASTEXITCODE -ne 0) { throw "$Program failed with exit code $LASTEXITCODE" }
}

function Copy-SetupArtwork([string]$Source, [string]$Destination) {
    Add-Type -AssemblyName System.Drawing
    $image = [Drawing.Image]::FromFile($Source)
    try {
        $maxWidth = 1520
        if ($image.Width -le $maxWidth) {
            Copy-Item -LiteralPath $Source -Destination $Destination
            return
        }
        $height = [Math]::Max(1, [int][Math]::Round($image.Height * $maxWidth / $image.Width))
        $banner = [Drawing.Bitmap]::new($maxWidth, $height)
        try {
            $graphics = [Drawing.Graphics]::FromImage($banner)
            try {
                $graphics.InterpolationMode = [Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
                $graphics.DrawImage($image, 0, 0, $maxWidth, $height)
            } finally { $graphics.Dispose() }
            $banner.Save($Destination, [Drawing.Imaging.ImageFormat]::Jpeg)
        } finally { $banner.Dispose() }
    } finally { $image.Dispose() }
}

$outputFile = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($Output)
if ([IO.Path]::GetExtension($outputFile) -ine '.exe' -or (Test-Path -LiteralPath $outputFile)) {
    throw 'Choose a new .exe output file.'
}
if (-not $QtRoot) { throw 'Pass -QtRoot or set QT_ROOT_DIR to the Qt installation prefix.' }
$QtRoot = (Resolve-Path -LiteralPath $QtRoot).Path
$deploy = Join-Path $QtRoot 'bin/windeployqt.exe'
if (-not (Test-Path -LiteralPath $deploy -PathType Leaf)) { throw "windeployqt.exe is missing from $QtRoot/bin." }
$Backend = (Resolve-Path -LiteralPath $Backend).Path

$temp = Join-Path ([IO.Path]::GetTempPath()) ("sc4-setup-" + [Guid]::NewGuid().ToString('N'))
try {
    $stage = Join-Path $temp 'SimCity4UHD-Setup'
    $build = Join-Path $temp 'build'
    New-Item -ItemType Directory -Path $stage | Out-Null

    Copy-Item -LiteralPath $Backend -Destination (Join-Path $stage 'app.exe')
    Copy-SetupArtwork (Join-Path $PSScriptRoot '../assets/screenshot.jpg') (Join-Path $stage 'screenshot.jpg')
    $russian = Join-Path $stage 'Russian/russian.sc4pack'
    if ($RussianSource) {
        Invoke-Checked $Backend @('russian-pack', 'pack', $RussianSource, $russian)
    } else {
        New-Item -ItemType Directory -Path (Split-Path -Parent $russian) | Out-Null
        Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'resources/russian.sc4pack') -Destination $russian
    }
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'resources/FONT-LICENSES.txt') -Destination $stage

    Invoke-Checked cmake @('-S', (Join-Path $PSScriptRoot 'qt'), '-B', $build, '-G', $Generator,
        "-DCMAKE_PREFIX_PATH=$QtRoot", '-DCMAKE_BUILD_TYPE=Release')
    Invoke-Checked cmake @('--build', $build, '--config', 'Release', '--parallel')
    Invoke-Checked cmake @('--install', $build, '--config', 'Release', '--prefix', $stage)
    # The installer uses raster Qt Widgets and loads JPEG and ICO artwork.
    Invoke-Checked $deploy @('--release', '--no-translations',
        '--skip-plugin-types', 'generic,networkinformation,tls',
        '--exclude-plugins', 'qgif',
        '--no-system-d3d-compiler', '--no-system-dxc-compiler', '--no-opengl-sw',
        (Join-Path $stage 'qt/Setup-UI.exe'))
    if (-not (Test-Path -LiteralPath (Join-Path $stage 'qt/platforms/qwindows.dll'))) {
        throw 'The Qt Windows platform plugin was not deployed.'
    }
    foreach ($file in @('setup.json', 'START-HERE.txt', 'THIRD-PARTY.txt', 'setup.ico')) {
        Copy-Item -LiteralPath (Join-Path $PSScriptRoot $file) -Destination $stage
    }
    $bootstrap = Join-Path $temp 'bootstrap'
    New-Item -ItemType Directory -Path $bootstrap | Out-Null
    Get-ChildItem -LiteralPath $PSScriptRoot -Filter '*.go' -File | Copy-Item -Destination $bootstrap
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'go.mod') -Destination $bootstrap

    Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'go.sum') -Destination $bootstrap

    $sevenZip = Get-Command 7z.exe -ErrorAction SilentlyContinue
    $extractor = if ($sevenZip) { $sevenZip.Source } else { Join-Path $env:ProgramFiles '7-Zip/7z.exe' }
    $runtimeTar = Join-Path $temp 'runtime.tar'
    Invoke-Checked tar.exe @('-cf', $runtimeTar, '-C', $stage, '.')
    Invoke-Checked $extractor @('a', '-txz', '-mx=9', '-md=32m', '-mmt=1',
        (Join-Path $bootstrap 'runtime.tar.xz'), $runtimeTar)

    New-Item -ItemType Directory -Force -Path ([IO.Path]::GetDirectoryName($outputFile)) | Out-Null
    Push-Location $bootstrap
    try {
        Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'setup.ico') -Destination .
        Set-Content -LiteralPath 'setup-icon.rc' -Encoding Ascii -Value '1 ICON "setup.ico"'
        Invoke-Checked windres.exe @('--target=pe-x86-64', '-i', 'setup-icon.rc', '-O', 'coff', '-o', 'setup-icon_windows_amd64.syso')
        $env:GOOS = 'windows'
        $env:GOARCH = 'amd64'
        $env:CGO_ENABLED = '0'
        Invoke-Checked go @('build', '-trimpath', '-tags', 'bundled', '-ldflags', '-H windowsgui -s -w', '-o', $outputFile, '.')
    } finally { Pop-Location }
} finally {
    if (Test-Path -LiteralPath $temp) { Remove-Item -LiteralPath $temp -Recurse -Force }
}
