package main

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"encoding/csv"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	_ "github.com/crtsh/ccadb_data" // Blank import to ensure it appears in build info for module cache lookup.
	"github.com/crtsh/ctloglists"
	"github.com/crtsh/ctsubmit/loglists"
	"github.com/google/certificate-transparency-go/loglist3"
)

// certInfo holds a parsed certificate with its raw DER bytes and SHA-256 hash.
type certInfo struct {
	cert   *x509.Certificate
	raw    []byte
	sha256 [sha256.Size]byte
}

// logGroup represents a set of logs sharing the same accepted roots.
type logGroup struct {
	rootsHash [sha256.Size]byte
	count     int
}

func main() {
	// Step 1: Load CCADB certificates from the ccadb_data module's cmd/ski_spki/data directory.
	fmt.Println("Loading CCADB certificates...")
	allCerts := loadCCADBCerts()
	fmt.Printf("Loaded %d certificates\n", len(allCerts))

	// Build index by SubjectKeyId (hex-encoded).
	certsBySKI := make(map[string][]*certInfo)
	certsByHash := make(map[[sha256.Size]byte]*certInfo)
	for _, ci := range allCerts {
		certsByHash[ci.sha256] = ci
		if len(ci.cert.SubjectKeyId) > 0 {
			ski := hex.EncodeToString(ci.cert.SubjectKeyId)
			certsBySKI[ski] = append(certsBySKI[ski], ci)
		}
	}

	// Step 2: Accepted roots are loaded automatically by the loglists package init().

	// Precompute: for each accepted roots pool, build a set of root cert SHA-256 hashes.
	rootHashSets := make(map[[sha256.Size]byte]map[[sha256.Size]byte]bool)
	for poolHash, pool := range ctloglists.AcceptedRootsMap {
		hashSet := make(map[[sha256.Size]byte]bool)
		for _, cert := range pool.RawCertificates() {
			hashSet[sha256.Sum256(cert.Raw)] = true
		}
		rootHashSets[poolHash] = hashSet
	}

	// Log list contexts, loaded by loglists.init().
	contexts := [4]*loglist3.LogList{
		loglists.UsableTLSLogs,
		loglists.ActiveTLSLogs,
		loglists.TestTLSLogs,
		loglists.UsableBIMILogs,
	}
	contextNames := [4]string{"Usable TLS", "Active TLS", "Test TLS", "Usable BIMI"}

	// Precompute: for each context, group logs by accepted roots hash.
	var contextLogGroups [4][]logGroup
	for i, ctx := range contexts {
		groups := make(map[[sha256.Size]byte]int)
		forEachLog(ctx, func(logID [sha256.Size]byte) {
			if rootsHash, ok := ctloglists.LogAcceptedRootsMap[logID]; ok {
				groups[rootsHash]++
			}
		})
		for rh, cnt := range groups {
			contextLogGroups[i] = append(contextLogGroups[i], logGroup{rootsHash: rh, count: cnt})
		}
		total := 0
		for _, g := range contextLogGroups[i] {
			total += g.count
		}
		fmt.Printf("Context %q: %d logs in %d root groups\n", contextNames[i], total, len(contextLogGroups[i]))
	}

	// Step 3 & 4: For each certificate, find optimal chains and record results.
	type row struct {
		certHash string
		parents  [4]string
	}
	rows := make([]row, 0, len(allCerts))

	for idx, ci := range allCerts {
		if (idx+1)%1000 == 0 || idx+1 == len(allCerts) {
			fmt.Printf("\rProcessing certificate %d/%d...", idx+1, len(allCerts))
		}

		chains := buildChains(ci, certsBySKI)

		r := row{
			certHash: strings.ToUpper(hex.EncodeToString(ci.sha256[:])),
		}

		for ctxIdx, groups := range contextLogGroups {
			var bestScore, bestLen int
			var bestNotAfter time.Time
			var bestParentHash [sha256.Size]byte
			hasResult := false

			for _, chain := range chains {
				if len(chain) < 2 {
					continue // No parent in this chain.
				}

				// Check if the chain's root is in each accepted roots pool.
				rootHash := sha256.Sum256(chain[len(chain)-1])

				score := 0
				for _, g := range groups {
					if rootHashSets[g.rootsHash][rootHash] {
						score += g.count
					}
				}

				if score > 0 {
					parentHash := sha256.Sum256(chain[1])
					var parentNotAfter time.Time
					if pi, ok := certsByHash[parentHash]; ok {
						parentNotAfter = pi.cert.NotAfter
					}

					if !hasResult || score > bestScore || (score == bestScore && len(chain) < bestLen) || (score == bestScore && len(chain) == bestLen && parentNotAfter.After(bestNotAfter)) || (score == bestScore && len(chain) == bestLen && parentNotAfter.Equal(bestNotAfter) && bytes.Compare(parentHash[:], bestParentHash[:]) < 0) {
						bestScore = score
						bestLen = len(chain)
						bestNotAfter = parentNotAfter
						bestParentHash = parentHash
						hasResult = true
					}
				}
			}

			if hasResult && bestLen >= 3 {
				r.parents[ctxIdx] = strings.ToUpper(hex.EncodeToString(bestParentHash[:]))
			}
		}

		rows = append(rows, r)
	}
	fmt.Println()

	// Sort by certificate SHA-256 hash.
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].certHash < rows[j].certHash
	})

	// Write CSV output.
	if err := os.MkdirAll("data", 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}
	outputPath := filepath.Join("data", "optimal_parents.csv")
	f, err := os.Create(outputPath)
	if err != nil {
		log.Fatalf("Failed to create output file: %v", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	w.Write([]string{"Certificate", "Parent (Usable TLS)", "Parent (Active TLS)", "Parent (Test TLS)", "Parent (Usable BIMI)"})
	for _, r := range rows {
		w.Write([]string{r.certHash, r.parents[0], r.parents[1], r.parents[2], r.parents[3]})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		log.Fatalf("Failed to write CSV: %v", err)
	}

	fmt.Printf("Wrote %d rows to %s\n", len(rows), outputPath)
}

// loadCCADBCerts reads all PEM certificates from the ccadb_data module's cmd/ski_spki/data directory.
func loadCCADBCerts() []*certInfo {
	dataDir := filepath.Join(getCcadbDataDir(), "cmd", "ski_spki", "data")
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		log.Fatalf("Failed to read CCADB data directory %s: %v", dataDir, err)
	}

	seen := make(map[[sha256.Size]byte]bool)
	var certs []*certInfo

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dataDir, entry.Name()))
		if err != nil {
			log.Printf("Warning: could not read %s: %v", entry.Name(), err)
			continue
		}

		reader := csv.NewReader(strings.NewReader(string(data)))
		reader.FieldsPerRecord = 2
		reader.LazyQuotes = true
		reader.TrimLeadingSpace = true
		reader.ReuseRecord = true
		records, err := reader.ReadAll()
		if err != nil {
			log.Printf("Warning: could not parse CSV %s: %v", entry.Name(), err)
			continue
		}

		for _, record := range records[1:] {
			block, _ := pem.Decode([]byte(record[1]))
			if block == nil {
				continue
			}
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				continue
			}
			hash := sha256.Sum256(block.Bytes)
			if seen[hash] {
				continue
			}
			seen[hash] = true
			certs = append(certs, &certInfo{
				cert:   cert,
				raw:    block.Bytes,
				sha256: hash,
			})
		}
	}
	return certs
}

