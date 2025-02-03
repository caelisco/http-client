package client

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	netURL "net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/caelisco/http-client/v2/form"
	"github.com/caelisco/http-client/v2/options"
	"github.com/caelisco/http-client/v2/response"
)

const (
	SchemeHTTP      string = "http://"
	SchemeHTTPS     string = "https://"
	SchemeWS        string = "ws://"
	SchemeWSS       string = "wss://"
	ContentType     string = "Content-Type"
	ContentEncoding string = "Content-Encoding"
	URLencoded      string = "application/x-www-form-urlencoded"
)

// doRequest performs the HTTP request to the server/resource.
// This function is the core of the HTTP client, handling the entire request-response cycle,
// including redirects, payload preparation, and response processing.
//
// Parameters:
// - method: The HTTP method (e.g., GET, POST, PUT)
// - url: The target URL for the request
// - payload: The data to be sent with the request (can be nil)
// - opts: Optional configuration parameters for the request
//
// Returns:
// - response.Response: A struct containing the processed response
// - error: Any error encountered during the request process
func doRequest(method string, url string, payload any, opts ...*options.Option) (response.Response, error) {
	st := time.Now()

	// Initialise options, combining defaults with user-provided options
	opt := options.New(opts...)

	// Set up initial request parameters
	if opt.UniqueIdentifierType != options.IdentifierNone {
		opt.AddHeader("X-TraceID", opt.GenerateIdentifier())
	}

	// Get the *http.Client for this request.
	// A custom *http.Client can be added to the options.Option struct
	// using opt.SetClient(client *http.Client)
	client := opt.GetClient()

	// Initialize base transport
	if client.Transport == nil {
		client.Transport = opt.Transport
	}

	// Always disable automatic redirects, we'll handle them manually
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if opt.CheckRedirects() {
			return fmt.Errorf("max redirects (%d) exceeded", opt.MaxRedirects)
		}
		return http.ErrUseLastResponse
	}

	// Normalise the initial URL by applying checks to the URL including parsing it to confirm it is valid
	url, err := normaliseURL(url, opt.ProtocolScheme)
	if err != nil {
		return response.Response{}, fmt.Errorf("supplied url did not pass url.Parse(): %w", err)
	}

	// Set up base response object
	resp := response.New(url, method, payload, opt)

	// Only create payload reader if there's actually a payload
	var payloadReader io.Reader
	var contentLength int64

	// Only allow the use of the payload with the appropriate methods: POST, PUT, PATCH
	if method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch {
		// Check if payload is an os.File and store filename if necessary
		if file, ok := payload.(*os.File); ok {
			opt.SetFile(file)
		}

		if opt.HasFileHandle() {
			payload = opt.GetFile()
		}

		// if the payload that is passed through is not nil, determine the type of reader that is required
		// to be able to send the payload to the server.
		if payload != nil {
			payloadReader, contentLength, err = opt.CreatePayloadReader(payload)
			if err != nil {
				return resp, fmt.Errorf("unable to create payload reader: %w", err)
			}
		}
	}

	// Prepare request
	req, err := prepareRequest(method, url, payloadReader, contentLength, opt)
	if err != nil {
		return resp, err
	}

	// Execute request
	opt.Log("sending request", "url", req.URL, "method", method, "headers", req.Header)
	resp.RequestTime = time.Now().Unix()

	httpResp, err := client.Do(req)
	if err != nil {
		resp.Error = err
		return resp, err
	}

	resp.ResponseTime = time.Now().Unix()

	// Check if this is a redirect response
	if isRedirect(httpResp.StatusCode) {
		// If redirects are not allowed, return the redirect response immediately
		if !opt.FollowRedirects {
			resp.PopulateResponse(httpResp, st)
			httpResp.Body.Close()
			return resp, nil
		}

		redirectURL := httpResp.Header.Get("Location")
		if redirectURL == "" {
			httpResp.Body.Close()
			return resp, fmt.Errorf("redirect location header missing")
		}

		// Parse and resolve the redirect URL
		parsedRedirect, err := netURL.Parse(redirectURL)
		if err != nil {
			httpResp.Body.Close()
			return resp, fmt.Errorf("invalid redirect URL: %w", err)
		}

		nextURL := httpResp.Request.URL.ResolveReference(parsedRedirect).String()
		httpResp.Body.Close()

		// Handle the redirect
		if opt.PreserveMethodOnRedirect {
			var newPayload any

			// If we have a file handle, reopen it
			if opt.HasFileHandle() {
				opt.ReopenFile()
			} else if payload != nil {
				// For non-file payloads, recreate them
				switch v := payload.(type) {
				case []byte:
					newPayload = v // Original byte slice can be reused
				case *bytes.Buffer:
					newPayload = bytes.NewBuffer(v.Bytes()) // Create new buffer with original content
				case string:
					newPayload = v // Original string can be reused
				}
			}

			return doRequest(method, nextURL, newPayload, opt)
		}

		// Switch to GET method as per HTTP spec for other redirects
		return doRequest(http.MethodGet, nextURL, nil, opt)
	}

	// Process final response
	return processResponse(httpResp, resp, opt, st)
}

