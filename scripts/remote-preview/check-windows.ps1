$ErrorActionPreference = 'Stop'
$originalLocal = $env:LOCALAPPDATA
$originalConfig = $env:MYCO_CONFIG
$previewProcess = $null
$otherProcess = $null
$root = Join-Path $env:RUNNER_TEMP 'remote preview & test'
try {
    New-Item -ItemType Directory -Force $root | Out-Null
    Copy-Item package/* $root -Recurse -Force
    $env:LOCALAPPDATA = Join-Path $root 'normal profile'
    $env:MYCO_CONFIG = 'must-not-be-inherited'
    $expected = Join-Path $env:LOCALAPPDATA 'myco-remote-preview\desktop\profile'
    $launcher = Join-Path $root 'Start-preview.cmd'
    $output = (& $launcher --check) -join "`n"
    if ($LASTEXITCODE -ne 0) { throw "Preview launcher failed: $output" }
    if (-not $output.Contains("USERPROFILE=$expected")) { throw 'Home was not isolated' }
    if (-not $output.Contains("APPDATA=$expected\AppData\Roaming")) { throw 'Config was not isolated' }
    if (-not $output.Contains("LOCALAPPDATA=$expected\AppData\Local")) { throw 'Browser data was not isolated' }
    if ($output.Contains('must-not-be-inherited')) { throw 'Inherited MYCO_CONFIG leaked' }
    if (-not $output.Contains('remote-preview-')) { throw 'Preview executables did not run' }
    if (-not (Test-Path "$expected\.myco\components\remote\0.1.0\ffmpeg.exe")) { throw 'Encoder was not staged' }
    if (Test-Path (Join-Path $env:LOCALAPPDATA 'myco')) { throw 'Normal App config was written' }
    # Two harmless processes with identical names: stopping the preview must
    # leave the one outside its exact executable path alone.
    $other = Join-Path $root 'other'
    New-Item -ItemType Directory -Force $other | Out-Null
    Copy-Item "$env:WINDIR\System32\ping.exe" "$root\myco-app.exe" -Force
    Copy-Item "$env:WINDIR\System32\ping.exe" "$other\myco-app.exe" -Force
    $previewProcess = Start-Process "$root\myco-app.exe" -ArgumentList '-t','127.0.0.1' -PassThru -WindowStyle Hidden
    $otherProcess = Start-Process "$other\myco-app.exe" -ArgumentList '-t','127.0.0.1' -PassThru -WindowStyle Hidden
    $stopper = Join-Path $root 'Stop-preview.cmd'
    & $stopper
    if ($LASTEXITCODE -ne 0) { throw 'Stop launcher failed' }
    if (-not $previewProcess.WaitForExit(5000)) { throw 'Preview process was not stopped' }
    $otherProcess.Refresh()
    if ($otherProcess.HasExited) { throw 'A different App path was stopped' }
    Write-Host 'PASS: Windows preview launcher, paths with spaces/ampersands, isolated config and staged encoder'
} finally {
    foreach ($p in @($previewProcess,$otherProcess)) { if ($null -ne $p -and -not $p.HasExited) { Stop-Process -Id $p.Id -ErrorAction SilentlyContinue } }
    $env:LOCALAPPDATA = $originalLocal
    $env:MYCO_CONFIG = $originalConfig
    Remove-Item $root -Recurse -Force -ErrorAction SilentlyContinue
}
