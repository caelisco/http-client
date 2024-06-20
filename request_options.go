package client

import (
	"fmt"
	"net/http"
	"strings"
)

type CompressionType string

const (
	CompressionNone    CompressionType = ""
	CompressionGzip    CompressionType = "gzip"
	CompressionDeflate CompressionType = "deflate"
	CompressionBrotli  CompressionType = "br"
	// Add other compression types as needed
)

// RequestOptions represents additional options for the HTTP request.
//
// DisableRedirect - Determines if redirects should be followed or not. The default option is
// false which means redirects will be followed.
type RequestOptions struct {
	Headers         []Header        // Custom headers to be added to the request.
	Cookies         []*http.Cookie  // Cookies to be included in the request.
	ProtocolScheme  string          // define a custom protocol scheme. It defaults to https
	Compression     CompressionType // CompressionType to use: none, gzip, deflate or brotli
	UserAgent       string          // User Agent to send with requests
	DisableRedirect bool            // Disable or enable redirects. Default is false - do not disable redirects
}

// AddHeader adds a new header to the RequestOptions.
func (opt *RequestOptions) AddHeader(key string, value string) {
	if opt.Headers == nil {
		opt.Headers = []Header{}
	}
	opt.Headers = append(opt.Headers, Header{Key: key, Value: value})
}

// ListHeaders prints out the list of headers in the RequestOptions.
func (opt *RequestOptions) ListHeaders() {
	for _, h := range opt.Headers {
		fmt.Printf("%s=%s", h.Key, h.Value)
	}
}

// ClearHeaders clears all headers in the RequestOptions.
func (opt *RequestOptions) ClearHeaders() {
	opt.Headers = nil
}

// AddCookie adds a new cookie to the RequestOptions.
func (opt *RequestOptions) AddCookie(cookie *http.Cookie) {
	if opt.Cookies == nil {
		opt.Cookies = []*http.Cookie{}
	}
	opt.Cookies = append(opt.Cookies, cookie)
}

// ListCookies prints out the list of cookies in the RequestOptions.
func (opt *RequestOptions) ListCookies() {
	for _, c := range opt.Cookies {
		fmt.Printf("%s=%s", c.Name, c.Value)
	}
}

// ClearCookies clears all cookies in the RequestOptions.
func (opt *RequestOptions) ClearCookies() {
	opt.Cookies = nil
}

func (opt *RequestOptions) SetProtocolScheme(scheme string) {
	if !strings.Contains(scheme, "://") {
		scheme += "://"
	}
	opt.ProtocolScheme = scheme
}

func (opt *RequestOptions) Compress(compressionType CompressionType) {
	opt.Compression = compressionType
}

func (opt *RequestOptions) DisableRedirects() bool {
	return true
}

func (opt *RequestOptions) EnableRedirects() bool {
	return false
}
