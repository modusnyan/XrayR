package controller

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"time"
)

// certificateDaysRemaining returns how long the leaf certificate in a PEM file
// stays valid. Bundles are scanned until the first CERTIFICATE block, so a file
// that also carries an intermediate chain still reports the leaf.
func certificateDaysRemaining(path string) (float64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	for {
		var block *pem.Block
		block, data = pem.Decode(data)
		if block == nil {
			return 0, errors.New("no certificate found in PEM file")
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return 0, fmt.Errorf("parse certificate: %w", err)
		}
		return time.Until(certificate.NotAfter).Hours() / 24, nil
	}
}
