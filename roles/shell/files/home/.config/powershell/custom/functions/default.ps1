# Default PowerShell functions — sourced by the host wrapper.

function which($cmd) {
    Get-Command $cmd -ErrorAction SilentlyContinue | Select-Object -ExpandProperty Source
}

function reload { . $PROFILE }

function versions {
    if (Get-Command pwsh      -ErrorAction SilentlyContinue) { (pwsh --version) }
    if (Get-Command git       -ErrorAction SilentlyContinue) { git --version }
    if (Get-Command python    -ErrorAction SilentlyContinue) { python --version }
    if (Get-Command go        -ErrorAction SilentlyContinue) { go version }
    if (Get-Command terraform -ErrorAction SilentlyContinue) { (terraform --version | Select-Object -First 1) }
    if (Get-Command kubectl   -ErrorAction SilentlyContinue) { (kubectl version --client --short 2>$null) }
    if (Get-Command docker    -ErrorAction SilentlyContinue) { docker --version }
    if (Get-Command yq        -ErrorAction SilentlyContinue) { yq --version }
    if (Get-Command jq        -ErrorAction SilentlyContinue) { jq --version }
}

function gi {
    param(
        [Parameter(Mandatory=$true, Position=0)]
        [string[]]$Technologies
    )

    # Join the array elements with a comma
    $argsString = $Technologies -join ','

    # Fetch the data and output it as clean text
    Invoke-RestMethod -Uri "https://gitignore.io/api/$argsString" -UserAgent "curl"
}
