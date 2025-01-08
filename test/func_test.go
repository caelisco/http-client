package client

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/andybalholm/brotli"
	client "github.com/caelisco/http-client"
	"github.com/caelisco/http-client/options"
	"github.com/golang/snappy"
	"github.com/pierrec/lz4/v4"
	"github.com/stretchr/testify/assert"
)

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
				reader = r.Body
			}

			// Read the decompressed data into the buffer
			_, err = io.Copy(&buff, reader)
			if err != nil {
				t.Logf("decompression err: %s", err)
				http.Error(w, "Failed to decompress data:"+err.Error(), http.StatusInternalServerError)
				return
			}

			// Once decompressed, send the data back to the client
			t.Logf("Server file size: %d bytes", buff.Len())
			_, err = w.Write(buff.Bytes())
			if err != nil {
				http.Error(w, "Failed to send decompressed data:"+err.Error(), http.StatusInternalServerError)
				return
			}
		case "/download":
			// Send a large response
			w.Header().Set("Content-Length", "1048576") // 1MB
			for i := 0; i < 1048576; i++ {
				w.Write([]byte("a"))
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
		})
	}
}

func TestFileUpload(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	// Create a temporary file
	content := "Hello, World!"
	tmpfile, err := os.CreateTemp("", "upload-*.txt")
	assert.NoError(t, err)
	defer os.Remove(tmpfile.Name())

	_, err = tmpfile.WriteString(content)
	assert.NoError(t, err)
	tmpfile.Seek(0, 0)

	// Track upload progress
	var lastProgress float64
	opt := options.New()
	opt.OnUploadProgress = func(bytesRead, totalBytes int64) {
		if totalBytes > 0 {
			lastProgress = float64(bytesRead) / float64(totalBytes) * 100
		}
	}

	// Upload the file
	resp, err := client.Post(server.URL+"/upload", tmpfile, opt)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify progress tracking worked
	assert.Equal(t, float64(100), lastProgress)

	// Verify the echoed content matches
	assert.Equal(t, content, resp.Body.String())
}

func TestFileDownload(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	// Create temporary directory for download
	tmpDir, err := os.MkdirTemp("", "download-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	downloadPath := filepath.Join(tmpDir, "downloaded.txt")

	// Track download progress
	var lastProgress float64
	opt := options.New()
	opt.OnDownloadProgress = func(bytesRead, totalBytes int64) {
		if totalBytes > 0 {
			lastProgress = float64(bytesRead) / float64(totalBytes) * 100
		}
	}
	opt.SetFileOutput(downloadPath)

	// Download the file
	resp, err := client.Get(server.URL+"/download", opt)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify progress tracking worked
	assert.Equal(t, float64(100), lastProgress)

	// Verify file size
	info, err := os.Stat(downloadPath)
	assert.NoError(t, err)
	assert.Equal(t, int64(1048576), info.Size())
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
		{"Small Buffer", 1024, 1048576},
		{"Large Buffer", 32768, 1048576},
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

	largeString := strings.Repeat("hello world ", 1000) // 12,000 bytes uncompressed

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

			t.Logf("[%s] Uncompressed size: %d bytes", tt.name, len(largeString))

			resp, err := client.Post(server.URL+"/upload", largeString, opt)
			assert.NoError(t, err)

			assert.Equal(t, http.StatusOK, resp.StatusCode)
			//assert.Equal(t, largeString, resp.String())
			t.Logf("resp: %s", resp.String())
		})
	}
}

func TestCustomCompression(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	largeString := strings.Repeat("hello world ", 1000) // 12,000 bytes uncompressed

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

			t.Logf("[%s] Uncompressed size: %d bytes", tt.name, len(largeString))

			resp, err := client.Post(server.URL+"/upload", largeString, opt)
			assert.NoError(t, err)

			assert.Equal(t, http.StatusOK, resp.StatusCode)
			//assert.Equal(t, largeString, resp.String())
			t.Logf("resp: %s", resp.String())
		})
	}
}
