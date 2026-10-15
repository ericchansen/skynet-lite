$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$picker = Join-Path $root 'pull.ps1'
$models = (Get-Content (Join-Path $root 'config.json') -Raw | ConvertFrom-Json).models
$calls = [System.Collections.Generic.List[object]]::new()
$exitCode = 0

function docker {
    $calls.Add(@($args))
    $global:LASTEXITCODE = $exitCode
}

function Read-Host { ' qwen , dolphin ' }

function Assert($condition, $message) {
    if (-not $condition) { throw $message }
}

& $picker -List
Assert ($calls.Count -eq 0) '-List must not call Docker.'

& $picker -Model qwen,dolphin,QWEN
Assert ($calls.Count -eq 3) 'Selections must be deduplicated and configure the Qwen alias.'
Assert ($calls[0][-1] -eq $models[0].pullModel) 'Wrong Qwen download.'
Assert (($calls[1][7..10] -join ' ') -eq 'create qwen38 -f /tmp/Modelfile.qwen38') 'Wrong Qwen alias setup.'
Assert ($calls[2][-1] -eq $models[2].model) 'Wrong Dolphin model.'
Assert ($calls[0][2] -eq (Join-Path $root 'docker-compose.yml')) 'Wrong Compose file.'
Assert (($calls[0][3..6] -join ' ') -eq 'exec -T ollama ollama') 'Wrong pull invocation.'

$calls.Clear()
foreach ($entry in $models) {
    $start = $calls.Count
    & $picker $entry.title
    Assert ($calls[$start][-1] -eq $(if ($entry.pullModel) { $entry.pullModel } else { $entry.model })) "Wrong model for '$($entry.title)'."
    if ($entry.modelfile) {
        Assert ($calls[$start + 1][7] -eq 'create') "Missing alias setup for '$($entry.title)'."
    }
}
Assert ($calls.Count -eq $models.Count + 1) 'Each model must pull once, plus Qwen alias setup.'

$calls.Clear()
& $picker
Assert ($calls.Count -eq 3) 'Interactive comma-separated selection failed.'

$calls.Clear()
& $picker -Model $models[3].title -Run
Assert ($calls.Count -eq 2) '-Run must pull then run.'
Assert ($calls[1][3] -eq 'exec' -and $calls[1][6] -eq 'run') 'Wrong run invocation.'
Assert ($calls[1][-1] -eq $models[3].model) 'Wrong run model.'

foreach ($selection in @('1', 'bogus', '')) {
    $calls.Clear()
    $failed = $false
    try { & $picker -Model @('qwen', $selection) } catch { $failed = $true }
    Assert $failed "Invalid selection '$selection' must fail."
    Assert ($calls.Count -eq 0) 'Validate all selections before downloading.'
}

$failed = $false
try { & $picker -Model qwen,qwen-moe -Run } catch { $failed = $true }
Assert $failed '-Run must reject multiple models.'

$calls.Clear()
$exitCode = 1
$failed = $false
try { & $picker -Model qwen,qwen-moe } catch { $failed = $_.Exception.Message -like 'Pull failed*' }
Assert $failed 'Docker failures must be reported.'
Assert ($calls.Count -eq 1) 'Stop downloading after a failed pull.'

$calls.Clear()
function docker {
    $calls.Add(@($args))
    $global:LASTEXITCODE = if ($args -contains 'run') { 1 } else { 0 }
}
$failed = $false
try { & $picker -Model hermes -Run } catch { $failed = $_.Exception.Message -like 'Run failed*' }
Assert $failed 'Run failures must be reported.'

Write-Host 'Model picker checks passed (Docker mocked; no downloads).'
