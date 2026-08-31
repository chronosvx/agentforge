#Requires -Version 5.1
# agentforge Windows 安装脚本
#
# 默认优先使用本机 Go 从源码构建；没有 Go 时尝试从 GitHub Releases 下载
# 最新版/指定版二进制。安装目录为 $env:LOCALAPPDATA\agentforge\bin。
#
# 用法：
#   powershell -ExecutionPolicy Bypass -File scripts\install.ps1
#   powershell -ExecutionPolicy Bypass -File scripts\install.ps1 -Version 0.2.0
#   powershell -ExecutionPolicy Bypass -File scripts\install.ps1 -AddToPath
param(
    [string]$Version = "",
    [switch]$AddToPath,
    [switch]$SkipPathHint
)

$ErrorActionPreference = 'Stop'

# 发布前请将 chronosvx 替换为实际 GitHub 用户名。
$Owner = 'chronosvx'
$Repo = 'agentforge'
$BaseDir = if ($env:LOCALAPPDATA) { $env:LOCALAPPDATA } else { Join-Path $HOME 'AppData\Local' }
$InstallDir = Join-Path $BaseDir 'agentforge'
$BinDir = Join-Path $InstallDir 'bin'
$ExePath = Join-Path $BinDir 'agentforge.exe'

New-Item -ItemType Directory -Force -Path $BinDir | Out-Null

function Get-Arch {
    $arch = $env:PROCESSOR_ARCHITECTURE
    if ($arch -eq 'AMD64') { return 'amd64' }
    if ($arch -eq 'ARM64') { return 'arm64' }
    throw "Unsupported processor architecture: $arch"
}

function Install-FromSource {
    Write-Host "==> building agentforge from source"
    $go = Get-Command go -ErrorAction SilentlyContinue
    if (-not $go) {
        throw "Go is not installed. Install Go from https://go.dev/dl or pass -Version to download a release binary."
    }
    $root = Split-Path -Parent $PSScriptRoot
    Push-Location $root
    try {
        & go build -o $ExePath ./cmd/agentforge
        if ($LASTEXITCODE -ne 0) {
            throw "go build failed with exit code $LASTEXITCODE"
        }
    }
    finally {
        Pop-Location
    }
}

function Install-FromRelease {
    param([string]$VersionToUse)

    $arch = Get-Arch
    if ($VersionToUse -eq '') {
        Write-Host "==> querying latest release from GitHub"
        $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Owner/$Repo/releases/latest" -Headers @{ 'User-Agent' = 'agentforge-installer' }
        $VersionToUse = $release.tag_name
    }

    $tag = if ($VersionToUse -match '^v') { $VersionToUse } else { "v$VersionToUse" }
    $url = "https://github.com/$Owner/$Repo/releases/download/$tag/agentforge-windows-$arch.exe"
    Write-Host "==> downloading $url"
    Invoke-WebRequest -Uri $url -OutFile $ExePath
}

if ($Version -ne '' -or -not (Get-Command go -ErrorAction SilentlyContinue)) {
    Install-FromRelease -VersionToUse $Version
}
else {
    Install-FromSource
}

Write-Host "==> installed to $ExePath"

if ($AddToPath) {
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if ($userPath -notlike "*$BinDir*") {
        $newPath = if ($userPath) { "$userPath;$BinDir" } else { $BinDir }
        [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
        Write-Host "==> added $BinDir to user PATH (new terminals only)"
    }
    else {
        Write-Host "==> $BinDir is already in user PATH"
    }
}
elseif (-not $SkipPathHint) {
    Write-Host ""
    Write-Host "请将以下目录加入 PATH，或重新打开终端后运行："
    Write-Host "  $BinDir"
    Write-Host ""
    Write-Host "当前终端可临时使用："
    Write-Host "  `$env:Path = `"$env:Path;$BinDir`""
    Write-Host "  agentforge --version"
}
