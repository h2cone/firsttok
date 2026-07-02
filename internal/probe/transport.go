package probe

import (
	"crypto/tls"
	"net/http"
)

// newTransport builds an HTTP transport that disables transparent compression
// (so decompression cannot pollute first-byte timing) and optionally skips TLS
// certificate verification.
func newTransport(verifySSL bool) http.RoundTripper {
	t := &http.Transport{
		DisableCompression: true,
		Proxy:              http.ProxyFromEnvironment,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: !verifySSL,
		},
	}
	return t
}
