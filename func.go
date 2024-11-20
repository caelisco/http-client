package client

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/caelisco/http-client/form"
	"github.com/caelisco/http-client/request"
	"github.com/caelisco/http-client/response"
)

const (
	useragent          = "caelisco/http-client/v0.4.0"
	SchemeHTTP  string = "http://"
	SchemeHTTPS string = "https://"
	SchemeWS    string = "ws://"
	SchemeWSS   string = "wss://"
)

// A global default client is used for all of the method-based requests.
var client = &http.Client{
	Timeout: 30 * time.Second, // Set an appropriate timeout
}

// doRequest performs the actual underlying HTTP request. RequestOptions are optional.
// If no protocol scheme is detected, it will automatically upgrade to https://
// Use RequestOptions.ProtocolScheme to define a different protocol
func doRequest(client *http.Client, method string, url string, payload []byte, options ...request.Options) (Response, error) {
	start := time.Now()

	// If no request.Options was passed through, create a default instance
	var opt RequestOptions
	if len(options) == 0 {
		opt = request.NewOptions()
	} else {
		opt = options[0]
	}

	// Check if there is a pre-defined protocol scheme, else default to https://
	url, err := normaliseURL(url, opt.ProtocolScheme)
	if err != nil {
		return response.Response{}, fmt.Errorf("supplied url did not pass url.Parse(): %w", err)
	}

	// Adjust the UserAgent
	if opt.UserAgent == "" {
		opt.UserAgent = useragent
	}
	opt.AddHeader("User-Agent", opt.UserAgent)

	// build the initial Response object
	response := response.New(url, method, payload, opt)

	var requestPayload io.Reader
	// Assuming there is a payload, check the options to see if compression is required
	// Apply the compression to the payload and set the appropriate header to inform
	// the server it is receiving compressed data
	if len(payload) > 0 {
		if opt.Compression != request.CompressionNone {
			var cbody bytes.Buffer
			var writer io.WriteCloser
			switch opt.Compression {
			case request.CompressionGzip:
				writer = gzip.NewWriter(&cbody)
			case request.CompressionDeflate:
				writer = zlib.NewWriter(&cbody)
			case request.CompressionBrotli:
				writer = brotli.NewWriter(&cbody)
			default:
				return response, fmt.Errorf("unsupported compression type: %s", opt.Compression)
			}
			_, err := writer.Write(payload)
			if err != nil {
				return response, err
			}
			writer.Close()
			requestPayload = &cbody
			opt.AddHeader("Content-Encoding", string(opt.Compression))
		} else {
			requestPayload = bytes.NewBuffer(payload)
		}
	}

	// ready the request
	request, err := http.NewRequest(method, url, requestPayload)
	if err != nil {
		response.Error = err
		return response, err
	}

	// Assign headers from the RequestOptions
	for _, v := range opt.Headers {
		request.Header.Set(v.Key, v.Value)
	}

	// Assign cookies from the RequestOptions
	for _, v := range opt.Cookies {
		request.AddCookie(v)
	}

	// Configure the HTTP client to follow or not follow redirects
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if opt.DisableRedirect {
			return http.ErrUseLastResponse
		}
		return nil
	}

	// To prevent out of memory if a very large payload is provided we can stream the bytes to a file
	// or any data structure that implements the io.Writer interface.
	// This is set in request.Options Writer
	var writer io.Writer = &response.Body
	if opt.Writer != nil {
		writer = opt.Writer
	}

	var r *http.Response
	// Perform the actual request
	response.RequestTime = time.Now().Unix()
	r, err = client.Do(request)

	if err != nil {
		response.Error = err
		return response, err
	}
	defer r.Body.Close()
	response.ResponseTime = time.Now().Unix()

	// convert the http.Response.Body to a bytes.Buffer
	// bytes.Buffer was a preferred choice because I found it to be more flexible than
	// returning []byte
	_, err = io.Copy(writer, r.Body)
	if err != nil {
		response.Error = err
		return response, err
	}
	response.ProcessedTime = time.Now().Unix()

	if opt.Writer != nil {
		err = writer.(io.Closer).Close()
		if err != nil {
			response.Error = err
			return response, err
		}
	}

	// Check if the writer implements io.Closer and close it if so
	if closer, ok := writer.(io.Closer); ok {
		err = closer.Close()
		if err != nil {
			response.Error = err
			return response, err
		}
	}

	// request has completed, add details to the response object
	response.PopulateResponse(r, start)

	return response, nil
}

