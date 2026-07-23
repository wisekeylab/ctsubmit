package pki

import (
	"crypto/sha256"
	"sync"
	"testing"

	"github.com/crtsh/ctloglists"
)

func TestValidateChainCacheKeyExcludesLeaf(t *testing.T) {
	// The cache key is based only on the CA chain (submittedChain[1:]),
	// so different leaves with the same issuer chain should produce the same key.
	var logID [sha256.Size]byte
	for id := range ctloglists.LogAcceptedRootsMap {
		logID = id
		break
	}
	if logID == [sha256.Size]byte{} {
		t.Fatal("ctloglists.LogAcceptedRootsMap is empty")
	}

	issuerChain := [][]byte{[]byte("intermediate"), []byte("root")}
	chainA := append([][]byte{[]byte("leaf-a")}, issuerChain...)
	chainB := append([][]byte{[]byte("leaf-b")}, issuerChain...)

	// Both chains will fail validation (they're not real certs), but
	// the second call should hit the cache despite having a different leaf.
	ValidateChain(logID, chainA, nil)
	ValidateChain(logID, chainB, nil)

	// Verify the cache has exactly one entry for this logID (both calls
	// should have used the same cache key).
	logValidateChainCacheMutex.RLock()
	defer logValidateChainCacheMutex.RUnlock()
	cacheMap := logValidateChainCacheMap[logID]
	if len(cacheMap) != 1 {
		t.Fatalf("expected 1 cache entry for shared CA chain, got %d", len(cacheMap))
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
