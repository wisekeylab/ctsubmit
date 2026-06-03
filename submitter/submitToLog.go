package submitter

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/crtsh/ctsubmit/config"
	"github.com/crtsh/ctsubmit/logger"
	"github.com/crtsh/ctsubmit/monitor"
	"github.com/crtsh/ctsubmit/utils"

	"github.com/crtsh/ctloglists"
	json "github.com/goccy/go-json"
	ctgo "github.com/google/certificate-transparency-go"

	"go.uber.org/zap"
)

var submissionHTTPClient *http.Client

func init() {
	submissionHTTPClient = &http.Client{Timeout: config.Config.Strategy.Submission.HTTPTimeout}
}

// submitToLog performs the HTTP submission to a single log and sends events on the channel.
// It sends eventTryNext if the try-next threshold is exceeded before the HTTP response,
// and eventSlow if the slow response threshold is exceeded (for metrics).
// It then sends eventSuccess or eventFailure when the HTTP response completes.
func submitToLog(ctx context.Context, start time.Time, strategyIdx int, submissionURL string, apiPath string, requestBody []byte, sha256IssuerSPKI *[sha256.Size]byte, entryType ctgo.LogEntryType, entryData []byte, events chan<- submissionEvent) {
	endpointURL, err := url.JoinPath(submissionURL, apiPath)
	if err != nil {
		logger.Logger.Error("Failed to construct submission URL",
			zap.String("submissionURL", submissionURL),
			zap.Error(err),
		)
		events <- submissionEvent{strategyIdx: strategyIdx, eventType: eventFailure, outcome: fmt.Sprintf("Failed: could not construct submission URL: %v", err)}
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, bytes.NewReader(requestBody))
	if err != nil {
		logger.Logger.Error("Failed to create HTTP request",
			zap.String("url", endpointURL),
			zap.Error(err),
		)
		events <- submissionEvent{strategyIdx: strategyIdx, eventType: eventFailure, outcome: fmt.Sprintf("Failed: could not create HTTP request: %v", err)}
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "github.com/crtsh/ct_submit")
	req_body_sha256 := sha256.Sum256(requestBody)
	req.Header.Set("Idempotency-Key", hex.EncodeToString(req_body_sha256[:]))
	fmt.Printf("%s [submit] contacting log strategyIdx=%d submissionURL=%s endpoint=%s\n", ts(start), strategyIdx, submissionURL, endpointURL)

	// Start one-shot timers for try-next and slow response detection.
	tryNextTimer := time.NewTimer(config.Config.Strategy.Submission.TryNextResponseThreshold)
	defer tryNextTimer.Stop()
	slowTimer := time.NewTimer(config.Config.Strategy.Submission.SlowResponseThreshold)
	defer slowTimer.Stop()

	// Run the HTTP request in a separate goroutine so we can race it against the timers.
	type httpResult struct {
		resp *http.Response
		err  error
	}
	httpCh := make(chan httpResult, 1)
	httpStart := time.Now()
	go func() {
		resp, err := submissionHTTPClient.Do(req)
		httpCh <- httpResult{resp: resp, err: err}
	}()

	tryNextSent := false
	slowSent := false
	for {
		select {
		case <-tryNextTimer.C:
			if !tryNextSent {
				tryNextSent = true
				fmt.Printf("%s [submit] try-next threshold exceeded strategyIdx=%d submissionURL=%s\n", ts(start), strategyIdx, submissionURL)
				events <- submissionEvent{strategyIdx: strategyIdx, eventType: eventTryNext}
			}

		case <-slowTimer.C:
			if !slowSent {
				slowSent = true
				fmt.Printf("%s [submit] slow threshold exceeded strategyIdx=%d submissionURL=%s\n", ts(start), strategyIdx, submissionURL)
				events <- submissionEvent{strategyIdx: strategyIdx, eventType: eventSlow}
			}

		case result := <-httpCh:
			timeTaken := time.Since(httpStart)
			if ctx.Err() != nil {
				// Context was cancelled around the same time the HTTP response arrived.
				if result.resp != nil {
					result.resp.Body.Close()
				}
				fmt.Printf("%s [submit] cancellation received (with late response) strategyIdx=%d submissionURL=%s\n", ts(start), strategyIdx, submissionURL)
				events <- submissionEvent{strategyIdx: strategyIdx, eventType: eventFailure, outcome: "Cancelled: quorum met", timeTaken: timeTaken}
				monitor.RecordSubmissionOutcome(submissionURL, "cancelled")
				return
			}
			fmt.Printf("%s [submit] HTTP completed strategyIdx=%d submissionURL=%s\n", ts(start), strategyIdx, submissionURL)
			processHTTPResponse(start, strategyIdx, submissionURL, endpointURL, result.resp, result.err, sha256IssuerSPKI, entryType, entryData, timeTaken, events)
			return

		case <-ctx.Done():
			// Context cancelled (quorum achieved). Wait briefly for any in-flight HTTP response,
			// but don't block indefinitely.
			fmt.Printf("%s [submit] cancellation received strategyIdx=%d submissionURL=%s\n", ts(start), strategyIdx, submissionURL)
			select {
			case result := <-httpCh:
				if result.resp != nil {
					fmt.Printf("%s [submit] cancelled attempt strategyIdx=%d got late response; ignoring result\n", ts(start), strategyIdx)
					result.resp.Body.Close()
				} else {
					fmt.Printf("%s [submit] cancelled attempt strategyIdx=%d completed with no response body\n", ts(start), strategyIdx)
				}
			case <-time.After(100 * time.Millisecond):
				fmt.Printf("%s [submit] cancelled attempt strategyIdx=%d still in-flight after 100ms; giving up wait\n", ts(start), strategyIdx)
			}
			events <- submissionEvent{strategyIdx: strategyIdx, eventType: eventFailure, outcome: "Cancelled: quorum met", timeTaken: time.Since(httpStart)}
			monitor.RecordSubmissionOutcome(submissionURL, "cancelled")
			return
		}
	}
}