// isRedirect checks if the status code indicates a redirect
// This function helps in identifying if a response is a redirect based on its status code
func isRedirect(statusCode int) bool {
	return statusCode == http.StatusMovedPermanently ||
		statusCode == http.StatusFound ||
		statusCode == http.StatusSeeOther ||
		statusCode == http.StatusTemporaryRedirect ||
		statusCode == http.StatusPermanentRedirect
}

// prepareRequest creates and configures the HTTP request
// This function handles the creation of the request, including setting up progress tracking
// and compression if needed.
func prepareRequest(method, url string, payloadReader io.Reader, contentLength int64, opt *options.Option) (*http.Request, error) {
	var reader io.Reader = payloadReader

	if reader != nil && opt.OnUploadProgress != nil &&
		(method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch) {

		if opt.GetProgressTracking() == options.TrackBeforeCompression {
			// Track before compression - use original size
			var totalSize int64 = contentLength
			if sizer, ok := payloadReader.(io.Seeker); ok {
				if size, err := sizer.Seek(0, io.SeekEnd); err == nil {
					sizer.Seek(0, io.SeekStart)
					totalSize = size
				}
			}
			reader = options.NewProgressReader(payloadReader, totalSize, opt.OnUploadProgress)
		}
	}

	// Handle compression
	if reader != nil && opt.Compression != options.CompressionNone {
		pr, pw := io.Pipe()
		go compressData(pw, reader, opt)
		reader = pr
		// Update headers for compression
		opt.Header.Set("Transfer-Encoding", "chunked")
		opt.Header.Del("Content-Length")
		if opt.Compression != options.CompressionCustom {
			opt.Header.Set(ContentEncoding, string(opt.Compression))
		} else if opt.CustomCompressionType != "" {
			opt.Header.Set(ContentEncoding, string(opt.CustomCompressionType))
		} else {
			opt.Header.Set(ContentEncoding, "application/octet-stream")
		}
	}

	// Add progress tracking after compression if specified
	if reader != nil && opt.OnUploadProgress != nil &&
		(method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch) &&
		opt.GetProgressTracking() == options.TrackAfterCompression {
		// Track after compression - use -1 for unknown size
		reader = options.NewProgressReader(reader, 0, opt.OnUploadProgress)
	}

	// Create the request
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return nil, err
	}

	// Set content length for requests with no body or uncompressed body
	if reader == nil {
		req.ContentLength = 0
	} else if opt.Compression == options.CompressionNone {
		req.ContentLength = contentLength
	}

	// Set headers and cookies
	req.Header = opt.Header
	for _, cookie := range opt.Cookies {
		req.AddCookie(cookie)
	}
	return req, nil
}

