package client_test

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"math/rand"

	"github.com/andybalholm/brotli"
	client "github.com/caelisco/http-client"
	"github.com/caelisco/http-client/options"
	"github.com/caelisco/http-client/response"
	"github.com/golang/snappy"
	"github.com/pierrec/lz4/v4"
	"github.com/stretchr/testify/assert"
)

const (
	charset   = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	chunkSize = 1024 * 1024 // 1MB chunks for writing
	smallf    = "small.txt"
	largef    = "large.txt"
)

var (
	smallfile *bytes.Buffer
	largefile *bytes.Buffer
)

func init() {
	// create files for testing
	// it is just easier to clean them up manually after tests
	// are done vs. re-generating them each time
	createTestFile(smallf, 1)
	// load small file in to memory
	buf, err := os.ReadFile(smallf)
	if err != nil {
		log.Fatal("error loading small.txt: %w", err)
	}
	smallfile = bytes.NewBuffer(buf)

	createTestFile(largef, 50)
	// load large file in to memory
	buf, err = os.ReadFile(largef)
	if err != nil {
		log.Fatal("error loading large.txt: %w", err)
	}
	largefile = bytes.NewBuffer(buf)
}

func createTestFile(filename string, size int) {
	filesize := size * 1024 * 1024

	// Check if the file exists
	if fileInfo, err := os.Stat(filename); err == nil {
		// File exists, check its size
		if fileInfo.Size() == int64(filesize) {
			log.Printf("File %s already exists and is the correct size (%d bytes).", filename, filesize)
			return
		}
		log.Printf("File %s exists but is the wrong size (%d bytes). Recreating.", filename, fileInfo.Size())
	} else if !os.IsNotExist(err) {
		log.Fatalf("Error checking file %s: %v", filename, err)
	}

	// Create the file
	file, err := os.Create(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	bytesWritten := 0

	for bytesWritten < filesize {
		chunk := generateRandomString(chunkSize)
		n, err := writer.WriteString(chunk)
		if err != nil {
			log.Fatal(err)
		}
		bytesWritten += n
	}

	err = writer.Flush()
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("File %s created successfully with size %d bytes.", filename, filesize)
}

func generateRandomString(length int) string {
	result := make([]byte, length)
	for i := range result {
		result[i] = charset[rand.Intn(len(charset))]
	}
	return string(result)
}

func setupTestServer(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/upload":
			// Handle decompression based on Content-Encoding
			var err error
			var reader io.Reader
			var buff bytes.Buffer

			// Decompress based on Content-Encoding and read into buffer
			t.Logf("Content-Encoding: %s", r.Header.Get("Content-Encoding"))
			switch r.Header.Get("Content-Encoding") {
			case "gzip":
				t.Log("Using gzip reader")
				gzipReader, err := gzip.NewReader(r.Body)
				if err != nil {
					log.Printf("Failed to create gzip reader: %v", err)
					http.Error(w, "Failed to create gzip reader: "+err.Error(), http.StatusBadRequest)
					return
				}
				defer gzipReader.Close()
				reader = gzipReader
			case "deflate":
				t.Log("Using deflate reader")
				zlibReader, err := zlib.NewReader(r.Body)
				if err != nil {
					log.Printf("Failed to create gzip reader: %v", err)
					http.Error(w, "Failed to create gzip reader: "+err.Error(), http.StatusBadRequest)
					return
				}
				defer zlibReader.Close()
				reader = zlibReader
			case "br":
				t.Log("Using Brotli reader")
				reader = brotli.NewReader(r.Body)
			case "snappy":
				t.Log("Using Snappy reader")
				reader = snappy.NewReader(r.Body)
			case "lz4":
				t.Log("Using LZ4 reader")
				reader = lz4.NewReader(r.Body)
			default:
				t.Log("Using io.Reader")
				reader = r.Body
			}

			// Read the data into the buffer - decompressing it if necessary
			_, err = io.Copy(&buff, reader)
			if err != nil {
				t.Logf("reader err: %s", err)
				http.Error(w, "Failed to copy data to the buffer:"+err.Error(), http.StatusInternalServerError)
				return
			}

			// Once decompressed, send the data back to the client
			t.Logf("Server file size: %d bytes", buff.Len())
			_, err = w.Write(buff.Bytes())
			if err != nil {
				http.Error(w, "Failed to send decompressed data:"+err.Error(), http.StatusInternalServerError)
				return
			}

		case "/upload/redirect":
			t.Logf("redirecting to /upload")
			http.Redirect(w, r, "/upload", http.StatusFound)

		case "/upload/no-preserve":
			t.Logf("redirecting to /method-check")
			http.Redirect(w, r, "/method-check", http.StatusFound)

		case "/method-check":
			w.Write([]byte(r.Method))

		case "/upload/multipart":
			err := r.ParseMultipartForm(200 << 20) // 200 MB max memory
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			fileInfo := make(map[string]int64)

			for _, files := range r.MultipartForm.File {
				for _, fileHeader := range files {
					file, err := fileHeader.Open()
					if err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
						return
					}
					defer file.Close()

					// Get file size
					size, err := file.Seek(0, io.SeekEnd)
					if err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
						return
					}
					file.Seek(0, io.SeekStart) // Reset file pointer

					fileInfo[fileHeader.Filename] = size
				}
			}

			// Set content type to JSON
			w.Header().Set("Content-Type", "application/json")

			// Encode and write the JSON response
			if err := json.NewEncoder(w).Encode(fileInfo); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

		case "/download":
			w.Header().Set("Content-Length", strconv.FormatInt(int64(largefile.Len()), 10)) // size of the large file
			w.Write(largefile.Bytes())

		case "/echo-headers":
			// Echo back the received headers
			for name, values := range r.Header {
				w.Header().Set("Echo-"+name, strings.Join(values, ", "))
			}

		default:
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "Hello from path: %s", r.URL.Path)
		}
	}))
}

