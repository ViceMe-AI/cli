$ErrorActionPreference = "Stop"

# Keep an existing PATH entry (including the npm launcher) authoritative.
$command = Get-Command viceme -CommandType Application, ExternalScript -ErrorAction SilentlyContinue | Select-Object -First 1
if ($command) {
  $candidate = $command.Source
} elseif ($env:VICEME_INSTALL_DIR) {
  $candidate = Join-Path $env:VICEME_INSTALL_DIR "viceme.exe"
} elseif ($env:LOCALAPPDATA) {
  $candidate = Join-Path $env:LOCALAPPDATA "ViceMe\bin\viceme.exe"
} else {
  [Console]::Error.WriteLine("CLI_NOT_FOUND")
  exit 127
}
if (-not (Test-Path -LiteralPath $candidate -PathType Leaf)) {
  [Console]::Error.WriteLine("CLI_NOT_FOUND")
  exit 127
}
[IO.Path]::GetFullPath($candidate)
