$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

$region = "global"
$baseUrl = if ($env:VICEME_POC_DOWNLOAD_BASE_URL) { $env:VICEME_POC_DOWNLOAD_BASE_URL } else { "https://viceme-shop-storage-poc.preview.tencent-zeabur.cn/start/poc/cli/releases" }
$apiBaseUrl = if ($env:VICEME_POC_API_BASE_URL) { $env:VICEME_POC_API_BASE_URL } else { "https://viceme-shop-web-poc.preview.tencent-zeabur.cn/api" }

$architecture = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString().ToLowerInvariant()
switch ($architecture) {
  "x64" { $goarch = "amd64" }
  "arm64" { $goarch = "arm64" }
  default { throw "Unsupported CPU architecture: $architecture" }
}

$temporary = Join-Path ([System.IO.Path]::GetTempPath()) ("viceme-poc-install-" + [Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $temporary | Out-Null
try {
  $version = $env:VICEME_POC_VERSION
  if (-not $version) {
    $latestPath = Join-Path $temporary "latest"
    Invoke-WebRequest -UseBasicParsing -TimeoutSec 120 -Uri "$baseUrl/latest" -OutFile $latestPath
    $version = (Get-Content -Raw $latestPath).Trim()
  }
  if ($version -notmatch '^\d+\.\d+\.\d+-poc\.\d+$') { throw "POC release index returned an invalid version" }

  $asset = "viceme_${version}_windows_${goarch}.exe"
  $releaseUrl = "$baseUrl/v$version"
  $binaryPath = Join-Path $temporary "viceme.exe"
  $checksumPath = Join-Path $temporary "viceme.sha256"
  Invoke-WebRequest -UseBasicParsing -TimeoutSec 300 -Uri "$releaseUrl/$asset" -OutFile $binaryPath
  Invoke-WebRequest -UseBasicParsing -TimeoutSec 120 -Uri "$releaseUrl/$asset.sha256" -OutFile $checksumPath
  $expected = ((Get-Content -Raw $checksumPath).Trim() -split '\s+')[0].ToLowerInvariant()
  if ($expected -notmatch '^[a-f0-9]{64}$') { throw "POC release checksum is invalid" }
  $actual = (Get-FileHash -Algorithm SHA256 -Path $binaryPath).Hash.ToLowerInvariant()
  if ($actual -ne $expected) { throw "ViceMe POC binary checksum verification failed" }

  $installDir = if ($env:VICEME_INSTALL_DIR) { $env:VICEME_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "ViceMe\bin" }
  New-Item -ItemType Directory -Force -Path $installDir | Out-Null
  $destination = Join-Path $installDir "viceme.exe"
  $agent = if ($env:VICEME_AGENT_TARGET) { $env:VICEME_AGENT_TARGET } else { "auto" }
  & $binaryPath bootstrap activate `
    --destination $destination `
    --agent $agent `
    --region $region `
    --api-base-url $apiBaseUrl `
    --release-channel poc `
    --release-base-url $baseUrl `
    --allow-channel-switch
  if ($LASTEXITCODE -ne 0) { throw "ViceMe POC bootstrap activation failed" }

  $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
  $parts = @($userPath -split ';' | Where-Object { $_ })
  if ($parts -notcontains $installDir) {
    [Environment]::SetEnvironmentVariable("Path", (($parts + $installDir) -join ';'), "User")
    $env:Path = "$installDir;$env:Path"
  }
} finally {
  Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $temporary
}
