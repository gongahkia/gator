[CmdletBinding()]
param(
  [Parameter(Mandatory)]
  [ValidateNotNullOrEmpty()]
  [string]$InstallRoot
)

$ErrorActionPreference = "Stop"
$executable = Join-Path $InstallRoot "bin\hot-cross-buns.exe"
if (-not (Test-Path -LiteralPath $executable -PathType Leaf)) {
  throw "Missing installed Windows executable: $executable"
}

$env:HCB_BENCHMARK_EXIT_AFTER_LOAD = "1"
$env:QT_QPA_PLATFORM = "offscreen"
& $executable -platform offscreen
if ($LASTEXITCODE -ne 0) {
  throw "Installed Windows executable failed with exit code $LASTEXITCODE"
}
