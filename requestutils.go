package main

import (
	"bufio"
	"bytes"
	"net/http"
	"net/http/httputil"
)

func Copy(request *http.Request) *http.Request {
	buffer, err := httputil.DumpRequest(request, true)
	if err != nil {
		return nil
	}
	buffer_reader := bytes.NewReader(buffer)
	buffered_reader := bufio.NewReader(buffer_reader)
	result, err := http.ReadRequest(buffered_reader)
	if err != nil {
		return nil
	}
	return result
}

func ChangeDestination(request *http.Request, host string) {
	request.URL.Host = host
	request.Header["Host"] = []string{host}
	request.Host = host
}
