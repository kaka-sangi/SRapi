param(
    [ValidateSet("check", "architecture-check", "api", "up", "down", "logs", "smoke-health", "smoke-gateway", "openapi", "openapi-check")]
    [string]$Command = "check"
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$Root = Resolve-Path (Join-Path $PSScriptRoot "..")

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

function Invoke-OpenApiScript {
    param([switch]$Check)
    $Script = Join-Path $PSScriptRoot "generate-openapi.ps1"
    if ($Check) {
        & $Script -Check
    }
    else {
        & $Script
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

function Assert-EntGenerated {
    $ApiDir = Join-Path $Root "apps/api"
    $EntDir = Join-Path $ApiDir "ent"
    $TempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("srapi-ent-" + [System.Guid]::NewGuid().ToString("N"))
    try {
        New-Item -ItemType Directory -Path $TempRoot | Out-Null
        $Before = Join-Path $TempRoot "ent.before"
        Copy-Item -Recurse $EntDir $Before
        Invoke-Step "go" @("run", "entgo.io/ent/cmd/ent@v0.14.6", "generate", "./ent/schema") $ApiDir
        Assert-SameDirectory -Expected $Before -Actual $EntDir -Message "Ent generated code changed; run go run entgo.io/ent/cmd/ent@v0.14.6 generate ./ent/schema from apps/api"
    }
    finally {
        if (Test-Path $TempRoot) {
            Remove-Item -Recurse -Force $TempRoot
        }
    }
}

function Get-ComposeCommand {
    Invoke-Step "docker" @("compose", "version")
    return @("docker", @("compose"))
}

$EnvFile = Join-Path $Root ".env"
$ExampleEnvFile = Join-Path $Root ".env.example"
if (-not (Test-Path $EnvFile) -and (Test-Path $ExampleEnvFile)) {
    Copy-Item $ExampleEnvFile $EnvFile
}

switch ($Command) {
    "check" {
        Invoke-Step "npx" @("--yes", "@redocly/cli", "lint", "packages/openapi/openapi.yaml")
        Invoke-Step "npx" @("--yes", "@redocly/cli", "bundle", "packages/openapi/openapi.yaml", "--output", "build/openapi/openapi.bundle.yaml")
        Invoke-OpenApiScript -Check
        Invoke-Step "npx" @("--yes", "-p", "typescript@5.9.3", "tsc", "-p", "packages/sdk/typescript/tsconfig.json", "--noEmit")
        Assert-EntGenerated
        Invoke-Step "go" @("test", "./...") (Join-Path $Root "apps/api")
        Invoke-Step "npx" @("--yes", "-p", "secretlint@13.0.2", "-p", "@secretlint/secretlint-rule-preset-recommend@13.0.2", "secretlint", "**/*")
    }
    "architecture-check" {
        Invoke-Step "go" @("test", "./internal/config", "./internal/architecture", "./internal/app", "./internal/platform/crypto", "./internal/platform/db", "./internal/platform/logger", "./internal/platform/redis", "./internal/modules/providers/preset", "./internal/persistence/entstore/...", "./internal/persistence/redisstore/...", "./internal/workers/...", "./internal/httpserver") (Join-Path $Root "apps/api")
    }
    "api" {
        Invoke-Step "go" @("run", "./cmd/srapi") (Join-Path $Root "apps/api")
    }
    "up" {
        $Compose = Get-ComposeCommand
        Invoke-Step $Compose[0] ($Compose[1] + @("--env-file", ".env", "-f", "deploy/docker-compose.yml", "up", "--build"))
    }
    "down" {
        $Compose = Get-ComposeCommand
        Invoke-Step $Compose[0] ($Compose[1] + @("--env-file", ".env", "-f", "deploy/docker-compose.yml", "down"))
    }
    "logs" {
        $Compose = Get-ComposeCommand
        Invoke-Step $Compose[0] ($Compose[1] + @("--env-file", ".env", "-f", "deploy/docker-compose.yml", "logs", "-f"))
    }
    "smoke-health" {
        $Port = if ($env:SERVER_PORT) { $env:SERVER_PORT } else { "8080" }
        Invoke-WebRequest -UseBasicParsing -Uri "http://localhost:$Port/api/v1/health" | Out-Null
    }
    "smoke-gateway" {
        Invoke-Step "node" @("tools/smoke-local.mjs")
    }
    "openapi" {
        Invoke-OpenApiScript
    }
    "openapi-check" {
        Invoke-OpenApiScript -Check
    }
}