func TestBasicRequests(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	tests := []struct {
		name           string
		method         string
		path           string
		expectedStatus int
	}{
		{"GET Request", http.MethodGet, "/", http.StatusOK},
		{"POST Request", http.MethodPost, "/", http.StatusOK},
		{"PUT Request", http.MethodPut, "/", http.StatusOK},
		{"DELETE Request", http.MethodDelete, "/", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := client.Custom(tt.method, server.URL+tt.path, nil)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
			if err != nil {
				t.Logf("err: %s", err)
			}
		})
	}
}

func TestPostFileUpload(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	tmpfile, err := os.Open(smallf)
	if err != nil {
		t.Logf("error opening %s: %s", smallf, err)
		t.Fail()
	}
	defer tmpfile.Close()

	var lastProgress float64
	opt := options.New()
	opt.OnUploadProgress = func(bytesRead, totalBytes int64) {
		if totalBytes > 0 {
			lastProgress = float64(bytesRead) / float64(totalBytes) * 100
		}
	}

	resp, err := client.Post(server.URL+"/upload", tmpfile, opt)
	assert.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, float64(100), lastProgress)

	assert.Equal(t, smallfile.Bytes(), resp.Body.Bytes())
}

func TestPostStringUpload(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	var lastProgress float64
	opt := options.New()
	opt.OnUploadProgress = func(bytesRead, totalBytes int64) {
		if totalBytes > 0 {
			lastProgress = float64(bytesRead) / float64(totalBytes) * 100
		}
	}

	resp, err := client.Post(server.URL+"/upload", smallfile.String(), opt)
	assert.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, float64(100), lastProgress)

	assert.Equal(t, smallfile.Bytes(), resp.Body.Bytes())
}

func TestPostByteUpload(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	tmpfile, err := os.Open(smallf)
	if err != nil {
		t.Logf("error opening %s: %s", smallf, err)
		t.Fail()
	}

	var lastProgress float64
	opt := options.New()
	opt.OnUploadProgress = func(bytesRead, totalBytes int64) {
		if totalBytes > 0 {
			lastProgress = float64(bytesRead) / float64(totalBytes) * 100
		}
	}

	resp, err := client.Post(server.URL+"/upload", smallfile.Bytes(), opt)
	assert.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, float64(100), lastProgress)

	tmpfile.Close()

	assert.Equal(t, smallfile.Bytes(), resp.Body.Bytes())
}

