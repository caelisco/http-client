package response

import (
	"crypto/tls"
	"net/http"
	"time"

	"github.com/caelisco/http-client/options"
)

// Response represents the HTTP response along with additional details.
type Response struct {
	UniqueIdentifier string                    // Internally generated UUID for the request
	URL              string                    // URL of the request
	Method           string                    // HTTP method of the request
	RequestPayload   any                       // Payload of the request
	Options          *options.Option           // Additional options for the request
	RequestTime      int64                     // The time when the request was made
	ResponseTime     int64                     // The time when the response was received
	ProcessedTime    int64                     // The time taken for the request to be processed
	Status           string                    // Status of the HTTP response
	StatusCode       int                       // HTTP status code of the response
	Proto            string                    // HTTP protocol used
	Header           http.Header               // HTTP headers of the response
	ContentLength    int64                     // Content length from the response
	TransferEncoding []string                  // Transfer encoding of the response
	CompressionType  options.CompressionType   // Type of compression
	Uncompressed     bool                      // Was the response compressed - https://pkg.go.dev/net/http#Response.Uncompressed
	Cookies          []*http.Cookie            // Cookies received in the response
	AccessTime       time.Duration             // Time taken to complete the request
	Body             options.WriteCloserBuffer // Response body as bytes
	Error            error                     // Error encountered during the request
	TLS              *tls.ConnectionState      // TLS connection state
	Redirected       bool                      // Was the request redirected
	Location         string                    // If redirected, what was the location
}

func New(url string, method string, payload any, opt *options.Option) Response {
	return Response{
		UniqueIdentifier: opt.GenerateIdentifier(),
		URL:              url,
		Method:           method,
		RequestPayload:   payload,
		Options:          opt,
		CompressionType:  opt.Compression,
	}
}

// Bytes is a helper function to get the underlying bytes.Buffer []byte
func (r *Response) Bytes() []byte {
	return r.Body.Bytes()
}

// String is a helper function to get the underlying bytes.Buffer string
func (r *Response) String() string {
	return r.Body.String()
}

func (r *Response) Length() int64 {
	return int64(r.Body.Len())
}

func (r *Response) PopulateResponse(resp *http.Response, start time.Time) {
	r.Status = resp.Status
	r.StatusCode = resp.StatusCode
	r.Proto = resp.Proto
	r.Header = resp.Header
	r.TransferEncoding = resp.TransferEncoding
	// store cookies from the response
	r.Cookies = resp.Cookies()
	r.AccessTime = time.Since(start)
	r.Uncompressed = resp.Uncompressed
	r.TLS = resp.TLS

	// Check for redirects
	if len(resp.Request.URL.String()) != len(r.URL) {
		r.Redirected = true
		r.Location = resp.Request.URL.String()
	}
}
