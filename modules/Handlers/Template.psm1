#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Handler for the 'template' group: renders 'src' through the given
  'engine' and writes the result to 'dest'.

.DESCRIPTION
  Ansible 'template'-style, but with a choice of three rendering engines
  (a required 'engine' field, no extension sniffing):
    - 'jinja'      - sandboxed, block-capable ({{ }}, {% if/for/set %})
    - 'eps'        - the real PowerShell Gallery 'EPS' module, full
                     unrestricted PowerShell in '<% %>'/'<%= %>' tags
    - 'herestring' - '$Name.Path' / '$(...)' interpolation only, same
                     sandboxed grammar as 'jinja', no block constructs
  See modules/TemplateEngines/{Jinja,Eps,HereString}.psm1 for each engine's
  own design notes.

  'src' is resolved relative to the install system directory (or the
  owning package's own directory for packages/<name>/main.yml) by
  Resolve-RelativePathsInPlace at load time - by the time this handler
  runs, 'src' is already an absolute path, same convention as 'copy.src'.

  The render context is this leaf's merged facts/vars/id-registry context
  (threaded in by ironstate.ps1's Invoke-Tasks/Invoke-PackageItem), with
  this task's own 'vars' (if any) layered on top, last-write-wins - then
  run back through ironstate's own '${{ }}' resolver (Templates.psm1),
  self-referentially, before being handed to the render engine. That
  extra pass matters because ironstate.ps1's one whole-document '-Soft'
  pass (before Expand-TaskTree even runs) only recognizes the 'facts'/
  'vars' namespaces - a '${{ }}' reference *inside* vars content itself
  (e.g. one vars key pointing at a bare sibling key, 'development.work'
  rather than 'vars.development.work') is deferred there and never
  revisited by Invoke-Tasks's later per-leaf strict pass either, since
  that one only re-resolves a leaf's own module fields, not the deep
  *content* already sitting inside 'vars:'. Without this, a template's
  render context could still contain literal, unresolved ironstate
  '${{ }}' syntax by the time it reaches Jinja/EPS/herestring.

  "installed" means 'dest' already holds exactly the freshly-rendered
  content - a plain string comparison, the template equivalent of Copy's
  SHA256 hash compare (the "source" here is a render result, not a static
  file, so there's nothing to hash on the source side).

  Single file only - unlike 'copy', there's no directory-tree templating.
#>

Set-StrictMode -Version Latest

Import-Module (Join-Path $PSScriptRoot '..\Common.psm1')
Import-Module (Join-Path $PSScriptRoot '..\Templates.psm1')
Import-Module (Join-Path $PSScriptRoot '..\TemplateEngines\Jinja.psm1')
Import-Module (Join-Path $PSScriptRoot '..\TemplateEngines\Eps.psm1')
Import-Module (Join-Path $PSScriptRoot '..\TemplateEngines\HereString.psm1')

function Resolve-TemplateRenderContext {
  # Strict, self-referential '${{ }}' resolution pass over the merged
  # render context - see this file's own .DESCRIPTION for why this is
  # needed on top of ironstate.ps1's existing soft/strict passes. Mutates
  # a wrapper, not $Context directly, matching Invoke-Tasks's own
  # 'Resolve-TemplatesInPlace' usage.
  param($Context)
  $wrapper = @{ ctx = $Context }
  Resolve-TemplatesInPlace -Data $wrapper -Context $Context -PackageName 'template'
  return $wrapper.ctx
}

function Get-TemplateRenderContext {
  # Layers this task's own 'vars' (if any) on top of the leaf's merged flat
  # $Context, key-by-key, last-write-wins. Deep-cloned (Common.psm1's
  # Copy-DeepData) rather than a shallow per-key copy: Resolve-
  # TemplateRenderContext below mutates the result in place, and a shallow
  # copy's nested dicts (e.g. 'git.profiles') would still be the exact same
  # objects shared by $Context/$vars across every other task - mutating
  # those in place here would leak this render's resolution into every
  # later leaf that reads the same site-wide vars.
  param($Item, $Context)
  $merged = Copy-DeepData -Data $Context
  $extra = Get-Prop $Item 'vars' @{}
  foreach ($key in @($extra.Keys)) { $merged[$key] = Copy-DeepData -Data $extra[$key] }
  return Resolve-TemplateRenderContext -Context $merged
}

function Get-TemplateRenderedContent {
  param($Item, $Context)

  $src = Get-Prop $Item 'src'
  if (-not $src -or -not (Test-Path $src)) {
    Write-Warning "Source path for template does not exist: $src"
    return $null
  }

  $raw = Get-Content -Raw -Path $src
  $renderContext = Get-TemplateRenderContext -Item $Item -Context $Context
  $engine = Get-Prop $Item 'engine'

  switch ($engine) {
    'jinja'      { return Render-JinjaTemplate -Content $raw -Context $renderContext }
    'eps'        { return Render-EpsTemplate -Content $raw -Context $renderContext }
    'herestring' { return Render-HereStringTemplate -Content $raw -Context $renderContext }
    default      { throw "Unknown template engine '$engine'" }
  }
}

function Get-TemplateHandler {
  [PSCustomObject]@{
    Test      = {
      param($Item, $Name, $Context)
      $dest = Resolve-UserPath (Get-Prop $Item 'dest')
      $src  = Get-Prop $Item 'src'
      if (-not (Test-Path $src)) { Write-Warning "Source path for template does not exist: $src"; return $false }
      if (-not (Test-Path $dest -PathType Leaf)) { return $false }

      try {
        $rendered = Get-TemplateRenderedContent -Item $Item -Context $Context
      } catch {
        Write-Warning "Template render failed for '$src': $($_.Exception.Message)"
        return $false
      }
      if ($null -eq $rendered) { return $false }

      return (Get-Content -Raw -Path $dest) -eq $rendered
    }
    Describe  = {
      param($Item, $Action, $Context)
      $dest = Resolve-UserPath (Get-Prop $Item 'dest')
      $src  = Get-Prop $Item 'src'
      if ($Action -eq 'Uninstall') { "remove $dest" } else { "render $src -> $dest (engine: $(Get-Prop $Item 'engine'))" }
    }
    Install   = {
      param($Item, $Name, $Context)
      $dest = Resolve-UserPath (Get-Prop $Item 'dest')
      $rendered = Get-TemplateRenderedContent -Item $Item -Context $Context
      if ($null -eq $rendered) { return }

      $destDir = Split-Path $dest -Parent
      if ($destDir -and -not (Test-Path $destDir)) { New-Item -ItemType Directory -Path $destDir -Force | Out-Null }
      Set-Content -Path $dest -Value $rendered -NoNewline -Encoding utf8NoBOM
    }
    Uninstall = {
      param($Item, $Name, $Context)
      $dest = Resolve-UserPath (Get-Prop $Item 'dest')
      if (Test-Path $dest) { Remove-Item -Path $dest -Force }
    }
  }
}

Export-ModuleMember -Function Get-TemplateHandler, Get-TemplateRenderedContent
