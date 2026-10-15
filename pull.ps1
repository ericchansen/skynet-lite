[CmdletBinding()]
param(
    [string[]]$Model,
    [switch]$List,
    [switch]$Run
)

$ErrorActionPreference = 'Stop'
$models = @((Get-Content (Join-Path $PSScriptRoot 'config.json') -Raw |
    ConvertFrom-Json).models)
$composeFile = Join-Path $PSScriptRoot 'docker-compose.yml'

if ($List -or -not $Model) {
    $models | ForEach-Object { Write-Host ("{0}: {1}" -f $_.title, $_.model) }
}
if ($List) { return }
if (-not $Model) {
    $Model = (Read-Host 'Choose model name(s), separated by commas').Split(',')
}

$selected = [System.Collections.Generic.List[object]]::new()
foreach ($choice in $Model) {
        $choice = $choice.Trim()
        $match = $models | Where-Object {
            $_.title -eq $choice -or $_.model -eq $choice -or $_.pullModel -eq $choice
        }
        if (-not $match) {
            throw "Unknown model '$choice'. Use -List to see available names and model IDs."
        }
        if ($selected.model -notcontains $match.model) { $selected.Add($match) }
    }

if ($Run -and $selected.Count -ne 1) {
    throw '-Run requires exactly one model.'
}

foreach ($entry in $selected) {
    $id = if ($entry.pullModel) { $entry.pullModel } else { $entry.model }
    & docker compose -f $composeFile exec -T ollama ollama pull $id
    if ($LASTEXITCODE -ne 0) {
        throw "Pull failed for '$id' (exit $LASTEXITCODE). Check that Docker Compose is running and Ollama is up to date."
    }
    if ($entry.modelfile) {
        & docker compose -f $composeFile exec -T ollama ollama create $entry.model -f "/tmp/$($entry.modelfile)"
        if ($LASTEXITCODE -ne 0) {
            throw "Model setup failed for '$($entry.model)' (exit $LASTEXITCODE)."
        }
    }
}

if ($Run) {
    & docker compose -f $composeFile exec ollama ollama run $selected[0].model
    if ($LASTEXITCODE -ne 0) {
        throw "Run failed for '$($selected[0].model)' (exit $LASTEXITCODE). Check Ollama logs for model support or VRAM errors."
    }
}
