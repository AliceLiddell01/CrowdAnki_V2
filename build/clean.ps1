#Requires -Version 7.0
[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# Определение абсолютного пути к корню репозитория
$repoRoot = [System.IO.Path]::GetFullPath((Join-Path -Path $PSScriptRoot -ChildPath '..'))

Write-Output "Очистка артефактов сборки в: $repoRoot"

# Каталоги для безопасного удаления
$targetsToRemove = @(
    (Join-Path -Path $repoRoot -ChildPath 'dist'),
    (Join-Path -Path $repoRoot -ChildPath 'build/.staging'),
    (Join-Path -Path $repoRoot -ChildPath '.ruff_cache')
)

foreach ($target in $targetsToRemove) {
    $normalizedPath = [System.IO.Path]::GetFullPath($target)

    # Проверка безопасности: удаляемый путь обязан быть внутри репозитория и не совпадать с корнем
    if (-not $normalizedPath.StartsWith($repoRoot, [System.StringComparison]::OrdinalIgnoreCase) -or
        $normalizedPath -eq $repoRoot) {
        throw "Небезопасный путь для удаления: $normalizedPath"
    }

    if (Test-Path -LiteralPath $normalizedPath) {
        Write-Output "Удаление каталога: $normalizedPath"
        Remove-Item -LiteralPath $normalizedPath -Recurse -Force
    }
}

# Очистка директорий __pycache__ внутри репозитория
$pyCacheDirs = Get-ChildItem -LiteralPath $repoRoot -Recurse -Directory -Filter '__pycache__' -ErrorAction SilentlyContinue
foreach ($dir in $pyCacheDirs) {
    if ($dir.FullName.StartsWith($repoRoot, [System.StringComparison]::OrdinalIgnoreCase) -and
        $dir.FullName -ne $repoRoot) {
        Remove-Item -LiteralPath $dir.FullName -Recurse -Force
    }
}

Write-Output "Очистка завершена успешно."


