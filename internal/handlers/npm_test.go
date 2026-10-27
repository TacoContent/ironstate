package handlers

import (
	"testing"
)

func TestNpmHandlerScanRolePath(t *testing.T) {
	npm := npmHandler{}
	if got := npm.ScanRole(); got != "roles/packages/npm" {
		t.Fatalf("ScanRole() = %q, want roles/packages/npm", got)
	}
}
