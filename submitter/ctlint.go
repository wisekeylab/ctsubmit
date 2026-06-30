package submitter

import (
	"fmt"

	"github.com/crtsh/ctsubmit/pki"

	"github.com/crtsh/ctlint"
)

func runCTLint(tbsCertificate []byte) []CTLintResult {
	var lres []CTLintResult

	dummyCert, err := pki.MakeDummyCertificate(tbsCertificate)
	if err != nil {
		lres = append(lres, CTLintResult{
			Finding:  fmt.Sprintf("Failed to create dummy certificate: %v", err),
			Severity: "fatal",
		})
	} else {
		results := ctlint.CheckCertificate(dummyCert, nil)
		for _, result := range results {
			lresult := CTLintResult{
				Finding: result[3:],
			}
			switch result[0:3] {
			case "I: ":
				lresult.Severity = "info"
			case "N: ":
				lresult.Severity = "notice"
			case "W: ":
				lresult.Severity = "warning"
			case "E: ":
				lresult.Severity = "error"
			case "B: ":
				lresult.Severity = "bug"
			case "F: ":
				lresult.Severity = "fatal"
			default:
				continue
			}
			lres = append(lres, lresult)
		}
	}

	return lres
}
