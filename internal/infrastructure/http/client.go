package http

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

// Client defines the interface for HTTP operations
type Client interface {
	Do(req *http.Request) (*http.Response, error)
}

// HTTPClient is an optimized HTTP client implementation
type HTTPClient struct {
	*http.Client
}

// NewHTTPClient creates a new optimized HTTP client
func NewHTTPClient() Client {
	tr := &http.Transport{
		TLSClientConfig:   &tls.Config{MinVersion: tls.VersionTLS12},
		ForceAttemptHTTP2: false,
		MaxIdleConns:      100,
		IdleConnTimeout:   3600 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   3600 * time.Second,
			KeepAlive: 3600 * time.Second,
		}).DialContext,
	}

	return &HTTPClient{
		Client: &http.Client{
			Timeout:   3600 * time.Second,
			Transport: tr,
		},
	}
}