// buildChains returns all possible chains from the given certificate through
// intermediate CA certificates to a self-signed root. Each chain is a slice
// of DER-encoded certificates. Returns nil for self-signed (root) certificates.
func buildChains(ci *certInfo, certsBySKI map[string][]*certInfo) [][][]byte {
	if isSelfSigned(ci) {
		return nil
	}
	return buildChainsHelper(ci, certsBySKI, make(map[[sha256.Size]byte]bool))
}

func buildChainsHelper(ci *certInfo, certsBySKI map[string][]*certInfo, visited map[[sha256.Size]byte]bool) [][][]byte {
	visited[ci.sha256] = true
	defer delete(visited, ci.sha256)

	parents := findParents(ci, certsBySKI)

	var chains [][][]byte
	for _, parent := range parents {
		if visited[parent.sha256] {
			continue
		}
		if isSelfSigned(parent) {
			// Parent is a root; chain ends here.
			chains = append(chains, [][]byte{ci.raw, parent.raw})
		} else {
			// Recursively build chains through this parent.
			for _, pc := range buildChainsHelper(parent, certsBySKI, visited) {
				chain := make([][]byte, 0, len(pc)+1)
				chain = append(chain, ci.raw)
				chain = append(chain, pc...)
				chains = append(chains, chain)
			}
		}
	}

	if len(chains) == 0 {
		// No parent found; chain is just this cert.
		chains = [][][]byte{{ci.raw}}
	}

	return chains
}

func isSelfSigned(ci *certInfo) bool {
	if ci.cert.AuthorityKeyId != nil && ci.cert.SubjectKeyId != nil {
		return bytes.Equal(ci.cert.AuthorityKeyId, ci.cert.SubjectKeyId)
	}
	return bytes.Equal(ci.cert.RawIssuer, ci.cert.RawSubject)
}

func findParents(ci *certInfo, certsBySKI map[string][]*certInfo) []*certInfo {
	if len(ci.cert.AuthorityKeyId) == 0 {
		return nil
	}
	aki := hex.EncodeToString(ci.cert.AuthorityKeyId)
	var parents []*certInfo
	for _, c := range certsBySKI[aki] {
		if c.sha256 != ci.sha256 {
			parents = append(parents, c)
		}
	}
	return parents
}

// forEachLog calls fn for each log (including tiled logs) in the log list.
func forEachLog(ll *loglist3.LogList, fn func(logID [sha256.Size]byte)) {
	if ll == nil {
		return
	}
	for _, op := range ll.Operators {
		for _, l := range op.Logs {
			fn(sha256.Sum256(l.Key))
		}
		for _, tl := range op.TiledLogs {
			fn(sha256.Sum256(tl.Key))
		}
	}
}

// getCcadbDataDir locates the ccadb_data module directory in the Go module cache.
func getCcadbDataDir() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		log.Fatal("Failed to read build info")
	}

	modCache := os.Getenv("GOMODCACHE")
	if modCache == "" {
		gopath := os.Getenv("GOPATH")
		if gopath == "" {
			home, _ := os.UserHomeDir()
			gopath = filepath.Join(home, "go")
		}
		modCache = filepath.Join(gopath, "pkg", "mod")
	}

	for _, dep := range bi.Deps {
		if dep.Path == "github.com/crtsh/ccadb_data" {
			return filepath.Join(modCache, dep.Path+"@"+dep.Version)
		}
	}
	log.Fatal("github.com/crtsh/ccadb_data dependency not found in build info")
	return ""
}
