# Install or upgrade the myco CLI on Windows (PowerShell 5.1 or 7), then run a
# health check. In Git Bash, WSL, macOS or Linux use install-client.sh.
#
#   irm https://raw.githubusercontent.com/chenhg5/myco/main/deploy/install-client.ps1 | iex
#
# Options (environment variables):
#   MYCO_VERSION=v0.18.26   install a specific release (default: latest)
#   MYCO_INSTALL_DIR=...    where to put myco.exe (default: the directory of an
#                           existing myco.exe, else %LOCALAPPDATA%\Programs\myco)
#   MYCO_NO_DOCTOR=1        skip the health check (used by join links)
# It adds that directory to your user PATH. No secrets are read or printed.
& {
  $ErrorActionPreference = 'Stop'
  $ProgressPreference = 'SilentlyContinue'   # Invoke-WebRequest is much faster without it
  [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

  $repo = 'chenhg5/myco'
  $version = if ($env:MYCO_VERSION) { $env:MYCO_VERSION } else { 'latest' }
  $cpu = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
  $arch = switch ($cpu) { 'AMD64' { 'amd64' } 'ARM64' { 'arm64' } default { throw "unsupported architecture: $cpu" } }
  $asset = "myco-windows-$arch.exe"

  $existing = Get-Command myco.exe -ErrorAction SilentlyContinue
  $dir = if ($env:MYCO_INSTALL_DIR) { $env:MYCO_INSTALL_DIR }
         elseif ($existing) { Split-Path $existing.Source }
         else { Join-Path $env:LOCALAPPDATA 'Programs\myco' }
  New-Item -ItemType Directory -Force -Path $dir | Out-Null

  $base = if ($version -eq 'latest') { "https://github.com/$repo/releases/latest/download" } else { "https://github.com/$repo/releases/download/$version" }
  $tmp = Join-Path ([IO.Path]::GetTempPath()) ("myco-" + [guid]::NewGuid())
  New-Item -ItemType Directory -Path $tmp | Out-Null
  try {
    Write-Host "==> downloading $base/$asset"
    # GitHub's download redirect answers 502/504 now and then: retry.
    foreach ($f in @($asset, 'SHA256SUMS')) {
      for ($i = 1; ; $i++) {
        try { Invoke-WebRequest -UseBasicParsing -Uri "$base/$f" -OutFile (Join-Path $tmp $f); break }
        catch { if ($i -ge 5) { throw }; Start-Sleep -Seconds 2 }
      }
    }
    $want = (Get-Content (Join-Path $tmp 'SHA256SUMS') | Where-Object { ($_ -split '\s+')[1] -eq $asset } | ForEach-Object { ($_ -split '\s+')[0] }) | Select-Object -First 1
    $got = (Get-FileHash -Algorithm SHA256 (Join-Path $tmp $asset)).Hash.ToLower()
    if (-not $want -or $want.ToLower() -ne $got) { throw "checksum mismatch for $asset (want $want, got $got); aborting" }
    Write-Host '==> checksum ok'

    $target = Join-Path $dir 'myco.exe'
    # A running myco.exe (the runner) cannot be overwritten, but it can be renamed.
    if (Test-Path $target) { Move-Item -Force $target "$target.old" -ErrorAction SilentlyContinue }
    Move-Item -Force (Join-Path $tmp $asset) $target
  } finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
  }
  Write-Host "==> installed: $target $(& $target version)"

  $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
  if (-not (($userPath -split ';') -contains $dir)) {
    [Environment]::SetEnvironmentVariable('Path', ((@($userPath, $dir) | Where-Object { $_ }) -join ';'), 'User')
    Write-Host "==> added $dir to your user PATH (new terminals and agent sessions will find myco)"
  }
  if (-not (($env:Path -split ';') -contains $dir)) { $env:Path = "$env:Path;$dir" }

  if ($env:MYCO_NO_DOCTOR -eq '1') { return }
  Write-Host ''
  Write-Host '==> myco doctor --all'
  & $target doctor --all --format text
  Write-Host ''
  Write-Host 'If a runner is already running, restart it to use the new version: myco runner service restart'
}
