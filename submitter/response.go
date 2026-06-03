package submitter

import ctgo "github.com/google/certificate-transparency-go"

type CTLintResult struct {
	Finding  string `json:"finding"`
	Severity string `json:"severity"`
}

type SubmissionResponse struct {
	LogResponse     []ctgo.AddChainResponse `json:"logResponse"`
	FinalTBSCertB64 string                  `json:"finalTBSCertB64,omitempty"`
	CTLint          []CTLintResult          `json:"ctlint,omitempty"`
	Strategy        []StrategyMember        `json:"strategy,omitempty"`
}
