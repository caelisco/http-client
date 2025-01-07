# http-client
A small wrapper around the HTTP Client in Go.

The client is not necessarily feature complete and is designed to work with simple requests to an HTTP server.

It has two modes of operation: 
1. Normal HTTP method function calls
2. A reusable client

In most cases, the HTTP method function calls are probably what you are looking for.

The reusable client is typically more useful in scenarios where you need to work with cookies, or in scenarios where it will be useful to keep track of past requests. An example could be for writing an API test that might require different headers/cookies to be set and used as part of the testing of each end point.

The readme contains documentation for v1.0.0

# Basic example
```go
package main

import (
	"fmt"
	"log"

	client "github.com/caelisco/http-client"
)

func main() {
	resp, err := client.Get("https://www.caelisco.net/http-client/get/")
	if err != nil || resp.StatusCode != 200 {
		log.Fatalf("There was an error with the request. StatusCode: %d Error: %s", resp.StatusCode, err)
	}
	// output the HTML response
	fmt.Println(resp.String())
}
```

# Options
http-client provides various options that can be sent through with requests.

```go
type Option struct {
	Verbose                  bool                              // Whether logging should be verbose or not
	Logger                   slog.Logger                       // Logging - default uses the slog TextHandler
	Header                   http.Header                       // Headers to be included in the request
	Cookies                  []*http.Cookie                    // Cookies to be included in the request
	ProtocolScheme           string                            // define a custom protocol scheme. It defaults to https
	Compression              CompressionType                   // CompressionType to use: none, gzip, deflate or brotli
	UserAgent                string                            // User Agent to send with requests
	FollowRedirect           bool                              // Disable or enable redirects. Default is true i.e.: follow redirects
	PreserveMethodOnRedirect bool                              // Default is true
	UniqueIdentifierType     UniqueIdentifierType              // Internal trace or identifier for the request
	Transport                *http.Transport                   // Create our own default transport
	ResponseWriter           ResponseWriter                    // Define the type of response writer
	DownloadBufferSize       *int                              // Control the size of the buffer when downloading a file
	OnUploadProgress         func(bytesRead, totalBytes int64) // To monitor and track progress when uploading
	OnDownloadProgress       func(bytesRead, totalBytes int64) // To monitor and track progress when downloading
}
```

# Basic example with headers
```go
package main

import (
	"fmt"
	"log"

	client "github.com/caelisco/http-client"
	"github.com/caelisco/http-client/options"
)

func main() {
	opt := options.New()
	opt.Header.Add("Accept-Format", "xml")
	resp, err := client.Get("https://www.caelisco.net/http-client/get/", opt)
	if err != nil || resp.StatusCode != 200 {
		log.Fatalf("There was an error with the request. StatusCode: %d Error: %s", resp.StatusCode, err)
	}
	// output the HTML response
	fmt.Println(resp.String())
}
```
# Upload a file
The latest version of http-client provides a more powerful interface for sending a payload by changing the payload from a `[]byte` to using `any`.

The main reason is efficency: converting a file to a []byte would require the entire file to be loaded in to memory first.

Now it is possible to provide a file handle and upload the file automatically.

```go
file, err := os.Open("README.md")
if err != nil {
    log.Fatal(err)
}
defer file.Close()

// Create options with progress tracking if desired
opt := options.New()
opt.OnUploadProgress = func(bytesRead, totalBytes int64) {
    progress := float64(bytesRead) / float64(totalBytes) * 100
    fmt.Printf("Upload progress: %.2f%%\n", progress)
}
// send the file
resp, err := client.Post("https://example.com/upload", file, opt)
```
Similarly, when downloading a file, it can be written directly to a file without needing to buffer it in memory.

The default is to set the file output to `bytes.Buffer`

```go
options.SetOutput(writerType ResponseWriterType, filepath ...string)
options.SetFileOutput(filepath string)
options.SetBufferOutput()
```


# Compressing a stream
You can specify a compression stream using the options.Compression by providing an option.CompressionType. Compression involves streaming the payload to the server using io.Pipe() and io.Copy() to reduce memory pressure. You can also define a custom buffer size.

http-client supports gzip, deflate and brotli.

```go
const (
	CompressionNone    CompressionType = ""
	CompressionGzip    CompressionType = "gzip"
	CompressionDeflate CompressionType = "deflate"
	CompressionBrotli  CompressionType = "br"
	CompressionCustom  CompressionType = "custom"
)
```

You can also define your own custom compression assuming the server is able to work with the compressed data.

```go
opt := options.New()
// set the compression to custom
opt.SetCompression(options.CompressionCustom)
// create a writer to compress the stream
opt.CustomCompressor = func(w *io.PipeWriter) (io.WriteCloser, error) {
	return snappy.NewWriter(w), nil
}
// define the compression type to be used with the content-encoding
opt.CustomCompressionType = options.CompressionType("snappy")
// perform the request
resp, err := client.Post(url, data, opt)
```

# Progress
http-client includes progress reporting for both uploading and downloading content.

An example also exists in [progress/progress.go](https://github.com/caelisco/http-client/blob/main/progress/progress.go).

```go
file, err := os.Open("README.md")
if err != nil {
    log.Fatal(err)
}
defer file.Close()

// Create options with progress tracking if desired
opt := options.New()
opt.OnUploadProgress = func(bytesRead, totalBytes int64) {
    progress := float64(bytesRead) / float64(totalBytes) * 100
    fmt.Printf("Upload progress: %.2f%%\n", progress)
}
// send the file
resp, err := client.Post("https://example.com/upload", file, opt)
```

