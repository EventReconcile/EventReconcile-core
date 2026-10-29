# Local contributor instructions

- Keep reconciliation algorithms and public contracts in this module only.
- Do not add CLI, adapter or network dependencies.
- Preserve deterministic output and explicit `PASS`, `FAIL` and `INCONCLUSIVE` semantics.
- Treat input datasets as untrusted and keep resource limits covered by tests.
- Run `go vet ./...` and `go test -count=1 ./...` before handing off changes.
- This workspace uses a local module replacement until a released core version is authorized.
