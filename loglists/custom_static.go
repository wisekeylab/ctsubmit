package loglists

import (
	"crypto/sha256"
	stdx509 "crypto/x509"
	"encoding/pem"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/crtsh/ctsubmit/config"
	"github.com/crtsh/ctsubmit/logger"

	"filippo.io/sunlight"

	"github.com/crtsh/ctloglists"
	ctgo "github.com/google/certificate-transparency-go"
	"github.com/google/certificate-transparency-go/loglist3"
	ctx509 "github.com/google/certificate-transparency-go/x509"
	"github.com/google/certificate-transparency-go/x509util"

	"golang.org/x/mod/sumdb/note"

	"go.uber.org/zap"
)

type CustomStaticLogCheckpoint struct {
	SubmissionURL string
	MonitoringURL string
	KeyName       string
	NoteVerifiers note.Verifiers
}

type customStaticTestLogEntry struct {
	operator string
	tiledLog *loglist3.TiledLog
}

type customStaticTestLogConfig struct {
	Operator         string
	Name             string
	SubmissionURL    string
	MonitoringURL    string
	CheckpointOrigin string
	MMD              int32
	PublicKeyFile    string
	AcceptedRootsDir string
}

var (
	customStaticTestLogs       []customStaticTestLogEntry
	customStaticAcceptedRoots  = map[[sha256.Size]byte]*x509util.PEMCertPool{}
	customStaticSCTVerifiers   = map[[sha256.Size]byte]*ctgo.SignatureVerifier{}
	customStaticCheckpoints    = map[string]CustomStaticLogCheckpoint{}
	customStaticSubmissionURLs = map[string]bool{}
)

func loadCustomStaticTestLogs() {
	if !config.Config.CustomStaticTestLogs.Enabled {
		return
	}
	if len(config.Config.CustomStaticTestLogs.Logs) == 0 {
		logger.Logger.Fatal("customStaticTestLogs.enabled is true, but no logs are configured")
	}

	for idx, cfg := range config.Config.CustomStaticTestLogs.Logs {
		logConfig := customStaticTestLogConfig{
			Operator:         cfg.Operator,
			Name:             cfg.Name,
			SubmissionURL:    cfg.SubmissionURL,
			MonitoringURL:    cfg.MonitoringURL,
			CheckpointOrigin: cfg.CheckpointOrigin,
			MMD:              cfg.MMD,
			PublicKeyFile:    cfg.PublicKeyFile,
			AcceptedRootsDir: cfg.AcceptedRootsDir,
		}
		tiledLog, checkpoint, roots, sctVerifier, err := buildCustomStaticTestLog(logConfig)
		if err != nil {
			logger.Logger.Fatal("Failed to load custom static test log", zap.Int("index", idx), zap.Error(err))
		}

		logID := logIDFromBytes(tiledLog.LogID)
		if _, ok := customStaticAcceptedRoots[logID]; ok {
			logger.Logger.Fatal("Duplicate custom static test log ID", zap.String("logName", tiledLog.Description))
		}

		customStaticTestLogs = append(customStaticTestLogs, customStaticTestLogEntry{operator: cfg.Operator, tiledLog: tiledLog})
		customStaticAcceptedRoots[logID] = roots
		customStaticSCTVerifiers[logID] = sctVerifier
		customStaticCheckpoints[checkpoint.MonitoringURL] = checkpoint
		customStaticSubmissionURLs[checkpoint.SubmissionURL] = true
		RegisterLogName(logID, cfg.Operator, cfg.Name)

		logger.Logger.Info("Loaded custom static test log", zap.String("operator", cfg.Operator), zap.String("logName", cfg.Name), zap.String("submissionURL", checkpoint.SubmissionURL), zap.String("monitoringURL", checkpoint.MonitoringURL))
	}

	if config.Config.CustomStaticTestLogs.IncludeWithPublicTestLogs {
		appendCustomStaticTestLogs(TestTLSLogs)
	}
}

