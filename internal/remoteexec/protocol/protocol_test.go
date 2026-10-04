package protocol

import (
	"bufio"
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/TacoContent/ironstate/internal/engine"
)

func TestJobRoundTripLeavesBundleBytesUnread(t *testing.T) {
	var buf bytes.Buffer
	job := Job{RunID: "abc", Options: JobOptions{Playbook: "playbook/site.yml", Apply: true}, Bundle: BundleInfo{Size: 4, SHA256: "x"}}
	if err := WriteJob(&buf, job); err != nil {
		t.Fatal(err)
	}
	buf.WriteString("DATA")
	r := bufio.NewReader(&buf)
	got, err := ReadJob(r)
	if err != nil {
		t.Fatal(err)
	}
	if got.RunID != "abc" || got.Options.Playbook != "playbook/site.yml" || !got.Options.Apply {
		t.Fatalf("job = %+v", got)
	}
	rest := make([]byte, 4)
	if _, err := r.Read(rest); err != nil || string(rest) != "DATA" {
		t.Fatalf("bundle bytes = %q, %v", rest, err)
	}
}

func TestReadJobRejectsWrongVersion(t *testing.T) {
	r := bufio.NewReader(strings.NewReader(`{"v":99,"type":"job"}` + "\n"))
	if _, err := ReadJob(r); err == nil || !strings.Contains(err.Error(), "version") {
		t.Fatalf("err = %v, want version mismatch", err)
	}
}

func TestParseEventIgnoresNonProtocolLines(t *testing.T) {
	for _, line := range []string{"welcome to host", `{"type":"done"}`, `{"v":1,"type":"done"` /* truncated */} {
		if _, ok := ParseEvent([]byte(line)); ok {
			t.Errorf("ParseEvent(%q) accepted a non-protocol line", line)
		}
	}
	ev, ok := ParseEvent([]byte(`{"v":1,"type":"done","exit_code":0}` + "\r\n"))
	if !ok || ev.Type != TypeDone || ev.ExitCode == nil || *ev.ExitCode != 0 {
		t.Fatalf("ParseEvent = %+v, %v", ev, ok)
	}
}

func TestWriterLinesStartWithPrefixAndDoneIsTerminal(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	_ = w.Log(LevelInfo, "hello <world>")
	_ = w.Fail("apply", errors.New("boom"), 2)
	_ = w.Done(0)
	_ = w.Log(LevelInfo, "after done")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3:\n%s", len(lines), buf.String())
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, `{"v":1,"type":`) {
			t.Errorf("line %q lacks protocol prefix", line)
		}
	}
	if !strings.Contains(lines[0], "<world>") {
		t.Errorf("HTML was escaped: %s", lines[0])
	}
	last, _ := ParseEvent([]byte(lines[2]))
	if last.Type != TypeDone || *last.ExitCode != 2 {
		t.Fatalf("last event = %+v, want done(2)", last)
	}
}

func TestLeafResultTruncatesLargeOutput(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	big := strings.Repeat("x", MaxFieldBytes+10)
	_ = w.LeafResult(engine.Result{Module: "shell", Exec: engine.ExecResult{Stdout: big, StdoutLines: []string{big}}})
	ev, ok := ParseEvent(buf.Bytes())
	if !ok || ev.Result == nil {
		t.Fatalf("no leaf_result parsed")
	}
	if !ev.Result.Exec.Truncated || len(ev.Result.Exec.Stdout) != MaxFieldBytes || len(ev.Result.Exec.StdoutLines) != 0 {
		t.Fatalf("not truncated: truncated=%v len=%d lines=%d", ev.Result.Exec.Truncated, len(ev.Result.Exec.Stdout), len(ev.Result.Exec.StdoutLines))
	}
}

func TestWriterKeepsRunLogAfterStreamFails(t *testing.T) {
	var log bytes.Buffer
	w := NewWriter(failingWriter{})
	w.SetLog(&log)
	if w.Cancelled() != nil {
		t.Fatal("cancelled before any failure")
	}
	_ = w.Log(LevelInfo, "first")
	if err := w.Cancelled(); err == nil || !strings.Contains(err.Error(), "controller disconnected") {
		t.Fatalf("Cancelled() = %v, want disconnect", err)
	}
	_ = w.Fail("apply", errors.New("stopped"), 1)
	lines := strings.Split(strings.TrimSpace(log.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("run log has %d lines, want log+error+done:\n%s", len(lines), log.String())
	}
	if ev, _ := ParseEvent([]byte(lines[2])); ev.Type != TypeDone {
		t.Fatalf("last run log line = %s", lines[2])
	}
}

func TestCancelReasonWinsAndControlRoundTrip(t *testing.T) {
	w := NewWriter(&bytes.Buffer{})
	w.Cancel(errors.New("first"))
	w.Cancel(errors.New("second"))
	if err := w.Cancelled(); err == nil || err.Error() != "first" {
		t.Fatalf("Cancelled() = %v", err)
	}
	var buf bytes.Buffer
	buf.WriteString("noise\n")
	_ = WriteCancel(&buf)
	r := bufio.NewReader(&buf)
	typ, err := ReadControl(r)
	if err != nil || typ != TypeCancel {
		t.Fatalf("ReadControl = %q, %v", typ, err)
	}
	if _, err := ReadControl(r); err == nil {
		t.Fatal("expected EOF after the only control line")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }

func FuzzParseEvent(f *testing.F) {
	f.Add([]byte(`{"v":1,"type":"leaf_result","result":{"module":"x"}}`))
	f.Add([]byte(`{"v":1,"type":"done","exit_code":1}`))
	f.Add([]byte("noise"))
	f.Fuzz(func(t *testing.T, line []byte) {
		ev, ok := ParseEvent(line)
		if ok && ev.Type == "" {
			t.Fatal("accepted event without a type")
		}
	})
}

func FuzzReadJob(f *testing.F) {
	f.Add([]byte(`{"v":1,"type":"job","bundle":{"size":3}}` + "\nabc"))
	f.Add([]byte("\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		job, err := ReadJob(bufio.NewReader(bytes.NewReader(data)))
		if err == nil && (job.V != Version || job.Type != TypeJob || job.Bundle.Size < 0) {
			t.Fatalf("accepted invalid job %+v", job)
		}
	})
}
