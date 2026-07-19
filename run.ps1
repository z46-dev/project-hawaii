param()

$ErrorActionPreference = "Stop"

$processes = @()

function Stop-ProcessTree {
    param(
        [Parameter(Mandatory = $true)]
        [int]$ProcessId
    )

    Get-CimInstance Win32_Process -Filter "ParentProcessId = $ProcessId" -ErrorAction SilentlyContinue |
        ForEach-Object {
            Stop-ProcessTree -ProcessId $_.ProcessId
        }

    Stop-Process -Id $ProcessId -Force -ErrorAction SilentlyContinue
}

function Stop-StartedProcesses {
    Write-Host "Cleaning up..."

    foreach ($process in $processes) {
        if ($null -ne $process -and -not $process.HasExited) {
            Stop-ProcessTree -ProcessId $process.Id
        }
    }
}

function Start-ProjectProcess {
    param(
        [Parameter(Mandatory = $true)]
        [string]$FilePath,

        [Parameter(Mandatory = $true)]
        [string]$Arguments,

        [Parameter(Mandatory = $true)]
        [string]$WorkingDirectory
    )

    $command = Get-Command $FilePath -ErrorAction Stop

    $startInfo = [System.Diagnostics.ProcessStartInfo]::new()
    $startInfo.FileName = $command.Source
    $startInfo.Arguments = $Arguments
    $startInfo.WorkingDirectory = $WorkingDirectory
    $startInfo.UseShellExecute = $false

    $process = [System.Diagnostics.Process]::new()
    $process.StartInfo = $startInfo

    if (-not $process.Start()) {
        throw "Failed to start $FilePath"
    }

    return $process
}

try {
    $processes += Start-ProjectProcess `
        -FilePath "bun" `
        -Arguments "run devel" `
        -WorkingDirectory (Join-Path $PSScriptRoot "client")

    $processes += Start-ProjectProcess `
        -FilePath "go" `
        -Arguments "run ." `
        -WorkingDirectory (Join-Path $PSScriptRoot "server")

    while ($true) {
        foreach ($process in $processes) {
            if ($process.HasExited) {
                exit $process.ExitCode
            }
        }

        Start-Sleep -Milliseconds 200
    }
} finally {
    Stop-StartedProcesses
}
