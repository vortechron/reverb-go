# reverb-go

Pusher protocol 7 server in Go, a drop-in for Laravel Reverb, plus the `broadcast` package.

- Module `github.com/vortechron/reverb-go`, `go 1.26.0`, `toolchain go1.27.3` (use GOTOOLCHAIN=auto).
- Layout: `broadcast/` (public API), `goravel/` (Goravel glue; the only package that imports Goravel), `internal/{apps,config,wire,hub,server,redisbus}`, `cmd/reverb-go`, `conformance/` (Node), `examples/echo-page/`.
- Rules: functions under 50 lines, files under 400 lines, no deep nesting, wrap errors with `%w`, no debug prints, no hardcoded secrets.
- Gates before any commit: `golangci-lint run ./...`, `go vet ./...`, `go test -race ./...`, coverage 80% (`sh scripts/coverage-gate.sh 80 coverage.out`), `bash scripts/vulncheck.sh` (govulncheck v1.9.0; known exceptions go in `.govulncheck-allow` with a reason).
- Behaviour must match Laravel Reverb. When unsure, read `src/Protocols/Pusher` in Reverb (vendored in the werewolf app) and run `conformance/run-reverb.sh` against it. Write any intended difference in `conformance/RESULTS.md`.
- Security rules already decided: `socket_id` and channel names are validated before signing and on subscribe; the connection limit uses `Hub.TryAdd`; client events never update a cache channel (`PublishNoCache`).
- Local ports 6000-6099 only. Redis, if used: DB 13 and a unique key prefix. Stop every process you start.
- Do not push or commit unless asked. No attribution lines in commits.
