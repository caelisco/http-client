package response

import (
	"crypto/tls"
	"net/http"
	"time"

	"github.com/caelisco/http-client/options"
)

// Response represents the HTTP response along with additional metadata
type Response struct {
	UniqueIdentifier string                    // Unique ID for the request, generated internally
	URL              string                    // URL the request was made to
	Method           string                    // HTTP method used (e.g., GET, POST)
	RequestPayload   any                       // Payload sent with the request
	Options          *options.Option           // Configuration options for the request
	RequestTime      int64                     // Timestamp of when the request was initiated
	ResponseTime     int64                     // Timestamp of when the response was received
	ProcessedTime    int64                     // Duration taken to process the request
	Status           string                    // HTTP status message (e.g., "200 OK")
	StatusCode       int                       // HTTP status code (e.g., 200, 404)
	Proto            string                    // Protocol used (e.g., HTTP/1.1)
	Header           http.Header               // Headers included in the response
	ContentLength    int64                     // Length of the response content
	TransferEncoding []string                  // Transfer encoding details from the response
	CompressionType  options.CompressionType   // Type of compression applied to the response
	Uncompressed     bool                      // Indicates if the response was uncompressed
	Cookies          []*http.Cookie            // Cookies received with the response
	AccessTime       time.Duration             // Time taken to complete the request
	Body             options.WriteCloserBuffer // The response body as a buffer
	Error            error                     // Any error encountered during the request
	TLS              *tls.ConnectionState      // Details about the TLS connection
	Redirected       bool                      // Indicates if the request was redirected
	Location         string                    // New location if the request was redirected
}

// New initializes a new Response instance with basic details
func New(url string, method string, payload any, opt *options.Option) Response {
	return Response{
		UniqueIdentifier: opt.GenerateIdentifier(), // Generate unique request identifier
		URL:              url,                      // Request URL
		Method:           method,                   // HTTP method
		RequestPayload:   payload,                  // Request payload
		Options:          opt,                      // Request options
		CompressionType:  opt.Compression,          // Compression type from options
	}
}

// Bytes returns the response body as a byte slice
func (r *Response) Bytes() []byte {
	if r.Body.IsEmpty() {
		return nil
	}
	return r.Body.Bytes()
}

// String returns the response body as a string
func (r *Response) String() string {
	if r.Body.IsEmpty() {
		return ""
	}
	return r.Body.String()
}

// Len returns the length of the response body
// If there is no body, it returns -1 to indicate there is
// an issue
func (r *Response) Len() int64 {
	if r.Body.IsEmpty() {
		return -1
	}
	return int64(r.Body.Len())
}

// PopulateResponse populates the Response struct with data from an http.Response
func (r *Response) PopulateResponse(resp *http.Response, start time.Time) {
	r.Status = resp.Status                     // Set HTTP status message
	r.StatusCode = resp.StatusCode             // Set HTTP status code
	r.Proto = resp.Proto                       // Set protocol used
	r.Header = resp.Header                     // Copy response headers
	r.TransferEncoding = resp.TransferEncoding // Copy transfer encoding
	r.Cookies = resp.Cookies()                 // Copy response cookies
	r.AccessTime = time.Since(start)           // Calculate and set access time
	r.Uncompressed = resp.Uncompressed         // Set uncompressed flag
	r.TLS = resp.TLS                           // Copy TLS connection state

	// Check and record if the request was redirected
	if len(resp.Request.URL.String()) != len(r.URL) {
		r.Redirected = true                    // Mark as redirected
		r.Location = resp.Request.URL.String() // Set the new location
	}
}
