package reconcile

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func ev(l uint64, tx string, order uint64, data any) Event {
	return Event{LedgerSeq: l, TxHash: tx, TxApplicationOrder: order, EventIndex: 0, Topics: []any{"transfer", "A", "B"}, Data: data}
}

var cov = Coverage{Ranges: []CoverageRange{{From: 100, To: 101}}, Provenance: "captured-final-ledger-export", Completeness: "complete"}

func TestMatchingPasses(t *testing.T) {
	r, err := Reconcile(Input{Reference: []Event{ev(100, "a", 0, map[string]any{"amount": 10})}, Observed: []Event{ev(100, "a", 0, map[string]any{"amount": 10})}, Coverage: cov})
	if err != nil || r.Overall != "PASS" {
		t.Fatalf("got %#v %v", r, err)
	}
}
func TestFindings(t *testing.T) {
	r, _ := Reconcile(Input{Reference: []Event{ev(100, "a", 0, 10), ev(100, "b", 1, 5)}, Observed: []Event{ev(100, "a", 0, 99), ev(100, "c", 1, 5)}, Coverage: cov})
	kinds := map[string]bool{}
	for _, f := range r.Findings {
		kinds[f.Kind] = true
	}
	for _, k := range []string{"conflicting-content", "missing-event", "unexpected-event"} {
		if !kinds[k] {
			t.Errorf("missing %s", k)
		}
	}
}
func TestDuplicateAndOrdering(t *testing.T) {
	r, _ := Reconcile(Input{Reference: []Event{ev(100, "a", 0, 10), ev(100, "b", 1, 5)}, Observed: []Event{ev(100, "b", 1, 5), ev(100, "a", 0, 10), ev(100, "a", 0, 10)}, Coverage: cov})
	kinds := map[string]bool{}
	for _, f := range r.Findings {
		kinds[f.Kind] = true
	}
	if !kinds["exact-duplicate-record"] || !kinds["ordering-violation"] {
		t.Fatalf("got %#v", r.Findings)
	}
}

func TestReferenceDuplicatesAreClassifiedBySource(t *testing.T) {
	e := ev(100, "a", 0, 10)
	r, err := Reconcile(Input{Reference: []Event{e, e}, Observed: []Event{e}, Coverage: cov})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range r.Findings {
		if f.Kind == "exact-duplicate-record" {
			if f.Source != "reference" || f.Count != 2 {
				t.Fatalf("unexpected finding %#v", f)
			}
			return
		}
	}
	t.Fatal("reference duplicate not found")
}

func TestConflictingDuplicatesAreNotExactDuplicates(t *testing.T) {
	a := ev(100, "a", 0, 10)
	b := ev(100, "a", 0, 11)
	r, err := Reconcile(Input{Reference: []Event{a}, Observed: []Event{a, b}, Coverage: cov})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range r.Findings {
		if f.Kind == "exact-duplicate-record" {
			t.Fatal("conflicting payloads cannot be exact duplicates")
		}
	}
	found := false
	for _, f := range r.Findings {
		if f.Kind == "conflicting-content" && f.Source == "observed" && f.Count == 2 {
			found = true
		}
	}
	if !found {
		t.Fatalf("conflicting duplicate not found: %#v", r.Findings)
	}
}

