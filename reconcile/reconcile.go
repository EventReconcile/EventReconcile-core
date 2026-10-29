package reconcile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

const SchemaVersion = "0.1-m0"
const CanonicalizationVersion = "json-encoding-json-v1"
const MaxRecoveryLedgers uint64 = 1_000_000

type Event struct {
	LedgerSeq          uint64 `json:"ledgerSeq"`
	TxHash             string `json:"txHash"`
	TxApplicationOrder uint64 `json:"txApplicationOrder"`
	EventIndex         uint64 `json:"eventIndex"`
	Topics             []any  `json:"topics"`
	Data               any    `json:"data"`
	SourceID           string `json:"sourceId,omitempty"`
}

type CoverageRange struct {
	From uint64 `json:"from"`
	To   uint64 `json:"to"`
}
type Coverage struct {
	Ranges       []CoverageRange `json:"ranges"`
	Provenance   string          `json:"provenance"`
	Completeness string          `json:"completeness,omitempty"`
}
type Input struct {
	Reference    []Event        `json:"reference"`
	Observed     []Event        `json:"observed"`
	Coverage     Coverage       `json:"coverage"`
	IdentityMode string         `json:"identityMode,omitempty"`
	Recovery     *RecoveryInput `json:"recovery,omitempty"`
}

type RecoveryInput struct {
	Reference    []Event          `json:"reference"`
	Before       []Event          `json:"before"`
	After        []Event          `json:"after"`
	Coverage     Coverage         `json:"coverage"`
	ClaimedFrom  uint64           `json:"claimedFrom"`
	ClaimedTo    uint64           `json:"claimedTo"`
	IdentityMode string           `json:"identityMode,omitempty"`
	Checkpoint   *CheckpointClaim `json:"checkpoint,omitempty"`
}
type CheckpointClaim struct {
	Ledger    uint64 `json:"ledger"`
	EventID   string `json:"eventId,omitempty"`
	Supported bool   `json:"supported"`
}
type RecoveryAuditCheck struct {
	Code    string `json:"code"`
	Status  string `json:"status"`
	Details any    `json:"details,omitempty"`
}
type RecoveryResult struct {
	Status             string               `json:"status"`
	BeforeMissingCount int                  `json:"beforeMissingCount"`
	AfterMissingCount  int                  `json:"afterMissingCount"`
	ResidualMissing    []string             `json:"residualMissing"`
	BeforeResidual     []string             `json:"beforeResidual"`
	ClaimedFrom        uint64               `json:"claimedFrom"`
	ClaimedTo          uint64               `json:"claimedTo"`
	ClaimCovered       bool                 `json:"claimCovered"`
	Limitation         string               `json:"limitation,omitempty"`
	CheckpointStatus   string               `json:"checkpointStatus,omitempty"`
	Audit              []RecoveryAuditCheck `json:"audit"`
}

type Finding struct {
	Kind     string `json:"kind"`
	ID       string `json:"id,omitempty"`
	Source   string `json:"source,omitempty"`
	Status   string `json:"status"`
	Count    int    `json:"count,omitempty"`
	Evidence string `json:"evidence,omitempty"`
}
type Assertion struct {
	Code    string `json:"code"`
	Status  string `json:"status"`
	Details any    `json:"details,omitempty"`
}
type EvidenceSummary struct {
	IdentityMode   string   `json:"identityMode"`
	ReferenceCount int      `json:"referenceCount"`
	ObservedCount  int      `json:"observedCount"`
	Coverage       Coverage `json:"coverage"`
	Normalization  string   `json:"normalization"`
}
type Report struct {
	SchemaVersion           string          `json:"schemaVersion"`
	CanonicalizationVersion string          `json:"canonicalizationVersion"`
	Overall                 string          `json:"overall"`
	Assertions              []Assertion     `json:"assertions"`
	Findings                []Finding       `json:"findings"`
	Evidence                EvidenceSummary `json:"evidence"`
	Limitations             []string        `json:"limitations"`
	Recovery                *RecoveryResult `json:"recovery,omitempty"`
	DeterministicDigest     string          `json:"deterministicDigest"`
}

func eventID(mode string, e Event) string {
	if mode == "source-scoped" {
		return "source:" + e.SourceID
	}
	return fmt.Sprintf("%d:%s:%d", e.LedgerSeq, e.TxHash, e.EventIndex)
}
func covered(c Coverage, ledger uint64) bool {
	for _, r := range c.Ranges {
		if ledger >= r.From && ledger <= r.To {
			return true
		}
	}
	return false
}
func Canonicalize(v any) ([]byte, error) { return json.Marshal(v) }
func digest(v any) (string, error) {
	b, err := Canonicalize(v)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}
