// SPDX-FileCopyrightText: 2026 OVH SAS
//
// SPDX-License-Identifier: Apache-2.0

// Package ovh is the core of the OVHcloud Go SDK: authentication, transport and the request
// machinery every API section shares.
//
// Create one Client and hand it to the section packages under pkg/:
//
//	c, err := ovh.NewClient() // credentials from OVH_CLIENT_ID / OVH_CLIENT_SECRET
//	if err != nil { ... }
//	ids, err := vrack.New(c).ListVracks(ctx)
//
// Any endpoint can also be called directly with Client.Do, which is what the section packages
// do underneath.
package ovh

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

// Region selects the OVHcloud API a client talks to, and the OAuth2 token endpoint that
// issues its tokens. A token from one region is rejected by the others.
type Region string

const (
	EU Region = "eu"
	CA Region = "ca"
	US Region = "us"
)

type regionEndpoints struct {
	api   string
	token string
}

// The public API hosts, which accept OAuth2 bearer tokens. (The manager hosts that publish the
// OpenAPI specifications reject them.) api.eu.ovhcloud.com and api.ca.ovhcloud.com replace
// eu.api.ovh.com and ca.api.ovh.com, which serve the same API and still answer; WithEndpoint
// selects one of them if needed. The token endpoints are unchanged: they are the ones the
// identity provider advertises in its OpenID configuration.
var regions = map[Region]regionEndpoints{
	EU: {api: "https://api.eu.ovhcloud.com", token: "https://www.ovh.com/auth/oauth2/token"},
	CA: {api: "https://api.ca.ovhcloud.com", token: "https://ca.ovh.com/auth/oauth2/token"},
	US: {api: "https://api.us.ovhcloud.com", token: "https://us.ovhcloud.com/auth/oauth2/token"},
}

// Environment variables NewClient reads when no option overrides them.
const (
	EnvClientID     = "OVH_CLIENT_ID"
	EnvClientSecret = "OVH_CLIENT_SECRET"
	EnvRegion       = "OVH_REGION"
)

// sdkUserAgent identifies the SDK and its version in every request. It cannot be replaced:
// WithUserAgent only appends the caller's own product to it.
var sdkUserAgent = fmt.Sprintf("ovhcloud-go-sdk/%s (%s; %s/%s)", Version, runtime.Version(), runtime.GOOS, runtime.GOARCH)

// Client sends requests to the OVHcloud API. It is safe for concurrent use; create one and
// share it.
type Client struct {
	endpoint         string
	httpClient       *http.Client
	tokens           oauth2.TokenSource
	userAgent        string
	headers          http.Header
	retry            RetryPolicy
	maxResponseBytes int64
	debug            bool
	debugBody        bool
	// wait sleeps between retry attempts; tests replace it so they need not sleep.
	wait func(ctx context.Context, d time.Duration) error
}

// config collects what the options ask for. Options record intent here; NewClient resolves it,
// so options can be given in any order.
type config struct {
	region       Region
	endpoint     string
	clientID     string
	clientSecret string
	tokens       oauth2.TokenSource
	httpClient   *http.Client
	timeout      time.Duration
	transport    []func(*http.Transport)
	client       Client
}

// Option configures a Client. See the With* functions.
type Option func(*config) error

// NewClient returns a client for the OVHcloud API.
//
// With no options it reads its credentials from OVH_CLIENT_ID and OVH_CLIENT_SECRET (an OAuth2
// service account) and its region from OVH_REGION (eu, ca or us; eu by default). Options take
// precedence over the environment.
func NewClient(opts ...Option) (*Client, error) {
	cfg := &config{
		region:       Region(strings.ToLower(os.Getenv(EnvRegion))),
		clientID:     os.Getenv(EnvClientID),
		clientSecret: os.Getenv(EnvClientSecret),
	}
	if cfg.region == "" {
		cfg.region = EU
	}
	cfg.client.userAgent = sdkUserAgent
	cfg.client.headers = http.Header{}

	for _, opt := range opts {
		if err := opt(cfg); err != nil {
			return nil, err
		}
	}

	endpoints, ok := regions[cfg.region]
	if !ok {
		return nil, fmt.Errorf("ovh: unknown region %q: want eu, ca or us", cfg.region)
	}

	c := cfg.client
	c.wait = sleep
	c.endpoint = strings.TrimRight(endpoints.api, "/")
	if cfg.endpoint != "" {
		c.endpoint = strings.TrimRight(cfg.endpoint, "/")
	}

	c.httpClient = cfg.httpClient
	if c.httpClient == nil {
		c.httpClient = newHTTPClient(cfg.timeout)
	} else if cfg.timeout != 0 {
		c.httpClient.Timeout = cfg.timeout
	}
	if len(cfg.transport) > 0 {
		transport, err := ownTransport(c.httpClient)
		if err != nil {
			return nil, err
		}
		for _, apply := range cfg.transport {
			apply(transport)
		}
	}

	switch {
	case cfg.tokens != nil:
		c.tokens = cfg.tokens
	case cfg.clientID != "" && cfg.clientSecret != "":
		oauth := &clientcredentials.Config{
			ClientID:     cfg.clientID,
			ClientSecret: cfg.clientSecret,
			TokenURL:     endpoints.token,
			Scopes:       []string{"all"},
		}
		// Fetch tokens through the same hardened client as API calls, not http.DefaultClient.
		ctx := context.WithValue(context.Background(), oauth2.HTTPClient, c.httpClient)
		c.tokens = oauth.TokenSource(ctx)
	case cfg.clientID != "" || cfg.clientSecret != "":
		return nil, errors.New("ovh: both a client ID and a client secret are needed; only one was given")
	default:
		return nil, fmt.Errorf("ovh: no credentials: set %s and %s, or use WithOAuth2 or WithTokenSource",
			EnvClientID, EnvClientSecret)
	}

	return &c, nil
}

