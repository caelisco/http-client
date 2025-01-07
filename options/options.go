package options

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/google/uuid"
	"github.com/oklog/ulid/v2"
)

type CompressionType string
type UniqueIdentifierType string
type UploadType string

const (
	CompressionNone    CompressionType = ""
	CompressionGzip    CompressionType = "gzip"
	CompressionDeflate CompressionType = "deflate"
	CompressionBrotli  CompressionType = "br"
	CompressionCustom  CompressionType = "custom"
)

const (
	IdentifierNone UniqueIdentifierType = ""
	IdentifierUUID UniqueIdentifierType = "uuid"
	IdentifierULID UniqueIdentifierType = "ulid"
)

// RequestOptions represents additional options for the HTTP request.
// DisableRedirect - Determines if redirects should be followed or not. The default option is
// false which means redirects will be followed.
type Option struct {
	Verbose                  bool                                           // Whether logging should be verbose or not
	Logger                   slog.Logger                                    // Logging - default uses the slog TextHandler
	Header                   http.Header                                    // Headers to be included in the request
	Cookies                  []*http.Cookie                                 // Cookies to be included in the request
	ProtocolScheme           string                                         // define a custom protocol scheme. It defaults to https
	Compression              CompressionType                                // CompressionType to use: none, gzip, deflate or brotli
	CustomCompressionType    CompressionType                                //
	CustomCompressor         func(w *io.PipeWriter) (io.WriteCloser, error) // Function for custom compression
	UserAgent                string                                         // User Agent to send with requests
	FollowRedirect           bool                                           // Disable or enable redirects. Default is true i.e.: follow redirects
	PreserveMethodOnRedirect bool                                           // Default is true
	UniqueIdentifierType     UniqueIdentifierType                           // Internal trace or identifier for the request
	Transport                *http.Transport                                // Create our own default transport
	ResponseWriter           ResponseWriter                                 // Define the type of response writer
	UploadBufferSize         *int                                           // Control the size of the buffer when uploading a file
	DownloadBufferSize       *int                                           // Control the size of the buffer when downloading a file
	OnUploadProgress         func(bytesRead, totalBytes int64)              // To monitor and track progress when uploading
	OnDownloadProgress       func(bytesRead, totalBytes int64)              // To monitor and track progress when downloading
}

func New(opts ...*Option) *Option {
	opt := &Option{
		Verbose:                  false,
		Logger:                   *slog.New(slog.NewTextHandler(os.Stdout, nil)),
		Header:                   http.Header{},
		FollowRedirect:           false,
		PreserveMethodOnRedirect: false,
		UniqueIdentifierType:     IdentifierULID,
		Transport:                defaultTransport(),
		ResponseWriter: ResponseWriter{
			Type: WriteToBuffer,
		},
	}

	// Check if opts is not nil and has at least one element
	if len(opts) > 0 && opts[0] != nil {
		opt.Merge(opts[0])
	}

	return opt
}

func (opt *Option) LogVerbose(msg string, args ...any) {
	if opt.Verbose {
		opt.Logger.Info(msg, args...)
	}
}

func (opt *Option) EnableLogging() {
	opt.Verbose = true
}

func (opt *Option) DisableLogging() {
	opt.Verbose = false
}

func (opt *Option) UseTextLogger() {
	opt.Verbose = true
	opt.Logger = *slog.New(slog.NewTextHandler(os.Stdout, nil))
}

func (opt *Option) UseJsonLogger() {
	opt.Verbose = true
	opt.Logger = *slog.New(slog.NewJSONHandler(os.Stdout, nil))
}

func (opt *Option) SetLogger(logger *slog.Logger) {
	opt.Verbose = true
	opt.Logger = *logger
}

// Kept for backwards compatability
func (opt *Option) AddHeader(key string, value string) {
	if opt.Header == nil {
		opt.Header = http.Header{}
	}
	opt.Header.Add(key, value)
}

// ListHeaders prints out the list of headers in the RequestOptions.
func (opt *Option) ListHeaders() {
	for k, v := range opt.Header {
		fmt.Printf("%s=%s", k, v)
	}
}

// ClearHeaders clears all headers in the RequestOptions.
func (opt *Option) ClearHeaders() {
	opt.Header = nil
}

// AddCookie adds a new cookie to the RequestOptions.
func (opt *Option) AddCookie(cookie *http.Cookie) {
	if opt.Cookies == nil {
		opt.Cookies = []*http.Cookie{}
	}
	opt.Cookies = append(opt.Cookies, cookie)
}

// ListCookies prints out the list of cookies in the RequestOptions.
func (opt *Option) ListCookies() {
	for _, c := range opt.Cookies {
		fmt.Printf("%s=%s", c.Name, c.Value)
	}
}

// ClearCookies clears all cookies in the RequestOptions.
func (opt *Option) ClearCookies() {
	opt.Cookies = nil
}

func (opt *Option) SetProtocolScheme(scheme string) {
	if !strings.Contains(scheme, "://") {
		scheme += "://"
	}
	opt.ProtocolScheme = scheme
}

func (opt *Option) SetCompression(compressionType CompressionType) {
	opt.Compression = compressionType
}

func (opt *Option) GetCompressor(w *io.PipeWriter) (io.WriteCloser, error) {
	switch opt.Compression {
	case CompressionGzip:
		return gzip.NewWriter(w), nil
	case CompressionDeflate:
		return zlib.NewWriter(w), nil
	case CompressionBrotli:
		return brotli.NewWriter(w), nil
	case CompressionCustom:
		if opt.CustomCompressor != nil {
			writer, err := opt.CustomCompressor(w)
			return writer, err
		}
		return nil, fmt.Errorf("custom compressor function is not defined")
	default:
		return nil, fmt.Errorf("unsupported compression type: %s", opt.Compression)
	}
}