func TestEqualOrderingKeysAreAmbiguousNotHashOrdered(t *testing.T) {
	a := ev(100, "z-hash", 1, 10)
	b := ev(100, "a-hash", 1, 11)
	r, err := Reconcile(Input{Reference: []Event{a, b}, Coverage: Coverage{Ranges: []CoverageRange{{From: 100, To: 100}}, Provenance: cov.Provenance, Completeness: "complete"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range r.Findings {
		if f.Kind == "ordering-ambiguity" {
			return
		}
	}
	t.Fatalf("expected ordering ambiguity, got %#v", r.Findings)
}

func TestEventsOutsideDeclaredCoverageAreExplicitlyInconclusive(t *testing.T) {
	e := ev(200, "outside", 0, 10)
	r, err := Reconcile(Input{Reference: []Event{e}, Observed: []Event{e}, Coverage: cov})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range r.Findings {
		if f.Kind == "event-outside-declared-coverage" && f.Status == "INCONCLUSIVE" {
			return
		}
	}
	t.Fatalf("expected scope finding, got %#v", r.Findings)
}
func TestInconclusiveCoverage(t *testing.T) {
	r, _ := Reconcile(Input{Reference: []Event{ev(100, "a", 0, 10)}, Observed: []Event{ev(100, "a", 0, 10)}, Coverage: Coverage{Provenance: "unknown"}})
	if r.Overall != "INCONCLUSIVE" {
		t.Fatal(r.Overall)
	}
}
func TestDeterministicDigest(t *testing.T) {
	in := Input{Reference: []Event{ev(100, "a", 0, 10)}, Observed: []Event{ev(100, "a", 0, 10)}, Coverage: cov}
	a, _ := Reconcile(in)
	b, _ := Reconcile(in)
	if a.DeterministicDigest != b.DeterministicDigest {
		t.Fatal("digest changed")
	}
}

func TestCanonicalizationSortsObjectKeys(t *testing.T) {
	a, err := Canonicalize(map[string]any{"z": 1, "a": 2})
	if err != nil {
		t.Fatal(err)
	}
	b, err := Canonicalize(map[string]any{"a": 2, "z": 1})
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatalf("canonical bytes differ: %s vs %s", a, b)
	}
	if CanonicalizationVersion == "" {
		t.Fatal("canonicalization version missing")
	}
}

func TestReportEvidenceIsDeterministicAndLimitationsAreExplicit(t *testing.T) {
	a := ev(100, "a", 0, 10)
	a.SourceID = "soro-1"
	in := Input{Reference: []Event{a}, Observed: []Event{a}, Coverage: Coverage{Ranges: []CoverageRange{{From: 100, To: 100}}, Provenance: "sorotrail-ndjson", Completeness: "unknown"}, IdentityMode: "source-scoped"}
	r1, err := Reconcile(in)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := Reconcile(in)
	if err != nil {
		t.Fatal(err)
	}
	b1, _ := json.Marshal(r1)
	b2, _ := json.Marshal(r2)
	if string(b1) != string(b2) {
		t.Fatal("report JSON is not deterministic")
	}
	if len(r1.Limitations) != 2 {
		t.Fatalf("limitations=%v", r1.Limitations)
	}
}

func TestGeneratedInputsRemainDeterministic(t *testing.T) {
	rng := rand.New(rand.NewSource(20261029))
	for caseNo := 0; caseNo < 100; caseNo++ {
		n := 1 + rng.Intn(24)
		reference := make([]Event, n)
		for i := range reference {
			reference[i] = ev(uint64(100+i), fmt.Sprintf("tx-%03d", i), uint64(i), map[string]any{"amount": i, "case": caseNo, "enabled": i%2 == 0})
		}
		observed := append([]Event(nil), reference...)
		if caseNo%3 == 0 {
			observed[len(observed)-1].Data = map[string]any{"amount": 999, "case": caseNo, "enabled": false}
		}
		if caseNo%5 == 0 {
			observed = observed[:len(observed)-1]
		}
		in := Input{Reference: reference, Observed: observed, Coverage: Coverage{Ranges: []CoverageRange{{From: 100, To: 200}}, Provenance: "generated-test", Completeness: "complete"}}
		first, err := Reconcile(in)
		if err != nil {
			t.Fatalf("case %d: %v", caseNo, err)
		}
		firstBytes, err := json.Marshal(first)
		if err != nil {
			t.Fatal(err)
		}
		for repeat := 0; repeat < 4; repeat++ {
			next, err := Reconcile(in)
			if err != nil {
				t.Fatalf("case %d repeat %d: %v", caseNo, repeat, err)
			}
			nextBytes, _ := json.Marshal(next)
			if first.DeterministicDigest != next.DeterministicDigest || string(firstBytes) != string(nextBytes) {
				t.Fatalf("case %d repeat %d was not deterministic", caseNo, repeat)
			}
		}
	}
}

func TestCanonicalizationIsStableForGeneratedMapInsertionOrders(t *testing.T) {
	for i := 0; i < 100; i++ {
		left := map[string]any{"z": i, "a": map[string]any{"nested": i * 2, "first": true}, "m": []any{i, i + 1}}
		right := map[string]any{"m": []any{i, i + 1}, "a": map[string]any{"first": true, "nested": i * 2}, "z": i}
		lb, err := Canonicalize(left)
		if err != nil {
			t.Fatal(err)
		}
		rb, err := Canonicalize(right)
		if err != nil {
			t.Fatal(err)
		}
		if string(lb) != string(rb) {
			t.Fatalf("iteration %d changed canonical bytes", i)
		}
	}
}

func TestVersionedSchemasMatchPublicContractMetadata(t *testing.T) {
	readSchema := func(name string) map[string]any {
		t.Helper()
		b, err := os.ReadFile(filepath.Join("..", "schema", name))
		if err != nil {
			t.Fatal(err)
		}
		var schema map[string]any
		if err := json.Unmarshal(b, &schema); err != nil {
			t.Fatal(err)
		}
		return schema
	}
	eventSchema := readSchema("event.schema.json")
	coverageSchema := readSchema("coverage.schema.json")
	reportSchema := readSchema("report.schema.json")
	if eventSchema["$id"] != "https://eventreconcile.dev/schema/0.1/event.json" || coverageSchema["$id"] != "https://eventreconcile.dev/schema/0.1/coverage.json" || reportSchema["$id"] != "https://eventreconcile.dev/schema/0.1/report.json" {
		t.Fatal("schema IDs are not stable")
	}
	required := func(schema map[string]any, name string) map[string]bool {
		out := map[string]bool{}
		for _, value := range schema["required"].([]any) {
			out[value.(string)] = true
		}
		return out
	}
	for _, field := range []string{"ledgerSeq", "txHash", "txApplicationOrder", "eventIndex", "topics"} {
		if !required(eventSchema, "event")[field] {
			t.Fatalf("event schema lost required field %s", field)
		}
	}
	for _, field := range []string{"ranges", "provenance", "completeness"} {
		if !required(coverageSchema, "coverage")[field] {
			t.Fatalf("coverage schema lost required field %s", field)
		}
	}
	properties := reportSchema["properties"].(map[string]any)
	if properties["schemaVersion"].(map[string]any)["const"] != SchemaVersion {
		t.Fatalf("report schema version mismatch: %v", properties["schemaVersion"])
	}
}

func TestRecoveryVerification(t *testing.T) {
	in := RecoveryInput{Reference: []Event{ev(100, "a", 0, 10), ev(101, "b", 1, 5)}, Before: []Event{ev(100, "a", 0, 10)}, After: []Event{ev(100, "a", 0, 10), ev(101, "b", 1, 5)}, Coverage: cov, ClaimedFrom: 100, ClaimedTo: 101}
	r, err := VerifyRecovery(in)
	if err != nil || r.Status != "PASS" || r.AfterMissingCount != 0 {
		t.Fatalf("got %#v %v", r, err)
	}
}

func TestReconcileRejectsExcessiveEventCount(t *testing.T) {
	events := make([]Event, 10001)
	for i := range events {
		events[i] = ev(uint64(i+1), fmt.Sprintf("tx-%d", i), 0, i)
	}
	if _, err := Reconcile(Input{Reference: events, Observed: events, Coverage: cov}); err == nil || !strings.Contains(err.Error(), "event limit") {
		t.Fatalf("expected event limit error, got %v", err)
	}
}

func TestRecoveryClaimOutsideCoverageIsInconclusive(t *testing.T) {
	r, err := VerifyRecovery(RecoveryInput{Reference: []Event{ev(100, "a", 0, 10)}, Before: []Event{}, After: []Event{ev(100, "a", 0, 10)}, Coverage: cov, ClaimedFrom: 100, ClaimedTo: 200})
	if err != nil || r.Status != "INCONCLUSIVE" || r.ClaimCovered || r.Limitation == "" {
		t.Fatalf("got %#v %v", r, err)
	}
}

func TestRecoveryResidualsAreBoundedToClaimedRange(t *testing.T) {
	reference := []Event{ev(100, "a", 0, 10), ev(200, "b", 1, 20)}
	r, err := VerifyRecovery(RecoveryInput{Reference: reference, Before: []Event{}, After: []Event{reference[0]}, Coverage: Coverage{Ranges: []CoverageRange{{From: 100, To: 200}}, Provenance: cov.Provenance, Completeness: "complete"}, ClaimedFrom: 100, ClaimedTo: 100})
	if err != nil || r.Status != "PASS" || len(r.ResidualMissing) != 0 || r.BeforeMissingCount != 1 {
		t.Fatalf("got %#v %v", r, err)
	}
}

func TestRecoveryRegressionWithinClaimFails(t *testing.T) {
	e := ev(100, "a", 0, 10)
	r, err := VerifyRecovery(RecoveryInput{Reference: []Event{e}, Before: []Event{e}, After: []Event{}, Coverage: cov, ClaimedFrom: 100, ClaimedTo: 100})
	if err != nil || r.Status != "FAIL" || r.AfterMissingCount != 1 {
		t.Fatalf("got %#v %v", r, err)
	}
}

func TestRecoveryWorkLimitIsExplicitlyInconclusive(t *testing.T) {
	r, err := VerifyRecovery(RecoveryInput{Reference: []Event{}, Coverage: Coverage{Ranges: []CoverageRange{{From: 1, To: MaxRecoveryLedgers + 1}}, Provenance: cov.Provenance, Completeness: "complete"}, ClaimedFrom: 1, ClaimedTo: MaxRecoveryLedgers + 1})
	if err != nil || r.Status != "INCONCLUSIVE" || r.Limitation == "" || len(r.Audit) == 0 {
		t.Fatalf("got %#v %v", r, err)
	}
}

func TestRecoveryEarlyOutcomesContainAuditEvidence(t *testing.T) {
	cases := []RecoveryInput{
		{Reference: []Event{ev(100, "a", 0, 10)}, Coverage: cov, ClaimedFrom: 100, ClaimedTo: 200},
		{Reference: []Event{ev(100, "a", 0, 10)}, Coverage: cov, ClaimedFrom: 100, ClaimedTo: 100, Checkpoint: &CheckpointClaim{Ledger: 100, Supported: false}},
		{Reference: []Event{ev(100, "a", 0, 10)}, Coverage: cov, ClaimedFrom: 100, ClaimedTo: 100, Checkpoint: &CheckpointClaim{Ledger: 101, Supported: true}},
	}
	for i, input := range cases {
		r, err := VerifyRecovery(input)
		if err != nil || len(r.Audit) == 0 {
			t.Fatalf("case %d missing audit evidence: %#v %v", i, r, err)
		}
	}
}

func TestReportIncludesRecoveryAssertionAndEvidence(t *testing.T) {
	recovery := &RecoveryInput{Reference: []Event{ev(100, "a", 0, 10), ev(101, "b", 1, 5)}, Before: []Event{ev(100, "a", 0, 10)}, After: []Event{ev(100, "a", 0, 10), ev(101, "b", 1, 5)}, Coverage: cov, ClaimedFrom: 100, ClaimedTo: 101}
	r, err := Reconcile(Input{Reference: recovery.Reference, Observed: recovery.After, Coverage: cov, Recovery: recovery})
	if err != nil {
		t.Fatal(err)
	}
	if r.Recovery == nil || r.Recovery.Status != "PASS" {
		t.Fatalf("recovery=%#v", r.Recovery)
	}
	found := false
	for _, a := range r.Assertions {
		if a.Code == "recovery-verification" && a.Status == "PASS" {
			found = true
		}
	}
	if !found {
		t.Fatalf("assertions=%#v", r.Assertions)
	}
}

func TestSupportedCheckpointClaimPasses(t *testing.T) {
	e := ev(100, "a", 0, 10)
	r, err := VerifyRecovery(RecoveryInput{Reference: []Event{e}, Before: []Event{}, After: []Event{e}, Coverage: cov, ClaimedFrom: 100, ClaimedTo: 100, Checkpoint: &CheckpointClaim{Ledger: 100, EventID: "100:a:0", Supported: true}})
	if err != nil || r.Status != "PASS" || r.CheckpointStatus != "PASS" || len(r.Audit) != 3 {
		t.Fatalf("got %#v %v", r, err)
	}
}

func TestUnsupportedCheckpointClaimIsInconclusive(t *testing.T) {
	r, err := VerifyRecovery(RecoveryInput{Coverage: cov, ClaimedFrom: 100, ClaimedTo: 100, Checkpoint: &CheckpointClaim{Ledger: 100, Supported: false}})
	if err != nil || r.Status != "INCONCLUSIVE" || r.CheckpointStatus != "unsupported" {
		t.Fatalf("got %#v %v", r, err)
	}
}

func TestInvalidCheckpointClaimFails(t *testing.T) {
	r, err := VerifyRecovery(RecoveryInput{Reference: []Event{ev(100, "a", 0, 10)}, Coverage: cov, ClaimedFrom: 100, ClaimedTo: 100, Checkpoint: &CheckpointClaim{Ledger: 100, EventID: "100:missing:0", Supported: true}})
	if err != nil || r.Status != "FAIL" || r.CheckpointStatus != "invalid" {
		t.Fatalf("got %#v %v", r, err)
	}
}

func TestMalformedInputRejected(t *testing.T) {
	_, err := Reconcile(Input{Reference: []Event{{LedgerSeq: 0}}, Coverage: cov})
	if err == nil {
		t.Fatal("expected malformed event error")
	}
}

func TestOversizedPayloadRejected(t *testing.T) {
	_, err := Reconcile(Input{Reference: []Event{ev(100, "large", 0, strings.Repeat("x", 1024*1024))}, Coverage: cov})
	if err == nil {
		t.Fatal("expected oversized payload error")
	}
}

func TestCoverageRangesNormalizeBeforeOverlapCheck(t *testing.T) {
	r, err := Reconcile(Input{Reference: []Event{ev(100, "a", 0, 10)}, Observed: []Event{ev(100, "a", 0, 10)}, Coverage: Coverage{Ranges: []CoverageRange{{From: 101, To: 101}, {From: 100, To: 100}}, Provenance: cov.Provenance, Completeness: "complete"}})
	if err != nil || r.Overall != "PASS" {
		t.Fatalf("got %#v %v", r, err)
	}
	r, err = Reconcile(Input{Coverage: Coverage{Ranges: []CoverageRange{{From: 101, To: 102}, {From: 100, To: 101}}, Provenance: cov.Provenance, Completeness: "complete"}})
	if err != nil || r.Overall != "FAIL" {
		t.Fatalf("expected overlap failure, got %#v %v", r, err)
	}
}

func TestSourceScopedIdentityIsExplicitlyInconclusive(t *testing.T) {
	a := ev(100, "tx-a", 0, 10)
	a.SourceID = "soro-1"
	r, err := Reconcile(Input{Reference: []Event{a}, Observed: []Event{a}, Coverage: cov, IdentityMode: "source-scoped"})
	if err != nil || r.Overall != "INCONCLUSIVE" || r.Assertions[1].Code != "identity-sufficiency" {
		t.Fatalf("got %#v %v", r, err)
	}
}

func TestSourceScopedIdentityRequiresSourceID(t *testing.T) {
	_, err := Reconcile(Input{Reference: []Event{ev(100, "tx-a", 0, 10)}, Coverage: cov, IdentityMode: "source-scoped"})
	if err == nil {
		t.Fatal("expected source id error")
	}
}

func TestMissingOutsideCoverageIsInconclusiveWithEvidence(t *testing.T) {
	r, err := Reconcile(Input{Reference: []Event{ev(200, "outside", 0, 10)}, Coverage: cov})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range r.Findings {
		if f.Kind == "missing-event" {
			if f.Status != "INCONCLUSIVE" || f.Evidence != "outside-declared-coverage" {
				t.Fatalf("unexpected finding %#v", f)
			}
			return
		}
	}
	t.Fatal("missing-event finding not found")
}

func TestUnexpectedWithinAndOutsideCoverageAreDistinguished(t *testing.T) {
	r, err := Reconcile(Input{Observed: []Event{ev(100, "inside", 0, 10), ev(200, "outside", 1, 11)}, Coverage: cov})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]Finding{}
	for _, f := range r.Findings {
		if f.Kind == "unexpected-event" {
			seen[f.ID] = f
		}
	}
	if seen["100:inside:0"].Status != "FAIL" || seen["100:inside:0"].Evidence != "covered-reference-range" {
		t.Fatalf("inside=%#v", seen["100:inside:0"])
	}
	if seen["200:outside:0"].Status != "INCONCLUSIVE" || seen["200:outside:0"].Evidence != "outside-declared-coverage" {
		t.Fatalf("outside=%#v", seen["200:outside:0"])
	}
}

func TestSharedM0ConformanceFixtures(t *testing.T) {
	type fixture struct {
		Name             string   `json:"name"`
		Input            Input    `json:"input"`
		ExpectedOverall  string   `json:"expectedOverall"`
		ExpectedFindings []string `json:"expectedFindings"`
	}
	b, err := os.ReadFile(filepath.Join("..", "conformance", "cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []fixture
	if err := json.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			r, err := Reconcile(tc.Input)
			if err != nil {
				t.Fatal(err)
			}
			if r.Overall != tc.ExpectedOverall {
				t.Fatalf("overall=%s want %s", r.Overall, tc.ExpectedOverall)
			}
			got := []string{}
			for _, f := range r.Findings {
				got = append(got, f.Kind)
			}
			if len(got) != len(tc.ExpectedFindings) {
				t.Fatalf("findings=%v want %v", got, tc.ExpectedFindings)
			}
			for i := range got {
				if got[i] != tc.ExpectedFindings[i] {
					t.Fatalf("findings=%v want %v", got, tc.ExpectedFindings)
				}
			}
		})
	}
}

