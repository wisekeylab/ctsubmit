package submitter

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/crtsh/ctsubmit/monitor"

	json "github.com/goccy/go-json"
	ctgo "github.com/google/certificate-transparency-go"
)

// ts returns the elapsed time since start, formatted for trace logging.
func ts(start time.Time) string { return fmt.Sprintf("+%.6fs", time.Since(start).Seconds()) }

func (sr *SubmissionRequest) submit(strategy []StrategyMember, sha256IssuerSPKI *[sha256.Size]byte, entryType ctgo.LogEntryType, entryData []byte) ([]ctgo.AddChainResponse, []*ctgo.SignedCertificateTimestamp, error) {
	// Determine the API path for the submission.
	apiPath := ""
	switch entryType {
	case ctgo.X509LogEntryType:
		apiPath = "ct/v1/add-chain"
	case ctgo.PrecertLogEntryType:
		apiPath = "ct/v1/add-pre-chain"
	default:
		return nil, nil, fmt.Errorf("unsupported log entry type: %v", entryType)
	}

	// Marshal the request body once, since it's the same for all logs. Strip the additional fields that ctsubmit recognizes but logs won't recognize.
	requestBody, err := json.Marshal(ctgo.AddChainRequest{Chain: sr.AddChainRequest.Chain})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal submission request: %w", err)
	}

	// Filter out excluded logs from the strategy.
	var eligible []int
	for i := range strategy {
		if strategy[i].Bucket != EXCLUDED {
			eligible = append(eligible, i)
		}
	}
	if len(eligible) == 0 {
		return nil, nil, fmt.Errorf("no eligible logs in strategy")
	}

	start := time.Now()
	fmt.Printf("%s [submit] eligible logs in strategy order (%d):\n", ts(start), len(eligible))
	for order, idx := range eligible {
		sm := strategy[idx]
		fmt.Printf("%s [submit]   #%d strategyIdx=%d url=%s operator=%s bucket=%d logType=%v\n", ts(start), order+1, idx, sm.SubmissionURL, sm.Operator, sm.Bucket, sm.LogType)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// On return, annotate any eligible logs that were never contacted.
	defer func() {
		for _, idx := range eligible {
			if strategy[idx].Outcome == "" {
				strategy[idx].Outcome = "Submission not attempted"
			}
		}
	}()

	events := make(chan submissionEvent, len(eligible))
	qs := newQuorumState()

	inFlight := 0
	inFlightIndices := make(map[int]bool)     // strategy indices currently in-flight (not yet slow)
	slowInFlightIndices := make(map[int]bool) // strategy indices in-flight but past try-next threshold
	launchSeq := 0

	// launchEligible launches a goroutine to submit to the log at eligible[eligibleIdx].
	launchEligible := func(eligibleIdx int) {
		strategyIdx := eligible[eligibleIdx]
		inFlight++
		inFlightIndices[strategyIdx] = true
		launchSeq++
		strategy[strategyIdx].BeganAfter = time.Since(start)
		fmt.Printf("%s [submit] launch #%d strategyIdx=%d url=%s operator=%s inFlight=%d\n", ts(start), launchSeq, strategyIdx, strategy[strategyIdx].SubmissionURL, strategy[strategyIdx].Operator, inFlight)

		go func(idx int) {
			submitToLog(ctx, start, idx, strategy[idx].SubmissionURL, apiPath, requestBody, sha256IssuerSPKI, entryType, entryData, events)
		}(strategyIdx)
	}

	// Start initial batch: up to sr.SCTs concurrent submissions.
	// Ensure at least one log of each required/preferred type is included in the initial batch.
	initialBatch := sr.SCTs
	if initialBatch > len(eligible) {
		initialBatch = len(eligible)
	}
	ensureLogType := func(logType LogType) {
		if launchSeq >= initialBatch {
			return
		}
		for i, strategyIdx := range eligible {
			if strategy[strategyIdx].BeganAfter != 0 {
				continue
			}
			if strategy[strategyIdx].LogType == logType {
				launchEligible(i)
				return
			}
		}
	}
	if sr.RequireAtLeastOneRFC6962SCT {
		ensureLogType(LOGTYPE_RFC6962)
	}
	if sr.PreferAtLeastOneStaticSCT {
		ensureLogType(LOGTYPE_STATIC)
	}
	for i := 0; i < len(eligible) && launchSeq < initialBatch; i++ {
		if strategy[eligible[i]].BeganAfter != 0 {
			continue
		}
		launchEligible(i)
	}

	// startNextEligible scans all unattempted eligible logs (in strategy order) and starts
	// the first one that would help meet quorum.  Previously skipped logs are reconsidered
	// because an in-flight request that made them redundant may have since failed.
	// If a log-type requirement/preference is unmet and not covered by any in-flight
	// submission, the first eligible log of that type is preferred over strategy order.
	startNextEligible := func() {
		// Check if we need to prioritize a specific log type.
		needRFC6962 := sr.RequireAtLeastOneRFC6962SCT && !qs.hasRFC6962SCT
		needStatic := sr.PreferAtLeastOneStaticSCT && !qs.hasStaticSCT
		if needRFC6962 || needStatic {
			// Check whether any non-slow in-flight submission already covers the needed log type.
			// Slow in-flight submissions are excluded because they triggered try-next precisely
			// because they are unreliable.
			for idx := range inFlightIndices {
				if needRFC6962 && strategy[idx].LogType == LOGTYPE_RFC6962 {
					needRFC6962 = false
				}
				if needStatic && strategy[idx].LogType == LOGTYPE_STATIC {
					needStatic = false
				}
			}
		}
		if needRFC6962 || needStatic {
			for i, strategyIdx := range eligible {
				if strategy[strategyIdx].BeganAfter != 0 {
					continue
				}
				sm := strategy[strategyIdx]
				if needRFC6962 && sm.LogType == LOGTYPE_RFC6962 || needStatic && sm.LogType == LOGTYPE_STATIC {
					if qs.wouldHelp(sr, sm, strategy, inFlightIndices) {
						fmt.Printf("%s [submit] next eligible helps quorum (log type preference): strategyIdx=%d url=%s operator=%s logType=%v\n", ts(start), strategyIdx, sm.SubmissionURL, sm.Operator, sm.LogType)
						launchEligible(i)
						return
					}
				}
			}
		}
		for i, strategyIdx := range eligible {
			if strategy[strategyIdx].BeganAfter != 0 {
				continue
			}
			sm := strategy[strategyIdx]
			if qs.wouldHelp(sr, sm, strategy, inFlightIndices) {
				fmt.Printf("%s [submit] next eligible helps quorum: strategyIdx=%d url=%s operator=%s\n", ts(start), strategyIdx, sm.SubmissionURL, sm.Operator)
				launchEligible(i)
				return
			}
		}
	}

	// Process events until quorum is met or all attempts are exhausted.
	for inFlight > 0 {
		event := <-events

		switch event.eventType {
		case eventSuccess:
			inFlight--
			delete(inFlightIndices, event.strategyIdx)
			delete(slowInFlightIndices, event.strategyIdx)
			strategy[event.strategyIdx].TimeTaken = event.timeTaken
			if qs.helpsQuorum(sr, strategy[event.strategyIdx]) {
				strategy[event.strategyIdx].Outcome = event.outcome
				qs.addSuccess(strategy, event.strategyIdx, event.response, event.sct)
			} else {
				strategy[event.strategyIdx].Outcome = "Submission successful, but doesn't help quorum"
			}
			fmt.Printf("%s [submit] success strategyIdx=%d url=%s operator=%s scts=%d/%d operators=%d/%d inFlight=%d\n", ts(start), event.strategyIdx, strategy[event.strategyIdx].SubmissionURL, strategy[event.strategyIdx].Operator, len(qs.responses), sr.SCTs, len(qs.operators), sr.Operators, inFlight)

			if qs.isQuorumMet(sr) {
				fmt.Printf("%s [submit] quorum met; cancelling remaining in-flight submissions (inFlight=%d)\n", ts(start), inFlight)
				for _, indices := range []map[int]bool{inFlightIndices, slowInFlightIndices} {
					for idx := range indices {
						strategy[idx].Outcome = "Submission cancelled (quorum met)"
					}
				}
				cancel()
				// Drain events from cancelled goroutines to capture their timeTaken.
				for inFlight > 0 {
					ev := <-events
					if ev.eventType == eventSuccess || ev.eventType == eventFailure {
						inFlight--
						strategy[ev.strategyIdx].TimeTaken = ev.timeTaken
					}
				}
				qs.trimToQuorum(sr, strategy)
				return qs.responses, qs.scts, nil
			}
			if inFlight == 0 {
				startNextEligible()
			}

		case eventFailure:
			inFlight--
			delete(inFlightIndices, event.strategyIdx)
			delete(slowInFlightIndices, event.strategyIdx)
			strategy[event.strategyIdx].Outcome = event.outcome
			strategy[event.strategyIdx].TimeTaken = event.timeTaken
			fmt.Printf("%s [submit] failure strategyIdx=%d url=%s operator=%s inFlight=%d\n", ts(start), event.strategyIdx, strategy[event.strategyIdx].SubmissionURL, strategy[event.strategyIdx].Operator, inFlight)
			if inFlight == 0 {
				startNextEligible()
			}

		case eventTryNext:
			// An in-flight submission has exceeded the try-next threshold; move it to the
			// slow set so wouldHelp no longer counts it optimistically, then start the next
			// eligible log (the slow submission continues and might still succeed).
			delete(inFlightIndices, event.strategyIdx)
			slowInFlightIndices[event.strategyIdx] = true
			fmt.Printf("%s [submit] try-next threshold exceeded strategyIdx=%d url=%s operator=%s; launching additional candidate\n", ts(start), event.strategyIdx, strategy[event.strategyIdx].SubmissionURL, strategy[event.strategyIdx].Operator)
			startNextEligible()

		case eventSlow:
			monitor.RecordSlowResponse(strategy[event.strategyIdx].SubmissionURL)
			fmt.Printf("%s [submit] slow response strategyIdx=%d url=%s operator=%s\n", ts(start), event.strategyIdx, strategy[event.strategyIdx].SubmissionURL, strategy[event.strategyIdx].Operator)
		}
	}

	// All logs attempted; check if quorum was met despite not being detected in the loop
	// (shouldn't happen, but be safe).
	if qs.isQuorumMet(sr) {
		qs.trimToQuorum(sr, strategy)
		return qs.responses, qs.scts, nil
	}

	return nil, nil, fmt.Errorf("quorum not achieved: %s", qs.quorumFailureReason(sr))
}