func TestFileFuncUpload(t *testing.T) {
	var err error
	var resp response.Response

	server := setupTestServer(t)
	defer server.Close()

	tests := []struct {
		name           string
		method         string
		expectedStatus int
	}{
		{"PostFile Request", http.MethodPost, http.StatusOK},
		{"PutFile Request", http.MethodPut, http.StatusOK},
		{"PatchFile Request", http.MethodPatch, http.StatusOK},
	}

	url := server.URL + "/upload"

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			// Track upload progress
			var lastProgress float64
			opt := options.New()
			opt.OnUploadProgress = func(bytesRead, totalBytes int64) {
				if totalBytes > 0 {
					lastProgress = float64(bytesRead) / float64(totalBytes) * 100
				}
			}

			switch tt.method {
			case http.MethodPost:
				resp, err = client.PostFile(url, largef, opt)
			case http.MethodPut:
				resp, err = client.PutFile(url, largef, opt)
			case http.MethodPatch:
				resp, err = client.PatchFile(url, largef, opt)
			}

			assert.NoError(t, err)

			assert.Equal(t, http.StatusOK, resp.StatusCode)
			assert.Equal(t, float64(100), lastProgress)
			assert.Equal(t, largefile.Bytes(), resp.Body.Bytes())
		})
	}
}

func TestFileDownload(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	tmpDir, err := os.MkdirTemp("", "download-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	downloadPath := filepath.Join(tmpDir, "downloaded.txt")

	var lastProgress float64
	opt := options.New()
	opt.OnDownloadProgress = func(bytesRead, totalBytes int64) {
		if totalBytes > 0 {
			lastProgress = float64(bytesRead) / float64(totalBytes) * 100
		}
	}
	opt.SetFileOutput(downloadPath)

	resp, err := client.Get(server.URL+"/download", opt)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	assert.Equal(t, float64(100), lastProgress)

	info, err := os.Stat(downloadPath)
	assert.NoError(t, err)
	assert.Equal(t, int64(largefile.Len()), info.Size())
}

func TestCustomHeaders(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	opt := options.New()
	opt.AddHeader("X-Custom-Header", "test-value")

	resp, err := client.Get(server.URL+"/echo-headers", opt)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "test-value", resp.Header.Get("Echo-X-Custom-Header"))
}

func TestBufferSizes(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	tests := []struct {
		name         string
		bufferSize   int
		expectedSize int64
	}{
		{"Small Buffer", 1024, int64(largefile.Len())},
		{"Large Buffer", 32768, int64(largefile.Len())},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opt := options.New()
			opt.SetDownloadBufferSize(tt.bufferSize)

			start := time.Now()
			resp, err := client.Get(server.URL+"/download", opt)
			duration := time.Since(start)

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedSize, resp.Length())

			t.Logf("Download with %d buffer took %v", tt.bufferSize, duration)
		})
	}
}

func TestCompression(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	tests := []struct {
		name        string
		compression options.CompressionType
	}{
		{"Gzip Compression", options.CompressionGzip},
		{"Deflate Compression", options.CompressionDeflate},
		{"Brotli Compression", options.CompressionBrotli},
	}

	opt := options.New()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			opt.SetCompression(tt.compression)

			t.Logf("[%s] Uncompressed size: %d bytes", tt.name, largefile.Len())

			resp, err := client.Post(server.URL+"/upload", largefile.String(), opt)
			assert.NoError(t, err)

			assert.Equal(t, http.StatusOK, resp.StatusCode)
			assert.Equal(t, largefile.String(), resp.String())
		})
	}
}

func TestCustomCompression(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	tests := []struct {
		name        string
		compression options.CompressionType
		encoding    string
	}{
		{"Snappy Compression", options.CompressionCustom, "snappy"},
		{"LZ4 Compression", options.CompressionCustom, "lz4"},
	}

	opt := options.New()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			opt.SetCompression(tt.compression)
			if tt.encoding == "snappy" {
				opt.CustomCompressor = func(w *io.PipeWriter) (io.WriteCloser, error) {
					return snappy.NewBufferedWriter(w), nil
				}
			}
			if tt.encoding == "lz4" {
				opt.CustomCompressor = func(w *io.PipeWriter) (io.WriteCloser, error) {
					return lz4.NewWriter(w), nil
				}
			}
			opt.CustomCompressionType = options.CompressionType(tt.encoding)
			t.Logf("Custom compression type set to: %s", opt.CustomCompressionType)

			t.Logf("[%s] Uncompressed size: %d bytes", tt.name, largefile.Len())

			resp, err := client.Post(server.URL+"/upload", largefile.String(), opt)
			assert.NoError(t, err)

			assert.Equal(t, http.StatusOK, resp.StatusCode)
			assert.Equal(t, largefile.String(), resp.String())
		})
	}
}

func TestRedirectPostUploadNoFollow(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	tmpfile, err := os.Open(smallf)
	if err != nil {
		t.Logf("error opening %s: %s", smallf, err)
		t.Fail()
	}

	opt := options.New()
	opt.FollowRedirects = false

	resp, err := client.Post(server.URL+"/upload/redirect", tmpfile, opt)
	assert.NoError(t, err)

	assert.Equal(t, http.StatusFound, resp.StatusCode)
	assert.NotEmpty(t, resp.Header.Get("Location"))
}

