package main

import (
	"fmt"
	"log"

	client "github.com/caelisco/http-client"
	"github.com/caelisco/http-client/request"
)

// Basic GET request example
func main() {
	opt := client.RequestOptions{}
	opt.UniqueIdentifier = request.IdentifierULID
	resp, err := client.Get("https://www.caelisco.net/", opt)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Received information from", resp.URL, "with internal ID:", resp.UniqueIdentifier, "body length:", resp.Length(), "content-length:", resp.ContentLength)
}
