package pki

import (
	"math/big"

	"github.com/google/certificate-transparency-go/asn1"
	"github.com/google/certificate-transparency-go/x509"
	"github.com/google/certificate-transparency-go/x509/pkix"
)

type tbsCertificatePartial struct {
	Version            int `asn1:"optional,explicit,default:0,tag:0"`
	SerialNumber       *big.Int
	SignatureAlgorithm pkix.AlgorithmIdentifier
}

func MakeDummyCertificate(tbsCertificate []byte) (*x509.Certificate, error) {
	// Decode enough of the TBSCertificate to discover the signature algorithm.
	var tbs tbsCertificatePartial
	var err error
	if _, err = asn1.Unmarshal(tbsCertificate, &tbs); err != nil {
		return nil, err
	}

	// Wrap the TBSCertificate in a dummy signature.
	var derCert []byte
	derCert, err = dummySign(tbsCertificate, tbs.SignatureAlgorithm)
	if err != nil {
		return nil, err
	}

	return x509.ParseCertificate(derCert)
}