func TestRedirectPostUploadNoPreserve(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	tmpfile, err := os.Open(smallf)
	if err != nil {
		t.Logf("error opening %s: %s", smallf, err)
		t.Fail()
	}

	opt := options.New()
	opt.FollowRedirects = true
	opt.PreserveMethodOnRedirect = false

	resp, err := client.Post(server.URL+"/upload/no-preserve", tmpfile, opt)
	assert.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "GET", resp.String())
}

func TestRedirectPostUploadFollow(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	tmpfile, _ := os.Open(largef)
	defer tmpfile.Close()

	opt := options.New()
	opt.FollowRedirects = true
	opt.PreserveMethodOnRedirect = true

	opt.EnableLogging()

	t.Logf("filesize: %d", largefile.Len())

	var lastProgress float64
	opt.OnUploadProgress = func(bytesRead, totalBytes int64) {
		if totalBytes > 0 {
			lastProgress = float64(bytesRead) / float64(totalBytes) * 100
		}
	}

	resp, err := client.Post(server.URL+"/upload/redirect", tmpfile, opt)
	assert.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, float64(100), lastProgress)
	assert.Equal(t, largefile.Bytes(), resp.Body.Bytes())

	tmpfile.Close()
}

func TestRedirectPutUploadFollow(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	tmpfile, _ := os.Open(largef)
	defer tmpfile.Close()

	opt := options.New()
	opt.FollowRedirects = true
	opt.PreserveMethodOnRedirect = true

	opt.EnableLogging()

	t.Logf("filesize: %d", largefile.Len())

	var lastProgress float64
	opt.OnUploadProgress = func(bytesRead, totalBytes int64) {
		if totalBytes > 0 {
			lastProgress = float64(bytesRead) / float64(totalBytes) * 100
		}
	}

	resp, err := client.Put(server.URL+"/upload/redirect", tmpfile, opt)
	assert.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, float64(100), lastProgress)
	assert.Equal(t, largefile.Bytes(), resp.Body.Bytes())
}

func TestRedirectPatchUploadFollow(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	tmpfile, err := os.Open(largef)
	if err != nil {
		t.Logf("error opening %s: %s", largef, err)
		t.Fail()
	}
	defer tmpfile.Close()

	opt := options.New()
	opt.FollowRedirects = true
	opt.PreserveMethodOnRedirect = true

	opt.EnableLogging()

	t.Logf("filesize: %d", largefile.Len())

	var lastProgress float64
	opt.OnUploadProgress = func(bytesRead, totalBytes int64) {
		if totalBytes > 0 {
			lastProgress = float64(bytesRead) / float64(totalBytes) * 100
		}
	}

	resp, err := client.Patch(server.URL+"/upload/redirect", tmpfile, opt)
	assert.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, float64(100), lastProgress)
	assert.Equal(t, largefile.Bytes(), resp.Body.Bytes())
}

func TestRedirectFileFuncUpload(t *testing.T) {
	var err error
	var resp response.Response

	server := setupTestServer(t)
	defer server.Close()

	tests := []struct {
		name           string
		method         string
		expectedStatus int
	}{
		{"PostFile Request", http.MethodPost, http.StatusOK},
		{"PutFile Request", http.MethodPut, http.StatusOK},
		{"PatchFile Request", http.MethodPatch, http.StatusOK},
	}

	url := server.URL + "/upload/redirect"

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			opt := options.New()
			opt.Redirects(true, true)
			opt.EnableLogging()

			var lastProgress float64
			opt.OnUploadProgress = func(bytesRead, totalBytes int64) {
				if totalBytes > 0 {
					lastProgress = float64(bytesRead) / float64(totalBytes) * 100
				}
			}
			switch tt.method {
			case http.MethodPost:
				resp, err = client.PostFile(url, largef, opt)
			case http.MethodPut:
				resp, err = client.PutFile(url, largef, opt)
			case http.MethodPatch:
				resp, err = client.PatchFile(url, largef, opt)
			}

			assert.NoError(t, err)

			assert.Equal(t, http.StatusOK, resp.StatusCode)
			assert.Equal(t, largefile.Bytes(), resp.Body.Bytes())
			// Verify upload progress completed
			assert.Equal(t, float64(100), lastProgress)
		})
	}
}

