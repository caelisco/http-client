package client

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/caelisco/http-client/form"
	"github.com/caelisco/http-client/options"
	"github.com/caelisco/http-client/response"
)

const (
	useragent          = "caelisco/http-client/v1.0.0"
	SchemeHTTP  string = "http://"
	SchemeHTTPS string = "https://"
	SchemeWS    string = "ws://"
	SchemeWSS   string = "wss://"
	ContentType string = "Content-Type"
)

// A global default client is used for all of the method-based requests.
var client = &http.Client{
	//Timeout: 30 * time.Second, // Set an appropriate timeout
	Timeout: 0,
}

// doRequest performs the HTTP request to the server/resource.
func doRequest(client *http.Client, method string, url string, payload any, opts ...*options.Option) (response.Response, error) {
	st := time.Now()

	opt := options.New(opts...)

	if client.Transport == nil {
		client.Transport = opt.Transport
	}

	url, err := normaliseURL(url, opt.ProtocolScheme)
	if err != nil {
		return response.Response{}, fmt.Errorf("supplied url did not pass url.Parse(): %w", err)
	}

	if opt.UserAgent == "" {
		opt.UserAgent = useragent
	}
	opt.Header.Add("User-Agent", opt.UserAgent)

	var totalSize int64 = -1
	var payloadReader io.Reader

	payloadReader, totalSize, err = createPayloadReader(payload, opt)
	if err != nil {
		return response.Response{}, fmt.Errorf("unable to create payload reader: %w", err)
	}

	// Wrap reader with progress tracking if callback provided and it's a POST/PUT
	if opt.OnUploadProgress != nil && payloadReader != nil &&
		(method == http.MethodPost || method == http.MethodPut) {
		payloadReader = options.ProgressReader(payloadReader, totalSize, opt.OnUploadProgress)
	}

	response := response.New(url, method, payload, opt)

	// Declare the io.Pipe variables for use with compression.
	// If
	var pr *io.PipeReader
	var pw *io.PipeWriter
	if payloadReader != nil && opt.Compression != options.CompressionNone {

		opt.LogVerbose("Compressing data", "compression type", opt.Compression)
		pr, pw = io.Pipe()
		// Goroutine to handle compression and closing of resources
		go func() {
			compressor, err := opt.GetCompressor(pw)
			if err != nil {
				pw.CloseWithError(fmt.Errorf("unsupported compression type: %s", opt.Compression))
				return
			}

			// Defer closures after successful creation
			defer func() {
				compressor.Close()
				pw.Close()
			}()

			// Optional UploadBufferSize
			if opt.UploadBufferSize != nil {
				buf := make([]byte, *opt.UploadBufferSize)
				if _, err := io.CopyBuffer(compressor, payloadReader, buf); err != nil {
					pw.CloseWithError(fmt.Errorf("compression error during copy: %w", err))
					return
				}
			} else {
				// Use standard io.Copy with its optimal default buffer
				if _, err := io.Copy(compressor, payloadReader); err != nil {
					log.Fatal(err)
					pw.CloseWithError(fmt.Errorf("compression error during copy: %w", err))
					return
				}
			}
		}()

		if opt.Compression != options.CompressionCustom {
			opt.Header.Add("Content-Encoding", string(opt.Compression))
		} else {
			if opt.CustomCompressionType != "" {
				opt.Header.Add("Content-Encoding", string(opt.CustomCompressionType))
			} else {
				opt.Header.Add("Content-Encoding", "application/octet-stream")
			}
		}
		// Remove Content-Length header since we're streaming and do not know the size of the file in advance
		opt.Header.Del("Content-Length")
		// Add Transfer-Encoding header to indicate streaming
		opt.Header.Add("Transfer-Encoding", "chunked")
	} else {
		// add the header for the content length if not compressed
		opt.Header.Add("Content-Length", fmt.Sprintf("%d", totalSize))
	}

	var req *http.Request

	// Ready the NewRequest
	// We will assume that most requests to the server will not be compressed.
	// If compression is being used, the pr (io.PipeReader) will be set
	if pr == nil {
		opt.LogVerbose("setting up NewRequest", "reader", "io.ReadCloser")
		req, err = http.NewRequest(method, url, payloadReader)
	} else {
		opt.LogVerbose("setting up NewRequest", "reader", "io.PipeReader")
		req, err = http.NewRequest(method, url, pr)
	}

	if err != nil {
		response.Error = err
		return response, err
	}

	// Set headers from the options
	for k, v := range opt.Header {
		req.Header[k] = v
	}

	// Set cookies from the options
	for _, v := range opt.Cookies {
		req.AddCookie(v)
	}

	// Determine if we follow any redirects.
	// When a redirect is followed, the http method can change from the original method to
	// a get. There is an option to preserve the original request method type when following the
	// redirect.
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		opt.LogVerbose("Server wanted to redirect", "Location", req.Response.Header.Get("Location"), "status code", req.Response.StatusCode)

		// Check if we need to follow redirects
		if !opt.FollowRedirect {
			return http.ErrUseLastResponse
		}

		// If following the redirect the default is to change the http.Method to a GET
		// If the option to preserve the method is enabled, the original http.Method
		// will be used.
		if opt.PreserveMethodOnRedirect {
			req.Method = via[0].Method // Use the original method from the first request
			opt.LogVerbose("Preserving original method", "http.Method", req.Method)
		} else {
			opt.LogVerbose("Not preserving method", "http.Method", req.Method)
		}

		return nil
	}

	// Initialize the writer based on the options
	writer, err := opt.InitialiseWriter()
	if err != nil {
		return response, fmt.Errorf("failed to initialise writer: %w", err)
	}
	defer writer.Close()

	opt.LogVerbose("sending request", "url", req.URL, "method", method, "headers", req.Header)
	response.RequestTime = time.Now().Unix()
	r, err := client.Do(req)
	if err != nil {
		response.Error = err
		return response, err
	}
	defer r.Body.Close()
	response.ResponseTime = time.Now().Unix()

	// Added logging for response details
	opt.LogVerbose("Response received",
		"status", r.Status,
		"content-length", r.ContentLength,
		"content-type", r.Header.Get("Content-Type"))

	contentLength := r.ContentLength

	if opt.OnDownloadProgress != nil {
		// Wrap the writer with progress tracking
		writer = options.ProgressWriter(writer, contentLength, opt.OnDownloadProgress)
	}
	defer writer.Close()

	// Added logging before body copy
	opt.LogVerbose("Preparing to copy response body",
		"buffer-size", opt.DownloadBufferSize,
		"progress-tracking-enabled", opt.OnDownloadProgress != nil)

	// Only use custom buffer size if explicitly set
	if opt.DownloadBufferSize != nil {
		buf := make([]byte, *opt.DownloadBufferSize)
		_, err = io.CopyBuffer(writer, r.Body, buf)
	} else {
		// Use standard io.Copy with its optimal default buffer
		_, err = io.Copy(writer, r.Body)
	}
	if err != nil {
		response.Error = err
		return response, err
	}

	// Use type assertion on the writer to determine if the writer is a WriteCloseBuffer.
	// When using the WriteCloseBuffer (bytes.Buffer) assign the buffer to the response.Body
	if buf, ok := writer.(*options.WriteCloserBuffer); ok {
		response.Body = *buf
	}

	opt.LogVerbose("io.Copy complete", "writer", writer)
	response.ProcessedTime = time.Now().Unix()

	// Added logging after body copy
	opt.LogVerbose("Response body copy completed",
		"error", err,
		"processed-time", response.ProcessedTime)

	response.PopulateResponse(r, st)

	return response, nil
}

