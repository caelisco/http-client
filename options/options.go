package options

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/google/uuid"
	"github.com/oklog/ulid/v2"
)

// ua defines the default User-Agent string for requests
const ua = "caelisco/http-client/v2.0.0"

// CompressionType defines the compression algorithm used for HTTP requests.
// It supports standard compression types (gzip, deflate, brotli) as well as
// custom compression implementations.
type CompressionType string

// Compression types supported by the client
const (
	CompressionNone    CompressionType = ""        // No compression
	CompressionGzip    CompressionType = "gzip"    // Gzip compression (RFC 1952)
	CompressionDeflate CompressionType = "deflate" // Deflate compression (RFC 1951)
	CompressionBrotli  CompressionType = "br"      // Brotli compression
	CompressionCustom  CompressionType = "custom"  // Custom compression implementation
)

// UniqueIdentifierType defines the type of unique identifier to use for request tracing.
// It supports both UUID and ULID formats.
type UniqueIdentifierType string

// Supported identifier types for request tracing
const (
	IdentifierNone UniqueIdentifierType = ""     // No identifier
	IdentifierUUID UniqueIdentifierType = "uuid" // UUID v4
	IdentifierULID UniqueIdentifierType = "ulid" // ULID timestamp-based identifier
	IdentifierRGS  UniqueIdentifierType = "rgs"  // Randomly generated string
)

// Common errors returned by Option methods
var (
	ErrInvalidWriterType  = errors.New("invalid writer type")
	ErrMissingFilePath    = errors.New("file path must be specified when using WriteToFile")
	ErrUnexpectedFilePath = errors.New("filepath should not be provided when using WriteToBuffer")
	ErrInvalidCompression = errors.New("unsupported compression type")
)

// Option provides configuration for HTTP requests. It allows customization of various aspects
// of the request including headers, compression, logging, response handling, and progress tracking.
// If no options are provided when making a request, a default configuration is automatically generated.
type Option struct {
	initialised              bool                                           // Internal - determine if the struct was initialised with a call to New()
	client                   *http.Client                                   // Default or custom *http.Client
	filename                 string                                         // keep track of the filename when using PrepareFile and following redirects
	file                     *os.File                                       // If using a file (PrepareFile) store it here for better management
	filesize                 int64                                          // size of file if being used
	entropy                  *ulid.MonotonicEntropy                         // for ULID
	Verbose                  bool                                           // Whether logging should be verbose or not
	Logger                   slog.Logger                                    // Logging - default uses the slog TextHandler
	Header                   http.Header                                    // Headers to be included in the request
	Cookies                  []*http.Cookie                                 // Cookies to be included in the request
	ProtocolScheme           string                                         // define a custom protocol scheme. It defaults to https
	Compression              CompressionType                                // CompressionType to use: none, gzip, deflate or brotli
	CustomCompressionType    CompressionType                                // When using a custom compression, specify the type to be used as the content-encoding header.
	CustomCompressor         func(w *io.PipeWriter) (io.WriteCloser, error) // Function for custom compression
	CustomDecompressor       func(r io.Reader) (io.Reader, error)           // Function for custom decompression
	UserAgent                string                                         // User Agent to send with requests
	FollowRedirects          bool                                           // Disable or enable redirects. Default is false i.e.: follow redirects
	PreserveMethodOnRedirect bool                                           // Default is false
	MaxRedirects             int                                            // Maximum number of redirects that can happen before the client gives up
	currRedirect             int                                            // count of current redirects that have been performed
	mu                       sync.Mutex                                     // mutex for currRedirect
	UniqueIdentifierType     UniqueIdentifierType                           // Internal trace or identifier for the request
	Transport                *http.Transport                                // Create our own default transport
	ResponseWriter           ResponseWriter                                 // Define the type of response writer
	UploadBufferSize         *int                                           // Control the size of the buffer when uploading a file
	DownloadBufferSize       *int                                           // Control the size of the buffer when downloading a file
	OnUploadProgress         func(bytesRead, totalBytes int64)              // To monitor and track progress when uploading
	OnDownloadProgress       func(bytesRead, totalBytes int64)              // To monitor and track progress when downloading
}

