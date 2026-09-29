#Requires -Version 5.1
<#
.SYNOPSIS
    把项目 Redis（sk-redis）里的 sk:* 数据同步到另一个 Redis 实例。

.DESCRIPTION
    用途：本机调试时想用 Navicat / RedisInsight 直接翻项目数据，但项目 Redis 映射在
    非默认端口（16379）上，于是把数据复制一份到常见端口（6379）的实例里看。

    实现要点（都是踩过的坑）：
      1. 用 MIGRATE ... COPY REPLACE，而不是 DUMP/RESTORE 循环：
         - COPY 保证源数据不动（项目还在正常用）；
         - REPLACE 让脚本可重复执行；
         - TTL 由 MIGRATE 自动跟随，不会像手写 RESTORE 那样漏掉 ttl 参数变成永久键。
      2. MIGRATE 由「源 Redis 主动连目标」，所以只需源容器能到目标；脚本通过
         目标容器的网络别名或宿主端口访问，实测 Docker Desktop 上跨 bridge 网络可达。
      3. Redis 7.x 的流对象序列化包含消费组，所以 Stream 的 g:order / consumers /
         last-delivered-id 也会一起过去（Redis 5/6 时代不会）。
      4. 脚本以 stdin 方式喂给容器内 sh，绕开 Windows/PowerShell 的引号与 CRLF 问题。

.NOTES
    这是「快照复制」，不是持续复制：源端 TTL 在走、还会有新数据，需要刷新时重跑即可。
    -SkipTokens 只控制「不复制」登录 token，不会删除目标端已存在的 token 键。

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File .\scripts\sync-redis.ps1
    powershell -ExecutionPolicy Bypass -File .\scripts\sync-redis.ps1 -SkipTokens
    powershell -ExecutionPolicy Bypass -File .\scripts\sync-redis.ps1 -Pattern "sk:cart*"
#>
param(
    # 源：项目 Redis 容器名（脚本在容器内执行 redis-cli，默认连自己的 6379）
    [string]$SourceContainer = "sk-redis",
    # 目标：宿主上的 Redis。默认走 Docker Desktop 的 host.docker.internal，
    # 比硬编码容器 IP（如 172.17.0.5）稳，容器重建后依然有效。
    [string]$TargetHost = "host.docker.internal",
    [int]$TargetPort = 6379,
    [string]$Pattern = "sk:*",
    # 不复制 sk:user:token:*（2795 个登录 token，会让键浏览器全是噪音）
    [switch]$SkipTokens
)

$ErrorActionPreference = "Stop"

$sh = @'
DST_HOST=__DST_HOST__
DST_PORT=__DST_PORT__

redis-cli --scan --pattern '__PATTERN__' > /tmp/sk-keys.txt
total=$(grep -c . /tmp/sk-keys.txt)
echo "pattern=__PATTERN__ keys=$total"

ok=0
fail=0
skipped=0

while IFS= read -r k; do
  [ -z "$k" ] && continue
  case "$k" in
    __SKIP_CASE__) skipped=$((skipped+1)); continue ;;
  esac

  r=$(redis-cli MIGRATE "$DST_HOST" "$DST_PORT" "$k" 0 5000 COPY REPLACE)
  if [ "$r" = "OK" ]; then
    ok=$((ok+1))
  else
    fail=$((fail+1))
    echo "FAIL $k :: $r"
  fi
done < /tmp/sk-keys.txt

echo "ok=$ok fail=$fail skipped=$skipped"
echo "target_dbsize=$(redis-cli -h "$DST_HOST" -p "$DST_PORT" DBSIZE)"
'@

$sh = $sh.Replace('__DST_HOST__', $TargetHost)
$sh = $sh.Replace('__DST_PORT__', "$TargetPort")
$sh = $sh.Replace('__PATTERN__', $Pattern)
$sh = $sh.Replace('__SKIP_CASE__', $(if ($SkipTokens) { 'sk:user:token:*' } else { '__never_match__' }))
# 换成 LF，否则容器里的 sh 会把 \r 当成命令的一部分
$sh = $sh -replace "`r`n", "`n"

Write-Host "源容器 : $SourceContainer" -ForegroundColor Cyan
Write-Host "目标   : ${TargetHost}:${TargetPort}" -ForegroundColor Cyan
Write-Host "键模式 : $Pattern$(if ($SkipTokens) { ' （跳过 sk:user:token:*）' })" -ForegroundColor Cyan
Write-Host ""

# 为什么不走管道（$sh | docker exec -i ... sh -s）：
# 实测本机 PowerShell 5.1 往子进程 stdin 写文本时会额外带上 BOM（甚至是两个），
# 而 BOM 不是空白字符，sh 会把首行 "<BOM>DST_HOST=..." 当成命令执行并报
# "not found"，赋值/注释全部失效。$OutputEncoding 和 TrimStart 都治不了它。
# 因此改为：落地成 ASCII（无 BOM）文件 → docker cp 进容器 → 按文件执行。
$tmp = Join-Path $env:TEMP "sk-sync-redis.sh"
# 内容全是 ASCII，用 ASCIIEncoding 写，确保零 BOM
[System.IO.File]::WriteAllText($tmp, $sh, (New-Object System.Text.ASCIIEncoding))

$containerPath = "/tmp/sk-sync-redis.sh"
docker cp $tmp "${SourceContainer}:${containerPath}" | Out-Null
try {
    docker exec $SourceContainer sh $containerPath
} finally {
    docker exec $SourceContainer rm -f $containerPath | Out-Null
    Remove-Item -Path $tmp -Force -ErrorAction SilentlyContinue
}
