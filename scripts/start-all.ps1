#Requires -Version 5.1
<#
.SYNOPSIS
    一键启动秒杀系统的全部组件。

.DESCRIPTION
    1. 拉起基础设施（MySQL / Redis / etcd）并等待健康
    2. 编译 6 个领域服务 + gateway + cron + worker
    3. 后台启动全部进程，日志写到 logs/

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File .\scripts\start-all.ps1
    powershell -ExecutionPolicy Bypass -File .\scripts\start-all.ps1 -SkipInfra -SkipBuild
    powershell -ExecutionPolicy Bypass -File .\scripts\start-all.ps1 -SkipInfra -SkipBuild -WithWeb
#>
param(
    [switch]$SkipBuild,
    [switch]$SkipInfra,
    # 顺带把前端 Vite dev server 也拉起来（默认不动，保持后端启动流程干净）
    [switch]$WithWeb
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

$logDir = Join-Path $root "logs"
$binDir = Join-Path $root "bin"
New-Item -ItemType Directory -Force -Path $logDir | Out-Null
New-Item -ItemType Directory -Force -Path $binDir | Out-Null

# 顺序即依赖：先领域服务，再网关、定时任务与消费者
$apps = @("user", "product", "stock", "cart", "order", "seckill", "gateway", "cron", "worker")

if (-not $SkipInfra) {
    Write-Host "[1/4] 启动基础设施 MySQL / Redis / etcd ..." -ForegroundColor Cyan
    docker compose up -d

    Write-Host "      等待 MySQL 健康 ..." -ForegroundColor DarkGray
    $deadline = (Get-Date).AddSeconds(180)
    while ($true) {
        $status = (docker inspect --format "{{.State.Health.Status}}" sk-mysql 2>$null)
        if ($status -eq "healthy") { break }
        if ((Get-Date) -gt $deadline) { throw "MySQL 未在 180s 内就绪，请检查 docker logs sk-mysql" }
        Start-Sleep -Seconds 2
    }
    Write-Host "      MySQL 已就绪" -ForegroundColor Green
} else {
    Write-Host "[1/4] 跳过基础设施启动" -ForegroundColor DarkGray
}

if (-not $SkipBuild) {
    Write-Host "[2/4] 编译全部服务 ..." -ForegroundColor Cyan
    go build -o "$binDir/" ./user ./product ./stock ./cart ./order ./seckill ./gateway ./cron ./worker
    if ($LASTEXITCODE -ne 0) { throw "编译失败" }
    Write-Host "      编译完成 -> $binDir" -ForegroundColor Green
} else {
    Write-Host "[2/4] 跳过编译" -ForegroundColor DarkGray
}

Write-Host "[3/4] 启动服务进程 ..." -ForegroundColor Cyan
foreach ($app in $apps) {
    $exe = Join-Path $binDir "$app.exe"
    if (-not (Test-Path $exe)) {
        Write-Host "      跳过 $app（未找到 $exe）" -ForegroundColor Yellow
        continue
    }
    Start-Process -FilePath $exe `
        -WorkingDirectory $root `
        -WindowStyle Hidden `
        -RedirectStandardOutput (Join-Path $logDir "$app.log") `
        -RedirectStandardError (Join-Path $logDir "$app.err.log")
    Write-Host "      - $app 已启动" -ForegroundColor Green
}

if ($WithWeb) {
    Write-Host "[5/5] 启动前端 Vite dev server ..." -ForegroundColor Cyan
    & (Join-Path $PSScriptRoot "start-web.ps1") -Background
}

Start-Sleep -Seconds 2
Write-Host "[4/4] 全部完成，日志目录：$logDir" -ForegroundColor Cyan
Write-Host ""
Write-Host "前端入口：" -ForegroundColor White
Write-Host "  http://127.0.0.1:5173   （需 -WithWeb，或单独执行 scripts\start-web.ps1）" -ForegroundColor DarkGray
Write-Host "验证网关：" -ForegroundColor White
Write-Host "  Invoke-RestMethod http://127.0.0.1:8080/health" -ForegroundColor DarkGray
Write-Host "运行压测：" -ForegroundColor White
Write-Host "  go run ./tools/loadtest -users 1000 -stock 100" -ForegroundColor DarkGray
