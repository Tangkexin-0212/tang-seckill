#Requires -Version 5.1
<#
.SYNOPSIS
    启动秒杀系统前端（Vite + Vue3）。

.DESCRIPTION
    网关既没有 CORS 也不托管静态资源，所以前端统一用"同源反代"的方式访问后端：
    浏览器只请求 127.0.0.1:5173/api/*，由 Vite 的 server.proxy 转发到 127.0.0.1:8080。
    这样不需要改动 gateway 一行代码，也永远不会有跨域问题。

    两种模式：
      dev（默认）  npm run dev      → http://127.0.0.1:5173   日常开发，热更新
      -Build       npm run build    → npm run preview → http://127.0.0.1:4173
                   验证生产构建产物时用，preview 也挂了同样的代理

    前提：Node 已安装（默认 C:\Program Files\nodejs），后端已由 scripts\start-all.ps1 启动。

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File .\scripts\start-web.ps1
    powershell -ExecutionPolicy Bypass -File .\scripts\start-web.ps1 -Build
    powershell -ExecutionPolicy Bypass -File .\scripts\start-web.ps1 -SkipInstall
#>
param(
    [switch]$Build,
    [switch]$Background,
    [switch]$SkipInstall,
    [int]$Port = 5173
)

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$webDir = Join-Path $root "web"
$logDir = Join-Path $root "logs"

if (-not (Test-Path $webDir)) {
    throw "找不到前端目录：$webDir"
}

# ---- Node 定位：优先 PATH，否则回落到默认安装位置 ----
# 注意：这里不能用 `?.`（PowerShell 7 专有），本脚本要兼容 Windows PowerShell 5.1
$nodeCmd = Get-Command node -ErrorAction SilentlyContinue
$nodeExe = $null
if ($nodeCmd) { $nodeExe = $nodeCmd.Source }

if (-not $nodeExe) {
    $fallback = "C:\Program Files\nodejs\node.exe"
    if (Test-Path $fallback) {
        $nodeExe = $fallback
        $env:Path = "C:\Program Files\nodejs;" + $env:Path
        Write-Host "      已自动补入 Node 路径：C:\Program Files\nodejs" -ForegroundColor DarkGray
    }
}
if (-not $nodeExe) {
    throw "未找到 Node。请安装 Node 20+ 后重试（https://nodejs.org）。"
}
Write-Host "      Node $(& $nodeExe -v)" -ForegroundColor DarkGray

# ---- 后端可达性提示（不阻塞启动） ----
try {
    Invoke-RestMethod "http://127.0.0.1:8080/health" -TimeoutSec 3 | Out-Null
    Write-Host "      gateway 127.0.0.1:8080 可达" -ForegroundColor DarkGray
} catch {
    Write-Host "      警告：gateway 127.0.0.1:8080 当前不可达，接口会报 20001/网络错误。" -ForegroundColor Yellow
    Write-Host "            先执行：powershell -ExecutionPolicy Bypass -File .\scripts\start-all.ps1" -ForegroundColor DarkGray
}

Push-Location $webDir
try {
    if (-not $SkipInstall -and -not (Test-Path (Join-Path $webDir "node_modules"))) {
        Write-Host "      首次运行，安装依赖（npm install）..." -ForegroundColor Cyan
        npm install --no-fund --no-audit
        if ($LASTEXITCODE -ne 0) { throw "npm install 失败" }
    }

    if ($Build) {
        Write-Host "      构建生产产物（npm run build）..." -ForegroundColor Cyan
        npm run build
        if ($LASTEXITCODE -ne 0) { throw "构建失败" }
        $target = "http://127.0.0.1:4173"
        $cmd = "npm run preview"
    } else {
        $target = "http://127.0.0.1:$Port"
        $cmd = "npm run dev -- --port $Port"
    }

    if ($Background) {
        New-Item -ItemType Directory -Force -Path $logDir | Out-Null
        Start-Process -FilePath "cmd.exe" `
            -ArgumentList "/c", $cmd `
            -WorkingDirectory $webDir `
            -WindowStyle Hidden `
            -RedirectStandardOutput (Join-Path $logDir "web.log") `
            -RedirectStandardError (Join-Path $logDir "web.err.log")
        Write-Host "      前端已在后台启动：$target" -ForegroundColor Green
        Write-Host "      日志：$(Join-Path $logDir 'web.log')" -ForegroundColor DarkGray
    } else {
        Write-Host ""
        Write-Host "  前端入口：$target" -ForegroundColor Green
        Write-Host "  按 Ctrl+C 停止" -ForegroundColor DarkGray
        Write-Host ""
        Invoke-Expression $cmd
    }
} finally {
    Pop-Location
}
