# Default PowerShell aliases — sourced by ~/.config/powershell/Microsoft.PowerShell_profile.ps1.

# Modern CLI replacements (gated on availability)
if (Get-Command bat  -ErrorAction SilentlyContinue) { 
    function Invoke-Bat {
        bat --paging=never @args
    }
    Remove-Item Alias:cat -ErrorAction SilentlyContinue
    Set-Alias -Name cat -Value Invoke-Bat -Scope Global
}


if (Get-Command btop -ErrorAction SilentlyContinue) { Set-Alias -Name top -Value btop -Option AllScope }


# Navigation — dot aliases need wrapper functions since Set-Alias cannot carry arguments.
# Note: '.' (pwd) and '.2'–'.9' (zsh dir-stack) have no PS equivalent and are omitted.
function Set-LocationHome { Set-Location ~ }
function Set-LocationUp1  { Set-Location .. }
function Set-LocationUp2  { Set-Location ../.. }
function Set-LocationUp3  { Set-Location ../../.. }
function Set-LocationUp4  { Set-Location ../../../.. }
function Set-LocationUp5  { Set-Location ../../../../.. }

Set-Alias -Name '~'      -Value Set-LocationHome -Option AllScope
Set-Alias -Name '..'     -Value Set-LocationUp1  -Option AllScope
Set-Alias -Name '...'    -Value Set-LocationUp2  -Option AllScope
Set-Alias -Name '....'   -Value Set-LocationUp3  -Option AllScope
Set-Alias -Name '.....'  -Value Set-LocationUp4  -Option AllScope
Set-Alias -Name '......' -Value Set-LocationUp5  -Option AllScope

# Listing
function Invoke-ListLong        { Get-ChildItem @args | Format-Table Mode, LastWriteTime, Length, Name -AutoSize }
function Invoke-ListAll         { Get-ChildItem -Force @args | Format-Table Mode, LastWriteTime, Length, Name -AutoSize }
function Invoke-ListDirectories { Get-ChildItem -Directory @args }

if (Get-Command eza -ErrorAction SilentlyContinue) {
    function ls   { eza --icons @args }
    function ll   { eza --icons -la --git @args }
    function la   { eza --icons -la @args }
    function tree { eza --icons --tree @args }
    function lsd { eza --icons -l --only-dirs @args }
} else {
    Set-Alias -Name l   -Value Invoke-ListLong        -Option AllScope
    Set-Alias -Name ll  -Value Invoke-ListAll          -Option AllScope
    Set-Alias -Name la  -Value Invoke-ListAll          -Option AllScope
    Set-Alias -Name lsd -Value Invoke-ListDirectories  -Option AllScope
    Set-Alias -Name tree -Value Invoke-ListLong -Option AllScope
}


# Git
if (Get-Command git -ErrorAction SilentlyContinue) {
    Set-Alias -Name g -Value git
}

# Kubernetes
if (Get-Command kubectl -ErrorAction SilentlyContinue) {
    Set-Alias -Name kc -Value kubectl
}

# Docker
if (Get-Command docker -ErrorAction SilentlyContinue) {
    Set-Alias -Name d -Value docker
}

# --- IP address utilities ---
function whatsmyip { (Invoke-RestMethod -Uri 'https://api.ipify.org').Trim() }
function ifconfigme { (Invoke-RestMethod -Uri 'https://ifconfig.me').Trim() }
function ips {
    Get-NetIPAddress |
        Where-Object { $_.AddressState -eq 'Preferred' -and $_.PrefixOrigin -ne 'WellKnown' } |
        Select-Object -ExpandProperty IPAddress
}

# --- Hex dump ---
Set-Alias -Name hd -Value Format-Hex

# sha256sum not in C:\xbin, use PowerShell's Get-FileHash
function sha256sum { Get-FileHash -Algorithm SHA256 @args | Select-Object Hash, Path }

# --- map (like xargs -n1: apply a command to each piped item) ---
# Usage: Get-ChildItem . -Name | map { param($f) Write-Host $f }
#    or: 'path1','path2' | map Split-Path
function map {
    $cmd = $args[0]
    $input | ForEach-Object { & $cmd $_ }
}

if (Get-Command terraform -ErrorAction SilentlyContinue) {
    Set-Alias -Name tf -Value terraform
}