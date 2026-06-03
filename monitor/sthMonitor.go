package monitor

import (
	"context"
	"crypto/x509"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/crtsh/ctsubmit/config"
	"github.com/crtsh/ctsubmit/logger"
	"github.com/crtsh/ctsubmit/utils"

	"filippo.io/sunlight"

	"github.com/crtsh/ctloglists"
	json "github.com/goccy/go-json"
	ctgo "github.com/google/certificate-transparency-go"

	"golang.org/x/mod/sumdb/note"

	"go.uber.org/zap"
)

type STHData struct {
	IsRFC6962Log  bool
	SubmissionURL string
	KeyName       string
	SigVerifier   *ctgo.SignatureVerifier
	NoteVerifiers note.Verifiers
	TreeSize      uint64
	Timestamp     *time.Time
	LastFetched   *time.Time
}

var (
	sthData       = make(map[string]*STHData)
	sthMutex      sync.RWMutex
	sthHTTPClient = &http.Client{Timeout: config.Config.STHMonitor.HTTPTimeout}
)

func init() {
	for _, operator := range ctloglists.CrtshV3Active.Operators {
		for _, log := range operator.Logs {
			pubKey, err := x509.ParsePKIXPublicKey(log.Key)
			if err != nil {
				logger.Logger.Warn("could not parse public key",
					zap.String("url", log.URL),
					zap.ByteString("key", log.Key),
					zap.Error(err))
				continue
			}
			sigVerifier, err := ctgo.NewSignatureVerifier(pubKey)
			if err != nil {
				logger.Logger.Warn("could not create signature verifier",
					zap.String("url", log.URL),
					zap.ByteString("key", log.Key),
					zap.Error(err))
				continue
			}

			logURL, _ := url.JoinPath(log.URL, "/")
			sthData[logURL] = &STHData{IsRFC6962Log: true, SigVerifier: sigVerifier, SubmissionURL: logURL}
		}

		for _, tiledLog := range operator.TiledLogs {
			submissionURL, _ := url.JoinPath(tiledLog.SubmissionURL, "/")
			monitoringURL, _ := url.JoinPath(tiledLog.MonitoringURL, "/")

			pubKey, err := x509.ParsePKIXPublicKey(tiledLog.Key)
			if err != nil {
				logger.Logger.Warn("Failed to parse static log public key",
					zap.String("url", monitoringURL),
					zap.ByteString("key", tiledLog.Key),
					zap.Error(err))
				continue
			}

			keyName := strings.TrimRight(strings.TrimPrefix(tiledLog.SubmissionURL, "https://"), "/")
			verifier, err := sunlight.NewRFC6962Verifier(keyName, pubKey)
			if err != nil {
				logger.Logger.Warn("Failed to create static log checkpoint verifier",
					zap.String("url", monitoringURL),
					zap.Error(err))
				continue
			}

			sthData[monitoringURL] = &STHData{KeyName: keyName, NoteVerifiers: note.VerifierList(verifier), SubmissionURL: submissionURL}
		}
	}
}

func STHMonitor(ctx context.Context) {
	logger.Logger.Info("Started STHMonitor")

	for {
		select {
		case <-time.After(config.Config.STHMonitor.RefreshInterval):
			FetchAllSTHs()
		case <-ctx.Done():
			logger.ShutdownWG.Done()
			logger.Logger.Info("Stopped STHMonitor")
			return
		}
	}
}

func FetchAllSTHs() {
	for url, sd := range sthData {
		if sd.IsRFC6962Log {
			go fetchSTH(url, sd)
		} else {
			go fetchCheckpoint(sd.SubmissionURL, url, sd)
		}
	}
}

func fetchSTH(logURL string, sd *STHData) {
	body := fetchResource(logURL, logURL+"ct/v1/get-sth")
	if body == nil {
		return
	}

	var sthResponse ctgo.GetSTHResponse
	var err error
	if err = json.Unmarshal(body, &sthResponse); err != nil {
		logger.Logger.Warn("Failed to unmarshal STH response", zap.String("url", logURL), zap.Error(err))
		RecordBadResponse(sd.SubmissionURL)
		return
	}

	var sth *ctgo.SignedTreeHead
	if sth, err = sthResponse.ToSignedTreeHead(); err != nil {
		logger.Logger.Warn("ToSignedTreeHead failed", zap.String("url", logURL), zap.Error(err))
		RecordBadResponse(sd.SubmissionURL)
		return
	}

	sthTimestamp := time.UnixMilli(int64(sthResponse.Timestamp))

	sthMutex.Lock()
	defer sthMutex.Unlock()

	if err = sd.SigVerifier.VerifySTHSignature(*sth); err != nil {
		logger.Logger.Warn("Invalid STH Signature", zap.String("url", logURL), zap.Time("sth_timestamp", sthTimestamp), zap.Uint64("tree_size", sthResponse.TreeSize))
		RecordBadResponse(sd.SubmissionURL)
		return
	}

	sd.TreeSize = sthResponse.TreeSize

	timestamp := time.Now()
	sd.LastFetched = &timestamp

	timestamp = sthTimestamp
	sd.Timestamp = &timestamp

	logger.Logger.Debug("Fetched STH",
		zap.String("url", logURL),
		zap.Uint64("tree_size", sthResponse.TreeSize),
		zap.Duration("age", time.Since(*sd.Timestamp)))
}