// WithRegion selects the API region. It also selects the OAuth2 token endpoint, which must
// match: a token issued in one region is rejected by the others with a 401 that looks exactly
// like a bad secret.
func WithRegion(r Region) Option {
	return func(c *config) error {
		c.region = Region(strings.ToLower(string(r)))
		return nil
	}
}

// WithOAuth2 authenticates with an OAuth2 service account (client credentials flow).
func WithOAuth2(clientID, clientSecret string) Option {
	return func(c *config) error {
		c.clientID, c.clientSecret = clientID, clientSecret
		return nil
	}
}

// WithTokenSource authenticates with tokens from ts, for callers who obtain tokens their own way.
func WithTokenSource(ts oauth2.TokenSource) Option {
	return func(c *config) error {
		if ts == nil {
			return errors.New("ovh: WithTokenSource: nil token source")
		}
		c.tokens = ts
		return nil
	}
}

// WithEndpoint overrides the API base URL chosen by the region, e.g. to go through a proxy or
// to test against a local server. Paths are appended to it, so it has no version suffix.
func WithEndpoint(baseURL string) Option {
	return func(c *config) error {
		u, err := url.Parse(baseURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("ovh: WithEndpoint: %q is not an absolute URL", baseURL)
		}
		c.endpoint = baseURL
		return nil
	}
}

// WithHTTPClient replaces the HTTP client. The TLS and proxy options need its Transport to be
// an *http.Transport.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *config) error {
		if hc == nil {
			return errors.New("ovh: WithHTTPClient: nil client")
		}
		c.httpClient = hc
		return nil
	}
}

// WithTimeout sets the per-request timeout, covering the whole exchange including reading the
// response body. Zero, the default, means no timeout. Combined with WithHTTPClient, it sets
// that client's Timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *config) error {
		c.timeout = d
		return nil
	}
}

// WithRetry enables retrying of transient failures. See RetryPolicy: a policy outside the
// retry limits (MaxRetryAttempts, MinRetryDelay, MaxRetryDelay) makes NewClient fail.
func WithRetry(p RetryPolicy) Option {
	return func(c *config) error {
		if err := p.validate(); err != nil {
			return fmt.Errorf("ovh: WithRetry: %w", err)
		}
		c.client.retry = p
		return nil
	}
}

// WithMaxResponseBytes caps how much of a response body is read. Reads past the cap fail with
// ErrResponseTooLarge rather than being truncated. Zero, the default, means unlimited.
func WithMaxResponseBytes(n int64) Option {
	return func(c *config) error {
		c.client.maxResponseBytes = n
		return nil
	}
}

// WithDebug logs every request and response with the standard log package. Credential-bearing
// headers are redacted and bodies are omitted.
func WithDebug() Option {
	return func(c *config) error {
		c.client.debug = true
		return nil
	}
}

// WithDebugBodies is WithDebug, with request and response bodies included.
//
// Bodies routinely carry secrets -- credentials being created, private keys, personal data --
// and are printed verbatim. Headers stay redacted. Use it only against test data or when you
// are certain the logs are not retained.
func WithDebugBodies() Option {
	return func(c *config) error {
		c.client.debug, c.client.debugBody = true, true
		return nil
	}
}

// WithUserAgent identifies the calling application, e.g. "my-tool/1.2". It is appended to the
// SDK's own User-Agent, which always comes first and names the SDK version:
//
//	ovhcloud-go-sdk/0.1.0 (go1.25.3; linux/amd64) my-tool/1.2
func WithUserAgent(product string) Option {
	return func(c *config) error {
		product = strings.TrimSpace(product)
		if product == "" || strings.ContainsAny(product, "\r\n") {
			return fmt.Errorf("ovh: WithUserAgent: %q is not a valid product", product)
		}
		c.client.userAgent = sdkUserAgent + " " + product
		return nil
	}
}

// WithHeader adds a header to every request. It cannot set User-Agent: use WithUserAgent.
func WithHeader(key, value string) Option {
	return func(c *config) error {
		if http.CanonicalHeaderKey(key) == "User-Agent" {
			return fmt.Errorf("ovh: WithHeader cannot set User-Agent: use WithUserAgent")
		}
		c.client.headers.Add(key, value)
		return nil
	}
}

// WithRootCAs replaces the certificate authorities used to verify the API's certificate, for a
// private or internal CA. It is not a way to skip verification, and no option to do that is
// offered.
func WithRootCAs(pool *x509.CertPool) Option {
	return withTransport(func(t *http.Transport) { t.TLSClientConfig.RootCAs = pool })
}

// WithRootCAsFromPEM is WithRootCAs for a PEM bundle. The bundle replaces the system trust
// store rather than adding to it.
func WithRootCAsFromPEM(pemCerts []byte) Option {
	return func(c *config) error {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pemCerts) {
			return errors.New("ovh: WithRootCAsFromPEM: no certificates found in the PEM data")
		}
		return WithRootCAs(pool)(c)
	}
}

// WithClientCertificates enables mutual TLS by presenting these certificates when asked.
func WithClientCertificates(certs ...tls.Certificate) Option {
	return withTransport(func(t *http.Transport) { t.TLSClientConfig.Certificates = certs })
}

// WithProxy sets how the proxy for a request is chosen. The default is
// http.ProxyFromEnvironment; nil disables proxying entirely, ignoring the environment.
func WithProxy(proxy func(*http.Request) (*url.URL, error)) Option {
	return withTransport(func(t *http.Transport) { t.Proxy = proxy })
}

func withTransport(apply func(*http.Transport)) Option {
	return func(c *config) error {
		c.transport = append(c.transport, apply)
		return nil
	}
}
