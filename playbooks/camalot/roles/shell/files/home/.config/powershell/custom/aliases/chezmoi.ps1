# chezmoi aliases
if (Get-Command chezmoi -ErrorAction SilentlyContinue) {
    function cm  { chezmoi @args }
    function cma { chezmoi add @args }
    function cmu { chezmoi add .; chezmoi apply }
    function cmp { chezmoi git push @args }
    function cmc { chezmoi git commit @args }
    function cmg { chezmoi git @args }
}