func fetchCheckpoint(submissionURL, monitoringURL string, sd *STHData) {
	body := fetchResource(submissionURL, monitoringURL+"checkpoint")
	if body == nil {
		return
	}

	sthMutex.Lock()
	defer sthMutex.Unlock()

	n, err := note.Open(body, sd.NoteVerifiers)
	if err != nil {
		logger.Logger.Warn("Failed to verify checkpoint note", zap.String("url", monitoringURL), zap.Error(err))
		RecordBadResponse(sd.SubmissionURL)
		return
	}
	if len(n.Sigs) < 1 {
		logger.Logger.Warn("Checkpoint note had no verified signatures", zap.String("url", monitoringURL))
		RecordBadResponse(sd.SubmissionURL)
		return
	}

	checkpoint, err := sunlight.ParseCheckpoint(n.Text)
	if err != nil {
		logger.Logger.Warn("Failed to parse checkpoint", zap.String("url", monitoringURL), zap.Error(err))
		RecordBadResponse(sd.SubmissionURL)
		return
	}
	if checkpoint.Origin != sd.KeyName {
		logger.Logger.Warn(
			"Unexpected checkpoint origin",
			zap.String("url", monitoringURL),
			zap.String("origin", checkpoint.Origin),
			zap.String("expected_origin", sd.KeyName),
		)
		RecordBadResponse(sd.SubmissionURL)
		return
	}

	timestampMillis, err := sunlight.RFC6962SignatureTimestamp(n.Sigs[0])
	if err != nil {
		logger.Logger.Warn("Failed to parse checkpoint timestamp", zap.String("url", monitoringURL), zap.Error(err))
		RecordBadResponse(sd.SubmissionURL)
		return
	}

	sd.TreeSize = uint64(checkpoint.N)

	lastFetched := time.Now()
	sd.LastFetched = &lastFetched

	timestamp := time.UnixMilli(timestampMillis)
	sd.Timestamp = &timestamp

	logger.Logger.Debug("Fetched checkpoint",
		zap.String("url", monitoringURL),
		zap.Uint64("tree_size", sd.TreeSize),
		zap.Duration("age", time.Since(*sd.Timestamp)))
}

func fetchResource(submissionURL, endpointURL string) []byte {
	req, err := http.NewRequest(http.MethodGet, endpointURL, nil)
	if err != nil {
		logger.Logger.Warn("Failed to create HTTP request", zap.String("url", endpointURL), zap.Error(err))
		return nil
	}

	req.Header.Set("User-Agent", "github.com/crtsh/ct_submit")

	resp, err := sthHTTPClient.Do(req)
	if err != nil {
		logger.Logger.Warn("Failed to fetch resource", zap.String("url", endpointURL), zap.Error(err))
		if utils.IsTimeoutError(err) {
			RecordTimeout(submissionURL)
		} else {
			RecordBadResponse(submissionURL)
		}
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.Logger.Warn("Unexpected HTTP status", zap.String("url", endpointURL), zap.Int("status", resp.StatusCode))
		if resp.StatusCode >= 500 && resp.StatusCode < 600 {
			Record5xxResponse(submissionURL, resp)
		} else if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			Record4xxResponse(submissionURL, resp)
		} else {
			RecordBadResponse(submissionURL)
		}
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Logger.Warn("Failed to read HTTP response body", zap.String("url", endpointURL), zap.Error(err))
		RecordBadResponse(submissionURL)
		return nil
	}

	return body
}

func GetSTHData(logURL string) (*STHData, bool) {
	sthMutex.RLock()
	defer sthMutex.RUnlock()

	sd, ok := sthData[logURL]
	if !ok {
		return nil, false
	}

	sdNew := *sd // Return a copy of the STHData.
	return &sdNew, true
}
