package loglists

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	stdx509 "crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBuildCustomStaticTestLog(t *testing.T) {
	dir := t.TempDir()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	publicKeyDER, err := stdx509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey() error = %v", err)
	}
	publicKeyPath := filepath.Join(dir, "log-public-key.pem")
	if err := os.WriteFile(publicKeyPath, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicKeyDER}), 0o600); err != nil {
		t.Fatalf("WriteFile(public key) error = %v", err)
	}

	rootDER := selfSignedRootDER(t, key)
	rootsDir := filepath.Join(dir, "roots")
	if err := os.Mkdir(rootsDir, 0o700); err != nil {
		t.Fatalf("Mkdir(roots) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(rootsDir, "root.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER}), 0o600); err != nil {
		t.Fatalf("WriteFile(root) error = %v", err)
	}

	tiledLog, checkpoint, roots, verifier, err := buildCustomStaticTestLog(customStaticTestLogConfig{
		Operator:         "Test Operator",
		Name:             "Test Static Log",
		SubmissionURL:    "https://static-test.example.com/submission",
		MonitoringURL:    "https://static-test.example.com/monitoring",
		CheckpointOrigin: "static-test.example.com",
		MMD:              86400,
		PublicKeyFile:    publicKeyPath,
		AcceptedRootsDir: rootsDir,
	})
	if err != nil {
		t.Fatalf("buildCustomStaticTestLog() error = %v", err)
	}

	wantLogID := sha256.Sum256(publicKeyDER)
	if !bytes.Equal(tiledLog.LogID, wantLogID[:]) {
		t.Fatalf("LogID = %x, want %x", tiledLog.LogID, wantLogID)
	}
	if tiledLog.Description != "Test Static Log" {
		t.Fatalf("Description = %q", tiledLog.Description)
	}
	if tiledLog.Type != "test" {
		t.Fatalf("Type = %q, want test", tiledLog.Type)
	}
	if checkpoint.KeyName != "static-test.example.com" {
		t.Fatalf("KeyName = %q", checkpoint.KeyName)
	}
	if roots == nil || len(roots.RawCertificates()) != 1 {
		t.Fatalf("accepted roots count = %d, want 1", len(roots.RawCertificates()))
	}
	if verifier == nil {
		t.Fatal("verifier is nil")
	}
}

func selfSignedRootDER(t *testing.T, key *ecdsa.PrivateKey) []byte {
	t.Helper()

	template := &stdx509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test Root"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              stdx509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := stdx509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate() error = %v", err)
	}
	return der
}
