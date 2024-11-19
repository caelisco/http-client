package request

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/caelisco/http-client/kv"
	"github.com/google/uuid"
	"github.com/oklog/ulid/v2"
)

type CompressionType string
type UniqueIdentifierType string

const (
	CompressionNone    CompressionType = ""
	CompressionGzip    CompressionType = "gzip"
	CompressionDeflate CompressionType = "deflate"
	CompressionBrotli  CompressionType = "br"
	// Add other compression types as needed
)

const (
	IdentifierNone UniqueIdentifierType = ""
	IdentifierUUID UniqueIdentifierType = "uuid"
	IdentifierULID UniqueIdentifierType = "ulid"
)

// RequestOptions represents additional options for the HTTP request.
//
// DisableRedirect - Determines if redirects should be followed or not. The default option is
// false which means redirects will be followed.
type Options struct {
	Headers          []kv.Header          // Custom headers to be added to the request
	Cookies          []*http.Cookie       // Cookies to be included in the request
	ProtocolScheme   string               // define a custom protocol scheme. It defaults to https
	Compression      CompressionType      // CompressionType to use: none, gzip, deflate or brotli
	UserAgent        string               // User Agent to send with requests
	DisableRedirect  bool                 // Disable or enable redirects. Default is false - do not disable redirects
	UniqueIdentifier UniqueIdentifierType // Internal trace or identifier for the request
	Writer           io.WriteCloser       // Define a custom resource you will write to other than the bytes.Buffer i.e.: a file
}

func NewOptions() Options {
	return Options{UniqueIdentifier: IdentifierULID}
}

// AddHeader adds a new header to the RequestOptions.
func (opt *Options) AddHeader(key string, value string) {
	if opt.Headers == nil {
		opt.Headers = []kv.Header{}
	}
	opt.Headers = append(opt.Headers, kv.Header{Key: key, Value: value})
}

// ListHeaders prints out the list of headers in the RequestOptions.
func (opt *Options) ListHeaders() {
	for _, h := range opt.Headers {
		fmt.Printf("%s=%s", h.Key, h.Value)
	}
}

// ClearHeaders clears all headers in the RequestOptions.
func (opt *Options) ClearHeaders() {
	opt.Headers = nil
}

// AddCookie adds a new cookie to the RequestOptions.
func (opt *Options) AddCookie(cookie *http.Cookie) {
	if opt.Cookies == nil {
		opt.Cookies = []*http.Cookie{}
	}
	opt.Cookies = append(opt.Cookies, cookie)
}

// ListCookies prints out the list of cookies in the RequestOptions.
func (opt *Options) ListCookies() {
	for _, c := range opt.Cookies {
		fmt.Printf("%s=%s", c.Name, c.Value)
	}
}

// ClearCookies clears all cookies in the RequestOptions.
func (opt *Options) ClearCookies() {
	opt.Cookies = nil
}

func (opt *Options) SetProtocolScheme(scheme string) {
	if !strings.Contains(scheme, "://") {
		scheme += "://"
	}
	opt.ProtocolScheme = scheme
}

func (opt *Options) Compress(compressionType CompressionType) {
	opt.Compression = compressionType
}

func (opt *Options) DisableRedirects() bool {
	return true
}

func (opt *Options) EnableRedirects() bool {
	return false
}

func (opt *Options) GenerateIdentifier() string {
	switch opt.UniqueIdentifier {
	case IdentifierUUID:
		return uuid.New().String()
	case IdentifierULID:
		return ulid.Make().String()
	}
	return ""
}

func (opt *Options) FileWriter(filename string) error {
	var err error
	opt.Writer, err = os.Create(filename)
	if err != nil {
		return err
	}
	return nil
}

func (opt *Options) Merge(src Options) {
	// Merge headers
	for _, sh := range src.Headers {
		found := false
		for i, th := range opt.Headers {
			if th.Key == sh.Key {
				opt.Headers[i] = sh
				found = true
				break
			}
		}
		if !found {
			opt.Headers = append(opt.Headers, sh)
		}
	}

	// Merge cookies
	for _, sc := range src.Cookies {
		found := false
		for i, tc := range opt.Cookies {
			if tc.Name == sc.Name {
				opt.Cookies[i] = sc
				found = true
				break
			}
		}
		if !found {
			opt.Cookies = append(opt.Cookies, sc)
		}
	}

	// Merge other fields, source takes priority if not empty
	if src.UniqueIdentifier != "" {
		opt.UniqueIdentifier = src.UniqueIdentifier
	}
	if src.Compression != "" {
		opt.Compression = src.Compression
	}
	if src.UserAgent != "" {
		opt.UserAgent = src.UserAgent
	}
	if src.ProtocolScheme != "" {
		opt.ProtocolScheme = src.ProtocolScheme
	}

	// DisableRedirect is a boolean, so we always take the source value
	opt.DisableRedirect = src.DisableRedirect

	if src.Writer != nil {
		opt.Writer = src.Writer
	}
}