func processHTTPResponse(start time.Time, strategyIdx int, submissionURL string, endpointURL string, resp *http.Response, err error, sha256IssuerSPKI *[sha256.Size]byte, entryType ctgo.LogEntryType, entryData []byte, timeTaken time.Duration, events chan<- submissionEvent) {
	monitor.RecordSubmissionResponseTime(submissionURL, timeTaken)

	if err != nil {
		if utils.IsTimeoutError(err) {
			monitor.RecordTimeout(submissionURL)
			fmt.Printf("%s [submit] ignored result strategyIdx=%d submissionURL=%s: timeout: %v\n", ts(start), strategyIdx, submissionURL, err)
			events <- submissionEvent{strategyIdx: strategyIdx, eventType: eventFailure, outcome: "Failed: timeout", timeTaken: timeTaken}
			monitor.RecordSubmissionOutcome(submissionURL, "timeout")
		} else {
			monitor.RecordBadResponse(submissionURL)
			fmt.Printf("%s [submit] ignored result strategyIdx=%d submissionURL=%s: HTTP error: %v\n", ts(start), strategyIdx, submissionURL, err)
			events <- submissionEvent{strategyIdx: strategyIdx, eventType: eventFailure, outcome: fmt.Sprintf("Failed: HTTP error: %v", err), timeTaken: timeTaken}
			monitor.RecordSubmissionOutcome(submissionURL, "http_error")
		}
		logger.Logger.Warn("Submission HTTP error", zap.String("url", endpointURL), zap.Error(err))
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		monitor.RecordBadResponse(submissionURL)
		fmt.Printf("%s [submit] ignored result strategyIdx=%d submissionURL=%s: failed to read response body: %v\n", ts(start), strategyIdx, submissionURL, err)
		logger.Logger.Warn("Failed to read submission response body", zap.String("url", endpointURL), zap.Error(err))
		events <- submissionEvent{strategyIdx: strategyIdx, eventType: eventFailure, outcome: fmt.Sprintf("Failed: could not read response body: %v", err), timeTaken: timeTaken}
		monitor.RecordSubmissionOutcome(submissionURL, "bad_response")
		return
	}

	if resp.StatusCode != http.StatusOK {
		switch {
		case resp.StatusCode >= 500:
			monitor.Record5xxResponse(submissionURL, resp)
		case resp.StatusCode >= 400:
			monitor.Record4xxResponse(submissionURL, resp)
		default:
			monitor.RecordBadResponse(submissionURL)
		}
		logger.Logger.Warn("Submission returned non-200 status", zap.String("url", endpointURL), zap.Int("status", resp.StatusCode), zap.String("body", string(body)))
		fmt.Printf("%s [submit] ignored result strategyIdx=%d submissionURL=%s: non-200 status=%d\n", ts(start), strategyIdx, submissionURL, resp.StatusCode)
		events <- submissionEvent{strategyIdx: strategyIdx, eventType: eventFailure, outcome: fmt.Sprintf("Failed: HTTP %d", resp.StatusCode), timeTaken: timeTaken}
		monitor.RecordSubmissionOutcome(submissionURL, fmt.Sprintf("%dxx", resp.StatusCode/100))
		return
	}

	// Unmarshal the JSON response.
	var addChainResponse ctgo.AddChainResponse
	if err := json.Unmarshal(body, &addChainResponse); err != nil {
		monitor.RecordBadResponse(submissionURL)
		fmt.Printf("%s [submit] ignored result strategyIdx=%d submissionURL=%s: invalid JSON response: %v\n", ts(start), strategyIdx, submissionURL, err)
		logger.Logger.Warn("Failed to unmarshal AddChainResponse", zap.String("url", endpointURL), zap.Error(err))
		events <- submissionEvent{strategyIdx: strategyIdx, eventType: eventFailure, outcome: fmt.Sprintf("Failed: invalid JSON response: %v", err), timeTaken: timeTaken}
		monitor.RecordSubmissionOutcome(submissionURL, "bad_response")
		return
	}

	// Encode the SCT.
	sct, err := addChainResponse.ToSignedCertificateTimestamp()
	if err != nil {
		fmt.Printf("%s [submit] ignored result strategyIdx=%d submissionURL=%s: failed to decode SCT: %v\n", ts(start), strategyIdx, submissionURL, err)
		logger.Logger.Warn("Failed to convert AddChainResponse to SignedCertificateTimestamp", zap.String("url", endpointURL), zap.Error(err))
		events <- submissionEvent{strategyIdx: strategyIdx, eventType: eventFailure, outcome: fmt.Sprintf("Failed: could not decode SCT: %v", err), timeTaken: timeTaken}
		monitor.RecordSubmissionOutcome(submissionURL, "bad_response")
		return
	}

	// Reject the SCT if its timestamp is in the future.  Allow 1 second of tolerance for clock skew.
	if time.UnixMilli(int64(sct.Timestamp)).After(time.Now().Add(time.Second)) {
		fmt.Printf("%s [submit] ignored SCT strategyIdx=%d submissionURL=%s: timestamp is in the future (%d)\n", ts(start), strategyIdx, submissionURL, sct.Timestamp)
		logger.Logger.Warn("Received SCT with future timestamp", zap.String("url", endpointURL), zap.Time("sct_timestamp", time.UnixMilli(int64(sct.Timestamp))), zap.Time("now", time.Now()))
		events <- submissionEvent{strategyIdx: strategyIdx, eventType: eventFailure, outcome: fmt.Sprintf("Failed: SCT timestamp in the future (%d)", sct.Timestamp), timeTaken: timeTaken}
		monitor.RecordSubmissionOutcome(submissionURL, "bad_response")
		return
	}

	// Verify the SCT's signature.
	err = verifySCTSignature(sct, sha256IssuerSPKI, entryType, entryData)
	if err != nil {
		fmt.Printf("%s [submit] ignored SCT strategyIdx=%d submissionURL=%s: signature verification failed: %v\n", ts(start), strategyIdx, submissionURL, err)
		logger.Logger.Warn("SCT signature verification failed", zap.String("url", endpointURL), zap.Error(err))
		events <- submissionEvent{strategyIdx: strategyIdx, eventType: eventFailure, outcome: fmt.Sprintf("Failed: SCT signature verification failed: %v", err), timeTaken: timeTaken}
		monitor.RecordSubmissionOutcome(submissionURL, "bad_response")
		return
	}

	fmt.Printf("%s [submit] accepted SCT strategyIdx=%d submissionURL=%s logID=%x timestamp=%d\n", ts(start), strategyIdx, submissionURL, sct.LogID.KeyID, sct.Timestamp)
	events <- submissionEvent{strategyIdx: strategyIdx, eventType: eventSuccess, response: addChainResponse, sct: sct, outcome: "Submission successful", timeTaken: timeTaken}
	monitor.RecordSubmissionOutcome(submissionURL, "success")
}

func verifySCTSignature(sct *ctgo.SignedCertificateTimestamp, sha256IssuerSPKI *[sha256.Size]byte, entryType ctgo.LogEntryType, entryData []byte) error {
	sv := ctloglists.LogSignatureVerifierMap[([sha256.Size]byte)(sct.LogID.KeyID)]
	if sv == nil {
		return fmt.Errorf("SCT is from an unknown log")
	}

	leaf := &ctgo.MerkleTreeLeaf{
		Version:  ctgo.V1,
		LeafType: ctgo.TimestampedEntryLeafType,
		TimestampedEntry: &ctgo.TimestampedEntry{
			EntryType: entryType,
			Timestamp: sct.Timestamp,
		},
	}
	if entryType == ctgo.PrecertLogEntryType {
		leaf.TimestampedEntry.PrecertEntry = &ctgo.PreCert{
			IssuerKeyHash:  *sha256IssuerSPKI,
			TBSCertificate: entryData,
		}
	} else {
		leaf.TimestampedEntry.X509Entry = &ctgo.ASN1Cert{
			Data: entryData,
		}
	}

	// Verify the SCT's signature against the constructed log entry.
	return sv.VerifySCTSignature(*sct, ctgo.LogEntry{Leaf: *leaf})
}