// New creates a default Option with pre-configured settings. If additional options are provided
// via the variadic parameter, they will be merged with the default settings, with the provided
// options taking precedence.
func New(opts ...*Option) *Option {
	if len(opts) > 0 {
		// if the variadic parameter Option
		if opts[0].initialised {
			return opts[0]
		}
		// If opts[0] is not initialized, initialize and merge it
		opt := defaultOption()
		opt.Merge(opts[0])
		return opt
	}
	// No options provided; return a new default Option
	return defaultOption()
}

// defaultOption initializes and returns a default Option with pre-configured settings.
func defaultOption() *Option {
	return &Option{
		initialised: true,
		entropy:     ulid.Monotonic(rand.Reader, 0),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		Verbose:                  false,
		Logger:                   *slog.New(slog.NewTextHandler(os.Stdout, nil)),
		Header:                   http.Header{},
		Compression:              CompressionNone,
		UserAgent:                ua,
		FollowRedirects:          false,
		PreserveMethodOnRedirect: false,
		MaxRedirects:             10,
		UniqueIdentifierType:     IdentifierULID,
		Transport:                defaultTransport(),
		ResponseWriter: ResponseWriter{
			Type: WriteToBuffer,
		},
	}
}

// defaultTransport creates and returns an http.Transport configured for typical internal/low-latency
// operations. The settings are optimized for reliable HTTP client usage in environments where
// request volume is moderate and network conditions are generally good.
func defaultTransport() *http.Transport {
	return &http.Transport{
		// Use proxy settings from environment variables (HTTP_PROXY, HTTPS_PROXY, NO_PROXY)
		Proxy: http.ProxyFromEnvironment,

		// Configure the dialer with conservative timeouts for typical internal network conditions
		DialContext: (&net.Dialer{
			Timeout:   15 * time.Second, // Shorter timeout for internal services
			KeepAlive: 15 * time.Second, // Moderate keep-alive for connection reuse
			DualStack: true,             // Support both IPv4 and IPv6
		}).DialContext,

		// Connection pooling settings for moderate traffic
		MaxIdleConns:        50, // Total idle connections in the pool
		MaxIdleConnsPerHost: 10, // Idle connections per host (internal services typically use few hosts)
		MaxConnsPerHost:     10, // Limit concurrent connections per host

		// Timeout settings optimized for internal network conditions
		IdleConnTimeout:       60 * time.Second, // How long to keep idle connections
		ResponseHeaderTimeout: 30 * time.Second, // Max time to wait for response headers
		TLSHandshakeTimeout:   5 * time.Second,  // Max time for TLS handshake
		ExpectContinueTimeout: 1 * time.Second,  // Timeout for 100-continue responses

		// Protocol and behavior settings
		ForceAttemptHTTP2:  true,  // Prefer HTTP/2 when available
		DisableCompression: false, // Allow response compression
		DisableKeepAlives:  false, // Enable connection reuse
	}
}

// Log logs a message with the configured logger if verbose logging is enabled.
// The message will be logged at INFO level with any additional arguments provided.
func (opt *Option) Log(msg string, args ...any) {
	if opt.Verbose {
		opt.Logger.Info(msg, args...)
	}
}

// EnableLogging turns on verbose logging for the Option instance.
func (opt *Option) EnableLogging() {
	opt.Verbose = true
}

// DisableLogging turns off verbose logging for the Option instance.
func (opt *Option) DisableLogging() {
	opt.Verbose = false
}

// UseTextLogger configures the Option to use a text-based logger and enables verbose logging.
// The logger will output to stdout using the default slog TextHandler format.
func (opt *Option) UseTextLogger() {
	opt.Verbose = true
	opt.Logger = *slog.New(slog.NewTextHandler(os.Stdout, nil))
}

// UseJsonLogger configures the Option to use a JSON-based logger and enables verbose logging.
// The logger will output to stdout using the default slog JSONHandler format.
func (opt *Option) UseJsonLogger() {
	opt.Verbose = true
	opt.Logger = *slog.New(slog.NewJSONHandler(os.Stdout, nil))
}

