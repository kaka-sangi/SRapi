param(
    [switch]$Check,
    [switch]$GoOnly,
    [switch]$TypescriptOnly
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$Root = Resolve-Path (Join-Path $PSScriptRoot "..")
$OpenApi = Join-Path $Root "packages/openapi/openapi.yaml"
$GoConfig = Join-Path $Root "packages/openapi/oapi-codegen.server.yaml"
$GoOutput = Join-Path $Root "apps/api/internal/openapi/openapi.gen.go"
$TsOutput = Join-Path $Root "packages/sdk/typescript/src"

function Invoke-Step {
    param(
        [string]$FilePath,
        [string[]]$Arguments,
        [string]$WorkingDirectory = $Root
    )
    Push-Location $WorkingDirectory
    try {
        & $FilePath @Arguments
        if ($LASTEXITCODE -ne 0) {
            throw "$FilePath exited with code $LASTEXITCODE"
        }
    }
    finally {
        Pop-Location
    }
}

function Assert-SameFile {
    param(
        [string]$Expected,
        [string]$Actual,
        [string]$Message
    )
    if (-not (Test-Path $Expected) -or -not (Test-Path $Actual)) {
        throw $Message
    }
    $ExpectedHash = (Get-FileHash -Algorithm SHA256 $Expected).Hash
    $ActualHash = (Get-FileHash -Algorithm SHA256 $Actual).Hash
    if ($ExpectedHash -ne $ActualHash) {
        throw $Message
    }
}

function Get-RelativeFiles {
    param([string]$Path)
    $TrimChars = [char[]]@([System.IO.Path]::DirectorySeparatorChar, [System.IO.Path]::AltDirectorySeparatorChar)
    $Base = (Resolve-Path $Path).Path.TrimEnd($TrimChars)
    return Get-ChildItem -Path $Base -File -Recurse | ForEach-Object {
        $_.FullName.Substring($Base.Length).TrimStart($TrimChars)
    } | Sort-Object
}

function Assert-SameDirectory {
    param(
        [string]$Expected,
        [string]$Actual,
        [string]$Message
    )
    if (-not (Test-Path $Expected) -or -not (Test-Path $Actual)) {
        throw $Message
    }
    $ExpectedFiles = Get-RelativeFiles $Expected
    $ActualFiles = Get-RelativeFiles $Actual

    $Diff = Compare-Object -ReferenceObject $ExpectedFiles -DifferenceObject $ActualFiles
    if ($Diff) {
        throw $Message
    }

    foreach ($Relative in $ExpectedFiles) {
        Assert-SameFile -Expected (Join-Path $Expected $Relative) -Actual (Join-Path $Actual $Relative) -Message $Message
    }
}

$GenerateGo = -not $TypescriptOnly
$GenerateTs = -not $GoOnly
$TempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("srapi-openapi-" + [System.Guid]::NewGuid().ToString("N"))

try {
    New-Item -ItemType Directory -Path $TempRoot | Out-Null

    if ($GenerateGo) {
        if ($Check) {
            $TempGo = Join-Path $TempRoot "openapi.gen.go"
            Invoke-Step "go" @("run", "github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.7.0", "-generate", "types,std-http", "-package", "openapi", "-o", $TempGo, $OpenApi)
            Assert-SameFile -Expected $GoOutput -Actual $TempGo -Message "$GoOutput is out of date; run tools/generate-openapi.ps1"
        }
        else {
            Invoke-Step "go" @("run", "github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.7.0", "-config", $GoConfig, $OpenApi)
        }
    }

    if ($GenerateTs) {
        if ($Check) {
            $TempTs = Join-Path $TempRoot "typescript"
            Invoke-Step "npx" @("--yes", "@hey-api/openapi-ts@0.97.2", "-i", $OpenApi, "-o", $TempTs, "-c", "@hey-api/client-fetch", "-p", "@hey-api/typescript", "@hey-api/sdk", "--no-log-file")
            Assert-SameDirectory -Expected $TsOutput -Actual $TempTs -Message "$TsOutput is out of date; run tools/generate-openapi.ps1"
        }
        else {
            Invoke-Step "npx" @("--yes", "@hey-api/openapi-ts@0.97.2", "-i", $OpenApi, "-o", $TsOutput, "-c", "@hey-api/client-fetch", "-p", "@hey-api/typescript", "@hey-api/sdk", "--no-log-file")
        }
    }
}
finally {
    if (Test-Path $TempRoot) {
        Remove-Item -Recurse -Force $TempRoot
    }
}
