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
	"sync"
	"testing"
	"time"

	"math/rand"

	"github.com/andybalholm/brotli"
	client "github.com/caelisco/http-client/v2"
	"github.com/caelisco/http-client/v2/options"
	"github.com/caelisco/http-client/v2/response"
	"github.com/golang/snappy"
	"github.com/pierrec/lz4/v4"
	"github.com/stretchr/testify/assert"
)

const (
	charset   = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	chunkSize = 1024 * 1024 // 1MB chunks for writing
	smallf    = "test-small.txt"
	largef    = "test-large.txt"
	downloadf = "test-download.txt"
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

		case "/max-redirects":
			t.Logf("redirecting to /max-redirects")
			http.Redirect(w, r, "/max-redirects", http.StatusFound)

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

		case "/download/compressed":
			compression := r.URL.Query().Get("compression")
			w.Header().Set("Content-Encoding", compression)

			var writer io.WriteCloser
			switch compression {
			case "gzip":
				writer = gzip.NewWriter(w)
			case "deflate":
				writer = zlib.NewWriter(w)
			case "br":
				writer = brotli.NewWriter(w)
			case "snappy":
				writer = snappy.NewBufferedWriter(w)
			case "lz4":
				writer = lz4.NewWriter(w)
			default:
				http.Error(w, "unsupported compression", http.StatusBadRequest)
				return
			}
			defer writer.Close()

			_, err := io.Copy(writer, bytes.NewReader(largefile.Bytes()))
			if err != nil {
				t.Logf("Compression error: %v", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

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

	var lastProgress int64
	opt := options.New()
	opt.OnUploadProgress = func(bytesRead, totalBytes int64) {
		if totalBytes > 0 {
			lastProgress = (bytesRead * 100) / totalBytes
		}
	}

	resp, err := client.Post(server.URL+"/upload", tmpfile, opt)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, int64(100), lastProgress)
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

	downloadPath := filepath.Join(tmpDir, downloadf)

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

func TestFileDownloadDirectToFile(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	var lastProgress float64
	opt := options.New()
	opt.SetFileOutput(downloadf)

	opt.OnDownloadProgress = func(bytesRead, totalBytes int64) {
		if totalBytes > 0 {
			lastProgress = float64(bytesRead) / float64(totalBytes) * 100
		}
	}

	resp, err := client.Get(server.URL+"/download", opt)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	assert.Equal(t, float64(100), lastProgress)

	info, err := os.Stat(downloadf)
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
			assert.Equal(t, tt.expectedSize, resp.Len())

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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			opt := options.New()
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

func TestStandardDecompression(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()
	tests := []struct {
		name         string
		compression  string
		expectedSize int64
	}{
		{"Gzip Decompression", "gzip", int64(largefile.Len())},
		{"Deflate Decompression", "deflate", int64(largefile.Len())},
		{"Brotli Decompression", "br", int64(largefile.Len())},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opt := options.New()
			opt.SetBufferOutput()
			opt.EnableLogging()

			var bytesReceived int64
			opt.OnDownloadProgress = func(bytesRead, totalBytes int64) {
				bytesReceived = bytesRead // Just track total bytes read
			}

			url := fmt.Sprintf("%s/download/compressed?compression=%s", server.URL, tt.compression)
			resp, err := client.Get(url, opt)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}

			// Debug info
			t.Logf("Response info: Status=%d, Len=%d, BodyEmpty=%v",
				resp.StatusCode, resp.Len(), resp.Body.IsEmpty())
			t.Logf("Response headers: %v", resp.Header)

			if resp.Body.IsEmpty() {
				t.Fatal("Response body is empty")
			}

			assert.Equal(t, http.StatusOK, resp.StatusCode)
			assert.Equal(t, tt.expectedSize, int64(resp.Len()))
			assert.Equal(t, largefile.Bytes(), resp.Body.Bytes())
			assert.Equal(t, int64(largefile.Len()), bytesReceived)

			t.Logf("[%s] Original size: %d, Compressed transfer",
				tt.name,
				largefile.Len())
		})
	}
}

