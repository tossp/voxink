# VoxInk repository instructions

## Current boundary
- Module: `github.com/tossp/voxink`; `go.mod` declares Go `1.26.3` with direct dependencies on Fyne v2, coder/websocket, malgo, and x/sys.
- This repository is in the design-and-scaffold stage, not a functioning desktop voice-input application.
- Recording, ASR network calls, Windows text injection, tray support, and GUI are not implemented.
- Treat capabilities described in `README.md` as design targets unless implementation code establishes otherwise.

## Entrypoint and architecture
- The only executable entrypoint is `cmd/voxink/main.go`.
- It currently prints `VoxInk scaffold: design and project skeleton only`.
- `internal/domain` is the only internal package.
- Keep its Provider, recognition-mode, session, and event types stable and implementation-agnostic.

## Commands and verification
- Use these direct Go commands; do not infer that the local Go toolchain is available.
- `go run ./cmd/voxink`
- `go list ./...`
- `go test ./...`
- `go vet ./...`
- There are no `*_test.go` files; no unit-test entrypoint or test suite exists yet.
- No repository CI, pre-commit hook, task runner, workspace/lockfile, formatter, linter, typecheck, or code-generation configuration exists.
- `TODO.md` records past actions and is not an automation gate.

## Documentation authority
- For product, architecture, provider, and Windows-input decisions, use the formal documents directly under `docs/` as authoritative.
- `docs/research/` is traceable research evidence, not a product commitment or substitute for the formal documents.
- Before implementation, recheck time-sensitive protocols, licenses, and maintenance status against their sources.

## Repository-specific restrictions
- A license has not been selected: do not add `LICENSE` or state a license without a separate decision.
- Never commit local runtime data or credentials; `.gitignore` covers `tmp/`, databases, environment files, key/PEM files, `credentials.json`, and `secrets/`.
