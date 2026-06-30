package pki

import (
	"bytes"
	"crypto/sha256"
	"time"

	"github.com/crtsh/ctloglists"
	"github.com/google/certificate-transparency-go/loglist3"
	"github.com/google/certificate-transparency-go/trillian/ctfe"
)

type ValidateChainCacheMap map[[sha256.Size]byte]bool

var logValidateChainCacheMap map[[sha256.Size]byte]ValidateChainCacheMap

func init() {
	logValidateChainCacheMap = make(map[[sha256.Size]byte]ValidateChainCacheMap)
	for logID := range ctloglists.LogAcceptedRootsMap {
		logValidateChainCacheMap[logID] = make(ValidateChainCacheMap)
	}
}

func ValidateChain(logID [sha256.Size]byte, submittedChain [][]byte, logTemporalInterval *loglist3.TemporalInterval) bool {
	var validateChainCacheMap map[[sha256.Size]byte]bool
	var chainSHA256 [sha256.Size]byte
	var chainIsValid, ok bool

	if len(submittedChain) > 1 { // Don't use the cache when there's no chain.
		// Check if ValidateChain has already been called for this chain of CA certificates / this log.
		validateChainCacheMap = logValidateChainCacheMap[logID]
		chainSHA256 = sha256.Sum256(bytes.Join(submittedChain[1:], nil))
		chainIsValid, ok = validateChainCacheMap[chainSHA256]
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
				validateChainCacheMap[chainSHA256] = chainIsValid
			}
		}
	}
	return chainIsValid
}