// SetLogger configures a custom logger and enables verbose logging.
// The provided logger will replace any existing logger configuration.
func (opt *Option) SetLogger(logger *slog.Logger) {
	opt.Verbose = true
	opt.Logger = *logger
}

// AddHeader adds a new header with the specified key and value to the request headers.
// If the headers map hasn't been initialized, it will be created.
// Kept for backwards compatability
func (opt *Option) AddHeader(key string, value string) {
	if opt.Header == nil {
		opt.Header = http.Header{}
	}
	// Use set over add to replace the key with the value
	opt.Header.Set(key, value)
}

// ClearHeaders removes all previously set headers from the Option.
func (opt *Option) ClearHeaders() {
	opt.Header = http.Header{}
}

// AddCookie adds a new cookie to the Option's cookie collection.
// If the cookie slice hasn't been initialized, it will be created.
func (opt *Option) AddCookie(cookie *http.Cookie) {
	if opt.Cookies == nil {
		opt.Cookies = []*http.Cookie{}
	}
	opt.Cookies = append(opt.Cookies, cookie)
}

// ClearCookies removes all previously set cookies from the Option.
func (opt *Option) ClearCookies() {
	opt.Cookies = []*http.Cookie{}
}

// SetProtocolScheme sets the protocol scheme (e.g., "http://", "https://") for requests.
// If the provided scheme doesn't end with "://", it will be automatically appended.
func (opt *Option) SetProtocolScheme(scheme string) {
	if !strings.HasSuffix(scheme, "://") {
		scheme += "://"
	}
	opt.ProtocolScheme = scheme
}

// CreatePayloadReader converts the given payload into an io.Reader along with its size.
// Supported payload types include:
// - nil: Returns a nil reader and a size of -1.
// - []byte: Returns a bytes.Reader for the byte slice and its length as size.
// - io.Reader: Returns the reader and attempts to determine its size if it implements io.Seeker.
// - string: Returns a strings.Reader for the string and its length as size.
// For unsupported payload types, an error is returned.
func (opt *Option) CreatePayloadReader(payload any) (io.Reader, int64, error) {
	switch v := payload.(type) {
	case nil:
		// No payload, return nil reader and size -1
		return nil, -1, nil
	case []byte:
		// Byte slice payload, return bytes.Reader and its length
		opt.Log("Setting payload reader", "reader", "bytes.Reader")
		return bytes.NewReader(v), int64(len(v)), nil
	case io.Reader:
		// io.Reader payload, determine size if possible using io.Seeker
		size := int64(-1)
		if seeker, ok := v.(io.Seeker); ok {
			// Ensure the file pointer is at the start before seeking the size
			seeker.Seek(0, io.SeekStart)
			currentPos, _ := seeker.Seek(0, io.SeekCurrent)
			size, _ = seeker.Seek(0, io.SeekEnd)
			seeker.Seek(currentPos, io.SeekStart)
		}
		opt.Log("Setting payload reader", "reader", "io.Reader")
		return v, size, nil
	case string:
		// String payload, return strings.Reader and its length
		opt.Log("Setting payload reader", "reader", "strings.Reader")
		return strings.NewReader(v), int64(len(v)), nil
	default:
		// Unsupported payload type, return an error
		return nil, -1, fmt.Errorf("unsupported payload type: %T", payload)
	}
}

// PrepareFile sets up a file for upload by opening it, checking its existence,
// setting metadata like size and content type, and configuring appropriate headers.
// It takes a filename string as input and returns an error if the file cannot be
// accessed, doesn't exist, or fails to open. The method also automatically sets
// Content-Disposition headers appropriate for file uploads.
func (opt *Option) PrepareFile(filename string) error {
	opt.filename = filename

	fileinfo, err := os.Stat(opt.filename)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("file does not exist: %s", opt.filename)
		}
		return fmt.Errorf("failed to access file: %v", err)
	}

	opt.filesize = fileinfo.Size()

	opt.file, err = os.Open(opt.filename)
	if err != nil {
		return fmt.Errorf("failed to open file: %v", err)
	}

	opt.InferContentType(opt.file, fileinfo)

	// Add Content-Disposition header
	contentDisposition := fmt.Sprintf(`form-data; name="file"; filename="%s"`, filepath.Base(opt.filename))
	opt.AddHeader("Content-Disposition", contentDisposition)

	return nil
}

