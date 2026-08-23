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
// live network endpoint (docs/plans/go-rewrite.md §4.5).
var httpGet = func(url string) (string, error) {
	resp, err := http.Get(url) //nolint:gosec // target is user-authored YAML content, same trust boundary as the rest of this tool
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

func filterLookup(value any, args []any) (any, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("lookup filter requires at least one argument")
	}
	action := strings.ToLower(toStr(args[0]))
	pieces := args[1:]
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
		return httpGet(target)
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
