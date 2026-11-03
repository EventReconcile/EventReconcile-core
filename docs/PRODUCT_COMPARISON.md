# Product comparison

This document is the public evidence summary for the EventReconcile repository suite. It exists so reviewers can inspect the product boundary and code-level behavior without relying on a project name or a generated roadmap.

## Audit conclusion

The repositories are public and contain a verifiable Go implementation on the `main` branch:

- [EventReconcile-core](https://github.com/EventReconcile/EventReconcile-core)
- [EventReconcile-cli](https://github.com/EventReconcile/EventReconcile-cli)
- [EventReconcile-adapters](https://github.com/EventReconcile/EventReconcile-adapters)

The implementation is pre-release. This document describes implemented behavior, not production adoption, global completeness, or a release guarantee.

## Product question

EventReconcile asks:

> Can an application or indexer show that its recorded event history agrees with a sufficiently evidenced reference history within a declared scope, including the bounded result of a recovery attempt?

It is read-only and offline-first. It does not repair datasets, sign transactions, write to Stellar, or silently fetch missing evidence from the network.

## Inputs and outputs

### Core input

The core accepts a JSON-compatible input containing:

- reference events;
- observed application or indexer events;
- declared coverage ranges;
- provenance and optional completeness metadata;
- an explicit identity mode; and
- optional before/after recovery snapshots with a claimed recovery range and checkpoint claim.

An event includes ledger sequence, transaction hash, transaction application order, event index, topics, data, and an optional source identifier. The public schemas are in [schema/](../schema/).

### Core output

The reconciler emits a machine-readable report containing:

- schema and canonicalization-version identifiers;
- overall `PASS`, `FAIL`, or `INCONCLUSIVE` status;
- assertion-level statuses;
- typed findings;
- evidence and coverage summaries;
- explicit limitations;
- optional recovery verification; and
- a deterministic digest.

The report is evidence-scoped. A matching reference and observed dataset does not prove global blockchain history completeness.

### CLI behavior

[EventReconcile-cli](https://github.com/EventReconcile/EventReconcile-cli) reads exactly one bounded JSON document and writes a new report file. It rejects oversized input, malformed JSON, input/output path collisions, and overwriting an existing output.

Exit codes are:

- `0`: PASS
- `1`: FAIL
- `2`: INCONCLUSIVE
- `3`: usage, input, or output error

### Adapter behavior

[EventReconcile-adapters](https://github.com/EventReconcile/EventReconcile-adapters) currently normalizes SoroTrail NDJSON exports into core-compatible input records. It validates record type, contract consistency, required fields, line and file limits, and preserves raw record evidence.

The adapter uses canonical `event_index` when present. It does not substitute `op_index` or an opaque source identifier for canonical event identity. When canonical identity is unavailable, the result remains source-scoped and downstream reconciliation cannot establish cross-source completeness.

## Implemented algorithms

The core implementation in [`reconcile/reconcile.go`](../reconcile/reconcile.go) performs the following operations:

1. Validate event payloads, event counts, coverage ranges, and identity mode.
2. Normalize the order of declared coverage ranges.
3. Group reference and observed records by canonical or source-scoped identity.
4. Classify exact duplicates and conflicting content within each dataset.
5. Detect missing reference identities and unexpected observed identities.
6. Compare topics and data for records sharing an identity.
7. Validate declared coverage, out-of-scope records, provenance, and coverage overlap.
8. Check the declared event ordering tuple: ledger sequence, transaction application order, then event index.
9. Sort findings into a stable presentation order.
10. Aggregate assertion statuses into the overall result.
11. Compute a deterministic report digest.

Canonical identity is currently:

`ledgerSeq:txHash:eventIndex`

Source-scoped identity is explicitly marked as insufficient for canonical cross-source conclusions.

## Recovery verification

Recovery verification compares two bounded observations against the same reference and declared coverage:

- before recovery;
- after recovery;
- claimed recovery range;
- optional checkpoint claim.

It verifies that the claimed range is covered, validates supported checkpoint claims, reconciles both snapshots, and compares residual missing identities within the claimed range.

The result is:

- `PASS` when residual missing identities decrease;
- `FAIL` when residual missing identities increase; or
- `INCONCLUSIVE` when the claim is unsupported, outside coverage, or otherwise insufficiently evidenced.

It does not certify unsupported historical ranges, prove global completeness, or prove that a recovery process is safe in all environments.

## Findings and guarantees

The implementation distinguishes:

- missing event identity;
- exact duplicate record;
- conflicting content;
- unexpected observed event;
- ordering violation;
- ordering ambiguity;
- event outside declared coverage;
- overlapping coverage ranges;
- insufficient reference coverage; and
- unverifiable provenance.

The guarantee is conditional:

> Within the supplied inputs, identity mode, declared coverage, and provenance evidence, the report deterministically classifies observed discrepancies and states when the evidence is insufficient.

It is not a guarantee that:

- the reference history is globally complete;
- Stellar retention or external indexer history is complete;
- a source export has canonical identity when it does not provide it;
- a recovery repaired every historical gap; or
- a matching dataset proves correctness outside the declared scope.

## Relationship to CursorWitness

EventReconcile and CursorWitness are separate products with different evidence boundaries.

| Concern | EventReconcile | CursorWitness |
|---|---|---|
| Primary evidence | Already-recorded reference and observed histories | Controlled consumer and lifecycle execution |
| Main question | Do persisted histories agree within supported evidence? | Does a consumer behave correctly under controlled faults and restarts? |
| Fault injection | Not performed | In scope |
| RPC replay | Not performed | Simulated interactions are in scope |
| Checkpoint/effect ordering | Evaluated as evidence when supplied | Exercised during consumer tests |
| Recovery result | Read-only before/after verification | Crash and restart behavior testing |
| Dataset repair | Not performed | Not the purpose |
| Blockchain writes/signing | Never performed | Not the purpose |

The products can interoperate through exported evidence, but EventReconcile does not duplicate the CursorWitness consumer-testing engine.

## Current limitations and follow-up

This is a pre-release implementation. Before describing it as a stable production release, the project should:

1. specify the canonicalization contract more formally;
2. expand JSON Schema validation and schema-implementation conformance tests;
3. add larger-scale and property-based determinism tests;
4. validate additional real export formats;
5. publish compatible package versions across the three repositories; and
6. complete an independent correctness and security review.

In particular, the current canonicalization version is an explicit project contract identifier, not a claim that every JSON implementation follows RFC 8785 semantics. Consumers should rely on the published version and test fixtures, not infer broader interoperability.
