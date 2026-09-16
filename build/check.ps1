#Requires -Version 7.0
[CmdletBinding()]
param(
    [Parameter()]
    [switch]$SkipBuild
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# Определение абсолютного пути к корню репозитория
$repoRoot = [System.IO.Path]::GetFullPath((Join-Path -Path $PSScriptRoot -ChildPath '..'))
Write-Output "=== Запуск комплексных локальных проверок проекта ==="
Write-Output "Корень репозитория: $repoRoot"

# 1. Обеспечение доступности компилятора Go
if (-not (Get-Command -Name 'go' -ErrorAction SilentlyContinue)) {
    $candidateGoPaths = @(
        'C:\Program Files\Go\bin',
        'C:\Go\bin'
    )
    foreach ($candidate in $candidateGoPaths) {
        $goExe = Join-Path -Path $candidate -ChildPath 'go.exe'
        if (Test-Path -LiteralPath $goExe) {
            $env:PATH = "$candidate;$env:PATH"
            break
        }
    }
    if (-not (Get-Command -Name 'go' -ErrorAction SilentlyContinue)) {
        throw "Ошибка: компилятор Go не найден. Проверьте установку Go."
    }
}

# 2. Проверки Go-кода
Write-Output "`n--- [1/4] Проверки Go-ядра ---"
$coreDir = Join-Path -Path $repoRoot -ChildPath 'core'
$prevLocation = Get-Location
try {
    Set-Location -LiteralPath $coreDir

    # Проверка форматирования gofmt
    Write-Output "Проверка форматирования gofmt..."
    $unformattedFiles = & gofmt -l .
    if (-not [string]::IsNullOrWhiteSpace(($unformattedFiles -join ''))) {
        throw "Следующие файлы Go требуют форматирования через gofmt:`n$($unformattedFiles -join "`n")"
    }

    # Анализ кода go vet
    Write-Output "Выполнение go vet ./... ..."
    & go vet ./...
    if ($LASTEXITCODE -ne 0) {
        throw "Ошибка при выполнении go vet (код: $LASTEXITCODE)"
    }

    # Запуск модульных тестов go test
    Write-Output "Выполнение тестов go test ./... ..."
    & go test -v ./...
    if ($LASTEXITCODE -ne 0) {
        throw "Тесты Go завершились с ошибкой (код: $LASTEXITCODE)"
    }

    # Тестовая компиляция
    Write-Output "Проверочная компиляция Go-команды..."
    & go build -o (Join-Path -Path $env:TEMP -ChildPath 'crowdanki-core-check.tmp') ./cmd/crowdanki-core
    if ($LASTEXITCODE -ne 0) {
        throw "Ошибка проверочной компиляции Go-ядра (код: $LASTEXITCODE)"
    }
    Remove-Item -LiteralPath (Join-Path -Path $env:TEMP -ChildPath 'crowdanki-core-check.tmp') -Force -ErrorAction SilentlyContinue
} finally {
    Set-Location -LiteralPath $prevLocation
}
Write-Output "Все проверки Go пройдены успешно."

# 3. Проверки Python-кода
Write-Output "`n--- [2/4] Проверки Python-адаптера ---"
if (-not (Get-Command -Name 'ruff' -ErrorAction SilentlyContinue)) {
    throw "Ошибка: инструмент ruff не найден. Установите ruff (pip install ruff)."
}

Write-Output "Запуск ruff check..."
& ruff check $repoRoot
if ($LASTEXITCODE -ne 0) {
    throw "Линтер Ruff обнаружил нарушения (код: $LASTEXITCODE)"
}

Write-Output "Запуск ruff format --check..."
& ruff format --check $repoRoot
if ($LASTEXITCODE -ne 0) {
    throw "Обнаружены проблемы форматирования Python через ruff format (код: $LASTEXITCODE)"
}

Write-Output "Проверка синтаксиса Python-файлов (py_compile)..."
$pyFiles = Get-ChildItem -Path (Join-Path -Path $repoRoot -ChildPath 'addon') -Filter '*.py' -Recurse
foreach ($pyFile in $pyFiles) {
    & python -m py_compile $pyFile.FullName
    if ($LASTEXITCODE -ne 0) {
        throw "Ошибка компиляции синтаксиса Python в файле: $($pyFile.FullName)"
    }
}
Write-Output "Запуск модульных тестов Python (unittest)..."
$prevPythonPath = $env:PYTHONPATH
try {
    $env:PYTHONPATH = (Join-Path -Path $repoRoot -ChildPath 'addon')
    & python -m unittest discover -s (Join-Path -Path $repoRoot -ChildPath 'addon/crowdanki_v2/tests') -v
    if ($LASTEXITCODE -ne 0) {
        throw "Модульные тесты Python завершились с ошибкой (код: $LASTEXITCODE)"
    }
} finally {
    $env:PYTHONPATH = $prevPythonPath
}

$addonPycache = Get-ChildItem -Path (Join-Path -Path $repoRoot -ChildPath 'addon') -Recurse -Directory -Filter '__pycache__' -ErrorAction SilentlyContinue
foreach ($cacheDir in $addonPycache) {
    Remove-Item -LiteralPath $cacheDir.FullName -Recurse -Force
}
Write-Output "Все проверки Python пройдены успешно."

# 4. Проверки PowerShell (PSScriptAnalyzer)
Write-Output "`n--- [3/4] Проверки сценариев PowerShell ---"
if (-not (Get-Module -ListAvailable -Name 'PSScriptAnalyzer')) {
    throw "Ошибка: модуль PSScriptAnalyzer не установлен. Выполните: Install-Module -Name PSScriptAnalyzer -Scope CurrentUser"
}

$scriptsToAnalyze = Get-ChildItem -Path (Join-Path -Path $repoRoot -ChildPath 'build') -Filter '*.ps1'
$hasErrors = $false
foreach ($script in $scriptsToAnalyze) {
    Write-Output "Анализ сценария: $($script.Name)..."
    $issues = Invoke-ScriptAnalyzer -Path $script.FullName
    if ($issues) {
        $issues | Format-Table -Property RuleName, Severity, ScriptName, Line, Message
        $errorsOrWarnings = $issues | Where-Object { $_.Severity -in @('Error', 'Warning') }
        if ($errorsOrWarnings) {
            $hasErrors = $true
        }
    }
}

if ($hasErrors) {
    throw "PSScriptAnalyzer выявил критические предупреждения или ошибки в скриптах."
}
Write-Output "Все проверки сценариев PowerShell пройдены успешно."

# 5. Проверочная сборка пакета аддона
if (-not $SkipBuild) {
    Write-Output "`n--- [4/4] Проверочная сборка .ankiaddon ---"
    $buildScript = Join-Path -Path $PSScriptRoot -ChildPath 'build.ps1'
    & $buildScript
    if ($LASTEXITCODE -ne 0) {
        throw "Сценарий сборки завершился с ошибкой (код: $LASTEXITCODE)"
    }
    Write-Output "Проверочная сборка пакета успешно завершена."
}

Write-Output "`n=== Все проверки проекта успешно пройдены! ==="




