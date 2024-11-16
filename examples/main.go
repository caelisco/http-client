package main

import (
	"fmt"
	"log"

	client "github.com/caelisco/http-client"
)

func main() {
	opt := client.RequestOptions{}
	resp, err := client.Get("https://www.caelisco.net/", opt)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("status code: %d, len: %d bufflen: %d\n", resp.StatusCode, resp.ContentLength, len(resp.Body.String()))
	c := client.New()
	c.Get("https://www.caelisco.net/")
	c.Get("https://www.caelisco.net/")
	c.Get("https://www.caelisco.net/")

	for i, v := range c.Responses() {
		log.Printf("[%d] %s | %s | %t\n", i, v.Status, v.Location, v.Redirected)
	}
}
