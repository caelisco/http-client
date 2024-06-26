package client

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/google/uuid"
)

// A global default client is used for all of the method-based requests.
var client = &http.Client{
	Timeout: 30 * time.Second, // Set an appropriate timeout
}

// doRequest performs the actual underlying HTTP request. RequestOptions are optional.
// If no protocol scheme is detected, it will automatically upgrade to https://
// Use RequestOptions.ProtocolScheme to define a different protocol
func doRequest(client *http.Client, method string, url string, payload []byte, options ...RequestOptions) (Response, error) {
	start := time.Now()

	var opt RequestOptions
	if len(options) > 0 {
		opt = options[0]
	}

	// Check if there is a pre-defined protocol scheme, else default to https://
	if opt.ProtocolScheme != "" {
		// Clean the protocol scheme prior to add in the new one
		// This can be used to force http:// over https://
		url = strings.TrimPrefix(url, "http://")
		url = strings.TrimPrefix(url, "https://")
		if !strings.Contains(opt.ProtocolScheme, "://") {
			opt.ProtocolScheme += "://"
		}
		if !strings.HasPrefix(url, opt.ProtocolScheme) {
			url = opt.ProtocolScheme + url
		}
	} else {
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			url = "https://" + url
		}
	}

	// Adjust the UserAgent
	if opt.UserAgent == "" {
		opt.UserAgent = "caelisco/client/1.1"
	}
	opt.AddHeader("User-Agent", opt.UserAgent)

	response := Response{
		UUID:            uuid.New(),
		URL:             url,
		Method:          method,
		RequestPayload:  payload,
		Options:         opt,
		CompressionType: opt.Compression,
	}

	var r *http.Response
	var requestPayload io.Reader
	if len(payload) > 0 {
		if opt.Compression != CompressionNone {
			var cbody bytes.Buffer
			var writer io.Writer
			switch opt.Compression {
			case CompressionGzip:
				writer = gzip.NewWriter(&cbody)
			case CompressionDeflate:
				writer = zlib.NewWriter(&cbody)
			case CompressionBrotli:
				writer = brotli.NewWriter(&cbody)
			default:
				return response, fmt.Errorf("unsupported compression type: %s", opt.Compression)
			}
			_, err := writer.Write(payload)
			if err != nil {
				return response, err
			}
			if closer, ok := writer.(io.Closer); ok {
				closer.Close()
			}
			requestPayload = &cbody
			opt.AddHeader("Content-Encoding", string(opt.Compression))
		} else {
			requestPayload = bytes.NewBuffer(payload)
		}
	}

	request, err := http.NewRequest(method, url, requestPayload)
	if err != nil {
		response.Error = err
		return response, err
	}

	// Assign headers from the RequestOptions
	for _, h := range opt.Headers {
		request.Header.Set(h.Key, h.Value)
	}

	// Assign cookies from the RequestOptions
	for _, c := range opt.Cookies {
		request.AddCookie(c)
	}

	// Configure the HTTP client to follow or not follow redirects
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if opt.DisableRedirect {
			return http.ErrUseLastResponse
		}
		return nil
	}

	response.RequestTime = time.Now().Unix()

	// Perform the actual request
	r, err = client.Do(request)

	if err != nil {
		response.Error = err
		return response, err
	}
	defer r.Body.Close()

	response.ResponseTime = time.Now().Unix()

	// Check if the request was redirected
	if len(r.Request.URL.String()) != len(response.URL) {
		response.Redirected = true
		response.Location = r.Request.URL.String()
	}

	// request has completed, add details to the response object
	response.Status = r.Status
	response.StatusCode = r.StatusCode
	response.Proto = r.Proto
	response.Header = r.Header
	response.TransferEncoding = r.TransferEncoding
	// store cookies from the response
	response.Cookies = r.Cookies()
	response.AccessTime = time.Since(start)
	response.Uncompressed = r.Uncompressed
	response.TLS = r.TLS

	// convert the http.Response.Body to a bytes.Buffer
	// bytes.Buffer was a preferred choice because I found it to be more flexible than
	// returning []]byte
	// To retrieve a string: response.Body.String()
	// To retrieve a []byte: response.Body.Bytes()
	_, err = io.Copy(&response.Body, r.Body)
	if err != nil {
		response.Error = err
		return response, err
	}

	return response, nil
}

// Get performs an HTTP GET request to the specified URL.
// It accepts the URL string as its first argument.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func Get(url string, opt ...RequestOptions) (Response, error) {
	return doRequest(client, http.MethodGet, url, nil, opt...)
}

// Post performs an HTTP POST request to the specified URL with the given payload.
// It accepts the URL string as its first argument and the payload as the second argument.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func Post(url string, payload []byte, opt ...RequestOptions) (Response, error) {
	return doRequest(client, http.MethodPost, url, payload, opt...)
}

// Put performs an HTTP PUT request to the specified URL with the given payload.
// It accepts the URL string as its first argument and the payload as the second argument.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func Put(url string, payload []byte, opt ...RequestOptions) (Response, error) {
	return doRequest(client, http.MethodPut, url, payload, opt...)
}

// Patch performs an HTTP PATCH request to the specified URL with the given payload.
// It accepts the URL string as its first argument and the payload as the second argument.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func Patch(url string, payload []byte, opt ...RequestOptions) (Response, error) {
	return doRequest(client, http.MethodPatch, url, payload, opt...)
}

// Delete performs an HTTP DELETE request to the specified URL.
// It accepts the URL string as its first argument.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func Delete(url string, opt ...RequestOptions) (Response, error) {
	return doRequest(client, http.MethodDelete, url, nil, opt...)
}

// Connect performs an HTTP CONNECT request to the specified URL.
// It accepts the URL string as its first argument.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func Connect(url string, opt ...RequestOptions) (Response, error) {
	return doRequest(client, http.MethodConnect, url, nil, opt...)
}

// Head performs an HTTP HEAD request to the specified URL.
// It accepts the URL string as its first argument.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func Head(url string, opt ...RequestOptions) (Response, error) {
	return doRequest(client, http.MethodHead, url, nil, opt...)
}

// Options performs an HTTP OPTIONS request to the specified URL.
// It accepts the URL string as its first argument.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func Options(url string, opt ...RequestOptions) (Response, error) {
	return doRequest(client, http.MethodHead, url, nil, opt...)
}

// Trace performs an HTTP TRACE request to the specified URL.
// It accepts the URL string as its first argument.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func Trace(url string, opt ...RequestOptions) (Response, error) {
	return doRequest(client, http.MethodTrace, url, nil, opt...)
}

// Custom performs a custom HTTP request method to the specified URL with the given payload.
// It accepts the HTTP method as its first argument, the URL string as the second argument,
// the payload as the third argument, and optionally additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func Custom(method string, url string, payload []byte, opt ...RequestOptions) (Response, error) {
	return doRequest(client, method, url, payload, opt...)
}
