package pki

import (
	"crypto/sha256"
	"sync"
	"testing"
	"time"

	"github.com/crtsh/ctloglists"
	"github.com/google/certificate-transparency-go/loglist3"
)

func TestValidateChainCacheKeyIncludesLeaf(t *testing.T) {
	issuerChain := [][]byte{[]byte("intermediate"), []byte("root")}
	chainA := append([][]byte{[]byte("leaf-a")}, issuerChain...)
	chainB := append([][]byte{[]byte("leaf-b")}, issuerChain...)

	if validateChainCacheKey(chainA, nil) == validateChainCacheKey(chainB, nil) {
		t.Fatal("cache key should change when the leaf certificate changes")
	}
}

func TestValidateChainCacheKeyUsesLengthPrefixes(t *testing.T) {
	chainA := [][]byte{[]byte("ab"), []byte("c")}
	chainB := [][]byte{[]byte("a"), []byte("bc")}

	if validateChainCacheKey(chainA, nil) == validateChainCacheKey(chainB, nil) {
		t.Fatal("cache key should distinguish chains with the same concatenated bytes")
	}
}

func TestValidateChainCacheKeyIncludesTemporalInterval(t *testing.T) {
	chain := [][]byte{[]byte("leaf"), []byte("issuer")}
	interval := &loglist3.TemporalInterval{
		StartInclusive: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
		EndExclusive:   time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC),
	}

	if validateChainCacheKey(chain, nil) == validateChainCacheKey(chain, interval) {
		t.Fatal("cache key should change when the log temporal interval changes")
	}
}

func TestValidateChainConcurrentCacheAccess(t *testing.T) {
	var logID [sha256.Size]byte
	for id := range ctloglists.LogAcceptedRootsMap {
		logID = id
		break
	}
	if logID == [sha256.Size]byte{} {
		t.Fatal("ctloglists.LogAcceptedRootsMap is empty")
	}

	submittedChain := [][]byte{[]byte("leaf"), []byte("issuer")}

	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 64; j++ {
				if ValidateChain(logID, submittedChain, nil) {
					t.Error("invalid test chain unexpectedly validated")
				}
			}
		}()
	}
	wg.Wait()
}