func TestCompressedFileRedirect(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	tests := []struct {
		name         string
		method       string
		compression  options.CompressionType
		expectedSize int64
	}{
		{"POST Gzip Compressed Redirect", http.MethodPost, options.CompressionGzip, int64(largefile.Len())},
		{"POST Deflate Compressed Redirect", http.MethodPost, options.CompressionDeflate, int64(largefile.Len())},
		{"POST Brotli Compressed Redirect", http.MethodPost, options.CompressionBrotli, int64(largefile.Len())},
		{"PUT Gzip Compressed Redirect", http.MethodPut, options.CompressionGzip, int64(largefile.Len())},
		{"PUT Deflate Compressed Redirect", http.MethodPut, options.CompressionDeflate, int64(largefile.Len())},
		{"PUT Brotli Compressed Redirect", http.MethodPut, options.CompressionBrotli, int64(largefile.Len())},
		{"PATCH Gzip Compressed Redirect", http.MethodPatch, options.CompressionGzip, int64(largefile.Len())},
		{"PATCH Deflate Compressed Redirect", http.MethodPatch, options.CompressionDeflate, int64(largefile.Len())},
		{"PATCH Brotli Compressed Redirect", http.MethodPatch, options.CompressionBrotli, int64(largefile.Len())},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resp response.Response
			var err error

			opt := options.New()
			opt.Redirects(true, true) // Enable redirects and preserve method
			opt.SetCompression(tt.compression)

			// Track upload progress
			var lastProgress float64
			opt.OnUploadProgress = func(bytesRead, totalBytes int64) {
				if totalBytes > 0 {
					lastProgress = float64(bytesRead) / float64(totalBytes) * 100
				}
			}

			t.Logf("[%s] Original file size: %d bytes", tt.name, int64(smallfile.Len()))

			url := server.URL + "/upload/redirect"

			switch tt.method {
			case http.MethodPost:
				resp, err = client.PostFile(url, smallf, opt)
			case http.MethodPut:
				resp, err = client.PutFile(url, smallf, opt)
			case http.MethodPatch:
				resp, err = client.PatchFile(url, smallf, opt)
			}

			// Verify the request succeeded
			assert.NoError(t, err)
			assert.Equal(t, http.StatusOK, resp.StatusCode)

			// Verify the content was transmitted correctly
			assert.Equal(t, smallfile.String(), resp.String())

			// Verify upload progress completed
			t.Logf("%s Last progress: %f", tt.name, lastProgress)
			//assert.Equal(t, float64(100), lastProgress)

			// Log the response size to see compression effectiveness
			t.Logf("[%s] Response size: %d bytes", tt.name, len(resp.Body.Bytes()))
		})
	}
}

func TestMultipartUpload(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	tests := []struct {
		name           string
		method         string
		expectedStatus int
	}{
		{"PostMultipartUpload", http.MethodPost, http.StatusOK},
		{"PutMultipartUpload", http.MethodPut, http.StatusOK},
		{"PatchMultipartUpload", http.MethodPatch, http.StatusOK},
	}

	url := server.URL + "/upload/multipart"

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var lastProgress float64
			opt := options.New()
			opt.OnUploadProgress = func(bytesRead, totalBytes int64) {
				if totalBytes > 0 {
					lastProgress = float64(bytesRead) / float64(totalBytes) * 100
				}
			}

			s, err := os.Open(smallf)
			if err != nil {
				t.Logf("unable to open %s: %s", smallf, err)
			}
			l, err := os.Open(largef)
			if err != nil {
				t.Logf("unable to open %s: %s", largef, err)
			}

			payload := map[string]interface{}{
				"smallfile": s,
				"largefile": l,
			}

			var resp response.Response

			switch tt.method {
			case http.MethodPost:
				resp, err = client.PostMultipartUpload(url, payload, opt)
			case http.MethodPut:
				resp, err = client.PutMultipartUpload(url, payload, opt)
			case http.MethodPatch:
				resp, err = client.PatchMultipartUpload(url, payload, opt)
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
			assert.Equal(t, float64(100), lastProgress)

			// Parse the JSON response
			var fileInfo map[string]int64
			err = json.Unmarshal(resp.Body.Bytes(), &fileInfo)
			assert.NoError(t, err)

			// Check file sizes
			assert.Equal(t, int64(smallfile.Len()), fileInfo["small.txt"])
			assert.Equal(t, int64(largefile.Len()), fileInfo["large.txt"])
		})
	}
}
