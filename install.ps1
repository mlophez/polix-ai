# Installs polix from its GitHub Releases on Windows, checking the SHA-256 of the binary against checksums.txt.
# Works on Windows PowerShell 5.1 and PowerShell 7 (also on macOS and Linux, where install.sh is the usual way).
#
#   powershell -NoProfile -ExecutionPolicy Bypass -Command "irm https://github.com/mlophez/polix-ai/releases/latest/download/install.ps1 | iex"
#
# Environment:
#   POLIX_VERSION         version to install (default: the latest release)
#   POLIX_INSTALL_DIR     folder of the binary (default: %LOCALAPPDATA%\Programs\polix on Windows, ~/.local/bin elsewhere)
#   POLIX_RELEASES_URL    releases page (default: https://github.com/mlophez/polix-ai/releases)
#   POLIX_NO_MODIFY_PATH  set it to leave the user PATH alone
#
# Errors are thrown, never `exit`: run through `irm | iex`, exit would close the terminal of the user.

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue' # the progress bar makes Invoke-WebRequest very slow on PowerShell 5.1
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

$releases = if ($env:POLIX_RELEASES_URL) { $env:POLIX_RELEASES_URL.TrimEnd('/') } else { 'https://github.com/mlophez/polix-ai/releases' }
$version = if ($env:POLIX_VERSION) { $env:POLIX_VERSION } else { 'latest' }

# $IsWindows, $IsMacOS and $IsLinux only exist from PowerShell 6; Windows PowerShell 5.1 only runs on Windows.
$os = 'windows'
if ($PSVersionTable.PSVersion.Major -ge 6 -and -not $IsWindows) {
    $os = if ($IsMacOS) { 'darwin' } else { 'linux' }
}

try {
    $arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()
} catch {
    $arch = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
}
switch -Regex ($arch) {
    '^(X64|AMD64)$' { $arch = 'amd64'; break }
    '^(Arm64|ARM64)$' { $arch = 'arm64'; break }
    default { throw "install.ps1: unsupported architecture $arch" }
}

$exeName = if ($os -eq 'windows') { 'polix.exe' } else { 'polix' }
$asset = "polix_${os}_${arch}" + $(if ($os -eq 'windows') { '.exe' } else { '' })
$dir = if ($env:POLIX_INSTALL_DIR) { $env:POLIX_INSTALL_DIR }
       elseif ($os -eq 'windows') { Join-Path $env:LOCALAPPDATA 'Programs\polix' }
       else { Join-Path $HOME '.local/bin' }
$base = if ($version -eq 'latest') { "$releases/latest/download" } else { "$releases/download/v$($version.TrimStart('v'))" }

$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("polix-install-" + [System.Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Force -Path $tmp | Out-Null
try {
    $sums = Join-Path $tmp 'checksums.txt'
    $file = Join-Path $tmp $asset
    try {
        Invoke-WebRequest -UseBasicParsing -Uri "$base/checksums.txt" -OutFile $sums
        Invoke-WebRequest -UseBasicParsing -Uri "$base/$asset" -OutFile $file
    } catch {
        throw "install.ps1: cannot download $base/${asset}: $($_.Exception.Message)"
    }

    $want = $null
    foreach ($line in Get-Content $sums) {
        $parts = $line.Trim() -split '\s+'
        if ($parts.Count -eq 2 -and $parts[1].TrimStart('*') -eq $asset) { $want = $parts[0].ToLower() }
    }
    if (-not $want) { throw "install.ps1: checksums.txt has no entry for $asset" }
    $got = (Get-FileHash -Algorithm SHA256 -Path $file).Hash.ToLower()
    if ($got -ne $want) { throw "install.ps1: checksum of $asset is $got, checksums.txt says $want" }

    New-Item -ItemType Directory -Force -Path $dir | Out-Null
    $target = Join-Path $dir $exeName
    if (Test-Path $target) {
        # Windows cannot overwrite a binary that is running (a hook of another session): move it aside first.
        $old = "$target.old"
        Remove-Item -Force -ErrorAction SilentlyContinue $old
        Move-Item -Force $target $old
        Remove-Item -Force -ErrorAction SilentlyContinue $old
    }
    Move-Item -Force $file $target
    if ($os -ne 'windows') { & chmod +x $target }
    Write-Host "installed polix $(& $target version) in $target"
} finally {
    Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $tmp
}

$sep = [System.IO.Path]::PathSeparator
if (($env:PATH -split [regex]::Escape($sep)) -notcontains $dir) {
    if ($os -eq 'windows' -and -not $env:POLIX_NO_MODIFY_PATH) {
        $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
        if (-not $userPath) { $userPath = '' }
        if (($userPath -split ';') -notcontains $dir) {
            [Environment]::SetEnvironmentVariable('Path', (($userPath.TrimEnd(';') + ';' + $dir).TrimStart(';')), 'User')
        }
        $env:PATH = "$env:PATH$sep$dir"
        Write-Host "added $dir to the user PATH: open a new terminal and restart Claude Code so the hooks find polix"
    } else {
        Write-Warning "$dir is not on the PATH; add it, or the hooks will not find polix"
    }
}