func ContentDigest(v any) (string, error) { return digest(v) }
func same(a, b any) bool {
	da, ea := digest(a)
	db, eb := digest(b)
	return ea == nil && eb == nil && da == db
}
func eventContentDigest(e Event) string {
	d, _ := digest(struct {
		Topics any `json:"topics"`
		Data   any `json:"data"`
	}{e.Topics, e.Data})
	return d
}
func groupEvents(mode string, events []Event) map[string][]Event {
	groups := map[string][]Event{}
	for _, e := range events {
		id := eventID(mode, e)
		groups[id] = append(groups[id], e)
	}
	return groups
}
func less(a, b Event) bool {
	if a.LedgerSeq != b.LedgerSeq {
		return a.LedgerSeq < b.LedgerSeq
	}
	if a.TxApplicationOrder != b.TxApplicationOrder {
		return a.TxApplicationOrder < b.TxApplicationOrder
	}
	return a.EventIndex < b.EventIndex
}
func sameOrder(a, b Event) bool {
	return a.LedgerSeq == b.LedgerSeq && a.TxApplicationOrder == b.TxApplicationOrder && a.EventIndex == b.EventIndex
}

func validateEvents(name string, events []Event) error {
	if len(events) > 10000 {
		return fmt.Errorf("%s exceeds event limit", name)
	}
	for _, e := range events {
		if e.LedgerSeq == 0 || e.TxHash == "" || e.Topics == nil {
			return fmt.Errorf("%s contains invalid event", name)
		}
		b, err := json.Marshal(e)
		if err != nil || len(b) > 1024*1024 {
			return fmt.Errorf("%s event exceeds payload limit", name)
		}
	}
	return nil
}
func validateCoverage(c Coverage) error {
	if c.Completeness != "" && c.Completeness != "complete" && c.Completeness != "partial" && c.Completeness != "unknown" {
		return fmt.Errorf("invalid coverage completeness %q", c.Completeness)
	}
	for _, r := range c.Ranges {
		if r.From == 0 || r.From > r.To {
			return fmt.Errorf("invalid coverage range")
		}
	}
	return nil
}

func normalizeCoverage(c Coverage) Coverage {
	ranges := append([]CoverageRange(nil), c.Ranges...)
	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].From != ranges[j].From {
			return ranges[i].From < ranges[j].From
		}
		return ranges[i].To < ranges[j].To
	})
	c.Ranges = ranges
	return c
}

