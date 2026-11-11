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
#   POLIX_NO_SETUP        set it to skip `polix install --user`, which installs the plugin of polix for the user
#
# Errors are thrown, never `exit`: run through `irm | iex`, exit would close the terminal of the user. The whole body
# runs in a script block, so `irm | iex` leaves no variable, preference or protocol change in the session.

& {
    $ErrorActionPreference = 'Stop'
    $ProgressPreference = 'SilentlyContinue' # the progress bar makes Invoke-WebRequest very slow on PowerShell 5.1

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
    $new = $null
    # TLS 1.2 for the downloads, which Windows PowerShell 5.1 may not enable; the setting of the session is put back.
    $protocol = [Net.ServicePointManager]::SecurityProtocol
    [Net.ServicePointManager]::SecurityProtocol = $protocol -bor [Net.SecurityProtocolType]::Tls12
    try {
        $sums = Join-Path $tmp 'checksums.txt'
        $file = Join-Path $tmp $asset
        foreach ($d in @(@("$base/checksums.txt", $sums), @("$base/$asset", $file))) {
            try {
                Invoke-WebRequest -UseBasicParsing -Uri $d[0] -OutFile $d[1]
            } catch {
                throw "install.ps1: cannot download $($d[0]): $($_.Exception.Message)"
            }
        }

        $want = $null
        foreach ($line in Get-Content $sums) {
            $parts = $line.Trim() -split '\s+'
            if ($parts.Count -eq 2 -and $parts[1].TrimStart('*') -eq $asset) { $want = $parts[0].ToLower() }
        }
        if (-not $want) { throw "install.ps1: checksums.txt has no entry for $asset" }
        # SHA-256 through .NET, not Get-FileHash: Windows PowerShell 5.1 started from PowerShell 7 inherits its
        # PSModulePath and cannot load the Get-FileHash of its own Microsoft.PowerShell.Utility module.
        $sha = [System.Security.Cryptography.SHA256]::Create()
        $stream = [System.IO.File]::OpenRead($file)
        try {
            $got = ([System.BitConverter]::ToString($sha.ComputeHash($stream)) -replace '-', '').ToLower()
        } finally {
            $stream.Dispose()
            $sha.Dispose()
        }
        if ($got -ne $want) { throw "install.ps1: checksum of $asset is $got, checksums.txt says $want" }

        New-Item -ItemType Directory -Force -Path $dir | Out-Null
        $target = Join-Path $dir $exeName
        # Copied next to the target first, so the last step is a rename on the same disk, never a copy in place.
        $new = "$target.new"
        Copy-Item -Force $file $new
        if ($os -ne 'windows') { & chmod +x $new }
        $old = "$target.old"
        $hadOld = Test-Path $target
        if ($hadOld) {
            # Windows cannot overwrite a binary that is running (a hook of another session): move it aside first.
            Remove-Item -Force -ErrorAction SilentlyContinue $old
            Move-Item -Force $target $old
        }
        try {
            Move-Item -Force $new $target
        } catch {
            # An antivirus may hold the new file: put the old binary back, so the machine is never left without polix.
            $why = $_.Exception.Message
            if ($hadOld) {
                try {
                    Move-Item -Force $old $target
                } catch {
                    throw "install.ps1: cannot replace ${target}: $why; and the old binary could not be put back from ${old}: $($_.Exception.Message)"
                }
            }
            throw "install.ps1: cannot replace ${target}: $why"
        }
        # A running polix.exe cannot be removed: polix removes it on its next run.
        if ($hadOld) { Remove-Item -Force -ErrorAction SilentlyContinue $old }
        Write-Host "installed polix $(& $target version) in $target"
    } finally {
        [Net.ServicePointManager]::SecurityProtocol = $protocol
        Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $tmp
        if ($new) { Remove-Item -Force -ErrorAction SilentlyContinue $new }
    }

    $sep = [System.IO.Path]::PathSeparator
    if (($env:PATH -split [regex]::Escape($sep)) -notcontains $dir) {
        if ($os -eq 'windows' -and -not $env:POLIX_NO_MODIFY_PATH) {
            # Read and written in the registry, not through [Environment]: that one expands the entries such as
            # %USERPROFILE%\bin and writes the Path back as a plain string, losing them for good.
            $key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment', $true)
            try {
                $userPath = [string]$key.GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
                # The type of an existing Path is kept (REG_SZ or REG_EXPAND_SZ); a new one may hold %VARIABLES%.
                $kind = [Microsoft.Win32.RegistryValueKind]::ExpandString
                if ($key.GetValueNames() -contains 'Path') { $kind = $key.GetValueKind('Path') }
                if (($userPath -split ';') -notcontains $dir) {
                    $key.SetValue('Path', (($userPath.TrimEnd(';') + ';' + $dir).TrimStart(';')), $kind)
                }
            } finally {
                $key.Dispose()
            }
            # Setting and removing a user variable through [Environment] broadcasts the change of the environment, so
            # new terminals see the new Path without signing out.
            [Environment]::SetEnvironmentVariable('POLIX_INSTALL_BROADCAST', '1', 'User')
            [Environment]::SetEnvironmentVariable('POLIX_INSTALL_BROADCAST', $null, 'User')
            $env:PATH = "$env:PATH$sep$dir"
            Write-Host "added $dir to the user PATH: open a new terminal and restart Claude Code so the hooks find polix"
        } else {
            Write-Warning "$dir is not on the PATH; add it, or the hooks will not find polix"
        }
    }

    # The plugin of the user, embedded in the binary just installed. The hooks belong to each repository: polix
    # install registers them there.
    # --user, hidden as --skills in later releases, since POLIX_VERSION may install a release that has no --skills.
    if (-not $env:POLIX_NO_SETUP) {
        & (Join-Path $dir $exeName) install --user --yes
        if ($LASTEXITCODE -ne 0) { Write-Warning "polix install --user failed: run it again by hand" }
    }
}
