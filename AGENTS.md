# ironstate Agent Guide

## Layout

- This is a Go 1.27.3 repository with three independent modules: the root CLI, public plugin SDK in `sdk/`, and sample plugin in `examples/ironstate-handler-hosts/`. Run `go test ./...` and `go mod tidy` from the module being changed; root tests do not include the nested modules.
- The executable entrypoint is `cmd/ironstate/main.go`; Cobra wiring stays thin in `internal/cli/`. The apply flow is: CLI/config -> `internal/packages` hierarchy loading -> `internal/tasks` flattening -> `internal/engine` dispatch -> `internal/handlers`.
- `sdk/proto/*.pb.go` and `sdk/proto/*_grpc.pb.go` are generated from `sdk/proto/handler.proto`; do not hand-edit generated files.

## Expectations

- All new code must include passing tests and adhere to the existing module structure and coding standards.
- All features must include documentation in docs/ corresponding to the implemented functionality. Update existing documentation as needed to reflect changes.
- All tests must pass successfully, even if they are not directly related to the changes being made.
- Code changes should not break existing functionality and must be backward compatible unless explicitly stated otherwise.

## Development Commands

- Build or test the root module: `go build ./...` and `go test ./... -coverprofile=coverage.out`.
- Run a focused root test with `go test ./internal/<package> -run '<TestName>' -count=1`; use `go test ./internal/pluginhost -run TestSampleHostsPluginRoundTrip -count=1` for the sample plugin host contract.
- Test the SDK or sample plugin from its directory: `go test ./...` in `sdk` or `examples/ironstate-handler-hosts`.
- Use `task tidy` after dependency changes. CI checks all three modules with `task tidy:check`; do not only tidy the root module.
- The closest routine CI subset is `task fmt:check`, `task lint`, `task vet`, and `task test`. `task ci` additionally needs `xget`, Docker for `task test:ssh`, CodeQL tooling, and a C compiler for the race test. `task lint-linux` runs the Linux-targeted linter from Windows.
- `task fuzz-smoke` is the focused fuzz gate; `task build-snapshot` runs GoReleaser. Snapshot/release builds set `IRONSTATE_SKIP_ANDROID=true` unless an Android NDK is available.

## Change Gotchas

- New built-in playbook modules must be added to both `internal/handlers/handlers.go`: `All()` for dispatch and `AllModuleNames` for YAML task recognition. Also update `internal/tasks/realfixture_test.go`'s `realModuleNames` and, when fields are user-facing, `ironstate.schema.json`.
- `ironstate` is dry-run by default. Real mutations require `--apply`; do not use `--apply` against the maintained sample playbook or inventory for routine verification.
- Playbook `.env` and `.secrets` are loaded from the process current directory, not from the selected playbook directory. `.secrets` values are registered for redaction; keep credentials out of committed playbooks.
- OS-specific behavior uses Go filename suffixes (for example `*_windows.go` and `*_other.go`). CI runs the full suite on Windows and Ubuntu, so keep both implementations and their tests valid.
- Release packaging embeds Windows resources through `go tool go-winres` and generates `dist/ironstate.1` from `docs/_man/ironstate.md` via Pandoc. Use `task release:dry-run` to render release artifacts without publishing.