func Reconcile(in Input) (Report, error) {
	if in.IdentityMode == "" {
		in.IdentityMode = "canonical"
	}
	if in.IdentityMode != "canonical" && in.IdentityMode != "source-scoped" {
		return Report{}, fmt.Errorf("unsupported identity mode %q", in.IdentityMode)
	}
	if err := validateEvents("reference", in.Reference); err != nil {
		return Report{}, err
	}
	if err := validateEvents("observed", in.Observed); err != nil {
		return Report{}, err
	}
	if err := validateCoverage(in.Coverage); err != nil {
		return Report{}, err
	}
	in.Coverage = normalizeCoverage(in.Coverage)
	if in.IdentityMode == "source-scoped" {
		all := append(append([]Event{}, in.Reference...), in.Observed...)
		for _, e := range all {
			if e.SourceID == "" {
				return Report{}, fmt.Errorf("source-scoped event is missing sourceId")
			}
		}
	}
	refGroups := groupEvents(in.IdentityMode, in.Reference)
	obsGroups := groupEvents(in.IdentityMode, in.Observed)
	ref := map[string]Event{}
	obs := map[string]Event{}
	for id, group := range refGroups {
		ref[id] = group[0]
	}
	for id, group := range obsGroups {
		obs[id] = group[0]
	}
	findings := []Finding{}
	for source, groups := range map[string]map[string][]Event{"reference": refGroups, "observed": obsGroups} {
		for id, group := range groups {
			if len(group) < 2 {
				continue
			}
			digests := map[string]bool{}
			for _, e := range group {
				digests[eventContentDigest(e)] = true
			}
			if len(digests) == 1 {
				findings = append(findings, Finding{Kind: "exact-duplicate-record", ID: id, Source: source, Status: "FAIL", Count: len(group)})
			} else {
				findings = append(findings, Finding{Kind: "conflicting-content", ID: id, Source: source, Status: "FAIL", Count: len(group)})
			}
		}
	}
	for id, r := range ref {
		o, ok := obs[id]
		if !ok {
			status := "INCONCLUSIVE"
			evidence := "outside-declared-coverage"
			if covered(in.Coverage, r.LedgerSeq) {
				status = "FAIL"
				evidence = "covered-reference-range"
			}
			findings = append(findings, Finding{Kind: "missing-event", ID: id, Status: status, Evidence: evidence})
		} else if !same(r.Topics, o.Topics) || !same(r.Data, o.Data) {
			findings = append(findings, Finding{Kind: "conflicting-content", ID: id, Status: "FAIL"})
		}
	}
	for id, o := range obs {
		if _, ok := ref[id]; !ok {
			status := "INCONCLUSIVE"
			evidence := "outside-declared-coverage"
			if covered(in.Coverage, o.LedgerSeq) {
				status = "FAIL"
				evidence = "covered-reference-range"
			}
			findings = append(findings, Finding{Kind: "unexpected-event", ID: id, Status: status, Evidence: evidence})
		}
	}
	if len(in.Coverage.Ranges) > 0 {
		for source, events := range map[string][]Event{"reference": in.Reference, "observed": in.Observed} {
			for _, e := range events {
				if !covered(in.Coverage, e.LedgerSeq) {
					findings = append(findings, Finding{Kind: "event-outside-declared-coverage", ID: eventID(in.IdentityMode, e), Source: source, Status: "INCONCLUSIVE", Evidence: "outside-declared-coverage"})
				}
			}
		}
	}
	for name, events := range map[string][]Event{"reference": in.Reference, "observed": in.Observed} {
		for i := 1; i < len(events); i++ {
			if !less(events[i-1], events[i]) {
				kind := "ordering-violation"
				if sameOrder(events[i-1], events[i]) && eventID(in.IdentityMode, events[i-1]) != eventID(in.IdentityMode, events[i]) {
					kind = "ordering-ambiguity"
				}
				findings = append(findings, Finding{Kind: kind, Source: name, ID: eventID(in.IdentityMode, events[i]), Status: "FAIL"})
			}
		}
	}
	for i := 1; i < len(in.Coverage.Ranges); i++ {
		if in.Coverage.Ranges[i].From <= in.Coverage.Ranges[i-1].To {
			findings = append(findings, Finding{Kind: "overlapping-coverage-ranges", Status: "FAIL", Count: 1})
			break
		}
	}
	if in.Coverage.Provenance == "unknown" || in.Coverage.Provenance == "unsupported" {
		findings = append(findings, Finding{Kind: "unverifiable-provenance", Status: "INCONCLUSIVE"})
	}
	if len(in.Coverage.Ranges) == 0 || in.Coverage.Completeness != "complete" {
		evidence := "coverage-completeness-undeclared"
		if in.Coverage.Completeness == "partial" {
			evidence = "partial-reference-coverage"
		}
		if in.Coverage.Completeness == "unknown" {
			evidence = "unknown-reference-coverage"
		}
		findings = append(findings, Finding{Kind: "insufficient-reference-coverage", Status: "INCONCLUSIVE", Evidence: evidence})
	}
	SortFindings(findings)
	status := "PASS"
	for _, f := range findings {
		if f.Status == "FAIL" {
			status = "FAIL"
			break
		}
		if f.Status == "INCONCLUSIVE" {
			status = "INCONCLUSIVE"
		}
	}
	coverageStatus := "PASS"
	if len(in.Coverage.Ranges) == 0 || in.Coverage.Completeness != "complete" || in.Coverage.Provenance == "" || in.Coverage.Provenance == "unknown" || in.Coverage.Provenance == "unsupported" {
		coverageStatus = "INCONCLUSIVE"
	}
	identityStatus := "PASS"
	if in.IdentityMode == "source-scoped" {
		identityStatus = "INCONCLUSIVE"
	}
	assertions := []Assertion{{Code: "reference-coverage", Status: coverageStatus, Details: in.Coverage}, {Code: "identity-sufficiency", Status: identityStatus, Details: map[string]string{"mode": in.IdentityMode}}, {Code: "event-reconciliation", Status: status, Details: map[string]int{"findingCount": len(findings)}}}
	var recoveryResult *RecoveryResult
	if in.Recovery != nil {
		rr, recoveryErr := VerifyRecovery(*in.Recovery)
		if recoveryErr != nil {
			return Report{}, recoveryErr
		}
		recoveryResult = &rr
		assertions = append(assertions, Assertion{Code: "recovery-verification", Status: rr.Status, Details: rr})
	}
	limitations := []string{}
	if in.IdentityMode == "source-scoped" {
		limitations = append(limitations, "canonical cross-source event identity is unavailable")
	}
	if in.Coverage.Completeness != "complete" {
		limitations = append(limitations, "reference completeness is not established")
	}
	if in.Coverage.Provenance == "" || in.Coverage.Provenance == "unknown" || in.Coverage.Provenance == "unsupported" {
		limitations = append(limitations, "reference provenance is not independently verifiable")
	}
	for _, f := range findings {
		if f.Kind == "event-outside-declared-coverage" {
			limitations = append(limitations, "some records fall outside declared coverage")
			break
		}
	}
	if recoveryResult != nil && recoveryResult.Limitation != "" {
		limitations = append(limitations, recoveryResult.Limitation)
	}
	evidence := EvidenceSummary{IdentityMode: in.IdentityMode, ReferenceCount: len(in.Reference), ObservedCount: len(in.Observed), Coverage: in.Coverage, Normalization: CanonicalizationVersion}
	base := struct {
		A []Assertion     `json:"assertions"`
		F []Finding       `json:"findings"`
		E EvidenceSummary `json:"evidence"`
		L []string        `json:"limitations"`
		R *RecoveryResult `json:"recovery,omitempty"`
	}{assertions, findings, evidence, limitations, recoveryResult}
	d, err := digest(base)
	if err != nil {
		return Report{}, err
	}
	return Report{SchemaVersion: SchemaVersion, CanonicalizationVersion: CanonicalizationVersion, Overall: statusOf(assertions), Assertions: assertions, Findings: findings, Evidence: evidence, Limitations: limitations, Recovery: recoveryResult, DeterministicDigest: d}, nil
}