// compressData handles the compression of request data
// This function runs in a separate goroutine to compress the request payload
// before sending it to the server.
func compressData(pw *io.PipeWriter, reader io.Reader, opt *options.Option) {
	defer pw.Close()

	compressor, err := opt.GetCompressor(pw)
	if err != nil {
		pw.CloseWithError(fmt.Errorf("unsupported compression type: %s", opt.Compression))
		return
	}
	defer compressor.Close()

	var copyErr error
	if opt.UploadBufferSize != nil {
		buf := make([]byte, *opt.UploadBufferSize)
		_, copyErr = io.CopyBuffer(compressor, reader, buf)
	} else {
		_, copyErr = io.Copy(compressor, reader)
	}

	if copyErr != nil {
		pw.CloseWithError(fmt.Errorf("compression error during copy: %w", copyErr))
	}
}

// processResponse handles the final response processing
// This function processes the HTTP response, including handling the response body,
// tracking download progress, and populating the response struct.
func processResponse(r *http.Response, resp response.Response, opt *options.Option, startTime time.Time) (response.Response, error) {
	defer r.Body.Close()

	// Get content encoding
	encoding := r.Header.Get("Content-Encoding")

	// Create decompressed reader
	decompressedBody, err := opt.GetDecompressor(r.Body, encoding)
	if err != nil {
		return resp, fmt.Errorf("failed to create decompressed reader: %w", err)
	}
	defer decompressedBody.Close()

	// Initialize writer
	writer, err := opt.InitialiseWriter()
	if err != nil {
		return resp, fmt.Errorf("failed to initialise writer: %w", err)
	}
	defer writer.Close()

	// Get total size from Content-Length header
	totalSize := r.ContentLength

	// Track progress at the read level instead of write level
	var reader io.Reader = decompressedBody
	if opt.OnDownloadProgress != nil {
		if encoding != "" {
			// For compressed content, we won't know the final size until we read it all
			// so we pass -1 to indicate unknown size
			totalSize = -1
		}
		reader = options.NewProgressReader(decompressedBody, totalSize, opt.OnDownloadProgress)
	}

	// Copy response body
	if opt.DownloadBufferSize != nil {
		buf := make([]byte, *opt.DownloadBufferSize)
		_, err = io.CopyBuffer(writer, reader, buf)
	} else {
		_, err = io.Copy(writer, reader)
	}

	if err != nil {
		resp.Error = err
		return resp, err
	}

	// Only store body in response if we're using a buffer writer
	if buf, ok := writer.(*options.WriteCloserBuffer); ok {
		resp.Body = *buf
	}

	resp.ProcessedTime = time.Now().Unix()
	resp.PopulateResponse(r, startTime)

	return resp, nil
}

// MultipartUpload performs a multipart form-data upload request to the specified URL.
// It supports file uploads and other form fields.
func MultipartUpload(method, url string, payload map[string]any, opts ...*options.Option) (response.Response, error) {
	opt := options.New(opts...)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	for key, value := range payload {
		switch v := value.(type) {
		case *os.File:
			part, err := writer.CreateFormFile(key, filepath.Base(v.Name()))
			if err != nil {
				return response.Response{}, err
			}
			_, err = io.Copy(part, v)
			if err != nil {
				return response.Response{}, err
			}
		default:
			writer.WriteField(key, fmt.Sprintf("%v", v))
		}
	}

	writer.Close()

	// Wrap the buffer with a ProgressReader if upload progress is enabled
	var finalReader io.Reader = body
	if opt.OnUploadProgress != nil {
		finalReader = options.NewProgressReader(body, int64(body.Len()), opt.OnUploadProgress)
	}

	opt.AddHeader(ContentType, writer.FormDataContentType())
	return doRequest(method, url, finalReader, opt)
}

// Get performs an HTTP GET to the specified URL.
// It accepts the URL string as its first argument.
// Optionally, you can provide additional Options to customize the request.
// Returns the HTTP response and an error if any.
func Get(url string, opts ...*options.Option) (response.Response, error) {
	return doRequest(http.MethodGet, url, nil, opts...)
}

