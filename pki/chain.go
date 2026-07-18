package pki

import (
	"crypto/sha256"
	"encoding/binary"
	"hash"
	"sync"
	"time"

	"github.com/crtsh/ctloglists"
	"github.com/crtsh/ctsubmit/loglists"
	"github.com/google/certificate-transparency-go/loglist3"
	"github.com/google/certificate-transparency-go/trillian/ctfe"
)

type ValidateChainCacheMap map[[sha256.Size]byte]bool

var logValidateChainCacheMap map[[sha256.Size]byte]ValidateChainCacheMap
var logValidateChainCacheMu sync.RWMutex

func init() {
	logValidateChainCacheMap = make(map[[sha256.Size]byte]ValidateChainCacheMap)
	for logID := range ctloglists.LogAcceptedRootsMap {
		logValidateChainCacheMap[logID] = make(ValidateChainCacheMap)
	}
	for _, operator := range loglists.TestTLSLogs.Operators {
		for _, tiledLog := range operator.TiledLogs {
			keyID := logIDFromBytes(tiledLog.LogID)
			if _, ok := loglists.CustomAcceptedRoots(keyID); ok {
				logValidateChainCacheMap[keyID] = make(ValidateChainCacheMap)
			}
		}
	}
}

func validateChainCacheKey(submittedChain [][]byte, logTemporalInterval *loglist3.TemporalInterval) [sha256.Size]byte {
	h := sha256.New()
	if logTemporalInterval == nil {
		h.Write([]byte{0})
	} else {
		h.Write([]byte{1})
		writeLengthPrefixed(h, []byte(logTemporalInterval.StartInclusive.UTC().Format(time.RFC3339Nano)))
		writeLengthPrefixed(h, []byte(logTemporalInterval.EndExclusive.UTC().Format(time.RFC3339Nano)))
	}

	for _, certDER := range submittedChain {
		writeLengthPrefixed(h, certDER)
	}

	var key [sha256.Size]byte
	copy(key[:], h.Sum(nil))
	return key
}

func writeLengthPrefixed(h hash.Hash, data []byte) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(data)))
	h.Write(length[:])
	h.Write(data)
}

func cachedValidateChainResult(logID, chainSHA256 [sha256.Size]byte) (bool, bool) {
	logValidateChainCacheMu.RLock()
	defer logValidateChainCacheMu.RUnlock()

	validateChainCacheMap, ok := logValidateChainCacheMap[logID]
	if !ok {
		return false, false
	}
	chainIsValid, ok := validateChainCacheMap[chainSHA256]
	return chainIsValid, ok
}

func cacheValidateChainResult(logID, chainSHA256 [sha256.Size]byte, chainIsValid bool) {
	logValidateChainCacheMu.Lock()
	defer logValidateChainCacheMu.Unlock()

	validateChainCacheMap, ok := logValidateChainCacheMap[logID]
	if !ok {
		return
	}
	validateChainCacheMap[chainSHA256] = chainIsValid
}

func logIDFromBytes(logID []byte) [sha256.Size]byte {
	var keyID [sha256.Size]byte
	copy(keyID[:], logID)
	return keyID
}

func ValidateChain(logID [sha256.Size]byte, submittedChain [][]byte, logTemporalInterval *loglist3.TemporalInterval) bool {
	var chainSHA256 [sha256.Size]byte
	var chainIsValid, ok bool

	if len(submittedChain) > 1 { // Don't use the cache when there's no chain.
		// Check if ValidateChain has already been called for this submitted chain / this log.
		chainSHA256 = validateChainCacheKey(submittedChain, logTemporalInterval)
		chainIsValid, ok = cachedValidateChainResult(logID, chainSHA256)
	}

	if !ok { // Cache miss or not used.
		// Validate the chain against the accepted roots of this log.
		if ll, ok2 := ctloglists.LogAcceptedRootsMap[logID]; ok2 {
			var start, end *time.Time
			if logTemporalInterval != nil {
				start = &logTemporalInterval.StartInclusive
				end = &logTemporalInterval.EndExclusive
			}
			cvo := ctfe.NewCertValidationOpts(ctloglists.AcceptedRootsMap[ll], time.Now(), false, false, start, end, false, nil)
			_, err := ctfe.ValidateChain(submittedChain, cvo)
			chainIsValid = (err == nil)
			if len(submittedChain) > 1 {
				cacheValidateChainResult(logID, chainSHA256, chainIsValid)
			}
		} else if roots, ok2 := loglists.CustomAcceptedRoots(logID); ok2 {
			var start, end *time.Time
			if logTemporalInterval != nil {
				start = &logTemporalInterval.StartInclusive
				end = &logTemporalInterval.EndExclusive
			}
			cvo := ctfe.NewCertValidationOpts(roots, time.Now(), false, false, start, end, false, nil)
			_, err := ctfe.ValidateChain(submittedChain, cvo)
			chainIsValid = (err == nil)
			if len(submittedChain) > 1 {
				cacheValidateChainResult(logID, chainSHA256, chainIsValid)
			}
		}
	}
	return chainIsValid
}
