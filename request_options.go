package client

import (
	"fmt"
	"net/http"
)

// RequestOptions represents additional options for the HTTP request.
type RequestOptions struct {
	Headers        []Header       // Custom headers to be added to the request.
	Cookies        []*http.Cookie // Cookies to be included in the request.
	ProtocolScheme string
}

// AddHeader adds a new header to the RequestOptions.
func (opt *RequestOptions) AddHeader(key string, value string) {
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
