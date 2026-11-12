// SPDX-FileCopyrightText: 2026 OVH SAS
//
// SPDX-License-Identifier: Apache-2.0

package ovh

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"
)

// newHardenedTransport returns the *http.Transport used for SDK requests.
//
// Every value mirrors http.DefaultTransport, with two deliberate differences:
//
//   - TLSClientConfig pins MinVersion to TLS 1.2. Go already refuses to offer anything below
//     1.2 as a client; this holds the floor if a future Go default relaxes.
//   - MaxIdleConnsPerHost is raised from Go's default of 2 to 10. An SDK talks to a single API
//     host, where a cap of 2 forces needless connection churn under concurrency.
func newHardenedTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}
}

// newHTTPClient returns the *http.Client used for SDK requests.
//
// It deliberately does not return http.DefaultClient: any other package can swap that global's
// transport out from under the SDK, and a shared client cannot carry an SDK-specific timeout.
func newHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: newHardenedTransport(),
		Timeout:   timeout,
	}
}

// ownTransport returns the *http.Transport hc drives, with a TLS config ready to be modified.
//
// It fails rather than doing nothing when the transport is not one we can configure. A silent
// no-op would be dangerous: a caller who sets a client certificate and gets no error will
// believe mTLS is active when it is not.
func ownTransport(hc *http.Client) (*http.Transport, error) {
	if hc.Transport == nil {
		return nil, fmt.Errorf("ovh: HTTP client has no Transport, so requests would use the shared http.DefaultTransport; set one before configuring TLS or proxy")
	}
	transport, ok := hc.Transport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("ovh: HTTP client Transport is %T, not *http.Transport; configure TLS and proxy on it directly", hc.Transport)
	}
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	return transport, nil
}
