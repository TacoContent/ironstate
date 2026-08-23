//go:build windows

package ui

// Windows consoles default to the system's legacy codepage (437/1252/...),
// which mangles this package's UTF-8 emoji/box-drawing output (mojibake
// like "ΓöÇ" for "─"). Setting the console's output codepage to UTF-8
// (65001) at startup fixes that without requiring users to run `chcp
// 65001` or set $OutputEncoding themselves first.

import "golang.org/x/sys/windows"

const cpUTF8 = 65001

func init() {
	_ = windows.SetConsoleOutputCP(cpUTF8)
}