func TestGoldenPassReportSummary(t *testing.T) {
	type golden struct {
		SchemaVersion           string   `json:"schemaVersion"`
		CanonicalizationVersion string   `json:"canonicalizationVersion"`
		Overall                 string   `json:"overall"`
		AssertionStatuses       []string `json:"assertionStatuses"`
		FindingKinds            []string `json:"findingKinds"`
	}
	b, err := os.ReadFile(filepath.Join("..", "conformance", "golden-pass.json"))
	if err != nil {
		t.Fatal(err)
	}
	var want golden
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	r, err := Reconcile(Input{Reference: []Event{ev(100, "a", 0, 10)}, Observed: []Event{ev(100, "a", 0, 10)}, Coverage: cov})
	if err != nil {
		t.Fatal(err)
	}
	got := golden{SchemaVersion: r.SchemaVersion, CanonicalizationVersion: r.CanonicalizationVersion, Overall: r.Overall, AssertionStatuses: []string{r.Assertions[0].Status, r.Assertions[1].Status, r.Assertions[2].Status}, FindingKinds: []string{}}
	for _, f := range r.Findings {
		got.FindingKinds = append(got.FindingKinds, f.Kind)
	}
	gb, _ := json.Marshal(got)
	wb, _ := json.Marshal(want)
	if string(gb) != string(wb) {
		t.Fatalf("golden mismatch: got %s want %s", gb, wb)
	}
}
