package becomeexec

import (
	"testing"

	"github.com/TacoContent/ironstate/sdk/handler"
)

func TestWrapForBecomeLeavesUnelevatedCommandUntouched(t *testing.T) {
	args := []string{"status"}
	executable, wrapped, err := WrapForBecome(handler.Become{}, "tool", args)
	if err != nil {
		t.Fatalf("WrapForBecome returned error: %v", err)
	}
	if executable != "tool" {
		t.Fatalf("executable = %q, want tool", executable)
	}
	if len(wrapped) != 1 || wrapped[0] != "status" {
		t.Fatalf("arguments = %#v, want [status]", wrapped)
	}
}
