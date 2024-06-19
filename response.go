package client

import (
	"bytes"
	"crypto/tls"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// Response represents the HTTP response along with additional details.
type Response struct {
	UUID             uuid.UUID
	URL              string         // URL of the request.
	Method           string         // HTTP method of the request.
	RequestPayload   []byte         // Payload of the request.
	Options          RequestOptions // Additional options for the request.
	RequestTime      int64          // The time when the request was made
	Status           string         // Status of the HTTP response.
	StatusCode       int            // HTTP status code of the response.
	Proto            string         // HTTP protocol used.
	Header           http.Header    // HTTP headers of the response.
	ContentLength    int64
	TransferEncoding []string // Transfer encoding of the response
	CompressionType  CompressionType
	Uncompressed     bool
	Cookies          []*http.Cookie // Cookies received in the response.
	AccessTime       time.Duration  // Time taken to complete the request.
	Body             bytes.Buffer   // Response body as bytes.
	Error            error          // Error encountered during the request.
	TLS              *tls.ConnectionState
}

func (r *Response) Retry(c ...http.Client) (Response, error) {
	// It is possible to supply your own client to the retry method. It is designed to allow
	// the Client to retry using the Clients http.Client which is unexported.
	if len(c) > 0 {
		client = &c[0]
	}
	return doRequest(client, r.Method, r.URL, r.RequestPayload, r.Options)
}
