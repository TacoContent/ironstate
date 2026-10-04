package remoteexec

import (
	"bytes"
	"strings"
	"testing"

	"github.com/TacoContent/ironstate/internal/remoteexec/protocol"
)

func TestParseRunLogIgnoresTruncatedFinalLine(t *testing.T) {
	var log bytes.Buffer
	writer := protocol.NewWriter(&log)
	if err := writer.Hello("run-1", "dev", "linux", "amd64", 1, "/cache/runs/run-1/events.ndjson"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Done(0); err != nil {
		t.Fatal(err)
	}
	result, err := parseRunLog("run-1", "#< CLIXML\n"+log.String()+`{"v":1,"type":"leaf_result"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Completed || result.Hello == nil || result.ExitCode != 0 || strings.Contains(result.RawLog, "CLIXML") {
		t.Fatalf("recovered result = %+v", result)
	}
}

func TestParseRunLogRejectsInvalidRunID(t *testing.T) {
	if _, err := parseRunLog("../other", ""); err == nil || !strings.Contains(err.Error(), "invalid run id") {
		t.Fatalf("error = %v", err)
	}
}