func buildCustomStaticTestLog(cfg customStaticTestLogConfig) (*loglist3.TiledLog, CustomStaticLogCheckpoint, *x509util.PEMCertPool, *ctgo.SignatureVerifier, error) {
	if strings.TrimSpace(cfg.Operator) == "" {
		return nil, CustomStaticLogCheckpoint{}, nil, nil, fmt.Errorf("operator is required")
	}
	if strings.TrimSpace(cfg.Name) == "" {
		return nil, CustomStaticLogCheckpoint{}, nil, nil, fmt.Errorf("name is required")
	}
	if strings.TrimSpace(cfg.SubmissionURL) == "" {
		return nil, CustomStaticLogCheckpoint{}, nil, nil, fmt.Errorf("submissionURL is required")
	}
	if strings.TrimSpace(cfg.MonitoringURL) == "" {
		return nil, CustomStaticLogCheckpoint{}, nil, nil, fmt.Errorf("monitoringURL is required")
	}
	if strings.TrimSpace(cfg.CheckpointOrigin) == "" {
		return nil, CustomStaticLogCheckpoint{}, nil, nil, fmt.Errorf("checkpointOrigin is required")
	}
	if cfg.MMD <= 0 {
		return nil, CustomStaticLogCheckpoint{}, nil, nil, fmt.Errorf("mmd must be positive")
	}

	submissionURL, err := normalizeBaseURL(cfg.SubmissionURL)
	if err != nil {
		return nil, CustomStaticLogCheckpoint{}, nil, nil, fmt.Errorf("invalid submissionURL: %w", err)
	}
	monitoringURL, err := normalizeBaseURL(cfg.MonitoringURL)
	if err != nil {
		return nil, CustomStaticLogCheckpoint{}, nil, nil, fmt.Errorf("invalid monitoringURL: %w", err)
	}

	keyDER, pubKey, err := loadPublicKey(cfg.PublicKeyFile)
	if err != nil {
		return nil, CustomStaticLogCheckpoint{}, nil, nil, err
	}
	sctVerifier, err := ctgo.NewSignatureVerifier(pubKey)
	if err != nil {
		return nil, CustomStaticLogCheckpoint{}, nil, nil, fmt.Errorf("failed to create SCT signature verifier: %w", err)
	}
	checkpointVerifier, err := sunlight.NewRFC6962Verifier(cfg.CheckpointOrigin, pubKey)
	if err != nil {
		return nil, CustomStaticLogCheckpoint{}, nil, nil, fmt.Errorf("failed to create checkpoint verifier: %w", err)
	}

	roots, err := loadAcceptedRoots(cfg.AcceptedRootsDir)
	if err != nil {
		return nil, CustomStaticLogCheckpoint{}, nil, nil, err
	}

	logID := sha256.Sum256(keyDER)
	tiledLog := &loglist3.TiledLog{
		Description:   cfg.Name,
		LogID:         logID[:],
		Key:           keyDER,
		SubmissionURL: submissionURL,
		MonitoringURL: monitoringURL,
		MMD:           cfg.MMD,
		Type:          "test",
	}
	checkpoint := CustomStaticLogCheckpoint{
		SubmissionURL: submissionURL,
		MonitoringURL: monitoringURL,
		KeyName:       cfg.CheckpointOrigin,
		NoteVerifiers: note.VerifierList(checkpointVerifier),
	}

	return tiledLog, checkpoint, roots, sctVerifier, nil
}

func normalizeBaseURL(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", fmt.Errorf("scheme must be http or https")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("host is required")
	}
	normalized, err := url.JoinPath(rawURL, "/")
	if err != nil {
		return "", err
	}
	return normalized, nil
}