func (opt *Option) EnableRedirects() {
	opt.FollowRedirect = true
}

func (opt *Option) DisableRedirects() {
	opt.FollowRedirect = false
}

func (opt *Option) EnablePreserveMethodOnRedirect() {
	opt.PreserveMethodOnRedirect = true
}

func (opt *Option) DisablePreserveMethodOnRedirect() {
	opt.PreserveMethodOnRedirect = false
}

func defaultTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
		// For large files, increase these timeouts
		ResponseHeaderTimeout: 30 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		// Keep HTTP/2 for better performance
		ForceAttemptHTTP2: true,
		// Only disable compression for already compressed files
		DisableCompression: false,
	}
}

func (opt *Option) SetTransport(transport *http.Transport) {
	opt.Transport = transport
}

func (opt *Option) GenerateIdentifier() string {
	switch opt.UniqueIdentifierType {
	case IdentifierUUID:
		return uuid.New().String()
	case IdentifierULID:
		return ulid.Make().String()
	}
	return ""
}

// SetDownloadBufferSize allows custom buffer size if needed
func (opt *Option) SetDownloadBufferSize(sizeInBytes int) {
	if sizeInBytes > 0 {
		opt.DownloadBufferSize = &sizeInBytes
	}
}

// InitialiseWriter sets up the appropriate writer based on the ResponseWriter configuration
func (opt *Option) InitialiseWriter() (io.WriteCloser, error) {
	switch opt.ResponseWriter.Type {
	case WriteToFile:
		if opt.ResponseWriter.FilePath == "" {
			return nil, fmt.Errorf("file path must be specified when using WriteToFile")
		}
		file, err := os.Create(opt.ResponseWriter.FilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to create file: %w", err)
		}
		opt.ResponseWriter.writer = file
	case WriteToBuffer:
		opt.ResponseWriter.writer = &WriteCloserBuffer{Buffer: &bytes.Buffer{}}
	default:
		return nil, fmt.Errorf("invalid writer type: %s", opt.ResponseWriter.Type)
	}
	return opt.ResponseWriter.writer, nil
}

// GetWriter returns the underlying io.WriteCloser
func (opt *Option) GetWriter() io.WriteCloser {
	return opt.ResponseWriter.writer
}

// SetOutput configures how the response should be written
// For WriteToBuffer: opt.SetOutput(WriteToBuffer)
// For WriteToFile: opt.SetOutput(WriteToFile, "path/to/file.txt")
func (opt *Option) SetOutput(writerType ResponseWriterType, filepath ...string) error {
	opt.ResponseWriter.Type = writerType

	switch writerType {
	case WriteToFile:
		if len(filepath) == 0 {
			return fmt.Errorf("filepath is required when using WriteToFile")
		}
		opt.ResponseWriter.FilePath = filepath[0]
	case WriteToBuffer:
		if len(filepath) > 0 {
			return fmt.Errorf("filepath should not be provided when using WriteToBuffer")
		}
		opt.ResponseWriter.FilePath = ""
	default:
		return fmt.Errorf("invalid writer type: %s", writerType)
	}

	return nil
}

// SetFileOutput configures the response writer to write to a file
func (opt *Option) SetFileOutput(filepath string) {
	opt.ResponseWriter = ResponseWriter{
		Type:     WriteToFile,
		FilePath: filepath,
	}
}

// SetBufferOutput configures the response writer to write to an in-memory buffer
func (opt *Option) SetBufferOutput() {
	opt.ResponseWriter = ResponseWriter{
		Type: WriteToBuffer,
	}
}

func (opt *Option) Merge(src *Option) {
	// Merge Headers
	if opt.Header == nil {
		opt.Header = make(http.Header)
	}
	// Replaces any existing values
	for key, values := range src.Header {
		opt.Header[key] = values
	}

	// Merge Cookies
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

	// Merge boolean and primitive fields with source priority
	opt.Verbose = src.Verbose
	opt.FollowRedirect = src.FollowRedirect
	opt.PreserveMethodOnRedirect = src.PreserveMethodOnRedirect

	if src.Transport != nil {
		opt.Transport = src.Transport
	}

	if src.ResponseWriter.Type != "" {
		opt.ResponseWriter = src.ResponseWriter
	}

	// Merge string fields if source is not empty
	if src.ProtocolScheme != "" {
		opt.ProtocolScheme = src.ProtocolScheme
	}

	if src.Compression != "" {
		opt.Compression = src.Compression
	}

	if src.CustomCompressionType != "" {
		opt.CustomCompressionType = src.CustomCompressionType
	}

	if src.CustomCompressor != nil {
		opt.CustomCompressor = src.CustomCompressor
	}

	if src.UserAgent != "" {
		opt.UserAgent = src.UserAgent
	}

	if src.UniqueIdentifierType != "" {
		opt.UniqueIdentifierType = src.UniqueIdentifierType
	}

	if src.DownloadBufferSize != nil {
		opt.DownloadBufferSize = src.DownloadBufferSize
	}

	// Merge progress callback functions
	if src.OnUploadProgress != nil {
		opt.OnUploadProgress = src.OnUploadProgress
	}

	if src.OnDownloadProgress != nil {
		opt.OnDownloadProgress = src.OnDownloadProgress
	}
}
