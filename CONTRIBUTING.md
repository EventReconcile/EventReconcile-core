# Contributing

Run `go vet ./...` and `go test -count=1 ./...` before submitting changes. Keep core independent of the CLI, adapters, network services, wallets and signing. New behavior must preserve deterministic reports and explicit `PASS`, `FAIL` or `INCONCLUSIVE` outcomes.

See `AGENTS.md` for repository boundaries and the project README for the related repositories.
