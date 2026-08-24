package filters

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/TacoContent/ironstate/internal/pathutil"
)

// httpGet is overridable in tests so 'lookup("url", ...)' never needs a
// live network endpoint (docs/plans/go-rewrite.md §4.5). headers is nil
// for a plain request.
var httpGet = func(url string, headers http.Header) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil) //nolint:gosec // target is user-authored YAML content, same trust boundary as the rest of this tool
	if err != nil {
		return "", err
	}
	for name, values := range headers {
		for _, v := range values {
			req.Header.Add(name, v)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// readFile is overridable in tests.
var readFile = func(path string) (string, bool, error) {
	data, err := os.ReadFile(path) //nolint:gosec // target is a user-authored YAML 'lookup("file", ...)' path, same trust boundary as the rest of this tool
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	return string(data), true, nil
}

func registerLookupFilter(r *Registry) {
	r.Register("lookup", filterLookup)
}

// extractLookupHeaders splits a trailing headers argument (a map, or a
// list of maps, of header-name -> value entries) off of a 'url' action's
// pieces, so the remaining pieces are exactly the URL text to concatenate
// - e.g. 'lookup("url", base, path, vars.api_headers)'. Every other
// action ignores this entirely (no HTTP request to attach headers to). A
// nil trailing piece is treated as "no headers" too (and still stripped
// off), not as a missing URL fragment - a headers argument computed from
// a fact/var that's legitimately absent/null (e.g. no auth token) should
// still let the request through headerless, matching how a nil/empty
// value WITHIN a header map is already skipped rather than sent empty.
// Requires at least 2 pieces so a lone URL piece that happens to be nil
// still falls through to the normal "missing piece omits the whole
// lookup" behavior below, rather than being misread as an absent
// headers argument.
func extractLookupHeaders(action string, pieces []any) ([]any, http.Header, error) {
	if action != "url" || len(pieces) < 2 {
		return pieces, nil, nil
	}
	last := pieces[len(pieces)-1]
	headers, ok, err := parseLookupHeaders(last)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return pieces, nil, nil
	}
	return pieces[:len(pieces)-1], headers, nil
}

func parseLookupHeaders(v any) (http.Header, bool, error) {
	switch entries := v.(type) {
	case nil:
		return http.Header{}, true, nil
	case []any:
		headers := make(http.Header)
		for _, raw := range entries {
			m, ok := raw.(map[string]any)
			if !ok {
				return nil, false, fmt.Errorf("lookup filter headers entry must be a map, got %T", raw)
			}
			addLookupHeaderEntries(headers, m)
		}
		return headers, true, nil
	case map[string]any:
		headers := make(http.Header)
		addLookupHeaderEntries(headers, entries)
		return headers, true, nil
	default:
		return nil, false, nil
	}
}

func addLookupHeaderEntries(headers http.Header, m map[string]any) {
	for name, raw := range m {
		if raw == nil {
			continue
		}
		if s, ok := raw.(string); ok && s == "" {
			continue
		}
		headers.Add(name, toStr(raw))
	}
}

func filterLookup(value any, args []any) (any, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("lookup filter requires at least one argument")
	}
	action := strings.ToLower(toStr(args[0]))
	pieces := args[1:]
	if len(pieces) == 0 {
		return nil, fmt.Errorf("lookup filter '%s' action requires at least one more argument", action)
	}

	pieces, headers, err := extractLookupHeaders(action, pieces)
	if err != nil {
		return nil, err
	}
	if len(pieces) == 0 {
		return nil, fmt.Errorf("lookup filter '%s' action requires at least one more argument", action)
	}

	var sb strings.Builder
	for _, piece := range pieces {
		if piece == nil {
			return nil, nil
		}
		s, ok := piece.(string)
		if ok && s == "" {
			return nil, nil
		}
		sb.WriteString(toStr(piece))
	}
	target := sb.String()

	switch action {
	case "env":
		return os.Getenv(target), nil
	case "url":
		return httpGet(target, headers)
	case "file":
		path := pathutil.ResolveUserPath(target)
		content, found, err := readFile(path)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, nil
		}
		return content, nil
	default:
		return nil, fmt.Errorf("lookup filter does not support action %q", action)
	}
}