// Get performs an HTTP GET to the specified URL.
// It accepts the URL string as its first argument.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func Get(url string, opts ...*options.Option) (response.Response, error) {
	return doRequest(client, http.MethodGet, url, nil, opts...)
}

// Post performs an HTTP POST to the specified URL with the given payload.
// It accepts the URL string as its first argument and the payload as the second argument.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func Post(url string, payload any, opts ...*options.Option) (response.Response, error) {
	return doRequest(client, http.MethodPost, url, payload, opts...)
}

// PostFormData performs an HTTP POST as an x-www-form-urlencoded payload to the specified URL.
// It accepts the URL string as its first argument and a map[string]string the payload.
// The map is converted to a url.QueryEscaped k/v pair that is sent to the server.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func PostFormData(url string, payload map[string]string, opts ...*options.Option) (response.Response, error) {
	opt := options.New(opts...)
	opt.Header.Add(ContentType, "application/x-www-form-urlencoded")
	return doRequest(client, http.MethodPost, url, form.Encode(payload), opt)
}

// Put performs an HTTP PUT to the specified URL with the given payload.
// It accepts the URL string as its first argument and the payload as the second argument.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func Put(url string, payload any, opts ...*options.Option) (response.Response, error) {
	return doRequest(client, http.MethodPut, url, payload, opts...)
}