// SetFile configures an already-opened file for use with the client.
// It takes an *os.File pointer and sets internal metadata like filename
// and filesize. Unlike PrepareFile, this method assumes the file is
// already opened and valid. If file stats cannot be read, the method
// will silently fail rather than return an error.
func (opt *Option) SetFile(file *os.File) {
	opt.file = file
	opt.filename = file.Name()
	// Get the file's FileInfo and retrieve the size
	fileInfo, err := file.Stat()
	if err != nil {
		// Handle error if needed, for example logging it
		return
	}

	// Set the file size (in bytes)
	opt.filesize = fileInfo.Size()
}

// HasFileHandle returns true if there is currently a file configured for use
// with this Option instance. This can be used to check if file operations
// should be performed during request processing.
func (opt *Option) HasFileHandle() bool {
	return opt.file != nil
}

// GetFile returns the currently configured *os.File, if any.
// Returns nil if no file has been set. The returned file may be
// either opened or closed depending on the stage of request processing.
func (opt *Option) GetFile() *os.File {
	return opt.file
}

// Filesize returns the size in bytes of the currently configured file.
// Returns 0 if no file is set or if the file size could not be determined.
// This value is set when the file is initially prepared or set.
func (opt *Option) Filesize() int64 {
	return opt.filesize
}

// ReopenFile attempts to reopen a previously closed file using the stored filename.
// This is particularly useful during redirect handling when a file needs to be
// re-read. Returns the reopened file and any error encountered. Logs the reopening
// attempt through the configured logger.
func (opt *Option) ReopenFile() (*os.File, error) {
	opt.Log("Reopening file", "filename", opt.filename, "filesize", opt.filesize)
	var err error
	opt.file, err = os.Open(opt.filename)
	if err != nil {
		return nil, err
	}
	return opt.file, err
}

// CloseFile closes the currently open file if one exists and resets related
// metadata. It logs the closure through the configured logger and returns any
// error encountered during closing. After closing, the file pointer is set to
// nil and the filesize is reset to 0.
func (opt *Option) CloseFile() error {
	opt.Log("Closing file", "filename", opt.filename)
	if opt.file != nil {
		err := opt.file.Close()
		opt.file = nil
		opt.filesize = 0
		return err
	}
	return nil
}

// InferContentType determines the MIME type of a file based on its content and extension.
// If it is unable to determine a MIME type, it defaults to application/octet-stream.
func (opt *Option) InferContentType(file *os.File, fileInfo os.FileInfo) error {
	// check if a content type has already been defined
	if opt.Header.Get("Content-Type") != "" {
		return nil
	}

	// default content type: application/octet-stream
	contentType := "application/octet-stream"

	// Use a buffer to read a portion of the file for detecting its MIME type.
	buffer := make([]byte, 512)
	_, err := file.Read(buffer)
	if err != nil {
		return err
	}

	// Reset the file pointer after reading.
	if _, err := file.Seek(0, 0); err != nil {
		return err
	}

	// Try to detect MIME type from file content.
	detectedContentType := http.DetectContentType(buffer)
	if detectedContentType != "" {
		contentType = detectedContentType
	}

	// Check for MIME type based on file extension and use it if available.
	extMimeType := mime.TypeByExtension(filepath.Ext(fileInfo.Name()))
	if extMimeType != "" {
		contentType = extMimeType
	}

	opt.AddHeader("Content-Type", contentType)
	return nil
}

// SetCompression configures the compression type to be used for the request.
// Valid compression types include: none, gzip, deflate, brotli, and custom.
func (opt *Option) SetCompression(compressionType CompressionType) {
	opt.Compression = compressionType
}

