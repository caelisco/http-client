package response

import (
	"bytes"
	"crypto/tls"
	"net/http"
	"time"

	"github.com/caelisco/http-client/request"
)

// Response represents the HTTP response along with additional details.
type Response struct {
	UniqueIdentifier string                  // Internally generated UUID for the request
	URL              string                  // URL of the request
	Method           string                  // HTTP method of the request
	RequestPayload   []byte                  // Payload of the request
	Options          request.Options         // Additional options for the request
	RequestTime      int64                   // The time when the request was made
	ResponseTime     int64                   // The time when the response was received
	Status           string                  // Status of the HTTP response
	StatusCode       int                     // HTTP status code of the response
	Proto            string                  // HTTP protocol used
	Header           http.Header             // HTTP headers of the response
	ContentLength    int64                   // Content length from the response
	TransferEncoding []string                // Transfer encoding of the response
	CompressionType  request.CompressionType // Type of compression
	Uncompressed     bool                    // Was the response compressed - https://pkg.go.dev/net/http#Response.Uncompressed
	Cookies          []*http.Cookie          // Cookies received in the response
	AccessTime       time.Duration           // Time taken to complete the request
	Body             bytes.Buffer            // Response body as bytes
	Error            error                   // Error encountered during the request
	TLS              *tls.ConnectionState    // TLS connection state
	Redirected       bool                    // Was the request redirected
	Location         string                  // If redirected, what was the location
}

// Bytes is a helper function to get the underlying bytes.Buffer []byte
func (r *Response) Bytes() []byte {
	return r.Body.Bytes()
}

// String is a helper function to get the underlying bytes.Buffer string
func (r *Response) String() string {
	return r.Body.String()
}

func (r *Response) Length() int {
	return r.Body.Len()
}
