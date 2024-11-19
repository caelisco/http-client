package main

import (
	"fmt"
	"log"

	client "github.com/caelisco/http-client"
)

// FileWriter demonstrates using an io.WriteCloser with a file handle.
// The response will be written in to a file using io.Copy which buffers
// the response body.

// Note: The response.Response struct includes Body which is a bytes.Buffer
// Because the response.Options made use of the FileWriter() the Body is ignored
// and the client streams the data to a file instead.
func main() {
	opt := client.RequestOptions{}
	opt.FileWriter("temp.file")
	resp, err := client.Get("https://www.caelisco.net/", opt)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Downloaded file with resp code:", resp.StatusCode, "and content length:", resp.ContentLength)
}
