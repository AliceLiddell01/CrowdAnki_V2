#Requires -Version 7.0
[CmdletBinding()]
param(
    [Parameter()]
    [string]$TargetOS = '',

    [Parameter()]
    [string]$TargetArch = '',

    [Parameter()]
    [string]$OutputName = ''
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# Определение абсолютного пути к корню репозитория
$repoRoot = [System.IO.Path]::GetFullPath((Join-Path -Path $PSScriptRoot -ChildPath '..'))
$configPath = Join-Path -Path $PSScriptRoot -ChildPath 'config.json'

if (-not (Test-Path -LiteralPath $configPath)) {
    throw "Не найден конфигурационный файл проекта: $configPath"
}

# Чтение единого источника метаданных проекта
$rawConfig = Get-Content -LiteralPath $configPath -Raw -Encoding utf8
$config = ConvertFrom-Json -InputObject $rawConfig

# Автоматическое определение целевой ОС при отсутствии явного параметра
if ([string]::IsNullOrWhiteSpace($TargetOS)) {
    if ($IsWindows) {
        $TargetOS = 'windows'
    } elseif ($IsLinux) {
        $TargetOS = 'linux'
    } elseif ($IsMacOS) {
        $TargetOS = 'darwin'
    } else {
        throw "Не удалось автоматически определить операционную систему. Укажите -TargetOS явно."
    }
}

# Автоматическое определение целевой архитектуры при отсутствии явного параметра
if ([string]::IsNullOrWhiteSpace($TargetArch)) {
    $osArch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
    if ($osArch -eq [System.Runtime.InteropServices.Architecture]::Arm64) {
        $TargetArch = 'arm64'
    } else {
        $TargetArch = 'amd64'
    }
}

$platformKey = "$TargetOS-$TargetArch"
$supportedPlatforms = $config.platforms.PSObject.Properties.Name

if ($platformKey -notin $supportedPlatforms) {
    $available = $supportedPlatforms -join ', '
    throw "Неподдерживаемая целевая платформа: '$platformKey'. Допустимые: $available"
}

$platformConfig = $config.platforms.$platformKey
$binaryName = $platformConfig.executable

Write-Output "=== Сборка аддона $($config.addon.name) ==="
Write-Output "Целевая платформа: $platformKey (бинарник: $binaryName)"

# Проверка и поиск компилятора Go
if (-not (Get-Command -Name 'go' -ErrorAction SilentlyContinue)) {
    $candidateGoPaths = @(
        'C:\Program Files\Go\bin',
        'C:\Go\bin'
    )
    $foundGo = $false
    foreach ($candidate in $candidateGoPaths) {
        $goExe = Join-Path -Path $candidate -ChildPath 'go.exe'
        if (Test-Path -LiteralPath $goExe) {
            $env:PATH = "$candidate;$env:PATH"
            $foundGo = $true
            break
        }
    }

    if (-not $foundGo) {
        throw "Ошибка: компилятор Go не найден в переменной PATH и стандартных директориях. Установите Go для сборки ядра."
    }
}

# Пути для сборки и вывода
$distDir = Join-Path -Path $repoRoot -ChildPath 'dist'
$stagingDir = Join-Path -Path $repoRoot -ChildPath 'build/.staging'

# Подготовка чистых директорий
if (Test-Path -LiteralPath $stagingDir) {
    Remove-Item -LiteralPath $stagingDir -Recurse -Force
}
$null = New-Item -ItemType Directory -Force -Path $stagingDir
$null = New-Item -ItemType Directory -Force -Path $distDir

# Копирование исходных файлов Python-адаптера
$addonSrcDir = Join-Path -Path $repoRoot -ChildPath 'addon'
$initFileSrc = Join-Path -Path $addonSrcDir -ChildPath '__init__.py'
$pkgDirSrc = Join-Path -Path $addonSrcDir -ChildPath $config.addon.package

if (-not (Test-Path -LiteralPath $initFileSrc) -or -not (Test-Path -LiteralPath $pkgDirSrc)) {
    throw "Отсутствуют обязательные файлы Python-адаптера в $addonSrcDir"
}

# Корневой __init__.py кладётся прямо в корень аддона
Copy-Item -LiteralPath $initFileSrc -Destination (Join-Path -Path $stagingDir -ChildPath '__init__.py')
# Пакет Python-адаптера копируется в staging
Copy-Item -LiteralPath $pkgDirSrc -Destination (Join-Path -Path $stagingDir -ChildPath $config.addon.package) -Recurse

# Исключение любых локальных кэшей Python из каталога staging
$stagingPycache = Get-ChildItem -LiteralPath $stagingDir -Recurse -Directory -Filter '__pycache__' -ErrorAction SilentlyContinue
foreach ($cacheDir in $stagingPycache) {
    Remove-Item -LiteralPath $cacheDir.FullName -Recurse -Force
}
$stagingPyc = Get-ChildItem -LiteralPath $stagingDir -Recurse -File -Include '*.pyc', '*.pyo' -ErrorAction SilentlyContinue
foreach ($cacheFile in $stagingPyc) {
    Remove-Item -LiteralPath $cacheFile.FullName -Force
}

# Генерация manifest.json из единого источника метаданных
$manifestObj = [ordered]@{
    package = [string]$config.addon.package
    name    = [string]$config.addon.name
}
$manifestJson = ConvertTo-Json -InputObject $manifestObj -Depth 5
$manifestDst = Join-Path -Path $stagingDir -ChildPath 'manifest.json'
Set-Content -LiteralPath $manifestDst -Value $manifestJson -Encoding utf8NoBOM

# Компиляция Go-ядра
Write-Output "Компиляция Go-ядра для $platformKey..."
$coreDir = Join-Path -Path $repoRoot -ChildPath 'core'
$binTargetDir = Join-Path -Path $stagingDir -ChildPath (Join-Path -Path $config.addon.package -ChildPath 'bin')
$null = New-Item -ItemType Directory -Force -Path $binTargetDir
$binTargetFile = Join-Path -Path $binTargetDir -ChildPath $binaryName

$prevCgo = $env:CGO_ENABLED
$prevGoos = $env:GOOS
$prevGoarch = $env:GOARCH

try {
    $env:CGO_ENABLED = '0'
    $env:GOOS = [string]$platformConfig.goos
    $env:GOARCH = [string]$platformConfig.goarch

    $prevLocation = Get-Location
    Set-Location -LiteralPath $coreDir

    & go build -trimpath -ldflags "-s -w" -o $binTargetFile "./$($config.core.source_dir)"
    if ($LASTEXITCODE -ne 0) {
        throw "Ошибка компиляции Go-ядра (код выхода: $LASTEXITCODE)"
    }
} finally {
    Set-Location -LiteralPath $prevLocation
    $env:CGO_ENABLED = $prevCgo
    $env:GOOS = $prevGoos
    $env:GOARCH = $prevGoarch
}

if (-not (Test-Path -LiteralPath $binTargetFile)) {
    throw "Исполняемый файл ядра не найден после сборки: $binTargetFile"
}

# Имя и путь итогового архива аддона
if ([string]::IsNullOrWhiteSpace($OutputName)) {
    $addonFileName = "$($config.addon.package).ankiaddon"
} else {
    $addonFileName = $OutputName
}
$outAddonPath = Join-Path -Path $distDir -ChildPath $addonFileName

if (Test-Path -LiteralPath $outAddonPath) {
    Remove-Item -LiteralPath $outAddonPath -Force
}

# Упаковка в ZIP-архив с расширением .ankiaddon (без включения корневой папки staging)
Write-Output "Упаковка аддона в $outAddonPath..."
[System.IO.Compression.ZipFile]::CreateFromDirectory(
    $stagingDir,
    $outAddonPath,
    [System.IO.Compression.CompressionLevel]::Optimal,
    $false
)

# Программная проверка созданного пакета
Write-Output "Проверка целостности и структуры собранного архива..."
$zipArchive = [System.IO.Compression.ZipFile]::OpenRead($outAddonPath)

try {
    $entries = @($zipArchive.Entries | ForEach-Object { $_.FullName })

    # 1. Проверка наличия обязательных файлов в корне архива
    if ('__init__.py' -notin $entries) {
        throw "Архив не содержит '__init__.py' в корне"
    }
    if ('manifest.json' -notin $entries) {
        throw "Архив не содержит 'manifest.json' в корне"
    }

    # 2. Проверка отсутствия запрещённых файлов и каталогов
    $forbiddenPrefixes = @(
        'addon/',
        'core/',
        'build/',
        '.git/',
        '.github/'
    )
    foreach ($entry in $entries) {
        foreach ($prefix in $forbiddenPrefixes) {
            if ($entry.StartsWith($prefix, [System.StringComparison]::OrdinalIgnoreCase)) {
                throw "Архив содержит неразрешённый путь исходников: $entry"
            }
        }
        if ($entry -like '*__pycache__*' -or
            $entry -like '*.pyc' -or
            $entry -like '*.go' -or
            $entry -like '*.ps1') {
            throw "Архив содержит мусорные файлы: $entry"
        }
    }

    # 3. Валидация содержимого manifest.json
    $manifestEntry = $zipArchive.GetEntry('manifest.json')
    if ($null -eq $manifestEntry) {
        throw "Не удалось получить запись manifest.json из архива"
    }
    $reader = [System.IO.StreamReader]::new($manifestEntry.Open(), [System.Text.Encoding]::UTF8)
    $readManifestContent = $reader.ReadToEnd()
    $reader.Close()

    $parsedManifest = ConvertFrom-Json -InputObject $readManifestContent
    if ($parsedManifest.package -ne $config.addon.package) {
        throw "Поле 'package' в manifest.json ($($parsedManifest.package)) не совпадает с ожидаемым ($($config.addon.package))"
    }
    if ($parsedManifest.name -ne $config.addon.name) {
        throw "Поле 'name' в manifest.json ($($parsedManifest.name)) не совпадает с ожидаемым ($($config.addon.name))"
    }

    # 4. Проверка присутствия исполняемого файла ядра
    $expectedBinaryEntry = "$($config.addon.package)/bin/$binaryName"
    if ($expectedBinaryEntry -notin $entries) {
        throw "Архив не содержит бинарный файл ядра: $expectedBinaryEntry"
    }
} finally {
    $zipArchive.Dispose()
}

# Очистка временного staging каталога
Remove-Item -LiteralPath $stagingDir -Recurse -Force

$addonSize = (Get-Item -LiteralPath $outAddonPath).Length
Write-Output "Сборка успешно завершена!"
Write-Output "Результат: $outAddonPath ($addonSize байт)"



