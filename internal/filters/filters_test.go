package filters

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func apply(t *testing.T, name string, value any, args ...any) any {
	t.Helper()
	v, err := New().Apply(name, value, args)
	if err != nil {
		t.Fatalf("%s(%v, %v) error: %v", name, value, args, err)
	}
	return v
}

func TestDefaultFilter(t *testing.T) {
	if got := apply(t, "default", nil, "fallback"); got != "fallback" {
		t.Errorf("default(nil) = %v", got)
	}
	if got := apply(t, "default", "x", "fallback"); got != "x" {
		t.Errorf("default(x) = %v", got)
	}
}

func TestToggleFilter(t *testing.T) {
	if got := apply(t, "toggle", "Eclipse.Temurin.21", "builtin"); got != "Eclipse.Temurin.21" {
		t.Errorf("toggle(string) = %v", got)
	}
	if got := apply(t, "toggle", true, "builtin"); got != "builtin" {
		t.Errorf("toggle(true) = %v, want fallback (bool doesn't count as override)", got)
	}
	if got := apply(t, "toggle", nil, "builtin"); got != "builtin" {
		t.Errorf("toggle(nil) = %v", got)
	}
}

func TestTernaryFilter(t *testing.T) {
	if got := apply(t, "ternary", true, "yes", "no"); got != "yes" {
		t.Errorf("ternary(true) = %v", got)
	}
	if got := apply(t, "ternary", false, "yes", "no"); got != "no" {
		t.Errorf("ternary(false) = %v", got)
	}
	if got := apply(t, "ternary", "", "yes", "no"); got != "no" {
		t.Errorf("ternary('') = %v, want 'no' (empty string is falsy)", got)
	}
}

func TestEnabledFilter(t *testing.T) {
	cases := []struct {
		name  string
		value any
		args  []any
		want  bool
	}{
		{"bare boolean true", true, nil, true},
		{"bare boolean false", false, nil, false},
		{"mapping with no args counts as on", map[string]any{"chrome": true}, nil, true},
		{"nested key true", map[string]any{"browsers": map[string]any{"chrome": true}}, []any{"browsers", "chrome"}, true},
		{"ancestor false wins over deeper true", map[string]any{"browsers": false}, []any{"browsers", "chrome"}, false},
		{"missing key", map[string]any{}, []any{"browsers"}, false},
		{"scalar leaf is off", map[string]any{"jdk": "Eclipse.Temurin.21"}, []any{"jdk"}, false},
	}
	for _, c := range cases {
		got := apply(t, "enabled", c.value, c.args...)
		if got != c.want {
			t.Errorf("%s: enabled(%v, %v) = %v, want %v", c.name, c.value, c.args, got, c.want)
		}
	}
}

func TestStringFilters(t *testing.T) {
	if got := apply(t, "upper", "abc"); got != "ABC" {
		t.Errorf("upper = %v", got)
	}
	if got := apply(t, "lower", "ABC"); got != "abc" {
		t.Errorf("lower = %v", got)
	}
	if got := apply(t, "trim", "  x  "); got != "x" {
		t.Errorf("trim = %v", got)
	}
	if got := apply(t, "quote", "x"); got != `"x"` {
		t.Errorf("quote default = %v", got)
	}
	if got := apply(t, "quote", "x", "'"); got != "'x'" {
		t.Errorf("quote custom = %v", got)
	}
	if got := apply(t, "quote", "   "); got != nil {
		t.Errorf("quote whitespace-only = %v, want nil", got)
	}
	for _, name := range []string{"upper", "lower", "trim", "quote", "sha1", "dirname", "basename", "resolve"} {
		if got := apply(t, name, nil); got != nil {
			t.Errorf("%s(nil) = %v, want nil (null-in/null-out)", name, got)
		}
	}
}

func TestLengthFilter(t *testing.T) {
	if got := apply(t, "length", nil); got != float64(0) {
		t.Errorf("length(nil) = %v", got)
	}
	if got := apply(t, "length", "hello"); got != float64(5) {
		t.Errorf("length(string) = %v", got)
	}
	if got := apply(t, "length", []any{1, 2, 3}); got != float64(3) {
		t.Errorf("length(list) = %v", got)
	}
}

func TestConcatFilter(t *testing.T) {
	if got := apply(t, "concat", "hello", " ", "world"); got != "hello world" {
		t.Errorf("concat scalar+extra = %v", got)
	}
	if got := apply(t, "concat", []any{"a", "b"}, ","); got != "a,b" {
		t.Errorf("concat list = %v", got)
	}
}

func TestJoinFilter(t *testing.T) {
	got := apply(t, "join", "base", "sub", "file.txt")
	want := filepath.Join("base", "sub", "file.txt")
	if got != want {
		t.Errorf("join = %v, want %v", got, want)
	}
}

func TestSplitFilter(t *testing.T) {
	got := apply(t, "split", "a,b,c", ",")
	want := []any{"a", "b", "c"}
	if len(got.([]any)) != len(want) {
		t.Fatalf("split = %#v", got)
	}
	// trailing delimiter drops one trailing empty element (round-trips concat)
	got2 := apply(t, "split", "a,b,", ",")
	list := got2.([]any)
	if len(list) != 2 {
		t.Fatalf("split trailing delimiter = %#v, want 2 elements", list)
	}
}

