#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Host facts made available to 'when' conditions and '${{ facts.* }}' templating.

.DESCRIPTION
  A deliberately small, easy-to-extend starter set - add more here as tasks
  need to branch on them. Facts are gathered fresh every run (unlike 'vars',
  which come from YAML and are merged/overridable).
#>

Set-StrictMode -Version Latest

function Get-InstallFacts {
  $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
  $principal = [Security.Principal.WindowsPrincipal]::new($identity)

  @{
    computer_name = $env:COMPUTERNAME
    user_name     = $env:USERNAME
    home          = $HOME
    os_version    = [System.Environment]::OSVersion.Version.ToString()
    os_build      = [System.Environment]::OSVersion.Version.Build
    is_admin      = $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
    pwsh_version  = $PSVersionTable.PSVersion.ToString()
  }
}

Export-ModuleMember -Function Get-InstallFacts