// Post performs an HTTP POST to the specified URL with the given payload.
// It accepts the URL string as its first argument and the payload as the second argument.
// Optionally, you can provide additional Options to customize the request.
// Returns the HTTP response and an error if any.
func Post(url string, payload any, opts ...*options.Option) (response.Response, error) {
	return doRequest(http.MethodPost, url, payload, opts...)
}

// PostFormData performs an HTTP POST as an x-www-form-urlencoded payload to the specified URL.
// It accepts the URL string as its first argument and a map[string]string the payload.
// The map is converted to a url.QueryEscaped k/v pair that is sent to the server.
// Optionally, you can provide additional Options to customize the request.
// Returns the HTTP response and an error if any.
func PostFormData(url string, payload map[string]string, opts ...*options.Option) (response.Response, error) {
	opt := options.New(opts...)
	opt.AddHeader(ContentType, URLencoded)

	return Post(url, form.Encode(payload), opt)
}

// PostFile uploads a file to the specified URL using an HTTP POST request.
// It accepts the URL string as its first argument and the filename as the second argument.
// The file is read from the specified filename and uploaded as the request payload.
// Optionally, you can provide additional Options to customize the request.
// Returns the HTTP response and an error if any.
func PostFile(url string, filename string, opts ...*options.Option) (response.Response, error) {
	opt := options.New(opts...)

	err := opt.PrepareFile(filename)
	if err != nil {
		return response.Response{}, err
	}
	defer opt.CloseFile()

	return Post(url, nil, opt)
}

// PostMultipartUpload performs a POST multipart form-data upload request to the specified URL.
// This is the most common method for file uploads and creating new resources with file attachments.
func PostMultipartUpload(url string, payload map[string]interface{}, opts ...*options.Option) (response.Response, error) {
	return MultipartUpload(http.MethodPost, url, payload, opts...)
}

// Put performs an HTTP PUT to the specified URL with the given payload.
// It accepts the URL string as its first argument and the payload as the second argument.
// Optionally, you can provide additional Options to customize the request.
// Returns the HTTP response and an error if any.
func Put(url string, payload any, opts ...*options.Option) (response.Response, error) {
	return doRequest(http.MethodPut, url, payload, opts...)
}

// PutFormData performs an HTTP PUT as an x-www-form-urlencoded payload to the specified URL.
// It accepts the URL string as its first argument and a map[string]string the payload.
// The map is converted to a url.QueryEscaped k/v pair that is sent to the server.
// Optionally, you can provide additional Options to customize the request.
// Returns the HTTP response and an error if any.
func PutFormData(url string, payload map[string]string, opts ...*options.Option) (response.Response, error) {
	opt := options.New(opts...)
	opt.AddHeader(ContentType, URLencoded)

	return Put(url, form.Encode(payload), opt)
}

// PutFile uploads a file to the specified URL using an HTTP PUT request.
// It accepts the URL string as its first argument and the filename as the second argument.
// The file is read from the specified filename and uploaded as the request payload.
// Optionally, you can provide additional Options to customize the request.
// Returns the HTTP response and an error if any.
func PutFile(url string, filename string, opts ...*options.Option) (response.Response, error) {
	opt := options.New(opts...)

	err := opt.PrepareFile(filename)
	if err != nil {
		return response.Response{}, err
	}
	defer opt.CloseFile()

	return Put(url, nil, opt)
}

// PutMultipartUpload performs a PUT multipart form-data upload request to the specified URL.
// This method is less common but can be used when updating an entire resource with new data,
// including file attachments.
func PutMultipartUpload(url string, payload map[string]interface{}, opts ...*options.Option) (response.Response, error) {
	return MultipartUpload(http.MethodPut, url, payload, opts...)
}