// Get performs an HTTP GET to the specified URL.
// It accepts the URL string as its first argument.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func Get(url string, opt ...RequestOptions) (Response, error) {
	return doRequest(client, http.MethodGet, url, nil, opt...)
}

// Post performs an HTTP POST to the specified URL with the given payload.
// It accepts the URL string as its first argument and the payload as the second argument.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func Post(url string, payload []byte, opt ...RequestOptions) (Response, error) {
	return doRequest(client, http.MethodPost, url, payload, opt...)
}

// FormPost performs an HTTP POST as an x-www-form-urlencoded payload to the specified URL.
// It accepts the URL string as its first argument and a map[string]string the payload.
// The map is converted to a url.QueryEscaped k/v pair that is sent to the server.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func FormPost(url string, payload map[string]string, opt ...RequestOptions) (Response, error) {
	switch len(opt) {
	case 0:
		option := RequestOptions{}
		option.AddHeader("Content-Type", "application/x-www-form-urlencoded")
		opt = append(opt, option)
	case 1:
		opt[0].AddHeader("Content-Type", "application/x-www-form-urlencoded")
	}
	return doRequest(client, http.MethodPost, url, form.Encode(payload), opt...)
}

// Put performs an HTTP PUT to the specified URL with the given payload.
// It accepts the URL string as its first argument and the payload as the second argument.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func Put(url string, payload []byte, opt ...RequestOptions) (Response, error) {
	return doRequest(client, http.MethodPut, url, payload, opt...)
}

// Patch performs an HTTP PATCH to the specified URL with the given payload.
// It accepts the URL string as its first argument and the payload as the second argument.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func Patch(url string, payload []byte, opt ...RequestOptions) (Response, error) {
	return doRequest(client, http.MethodPatch, url, payload, opt...)
}

// Delete performs an HTTP DELETE to the specified URL.
// It accepts the URL string as its first argument.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func Delete(url string, opt ...RequestOptions) (Response, error) {
	return doRequest(client, http.MethodDelete, url, nil, opt...)
}

// Connect performs an HTTP CONNECT to the specified URL.
// It accepts the URL string as its first argument.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func Connect(url string, opt ...RequestOptions) (Response, error) {
	return doRequest(client, http.MethodConnect, url, nil, opt...)
}

// Head performs an HTTP HEAD to the specified URL.
// It accepts the URL string as its first argument.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func Head(url string, opt ...RequestOptions) (Response, error) {
	return doRequest(client, http.MethodHead, url, nil, opt...)
}

// Options performs an HTTP OPTIONS to the specified URL.
// It accepts the URL string as its first argument.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func Options(url string, opt ...RequestOptions) (Response, error) {
	return doRequest(client, http.MethodHead, url, nil, opt...)
}

// Trace performs an HTTP TRACE to the specified URL.
// It accepts the URL string as its first argument.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func Trace(url string, opt ...RequestOptions) (Response, error) {
	return doRequest(client, http.MethodTrace, url, nil, opt...)
}

// Custom performs a custom HTTP method to the specified URL with the given payload.
// It accepts the HTTP method as its first argument, the URL string as the second argument,
// the payload as the third argument, and optionally additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func Custom(method string, url string, payload []byte, opt ...RequestOptions) (Response, error) {
	return doRequest(client, method, url, payload, opt...)
}