func TestCustomDecompression(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()
	tests := []struct {
		name        string
		compression options.CompressionType
		encoding    string
	}{
		{"Snappy Decompression", options.CompressionCustom, "snappy"},
		{"LZ4 Decompression", options.CompressionCustom, "lz4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opt := options.New()
			opt.SetCompression(tt.compression)
			if tt.encoding == "snappy" {
				opt.CustomDecompressor = func(r io.Reader) (io.Reader, error) {
					return snappy.NewReader(r), nil
				}
			}
			if tt.encoding == "lz4" {
				opt.CustomDecompressor = func(r io.Reader) (io.Reader, error) {
					return lz4.NewReader(r), nil
				}
			}
			opt.CustomCompressionType = options.CompressionType(tt.encoding)
			opt.SetBufferOutput()
			opt.EnableLogging()

			url := fmt.Sprintf("%s/download/compressed?compression=%s", server.URL, tt.encoding)
			resp, err := client.Get(url, opt)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			assert.Equal(t, largefile.Bytes(), resp.Body.Bytes())
		})
	}
}

func TestStandardDecompressionToFile(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	tests := []struct {
		name         string
		compression  string
		expectedSize int64
	}{
		{"Gzip Decompression", "gzip", int64(largefile.Len())},
		{"Deflate Decompression", "deflate", int64(largefile.Len())},
		{"Brotli Decompression", "br", int64(largefile.Len())},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary file for each test case
			tmpFile, err := os.CreateTemp("", fmt.Sprintf("decompress-%s-*.txt", tt.compression))
			assert.NoError(t, err)
			defer os.Remove(tmpFile.Name()) // Clean up after test
			tmpFile.Close()                 // Close it so the client can write to it

			opt := options.New()
			opt.SetFileOutput(tmpFile.Name())
			opt.EnableLogging()

			var bytesReceived int64
			opt.OnDownloadProgress = func(bytesRead, totalBytes int64) {
				bytesReceived = bytesRead
			}

			url := fmt.Sprintf("%s/download/compressed?compression=%s", server.URL, tt.compression)
			resp, err := client.Get(url, opt)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}

			// Debug info
			t.Logf("Response info: Status=%d, Compression=%s", resp.StatusCode, tt.compression)
			t.Logf("Response headers: %v", resp.Header)

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			// Read the downloaded file and verify its contents
			downloadedContent, err := os.ReadFile(tmpFile.Name())
			assert.NoError(t, err)
			assert.Equal(t, largefile.Bytes(), downloadedContent)

			// Verify file size matches expected size
			info, err := os.Stat(tmpFile.Name())
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedSize, info.Size())
			assert.Equal(t, int64(largefile.Len()), bytesReceived)

			t.Logf("[%s] Original size: %d, File size: %d",
				tt.name,
				largefile.Len(),
				info.Size())
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

func TestRedirectMaxRedirects(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	tmpfile, err := os.Open(smallf)
	if err != nil {
		t.Logf("error opening %s: %s", smallf, err)
		t.Fail()
	}

	opt := options.New()
	opt.EnableLogging()
	opt.FollowRedirects = true
	opt.PreserveMethodOnRedirect = false
	opt.MaxRedirects = 5

	resp, err := client.Post(server.URL+"/max-redirects", tmpfile, opt)
	assert.Equal(t, fmt.Sprintf("Get \"/max-redirects\": max redirects (%d) exceeded", opt.MaxRedirects), err.Error())
	assert.Equal(t, "", resp.String())
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
				smallf: s,
				largef: l,
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
			assert.Equal(t, int64(smallfile.Len()), fileInfo[smallf])
			assert.Equal(t, int64(largefile.Len()), fileInfo[largef])
		})
	}
}

// TestResults stores metrics from concurrent request tests
type TestResults struct {
	TotalRequests  int
	Duration       time.Duration
	RequestsPerSec float64
	SuccessRate    float64
	ErrorCount     int
}