func loadPublicKey(filename string) ([]byte, any, error) {
	if strings.TrimSpace(filename) == "" {
		return nil, nil, fmt.Errorf("publicKeyFile is required")
	}
	keyDER, err := readDEROrPEM(filename, "PUBLIC KEY")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read publicKeyFile: %w", err)
	}
	pubKey, err := stdx509.ParsePKIXPublicKey(keyDER)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse publicKeyFile as PKIX public key: %w", err)
	}
	return keyDER, pubKey, nil
}

func loadAcceptedRoots(dirname string) (*x509util.PEMCertPool, error) {
	if strings.TrimSpace(dirname) == "" {
		return nil, fmt.Errorf("acceptedRootsDir is required")
	}
	entries, err := os.ReadDir(dirname)
	if err != nil {
		return nil, fmt.Errorf("failed to read acceptedRootsDir: %w", err)
	}

	pool := x509util.NewPEMCertPool()
	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dirname, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read accepted root %s: %w", path, err)
		}
		certs, err := parseCertificates(data)
		if err != nil {
			return nil, fmt.Errorf("failed to parse accepted root %s: %w", path, err)
		}
		for _, cert := range certs {
			pool.AddCert(cert)
			count++
		}
	}
	if count == 0 {
		return nil, fmt.Errorf("acceptedRootsDir contains no accepted root certificates")
	}
	return pool, nil
}

func readDEROrPEM(filename, blockType string) ([]byte, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	rest := data
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type == blockType {
			return block.Bytes, nil
		}
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("file is empty")
	}
	return data, nil
}

func parseCertificates(data []byte) ([]*ctx509.Certificate, error) {
	var certs []*ctx509.Certificate
	rest := data
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := ctx509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, err
		}
		certs = append(certs, cert)
	}
	if len(certs) > 0 {
		return certs, nil
	}

	cert, err := ctx509.ParseCertificate(data)
	if err != nil {
		return nil, err
	}
	return []*ctx509.Certificate{cert}, nil
}

func appendCustomStaticTestLogs(logList *loglist3.LogList) {
	if logList == nil || len(customStaticTestLogs) == 0 {
		return
	}
	for _, customLog := range customStaticTestLogs {
		operator := findOperator(logList, customLog.operator)
		if operator == nil {
			operator = &loglist3.Operator{Name: customLog.operator}
			logList.Operators = append(logList.Operators, operator)
		}
		tiledLogCopy := *customLog.tiledLog
		operator.TiledLogs = append(operator.TiledLogs, &tiledLogCopy)
	}
}

func findOperator(logList *loglist3.LogList, name string) *loglist3.Operator {
	for _, operator := range logList.Operators {
		if operator.Name == name {
			return operator
		}
	}
	return nil
}

func logIDFromBytes(logID []byte) [sha256.Size]byte {
	var keyID [sha256.Size]byte
	copy(keyID[:], logID)
	return keyID
}

func CustomAcceptedRoots(logID [sha256.Size]byte) (*x509util.PEMCertPool, bool) {
	roots, ok := customStaticAcceptedRoots[logID]
	return roots, ok
}

func CustomSCTVerifier(logID [sha256.Size]byte) (*ctgo.SignatureVerifier, bool) {
	verifier, ok := customStaticSCTVerifiers[logID]
	return verifier, ok
}

func CustomCheckpoint(monitoringURL string) (CustomStaticLogCheckpoint, bool) {
	checkpoint, ok := customStaticCheckpoints[monitoringURL]
	return checkpoint, ok
}

func IsCustomSubmissionURL(submissionURL string) bool {
	return customStaticSubmissionURLs[submissionURL]
}

func LogsForMonitoring() *loglist3.LogList {
	logList := &loglist3.LogList{}
	for _, operator := range ctloglists.CrtshV3Active.Operators {
		op := *operator
		op.Logs = append([]*loglist3.Log{}, operator.Logs...)
		op.TiledLogs = append([]*loglist3.TiledLog{}, operator.TiledLogs...)
		logList.Operators = append(logList.Operators, &op)
	}
	appendCustomStaticTestLogs(logList)
	return logList
}
