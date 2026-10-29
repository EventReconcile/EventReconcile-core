# EventReconcile-core

Authoritative Go domain implementation for event identity, content comparison, coverage assessment, deterministic reconciliation and bounded recovery verification.

The module is currently developed locally and is not yet a published package. The local `go.mod` replacement is intentional for pre-publication cross-repository verification.

Offline development:

```powershell
go vet ./...
go test -count=1 ./...
```

The core owns reconciliation behavior and public JSON-compatible contracts. It must not import the CLI or source adapters.