// GetCompressor returns an appropriate io.WriteCloser based on the configured compression type.
func (opt *Option) GetCompressor(w *io.PipeWriter) (io.WriteCloser, error) {
	switch opt.Compression {
	case CompressionGzip:
		return gzip.NewWriter(w), nil
	case CompressionDeflate:
		return zlib.NewWriter(w), nil
	case CompressionBrotli:
		return brotli.NewWriter(w), nil
	case CompressionCustom:
		if opt.CustomCompressor == nil {
			return nil, fmt.Errorf("custom compression specified but no compressor provided")
		}
		return opt.CustomCompressor(w)
	case CompressionNone:
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported compression type: %s", opt.Compression)
	}
}

// GetDecompressor returns an appropriate io.Reader for the given encoding.
func (opt *Option) GetDecompressor(r io.ReadCloser, encoding string) (io.ReadCloser, error) {
	switch encoding {
	case "":
		return r, nil
	case string(CompressionGzip):
		return gzip.NewReader(r)
	case string(CompressionDeflate):
		return zlib.NewReader(r)
	case string(CompressionBrotli):
		return io.NopCloser(brotli.NewReader(r)), nil
	default:
		// Try custom decompressor if available
		if opt.CustomDecompressor != nil {
			reader, err := opt.CustomDecompressor(r)
			if err != nil {
				return nil, err
			}
			if rc, ok := reader.(io.ReadCloser); ok {
				return rc, nil
			}
			return io.NopCloser(reader), nil
		}
		return nil, fmt.Errorf("unsupported compression type: %s", encoding)
	}
}

func (opt *Option) Redirects(enabled bool, preserve bool) {
	opt.FollowRedirects = enabled
	opt.PreserveMethodOnRedirect = preserve
}

// EnableRedirects configures the Option to follow HTTP redirects.
func (opt *Option) EnableRedirects() {
	opt.FollowRedirects = true
}

// DisableRedirects configures the Option to not follow HTTP redirects.
func (opt *Option) DisableRedirects() {
	opt.FollowRedirects = false
}

// EnablePreserveMethodOnRedirect configures redirects to maintain the original HTTP method.
func (opt *Option) EnablePreserveMethodOnRedirect() {
	opt.PreserveMethodOnRedirect = true
}

// DisablePreserveMethodOnRedirect configures redirects to not maintain the original HTTP method.
func (opt *Option) DisablePreserveMethodOnRedirect() {
	opt.PreserveMethodOnRedirect = false
}

func (opt *Option) SetMaxRedirects(size int) {
	opt.MaxRedirects = size
}

func (opt *Option) GetMaxRedirects() int {
	return opt.MaxRedirects
}

func (opt *Option) CheckRedirects() bool {
	opt.mu.Lock()
	defer opt.mu.Unlock()
	opt.currRedirect++
	opt.Log("CurrentRedirects", "currRedirect", opt.currRedirect, "MaxRedirect", opt.MaxRedirects)
	return opt.currRedirect == opt.MaxRedirects
}

// SetTransport configures a custom HTTP transport for the requests.
// This allows fine-grained control over connection pooling, timeouts, and other transport-level settings.
func (opt *Option) SetTransport(transport *http.Transport) {
	opt.Transport = transport
}

// GenerateIdentifier creates a unique identifier based on the configured UniqueIdentifierType.
// Returns a UUID or ULID string, or an empty string if no identifier type is configured.
func (opt *Option) GenerateIdentifier() string {
	switch opt.UniqueIdentifierType {
	case IdentifierUUID:
		return uuid.New().String()
	case IdentifierULID:
		opt.mu.Lock()
		defer opt.mu.Unlock()
		return ulid.MustNew(ulid.Timestamp(time.Now()), opt.entropy).String()
	case IdentifierRGS:
		str, err := generateRandomString(15)
		if err != nil {
			opt.Logger.Warn("generateRandomString failed, falling back to uuid.", "error", err)
			return uuid.New().String()
		}
		return str
	}
	return ""
}

