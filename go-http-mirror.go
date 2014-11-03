package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"time"

	"github.com/dataence/cityhash"
)

type StatRecord struct {
	RequestID      uint32
	WasSent        bool
	WasReceived    bool
	ResponseLength int
	ResponseStatus int
	Settings       SinkSettings
}

type RequestInfo struct {
	Request   *http.Request
	RequestID uint32
}

type HttpMirror struct {
	Proxy      *httputil.ReverseProxy
	Requests   chan *RequestInfo
	Settings   SinkSettings
	Statistics chan *StatRecord
}

func (mirror *HttpMirror) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	mirror.Proxy.ServeHTTP(response, request)
}

type RedirectResponseWriter struct {
	Writer  http.ResponseWriter
	Headers http.Header
	Status  int
}

func CreateRedirectWriter(src_writer http.ResponseWriter) RedirectResponseWriter {
	return RedirectResponseWriter{src_writer, make(http.Header), 0}
}

func (writer *RedirectResponseWriter) Header() http.Header {
	if writer.Writer != nil {
		return writer.Writer.Header()
	}
	return writer.Headers
}

func (writer *RedirectResponseWriter) Write(data []byte) (int, error) {
	if writer.Writer != nil {
		return writer.Writer.Write(data)
	}
	return len(data), nil
}

func (writer *RedirectResponseWriter) WriteHeader(code int) {
	if writer.Writer != nil {
		writer.Writer.WriteHeader(code)
	}
	writer.Status = code
}

func (writer *RedirectResponseWriter) ContentLength() int {
	if te, te_err := writer.Header()["Transfer-Encoding"]; !te_err || te[0] == "identity" {
		if cl, cl_err := writer.Header()["Content-Length"]; cl_err {
			if length, err := strconv.ParseInt(cl[0], 0, 0); err == nil {
				return int(length)
			}
		}
	}
	return 0
}

func GetRequestID(request *http.Request) uint32 {
	dump, err := httputil.DumpRequestOut(request, true)
	now := time.Now().String()
	dump = append(dump, []byte(now)...)
	var request_id uint32 = 0
	if err != nil {
		request_id = cityhash.CityHash32(dump, uint32(len(dump)))
	}
	return request_id
}

func (mirror *HttpMirror) SendRequest(request_info *RequestInfo, response *RedirectResponseWriter, send_copy bool) {
	stat_record := &StatRecord{request_info.RequestID, false, false, 0, 0, mirror.Settings}
	defer func() { mirror.Statistics <- stat_record }()
	if rand.Float64() < mirror.Settings.LossProbability {
		return
	}
	stat_record.WasSent = true
	if latency := mirror.Settings.Latency.Nanoseconds(); latency > 0 {
		timeout := time.Duration(rand.Int63n(latency))
		fmt.Println("For ", mirror.Settings.Address, " timeout is ", timeout)
		time.Sleep(timeout)
	}
	request := request_info.Request
	if send_copy {
		request = Copy(request)
		ChangeDestination(request, mirror.Settings.Address)
	}
	if request != nil {
		mirror.ServeHTTP(response, request)
	}
	stat_record.ResponseStatus = response.Status
	stat_record.ResponseLength = response.ContentLength()
	stat_record.WasReceived = true
}

func mirrorWork(mirror *HttpMirror) {
	for src_request := range mirror.Requests {
		response := CreateRedirectWriter(nil)
		mirror.SendRequest(src_request, &response, true)
	}
}
func HandleRequest(response http.ResponseWriter, request *http.Request) {
	//mirror.Proxy.ServeHTTP(response, request)
}
func CreateMirror(settings SinkSettings) (*HttpMirror, error) {
	dst_url, err := url.Parse(settings.Address)
	if err != nil {
		return nil, err
	}
	if dst_url.Scheme == "" {
		log.Printf("Empty scheme of address %q. Tring add 'http' scheme.", settings.Address)
		settings.Address = "http://" + settings.Address
		return CreateMirror(settings)
	}
	http_mirror := new(HttpMirror)
	http_mirror.Requests = make(chan *RequestInfo)
	http_mirror.Proxy = httputil.NewSingleHostReverseProxy(dst_url)
	http_mirror.Settings = settings
	return http_mirror, nil
}

type MirrorService struct {
	Destination *HttpMirror
	Mirrors     []*HttpMirror
	Statistics  chan *StatRecord
}

func (service *MirrorService) ServeHTTP(responseWriter http.ResponseWriter, request *http.Request) {
	fmt.Println("Get request")
	request_info := &RequestInfo{request, GetRequestID(request)}
	for _, mirror := range service.Mirrors {
		mirror.Requests <- request_info
	}
	response := CreateRedirectWriter(responseWriter)
	service.Destination.SendRequest(request_info, &response, false)
}

func CreateMirrorService(settings Settings) (*MirrorService, error) {
	// TODO: вынести создание зеркала и запуск потока в отдельную функцию (метод).
	mirror_service := new(MirrorService)
	mirror_service.Statistics = make(chan *StatRecord, 100)
	go statWork(mirror_service)
	http_destination, err := CreateMirror(settings.Destination)
	if err != nil {
		return nil, err
	}
	http_destination.Statistics = mirror_service.Statistics
	mirror_service.Destination = http_destination
	mirror_service.Mirrors = make([]*HttpMirror, 0, 5)
	for _, mirror := range settings.Mirrors {
		http_mirror, err := CreateMirror(mirror)
		if err != nil {
			return nil, err
		}
		http_mirror.Statistics = mirror_service.Statistics
		mirror_service.Mirrors = append(mirror_service.Mirrors, http_mirror)
	}
	for _, mirror := range mirror_service.Mirrors {
		go mirrorWork(mirror)
	}
	return mirror_service, nil
}

func statWork(service *MirrorService) {
	for stat := range service.Statistics {
		fmt.Println("Request ", stat.RequestID, " server ", stat.Settings.Address, " Length ", stat.ResponseLength, " Status ", stat.ResponseStatus)
	}
}

func Initialize() {
	now := time.Now()
	rand.Seed(now.Unix()*1000 + now.UnixNano()/1000)
}

func main() {
	settings := ParseSettings()
	fmt.Println("Settings: ", settings)

	mirror_service, err := CreateMirrorService(settings)
	if err != nil {
		log.Fatalf("Can't create service: %s", err)
	}

	http.Handle("/", mirror_service)
	err = http.ListenAndServe(settings.Listen, nil)
	if err != nil {
		log.Fatalf("Can't start services: %s", err)
	}
}
