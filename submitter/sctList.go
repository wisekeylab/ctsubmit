package submitter

import (
	ctgo "github.com/google/certificate-transparency-go"
	"github.com/google/certificate-transparency-go/tls"
	"github.com/google/certificate-transparency-go/x509util"
)

func marshalSCTList(scts []*ctgo.SignedCertificateTimestamp) ([]byte, error) {
	sctList, err := x509util.MarshalSCTsIntoSCTList(scts)
	if err != nil {
		return nil, err
	}

	sctListBytes, err := tls.Marshal(*sctList)
	if err != nil {
		return nil, err
	}

	return sctListBytes, nil
}
