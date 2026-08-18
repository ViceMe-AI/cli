$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

# Windows PowerShell 5.1 does not enable TLS 1.2 by default on older builds;
# both release endpoints require it. PowerShell 7 ignores this setting.
try {
  [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
} catch {}

function Get-ReleaseFile {
  param([string]$Uri, [string]$OutFile, [int]$TimeoutSec)
  for ($attempt = 1; $attempt -le 3; $attempt++) {
    try {
      Invoke-WebRequest -UseBasicParsing -TimeoutSec $TimeoutSec -Uri $Uri -OutFile $OutFile
      return
    } catch {
      if ($attempt -eq 3) { throw }
      Start-Sleep -Seconds (2 * $attempt)
    }
  }
}

$region = if ($env:VICEME_REGION) { $env:VICEME_REGION } else { "cn" }
switch ($region) {
  "cn" { $baseUrl = if ($env:VICEME_DOWNLOAD_BASE_URL) { $env:VICEME_DOWNLOAD_BASE_URL } else { "https://s3.viceme.cn/start/cli/releases" } }
  "global" { $baseUrl = if ($env:VICEME_DOWNLOAD_BASE_URL) { $env:VICEME_DOWNLOAD_BASE_URL } else { "https://s3.viceme.ai/start/cli/releases" } }
  default { throw "VICEME_REGION must be cn or global" }
}

$architecture = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString().ToLowerInvariant()
switch ($architecture) {
  "x64" { $goarch = "amd64" }
  "arm64" { $goarch = "arm64" }
  default { throw "Unsupported CPU architecture: $architecture" }
}

$temporary = Join-Path ([System.IO.Path]::GetTempPath()) ("viceme-install-" + [Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $temporary | Out-Null
try {
  $version = $env:VICEME_VERSION
  if (-not $version) {
    $latestPath = Join-Path $temporary "latest"
    Get-ReleaseFile -Uri "$baseUrl/latest" -OutFile $latestPath -TimeoutSec 120
    $version = (Get-Content -Raw $latestPath).Trim()
  }
  if ($version -notmatch '^\d+\.\d+\.\d+$') { throw "Release index returned an invalid version" }

  $asset = "viceme_${version}_windows_${goarch}.exe"
  $releaseUrl = "$baseUrl/v$version"
  $binaryPath = Join-Path $temporary "viceme.exe"
  $checksumPath = Join-Path $temporary "viceme.sha256"
  Get-ReleaseFile -Uri "$releaseUrl/$asset" -OutFile $binaryPath -TimeoutSec 300
  Get-ReleaseFile -Uri "$releaseUrl/$asset.sha256" -OutFile $checksumPath -TimeoutSec 120
  $expected = ((Get-Content -Raw $checksumPath).Trim() -split '\s+')[0].ToLowerInvariant()
  if ($expected -notmatch '^[a-f0-9]{64}$') { throw "Release checksum is invalid" }
  $actual = (Get-FileHash -Algorithm SHA256 -Path $binaryPath).Hash.ToLowerInvariant()
  if ($actual -ne $expected) { throw "ViceMe binary checksum verification failed" }

  $installDir = if ($env:VICEME_INSTALL_DIR) { $env:VICEME_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "ViceMe\bin" }
  New-Item -ItemType Directory -Force -Path $installDir | Out-Null
  $destination = Join-Path $installDir "viceme.exe"
  & $binaryPath bootstrap activate --destination $destination --agent auto --region $region
  if ($LASTEXITCODE -ne 0) { throw "ViceMe bootstrap activation failed" }

  $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
  $parts = @($userPath -split ';' | Where-Object { $_ })
  if ($parts -notcontains $installDir) {
    [Environment]::SetEnvironmentVariable("Path", (($parts + $installDir) -join ';'), "User")
    $env:Path = "$installDir;$env:Path"
  }
} finally {
  Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $temporary
}
