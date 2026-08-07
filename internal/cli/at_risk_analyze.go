// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored transcendence command. Survives `generate --force` regen-merge.

// pp:data-source live

package cli

// lessAtRisk orders two at-risk students for the ranked output: more concerns
// first, ties broken by user ID.
//
// Extracted verbatim from the sort.Slice closure in at_risk.go so the ordering
// rule can be exercised directly. Behaviour is unchanged by this extraction.
func lessAtRisk(a, b atRiskStudent) bool {
	if a.Total != b.Total {
		return a.Total > b.Total
	}
	return a.UserID < b.UserID
}