func TestSharedConcurrentRequests(t *testing.T) {
	server := setupTestServer(t)

	// Force cleanup after each test case
	t.Cleanup(func() {
		server.Close()
		// Add small delay to ensure connections close
		time.Sleep(1 * time.Second)
	})

	tests := []struct {
		name          string
		numGoroutines int
		requestsPerGo int
		scenario      string // "mixed", "upload", "download"
	}{
		{"Light Concurrent Mixed Load (Shared)", 10, 5, "mixed"},
		{"Heavy Concurrent Mixed Load (Shared)", 50, 10, "mixed"},
		{"Concurrent File Uploads (Shared)", 20, 5, "upload"},
		{"Concurrent File Downloads (Shared)", 20, 5, "download"},
	}

	var results []TestResults

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var wg sync.WaitGroup
			errors := make(chan error, tt.numGoroutines*tt.requestsPerGo)
			start := time.Now()

			for i := 0; i < tt.numGoroutines; i++ {
				wg.Add(1)
				go func(routineNum int) {
					defer wg.Done()
					for j := 0; j < tt.requestsPerGo; j++ {
						var err error
						switch tt.scenario {
						case "mixed":
							err = performMixedRequests(server.URL, routineNum, j)
						case "upload":
							err = performUploadRequest(server.URL, routineNum, j)
						case "download":
							err = performDownloadRequest(server.URL, routineNum, j)
						}
						if err != nil {
							errors <- fmt.Errorf("routine %d request %d: %w", routineNum, j, err)
						}
					}
				}(i)
			}

			wg.Wait()
			close(errors)

			// Collect errors
			var errs []error
			for err := range errors {
				errs = append(errs, err)
			}

			duration := time.Since(start)
			totalRequests := tt.numGoroutines * tt.requestsPerGo
			successRate := float64(totalRequests-len(errs)) / float64(totalRequests) * 100
			requestsPerSec := float64(totalRequests) / duration.Seconds()

			// Create TestResults for this test
			result := TestResults{
				TotalRequests:  totalRequests,
				Duration:       duration,
				RequestsPerSec: requestsPerSec,
				SuccessRate:    successRate,
				ErrorCount:     len(errs),
			}
			results = append(results, result)

			// Log the results
			t.Logf("\nResults for %s:", tt.name)
			t.Logf("Total requests: %d", result.TotalRequests)
			t.Logf("Duration: %v", result.Duration)
			t.Logf("Requests/second: %.2f", result.RequestsPerSec)
			t.Logf("Success rate: %.2f%%", result.SuccessRate)
			t.Logf("Error count: %d", result.ErrorCount)

			// Assert high success rate
			assert.GreaterOrEqual(t, result.SuccessRate, 95.0, "Success rate should be at least 95%")
		})
	}

	// Print comparison summary at the end
	t.Log("\nTest Scenario Comparisons:")
	for i, result := range results {
		t.Logf("\n%s:", tests[i].name)
		t.Logf("Requests/sec: %.2f", result.RequestsPerSec)
		t.Logf("Success rate: %.2f%%", result.SuccessRate)
		t.Logf("Total requests: %d", result.TotalRequests)
		t.Logf("Duration: %v", result.Duration)
		t.Logf("Error count: %d", result.ErrorCount)
	}
}

