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
$cachePath = $null
$verified = $false
$activationExit = 1
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
  # A cached binary is trusted only against a fresh checksum from this origin.
  Get-ReleaseFile -Uri "$releaseUrl/$asset.sha256" -OutFile $checksumPath -TimeoutSec 120
  $expected = ((Get-Content -Raw $checksumPath).Trim() -split '\s+')[0].ToLowerInvariant()
  if ($expected -notmatch '^[a-f0-9]{64}$') { throw "Release checksum is invalid" }

  # Windows' per-user temporary directory supplies the inherited private ACL.
  $cacheRoot = Join-Path ([System.IO.Path]::GetTempPath()) "viceme-bootstrap-cache"
  if (Test-Path -LiteralPath $cacheRoot) {
    $cacheDirectory = Get-Item -Force -LiteralPath $cacheRoot
    if (-not $cacheDirectory.PSIsContainer -or ($cacheDirectory.Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw "Unsafe ViceMe download cache" }
  } else {
    New-Item -ItemType Directory -Path $cacheRoot -Force | Out-Null
  }
  Get-ChildItem -LiteralPath $cacheRoot -File -Force | Where-Object { $_.LastWriteTimeUtc -lt [DateTime]::UtcNow.AddDays(-2) } | Remove-Item -Force -ErrorAction SilentlyContinue
  $keyBytes = [Text.Encoding]::UTF8.GetBytes("$releaseUrl/$asset`n$expected`n")
  $hasher = [Security.Cryptography.SHA256]::Create()
  try { $cacheKey = ([BitConverter]::ToString($hasher.ComputeHash($keyBytes))).Replace('-', '').ToLowerInvariant() } finally { $hasher.Dispose() }
  $cachePath = Join-Path $cacheRoot $cacheKey
  $cachedFile = Get-Item -Force -LiteralPath $cachePath -ErrorAction SilentlyContinue
  if ($cachedFile -and -not $cachedFile.PSIsContainer) {
    if (-not ($cachedFile.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
      try {
        Copy-Item -LiteralPath $cachePath -Destination $binaryPath
      } catch {
        # A concurrent successful activation may have removed this cache entry.
        Remove-Item -Force -ErrorAction SilentlyContinue -LiteralPath $binaryPath
      }
      if (Test-Path -LiteralPath $binaryPath) {
        if ((Get-FileHash -Algorithm SHA256 -LiteralPath $binaryPath).Hash.ToLowerInvariant() -ne $expected) {
          Remove-Item -Force -ErrorAction SilentlyContinue -LiteralPath $binaryPath, $cachePath
        }
      }
    }
  }
  if (-not (Test-Path -LiteralPath $binaryPath)) {
    Get-ReleaseFile -Uri "$releaseUrl/$asset" -OutFile $binaryPath -TimeoutSec 300
  }
  $actual = (Get-FileHash -Algorithm SHA256 -Path $binaryPath).Hash.ToLowerInvariant()
  if ($actual -ne $expected) { throw "ViceMe binary checksum verification failed" }
  $verified = $true

  $installDir = if ($env:VICEME_INSTALL_DIR) { $env:VICEME_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "ViceMe\bin" }
  New-Item -ItemType Directory -Force -Path $installDir | Out-Null
  $destination = Join-Path $installDir "viceme.exe"
  & $binaryPath bootstrap activate --destination $destination --agent auto --region $region
  $activationExit = $LASTEXITCODE
  # Preserve the CLI's policy exit code so the host requests permission instead
  # of treating a first installation as a generic PowerShell failure.
  if ($activationExit -ne 0) { exit $activationExit }

  $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
  $parts = @($userPath -split ';' | Where-Object { $_ })
  if ($parts -notcontains $installDir) {
    [Environment]::SetEnvironmentVariable("Path", (($parts + $installDir) -join ';'), "User")
    $env:Path = "$installDir;$env:Path"
  }
} finally {
  if ($verified -and $activationExit -eq 6) {
    $cacheStage = Join-Path $cacheRoot (".retain-" + [Guid]::NewGuid().ToString("N"))
    try {
      Copy-Item -LiteralPath $binaryPath -Destination $cacheStage
      if (Test-Path -LiteralPath $cachePath) {
        [IO.File]::Replace($cacheStage, $cachePath, $null)
      } else {
        [IO.File]::Move($cacheStage, $cachePath)
      }
    } catch {
      [Console]::Error.WriteLine("ViceMe could not retain the verified download for retry")
    } finally {
      Remove-Item -Force -ErrorAction SilentlyContinue -LiteralPath $cacheStage
    }
  } elseif ($cachePath) {
    Remove-Item -Force -ErrorAction SilentlyContinue -LiteralPath $cachePath
  }
  Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $temporary
}