// Patch performs an HTTP PATCH to the specified URL with the given payload.
// It accepts the URL string as its first argument and the payload as the second argument.
// Optionally, you can provide additional Options to customize the request.
// Returns the HTTP response and an error if any.
func Patch(url string, payload any, opts ...*options.Option) (response.Response, error) {
	return doRequest(http.MethodPatch, url, payload, opts...)
}

// PatchFormData performs an HTTP PATCH as an x-www-form-urlencoded payload to the specified URL.
// It accepts the URL string as its first argument and a map[string]string the payload.
// The map is converted to a url.QueryEscaped k/v pair that is sent to the server.
// Optionally, you can provide additional Options to customize the request.
// Returns the HTTP response and an error if any.
func PatchFormData(url string, payload map[string]string, opts ...*options.Option) (response.Response, error) {
	opt := options.New(opts...)
	opt.AddHeader(ContentType, URLencoded)

	return Patch(url, form.Encode(payload), opt)
}

// PatchFile uploads a file to the specified URL using an HTTP PATCH request.
// It accepts the URL string as its first argument and the filename as the second argument.
// The file is read from the specified filename and uploaded as the request payload.
// Optionally, you can provide additional Options to customize the request.
// Returns the HTTP response and an error if any.
func PatchFile(url string, filename string, opts ...*options.Option) (response.Response, error) {
	opt := options.New(opts...)

	err := opt.PrepareFile(filename)
	if err != nil {
		return response.Response{}, err
	}
	defer opt.CloseFile()

	return Patch(url, nil, opt)
}

// PatchMultipartUpload performs a PATCH multipart form-data upload request to the specified URL.
// This method can be used for partial updates to a resource, which might include updating or
// adding new file attachments.
func PatchMultipartUpload(url string, payload map[string]interface{}, opts ...*options.Option) (response.Response, error) {
	return MultipartUpload(http.MethodPatch, url, payload, opts...)
}

// Delete performs an HTTP DELETE to the specified URL.
// It accepts the URL string as its first argument.
// Optionally, you can provide additional Options to customize the request.
// Returns the HTTP response and an error if any.
func Delete(url string, opts ...*options.Option) (response.Response, error) {
	return doRequest(http.MethodDelete, url, nil, opts...)
}

// Connect performs an HTTP CONNECT to the specified URL.
// It accepts the URL string as its first argument.
// Optionally, you can provide additional Options to customize the request.
// Returns the HTTP response and an error if any.
func Connect(url string, opts ...*options.Option) (response.Response, error) {
	return doRequest(http.MethodConnect, url, nil, opts...)
}

// Head performs an HTTP HEAD to the specified URL.
// It accepts the URL string as its first argument.
// Optionally, you can provide additional Options to customize the request.
// Returns the HTTP response and an error if any.
func Head(url string, opts ...*options.Option) (response.Response, error) {
	return doRequest(http.MethodHead, url, nil, opts...)
}

// Options performs an HTTP OPTIONS to the specified URL.
// It accepts the URL string as its first argument.
// Optionally, you can provide additional Options to customize the request.
// Returns the HTTP response and an error if any.
func Options(url string, opts ...*options.Option) (response.Response, error) {
	return doRequest(http.MethodOptions, url, nil, opts...)
}

// Trace performs an HTTP TRACE to the specified URL.
// It accepts the URL string as its first argument.
// Optionally, you can provide additional Options to customize the request.
// Returns the HTTP response and an error if any.
func Trace(url string, opts ...*options.Option) (response.Response, error) {
	return doRequest(http.MethodTrace, url, nil, opts...)
}

// Custom performs a custom HTTP method to the specified URL with the given payload.
// It accepts the HTTP method as its first argument, the URL string as the second argument,
// the payload as the third argument, and optionally additional Options to customize the request.
// Returns the HTTP response and an error if any.
func Custom(method string, url string, payload any, opts ...*options.Option) (response.Response, error) {
	return doRequest(method, url, payload, opts...)
}