func TestPrefixFilter(t *testing.T) {
	if got := apply(t, "prefix", "value", "key"); got != "key value" {
		t.Errorf("prefix scalar = %v", got)
	}
	got := apply(t, "prefix", []any{"a", "b"}, "k")
	list := got.([]any)
	if list[0] != "k a" || list[1] != "k b" {
		t.Errorf("prefix list = %#v", list)
	}
}

func TestDirnameBasename(t *testing.T) {
	if got := apply(t, "dirname", `C:\foo\bar.txt`); got != `C:\foo` {
		t.Errorf("dirname = %v", got)
	}
	if got := apply(t, "basename", `C:\foo\bar.txt`); got != "bar.txt" {
		t.Errorf("basename = %v", got)
	}
	if got := apply(t, "dirname", "bar.txt"); got != "" {
		t.Errorf("dirname no separator = %v, want empty string", got)
	}
}

func TestExistsFilter(t *testing.T) {
	tmp := t.TempDir()
	file := filepath.Join(tmp, "exists.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := apply(t, "exists", file); got != true {
		t.Errorf("exists(real file) = %v", got)
	}
	if got := apply(t, "exists", filepath.Join(tmp, "missing.txt")); got != false {
		t.Errorf("exists(missing) = %v", got)
	}
	if got := apply(t, "exists", filepath.Join(tmp, "missing.txt"), false); got != true {
		t.Errorf("exists(missing, expected=false) = %v", got)
	}
	if got := apply(t, "exists", nil); got != false {
		t.Errorf("exists(nil) = %v, want false (default expected=true)", got)
	}
}

// TestExistsFilterExpandsTilde guards against the exists filter checking
// a literal '~' path segment instead of the user's home directory - the
// original PowerShell's Test-Path resolves '~' automatically.
func TestExistsFilterExpandsTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory available")
	}
	marker := filepath.Join(home, ".ironstate-exists-filter-test-marker")
	if err := os.WriteFile(marker, []byte("x"), 0o600); err != nil {
		t.Skip("cannot write to home directory in this environment")
	}
	defer os.Remove(marker)

	if got := apply(t, "exists", "~/.ironstate-exists-filter-test-marker"); got != true {
		t.Errorf("exists(~/...) = %v, want true", got)
	}
}

func TestSHA1Filter(t *testing.T) {
	got := apply(t, "sha1", "hello")
	if got != "aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d" {
		t.Errorf("sha1 = %v", got)
	}
}

func TestFromJSONAndJSONQuery(t *testing.T) {
	parsed := apply(t, "from_json", `{"a":{"b":1},"list":[1,2]}`)
	m, ok := parsed.(map[string]any)
	if !ok {
		t.Fatalf("from_json result type = %T", parsed)
	}

	// jq absent: fallback only supports a single bare property name.
	origLookPath := lookPath
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	defer func() { lookPath = origLookPath }()

	if got := apply(t, "json_query", m, "a"); got == nil {
		t.Errorf("json_query fallback = %v", got)
	}

	// jq present: exercised via an injected fake process runner.
	origRunJQ := runJQ
	lookPath = func(string) (string, error) { return "/usr/bin/jq", nil }
	runJQ = func(filterExpr string, input []byte) ([]byte, error) {
		if filterExpr != ".list" {
			t.Fatalf("unexpected jq filter: %s", filterExpr)
		}
		return []byte("[1,2]\n"), nil
	}
	defer func() { runJQ = origRunJQ }()

	got := apply(t, "json_query", m, ".list")
	list, ok := got.([]any)
	if !ok || len(list) != 2 {
		t.Fatalf("json_query via jq = %#v", got)
	}
}

func TestLookupFilter(t *testing.T) {
	origHTTPGet := httpGet
	httpGet = func(url string) (string, error) {
		if url != "https://example.com/x.keys" {
			t.Fatalf("unexpected URL: %s", url)
		}
		return "ssh-ed25519 AAAA", nil
	}
	defer func() { httpGet = origHTTPGet }()

	got := apply(t, "lookup", nil, "url", "https://example.com/", "x", ".keys")
	if got != "ssh-ed25519 AAAA" {
		t.Errorf("lookup url = %v", got)
	}

	// a missing piece (nil) makes the whole lookup a no-op, not a
	// wrongly-composed request.
	got = apply(t, "lookup", nil, "url", "https://example.com/", nil, ".keys")
	if got != nil {
		t.Errorf("lookup with missing piece = %v, want nil", got)
	}

	origReadFile := readFile
	readFile = func(path string) (string, bool, error) { return "content", true, nil }
	defer func() { readFile = origReadFile }()
	got = apply(t, "lookup", nil, "file", "~/some/path")
	if got != "content" {
		t.Errorf("lookup file = %v", got)
	}
}

func TestNamesIncludesEveryBuiltin(t *testing.T) {
	names := New().Names()
	want := []string{
		"default", "toggle", "ternary", "enabled", "upper", "lower", "trim",
		"quote", "length", "concat", "join", "split", "prefix", "dirname",
		"basename", "resolve", "exists", "sha1", "from_json", "json_query", "lookup",
	}
	set := map[string]bool{}
	for _, n := range names {
		set[n] = true
	}
	for _, w := range want {
		if !set[w] {
			t.Errorf("missing built-in filter %q", w)
		}
	}
}