// GenerateRandomString generates a cryptographically secure random string of the specified length
const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func generateRandomString(length int) (string, error) {
	result := make([]byte, length)
	maxByte := byte(256 - (256 % len(charset)))

	for i := 0; i < length; {
		b := make([]byte, 1)
		_, err := rand.Read(b)
		if err != nil {
			return "", fmt.Errorf("failed to generate random string: %w", err)
		}

		// Reject numbers that would make the distribution uneven
		if b[0] >= maxByte {
			continue
		}

		// Map the byte to our charset
		result[i] = charset[b[0]%byte(len(charset))]
		i++
	}

	return string(result), nil
}

// SetDownloadBufferSize configures the buffer size used when downloading files.
// The size must be positive; otherwise, the setting will be ignored.
func (opt *Option) SetDownloadBufferSize(size int) {
	if size > 0 {
		opt.DownloadBufferSize = &size
	}
}

// InitialiseWriter sets up the appropriate writer based on the ResponseWriter configuration.
// Returns an error if the writer type is invalid or if required parameters are missing.
func (opt *Option) InitialiseWriter() (io.WriteCloser, error) {
	switch opt.ResponseWriter.Type {
	case WriteToFile:
		if opt.ResponseWriter.FilePath == "" {
			return nil, ErrMissingFilePath
		}
		file, err := os.Create(opt.ResponseWriter.FilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to create file: %w", err)
		}
		opt.ResponseWriter.writer = file
	case WriteToBuffer:
		if opt.ResponseWriter.FilePath != "" {
			return nil, ErrUnexpectedFilePath
		}
		opt.ResponseWriter.writer = &WriteCloserBuffer{Buffer: &bytes.Buffer{}}
	default:
		return nil, ErrInvalidWriterType
	}
	return opt.ResponseWriter.writer, nil
}

// GetWriter returns the currently configured io.WriteCloser instance.
func (opt *Option) GetWriter() io.WriteCloser {
	return opt.ResponseWriter.writer
}

// SetOutput configures how the response should be written, either to a file or buffer.
// For file output, a filepath must be provided. Returns an error if the configuration is invalid.
func (opt *Option) SetOutput(writerType ResponseWriterType, filepath ...string) error {
	opt.ResponseWriter.Type = writerType

	switch writerType {
	case WriteToFile:
		if len(filepath) == 0 {
			return ErrMissingFilePath
		}
		opt.ResponseWriter.FilePath = filepath[0]
	case WriteToBuffer:
		if len(filepath) > 0 {
			return ErrUnexpectedFilePath
		}
		opt.ResponseWriter.FilePath = ""
	default:
		return ErrInvalidWriterType
	}

	return nil
}

// SetFileOutput configures the response writer to write responses to a file at the specified path.
func (opt *Option) SetFileOutput(filepath string) {
	opt.ResponseWriter = ResponseWriter{
		Type:     WriteToFile,
		FilePath: filepath,
	}
}

// SetBufferOutput configures the response writer to write responses to an in-memory buffer.
func (opt *Option) SetBufferOutput() {
	opt.ResponseWriter = ResponseWriter{
		Type: WriteToBuffer,
	}
}

// Merge combines the settings from another Option instance into this one.
// Settings from the source Option take precedence over existing settings.
// This includes headers, cookies, compression settings, and all other configuration options.
func (opt *Option) Merge(src *Option) {
	if src == nil {
		return
	}
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
	opt.FollowRedirects = src.FollowRedirects
	opt.PreserveMethodOnRedirect = src.PreserveMethodOnRedirect
	opt.MaxRedirects = src.MaxRedirects

	if src.Logger != (slog.Logger{}) {
		opt.Logger = src.Logger
	}

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

	if src.CustomDecompressor != nil {
		opt.CustomDecompressor = src.CustomDecompressor
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

// GetClient returns the HTTP client to be used for requests.
// If a custom client has been set via SetClient, that client is returned.
// Otherwise, returns a new default http.Client instance.
func (o *Option) GetClient() *http.Client {
	if o.client != nil {
		return o.client
	}
	return &http.Client{}
}

// SetClient configures a custom HTTP client to be used for requests.
// This client will be used instead of the default client for all subsequent
// requests made with this Option instance. The provided client should be
// configured with any desired settings (timeouts, transport, etc) before
// being set.
func (opt *Option) SetClient(client *http.Client) {
	opt.client = client
}
