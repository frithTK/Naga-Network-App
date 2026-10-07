package subscription

import (
	"crypto/x509"
	_ "embed"
)

// Public Let's Encrypt trust anchors. Windows 10 without CA updates often
// lacks ISRG Root X2 (YE2 → Root YE → X2).
//
//go:embed certs/isrg-root-x1.pem
var isrgRootX1PEM []byte

//go:embed certs/isrg-root-x2.pem
var isrgRootX2PEM []byte

func subscriptionRootCAs() *x509.CertPool {
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	pool.AppendCertsFromPEM(isrgRootX1PEM)
	pool.AppendCertsFromPEM(isrgRootX2PEM)
	return pool
}
