#Requires -Version 5.1
<#
.SYNOPSIS
    停止秒杀系统的全部应用进程（默认保留基础设施容器）。
.EXAMPLE
    powershell -ExecutionPolicy Bypass -File .\scripts\stop-all.ps1
    powershell -ExecutionPolicy Bypass -File .\scripts\stop-all.ps1 -WithInfra
#>
param(
    [switch]$WithInfra
)

$ErrorActionPreference = "Continue"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

$apps = @("user", "product", "stock", "cart", "order", "seckill", "gateway", "cron", "worker")

foreach ($app in $apps) {
    $procs = Get-Process -Name $app -ErrorAction SilentlyContinue
    if ($procs) {
        $procs | Stop-Process -Force
        Write-Host "- 已停止 $app" -ForegroundColor Green
    }
}

if ($WithInfra) {
    Write-Host "停止基础设施容器 ..." -ForegroundColor Cyan
    docker compose down
}
