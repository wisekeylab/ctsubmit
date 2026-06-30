package pki

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"embed"
	"encoding/csv"
	"encoding/hex"
	"strings"

	"github.com/crtsh/ctsubmit/loglists"

	"github.com/crtsh/ccadb_data"
	"github.com/google/certificate-transparency-go/loglist3"
)

//go:embed data/optimal_parents.csv
var csvFS embed.FS

// Column indices in the optimal_parents.csv (after the Certificate column).
const (
	colUsableTLS = iota
	colActiveTLS
	colTestTLS
	colUsableBIMI
	numContexts
)

// optimalParent maps a hex-encoded Authority Key Identifier to the optimal parent SHA-256 hash for each context.
// A zero-value hash means no optimal parent exists for that context.
var optimalParent map[string][numContexts][sha256.Size]byte

func init() {
	ccadb_data.LoadAllCACertificates() // Ensure ccadb_data is initialized before loading optimal parents.

	optimalParent = make(map[string][numContexts][sha256.Size]byte)

	data, err := csvFS.ReadFile("data/optimal_parents.csv")
	if err != nil {
		return
	}

	reader := csv.NewReader(strings.NewReader(string(data)))
	reader.ReuseRecord = true
	records, err := reader.ReadAll()
	if err != nil || len(records) == 0 {
		return
	}

	for _, record := range records[1:] {
		if len(record) < 1+numContexts {
			continue
		}

		aki := strings.ToUpper(record[0])

		var parents [numContexts][sha256.Size]byte
		for i := 0; i < numContexts; i++ {
			if record[1+i] == "" {
				continue
			}
			parentHash, err := hex.DecodeString(record[1+i])
			if err != nil || len(parentHash) != sha256.Size {
				continue
			}
			copy(parents[i][:], parentHash)
		}

		optimalParent[aki] = parents
	}
}

// contextIndex returns the optimal_parents.csv column index for the given log list.
func contextIndex(logList *loglist3.LogList) int {
	switch logList {
	case loglists.UsableTLSLogs:
		return colUsableTLS
	case loglists.TestTLSLogs:
		return colTestTLS
	case loglists.UsableBIMILogs:
		return colUsableBIMI
	default:
		return colActiveTLS
	}
}

// DiscoverChain extends the given partial certificate chain by looking up optimal parents.
// The partial chain must contain at least one DER-encoded certificate.
// The returned chain includes all submitted certificates followed by any discovered intermediates.
// Accepted roots are not appended; the chain stops at the last intermediate.
func DiscoverChain(partialChain [][]byte, logList *loglist3.LogList) [][]byte {
	chain := make([][]byte, len(partialChain))
	copy(chain, partialChain)
	ctxIdx := contextIndex(logList)

	var zeroHash [sha256.Size]byte

	// Track all certs already in the partial chain to avoid cycles.
	seen := make(map[[sha256.Size]byte]bool, len(partialChain))
	for _, der := range partialChain {
		seen[sha256.Sum256(der)] = true
	}

	// Parse the last certificate to get its AKI for lookup.
	lastCert, err := x509.ParseCertificate(partialChain[len(partialChain)-1])
	if err != nil {
		return nil
	}
	aki := strings.ToUpper(hex.EncodeToString(lastCert.AuthorityKeyId))

	for {
		parents, ok := optimalParent[aki]
		if !ok {
			break
		}
		parentHash := parents[ctxIdx]
		if parentHash == zeroHash {
			break
		}
		if seen[parentHash] {
			break // Cycle detected.
		}
		seen[parentHash] = true

		parentDER, ok := ccadb_data.GetCACertificateBySHA256(parentHash)
		if !ok {
			break
		}

		// Parse the parent to check if it's self-signed (a root).
		parentCert, err := x509.ParseCertificate(parentDER)
		if err != nil {
			break
		}

		// A self-signed root signals "chain is complete" — don't append it,
		// since CT logs don't require the root in submissions.
		if isSelfSignedCert(parentCert) {
			break
		}

		chain = append(chain, parentDER)

		aki = strings.ToUpper(hex.EncodeToString(parentCert.AuthorityKeyId))
		if aki == "" {
			break // No AKI means this is a root; stop.
		}
	}

	if len(chain) > len(partialChain) {
		return chain
	}
	return nil // No additional chain discovered; return nil so the caller knows discovery failed.
}

// isSelfSignedCert checks whether a certificate is self-signed.
func isSelfSignedCert(cert *x509.Certificate) bool {
	if cert.AuthorityKeyId != nil && cert.SubjectKeyId != nil {
		return bytes.Equal(cert.AuthorityKeyId, cert.SubjectKeyId)
	}
	return bytes.Equal(cert.RawIssuer, cert.RawSubject)
}