func TestNonSharedConcurrentRequests(t *testing.T) {
	server := setupTestServer(t)

	// Force cleanup after each test case
	t.Cleanup(func() {
		server.Close()
		// Add small delay to ensure connections close
		time.Sleep(1 * time.Second)
	})

	tests := []struct {
		name          string
		numGoroutines int
		requestsPerGo int
		scenario      string
	}{
		{"Light Concurrent Mixed Load (Non-Shared)", 10, 5, "mixed"},
		{"Heavy Concurrent Mixed Load (Non-Shared)", 50, 10, "mixed"},
		{"Concurrent File Uploads (Non-Shared)", 20, 5, "upload"},
		{"Concurrent File Downloads (Non-Shared)", 20, 5, "download"},
	}

	var results []TestResults
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var wg sync.WaitGroup
			errors := make(chan error, tt.numGoroutines*tt.requestsPerGo)
			start := time.Now()

			for i := 0; i < tt.numGoroutines; i++ {
				wg.Add(1)
				go func(routineNum int) {
					defer wg.Done()
					for j := 0; j < tt.requestsPerGo; j++ {
						// Create options with per-request client for each request
						opt := options.New()
						opt.UsePerRequestClient()

						var err error
						switch tt.scenario {
						case "mixed":
							err = performMixedRequests(server.URL, routineNum, j)
						case "upload":
							err = performUploadRequest(server.URL, routineNum, j)
						case "download":
							err = performDownloadRequest(server.URL, routineNum, j)
						}
						if err != nil {
							errors <- fmt.Errorf("routine %d request %d: %w", routineNum, j, err)
						}
					}
				}(i)
			}

			wg.Wait()
			close(errors)

			// Collect errors
			var errs []error
			for err := range errors {
				errs = append(errs, err)
			}

			duration := time.Since(start)
			totalRequests := tt.numGoroutines * tt.requestsPerGo
			successRate := float64(totalRequests-len(errs)) / float64(totalRequests) * 100
			requestsPerSec := float64(totalRequests) / duration.Seconds()

			result := TestResults{
				TotalRequests:  totalRequests,
				Duration:       duration,
				RequestsPerSec: requestsPerSec,
				SuccessRate:    successRate,
				ErrorCount:     len(errs),
			}
			results = append(results, result)

			// Log the results
			t.Logf("\nResults for %s:", tt.name)
			t.Logf("Total requests: %d", result.TotalRequests)
			t.Logf("Duration: %v", result.Duration)
			t.Logf("Requests/second: %.2f", result.RequestsPerSec)
			t.Logf("Success rate: %.2f%%", result.SuccessRate)
			t.Logf("Error count: %d", result.ErrorCount)

			assert.GreaterOrEqual(t, result.SuccessRate, 95.0, "Success rate should be at least 95%")
		})
	}

	// Print comparison summary
	t.Log("\nTest Scenario Comparisons (Non-Shared Clients):")
	for i, result := range results {
		t.Logf("\n%s:", tests[i].name)
		t.Logf("Requests/sec: %.2f", result.RequestsPerSec)
		t.Logf("Success rate: %.2f%%", result.SuccessRate)
		t.Logf("Total requests: %d", result.TotalRequests)
		t.Logf("Duration: %v", result.Duration)
		t.Logf("Error count: %d", result.ErrorCount)
	}
}

func performMixedRequests(baseURL string, routineNum, reqNum int) error {
	switch reqNum % 3 {
	case 0:
		resp, err := client.Get(baseURL + "/echo-headers")
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}
	case 1:
		opt := options.New()
		opt.AddHeader(fmt.Sprintf("X-Test-%d-%d", routineNum, reqNum), "test")
		resp, err := client.Post(baseURL+"/upload", []byte("test data"), opt)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}
	case 2:
		opt := options.New()
		opt.SetBufferOutput()
		resp, err := client.Get(baseURL + "/download")
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}
	}
	return nil
}

func performUploadRequest(baseURL string, routineNum, reqNum int) error {
	file, err := os.Open(smallf) // Using the small file for quicker tests
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	opt := options.New()
	opt.AddHeader(fmt.Sprintf("X-Upload-Test-%d-%d", routineNum, reqNum), "test")

	resp, err := client.Post(baseURL+"/upload", file, opt)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return nil
}

func performDownloadRequest(baseURL string, routineNum, reqNum int) error {
	opt := options.New()
	tempDir := os.TempDir()
	downloadPath := filepath.Join(tempDir, fmt.Sprintf("download-%d-%d.txt", routineNum, reqNum))
	opt.SetFileOutput(downloadPath)
	defer os.Remove(downloadPath) // Clean up after test

	resp, err := client.Get(baseURL+"/download", opt)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return nil
}
