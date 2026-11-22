# auto_ai 测试运行脚本
# 用法: .\run_tests.ps1              → 跑全部用例
#       .\run_tests.ps1 -Filter login → 只跑名称含 login 的用例
#       .\run_tests.ps1 -Debug        → 显示浏览器窗口

param(
    [string]$Filter = "",
    [switch]$Debug
)

$reportsDir = "reports"
$timestamp = Get-Date -Format "yyyyMMdd_HHmmss"

# 确保目录存在
if (!(Test-Path $reportsDir)) {
    New-Item -ItemType Directory -Force -Path $reportsDir | Out-Null
}
if (!(Test-Path "screenshots")) {
    New-Item -ItemType Directory -Force -Path "screenshots" | Out-Null
}

$htmlReport = "$reportsDir/report_$timestamp.html"
$xmlReport  = "$reportsDir/report_$timestamp.xml"

Write-Host "==================== auto_ai 自动化测试 ====================" -ForegroundColor Cyan
Write-Host "HTML 报告: $htmlReport"
Write-Host "XML  报告: $xmlReport"
Write-Host "===========================================================" -ForegroundColor Cyan

$pytestArgs = @(
    "tests/",
    "-v",
    "-s",
    "--html=$htmlReport",
    "--self-contained-html",
    "--junitxml=$xmlReport"
)

if ($Filter) {
    $pytestArgs += "-k"
    $pytestArgs += $Filter
    Write-Host "筛选条件: $Filter" -ForegroundColor Yellow
}

if ($Debug) {
    $env:AUTO_AI_HEADLESS = "false"
    Write-Host "调试模式: 显示浏览器窗口" -ForegroundColor Yellow
}

Write-Host ""
pytest @pytestArgs