// PutFormData performs an HTTP PUT as an x-www-form-urlencoded payload to the specified URL.
// It accepts the URL string as its first argument and a map[string]string the payload.
// The map is converted to a url.QueryEscaped k/v pair that is sent to the server.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func PutFormData(url string, payload map[string]string, opts ...*options.Option) (response.Response, error) {
	opt := options.New(opts...)
	opt.Header.Add(ContentType, "application/x-www-form-urlencoded")
	return doRequest(client, http.MethodPut, url, form.Encode(payload), opt)
}

// Patch performs an HTTP PATCH to the specified URL with the given payload.
// It accepts the URL string as its first argument and the payload as the second argument.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func Patch(url string, payload any, opts ...*options.Option) (response.Response, error) {
	return doRequest(client, http.MethodPatch, url, payload, opts...)
}

// PatchFormData performs an HTTP PATCH as an x-www-form-urlencoded payload to the specified URL.
// It accepts the URL string as its first argument and a map[string]string the payload.
// The map is converted to a url.QueryEscaped k/v pair that is sent to the server.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func PatchFormData(url string, payload map[string]string, opts ...*options.Option) (response.Response, error) {
	// Use the first options or create default
	opt := options.New(opts...)
	return doRequest(client, http.MethodPatch, url, form.Encode(payload), opt)
}

// Delete performs an HTTP DELETE to the specified URL.
// It accepts the URL string as its first argument.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func Delete(url string, opts ...*options.Option) (response.Response, error) {
	return doRequest(client, http.MethodDelete, url, nil, opts...)
}

// Connect performs an HTTP CONNECT to the specified URL.
// It accepts the URL string as its first argument.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func Connect(url string, opts ...*options.Option) (response.Response, error) {
	return doRequest(client, http.MethodConnect, url, nil, opts...)
}

// Head performs an HTTP HEAD to the specified URL.
// It accepts the URL string as its first argument.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func Head(url string, opts ...*options.Option) (response.Response, error) {
	return doRequest(client, http.MethodHead, url, nil, opts...)
}

// Options performs an HTTP OPTIONS to the specified URL.
// It accepts the URL string as its first argument.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func Options(url string, opts ...*options.Option) (response.Response, error) {
	return doRequest(client, http.MethodHead, url, nil, opts...)
}

// Trace performs an HTTP TRACE to the specified URL.
// It accepts the URL string as its first argument.
// Optionally, you can provide additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func Trace(url string, opts ...*options.Option) (response.Response, error) {
	return doRequest(client, http.MethodTrace, url, nil, opts...)
}

// Custom performs a custom HTTP method to the specified URL with the given payload.
// It accepts the HTTP method as its first argument, the URL string as the second argument,
// the payload as the third argument, and optionally additional RequestOptions to customize the request.
// Returns the HTTP response and an error if any.
func Custom(method string, url string, payload any, opts ...*options.Option) (response.Response, error) {
	return doRequest(client, method, url, payload, opts...)
}