func statusOf(assertions []Assertion) string {
	status := "PASS"
	for _, a := range assertions {
		if a.Status == "FAIL" {
			return "FAIL"
		}
		if a.Status == "INCONCLUSIVE" {
			status = "INCONCLUSIVE"
		}
	}
	return status
}

// VerifyRecovery compares bounded before/after observations. It never certifies
// history outside the supplied reference and coverage evidence.
func VerifyRecovery(in RecoveryInput) (RecoveryResult, error) {
	if in.ClaimedFrom == 0 || in.ClaimedFrom > in.ClaimedTo {
		return RecoveryResult{Status: "FAIL", ClaimedFrom: in.ClaimedFrom, ClaimedTo: in.ClaimedTo}, fmt.Errorf("invalid claimed recovery range")
	}
	if in.ClaimedTo-in.ClaimedFrom+1 > MaxRecoveryLedgers {
		return RecoveryResult{Status: "INCONCLUSIVE", ClaimedFrom: in.ClaimedFrom, ClaimedTo: in.ClaimedTo, ClaimCovered: false, Limitation: "claimed recovery range exceeds recovery work limit", Audit: []RecoveryAuditCheck{{Code: "claim-coverage", Status: "INCONCLUSIVE", Details: "recovery-work-limit"}}}, nil
	}
	claimCovered := true
	for ledger := in.ClaimedFrom; ledger <= in.ClaimedTo; ledger++ {
		if !covered(in.Coverage, ledger) {
			claimCovered = false
			break
		}
		if ledger == ^uint64(0) {
			break
		}
	}
	if !claimCovered {
		return RecoveryResult{Status: "INCONCLUSIVE", ClaimedFrom: in.ClaimedFrom, ClaimedTo: in.ClaimedTo, ClaimCovered: false, Limitation: "claimed recovery range exceeds declared coverage", Audit: []RecoveryAuditCheck{{Code: "claim-coverage", Status: "INCONCLUSIVE", Details: "declared-coverage"}}}, nil
	}
	checkpointStatus := "not-claimed"
	if in.Checkpoint != nil {
		if !in.Checkpoint.Supported {
			return RecoveryResult{Status: "INCONCLUSIVE", ClaimedFrom: in.ClaimedFrom, ClaimedTo: in.ClaimedTo, ClaimCovered: true, CheckpointStatus: "unsupported", Limitation: "checkpoint claim is unsupported", Audit: []RecoveryAuditCheck{{Code: "claim-coverage", Status: "PASS", Details: true}, {Code: "checkpoint-claim", Status: "INCONCLUSIVE", Details: "unsupported"}}}, nil
		}
		if in.Checkpoint.Ledger == 0 || in.Checkpoint.Ledger < in.ClaimedFrom || in.Checkpoint.Ledger > in.ClaimedTo || !covered(in.Coverage, in.Checkpoint.Ledger) {
			return RecoveryResult{Status: "FAIL", ClaimedFrom: in.ClaimedFrom, ClaimedTo: in.ClaimedTo, ClaimCovered: true, CheckpointStatus: "invalid", Limitation: "checkpoint claim is outside the covered recovery range", Audit: []RecoveryAuditCheck{{Code: "claim-coverage", Status: "PASS", Details: true}, {Code: "checkpoint-claim", Status: "FAIL", Details: "outside-covered-range"}}}, nil
		}
		if in.Checkpoint.EventID != "" {
			found := false
			for _, e := range in.Reference {
				if eventID(in.IdentityMode, e) == in.Checkpoint.EventID && e.LedgerSeq == in.Checkpoint.Ledger {
					found = true
					break
				}
			}
			if !found {
				return RecoveryResult{Status: "FAIL", ClaimedFrom: in.ClaimedFrom, ClaimedTo: in.ClaimedTo, ClaimCovered: true, CheckpointStatus: "invalid", Limitation: "checkpoint event is absent from reference history", Audit: []RecoveryAuditCheck{{Code: "claim-coverage", Status: "PASS", Details: true}, {Code: "checkpoint-claim", Status: "FAIL", Details: "event-absent-from-reference"}}}, nil
			}
		}
		checkpointStatus = "PASS"
	}
	before, err := Reconcile(Input{Reference: in.Reference, Observed: in.Before, Coverage: in.Coverage, IdentityMode: in.IdentityMode})
	if err != nil {
		return RecoveryResult{}, err
	}
	after, err := Reconcile(Input{Reference: in.Reference, Observed: in.After, Coverage: in.Coverage, IdentityMode: in.IdentityMode})
	if err != nil {
		return RecoveryResult{}, err
	}
	missing := func(r Report) []string {
		out := []string{}
		for _, f := range r.Findings {
			if f.Kind == "missing-event" {
				if in.IdentityMode != "source-scoped" {
					var ledger uint64
					if _, err := fmt.Sscanf(f.ID, "%d:", &ledger); err != nil || ledger < in.ClaimedFrom || ledger > in.ClaimedTo {
						continue
					}
				}
				out = append(out, f.ID)
			}
		}
		sort.Strings(out)
		return out
	}
	bm, am := missing(before), missing(after)
	status := "INCONCLUSIVE"
	if len(am) < len(bm) {
		status = "PASS"
	} else if len(am) > len(bm) {
		status = "FAIL"
	}
	checkpointCheck := RecoveryAuditCheck{Code: "checkpoint-claim", Status: "NOT_APPLICABLE", Details: checkpointStatus}
	if checkpointStatus == "PASS" {
		checkpointCheck.Status = "PASS"
	}
	residualCheck := RecoveryAuditCheck{Code: "residual-gap-movement", Status: status, Details: map[string]int{"before": len(bm), "after": len(am)}}
	return RecoveryResult{Status: status, BeforeMissingCount: len(bm), AfterMissingCount: len(am), ResidualMissing: am, BeforeResidual: bm, ClaimedFrom: in.ClaimedFrom, ClaimedTo: in.ClaimedTo, ClaimCovered: true, CheckpointStatus: checkpointStatus, Audit: []RecoveryAuditCheck{{Code: "claim-coverage", Status: "PASS", Details: true}, checkpointCheck, residualCheck}}, nil
}

// SortFindings gives callers a stable presentation order without changing semantic results.
func SortFindings(findings []Finding) {
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Kind != findings[j].Kind {
			return findings[i].Kind < findings[j].Kind
		}
		if findings[i].ID != findings[j].ID {
			return findings[i].ID < findings[j].ID
		}
		if findings[i].Source != findings[j].Source {
			return findings[i].Source < findings[j].Source
		}
		if findings[i].Status != findings[j].Status {
			return findings[i].Status < findings[j].Status
		}
		return findings[i].Count < findings[j].Count
	})
}
