package filters

import _ "embed"

//go:embed embed/shim.ps1
var shimPS1 []byte

//go:embed embed/shim.sh
var shimSH []byte

//go:embed embed/shim.zsh
var shimZsh []byte

//go:embed embed/shim.fish
var shimFish []byte

//go:embed embed/shim.nu
var shimNu []byte
