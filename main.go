package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

var startTime time.Time
var config = make(map[string]string)
var hostname string

func uptime() time.Duration {
	return time.Since(startTime)
}

func init() {
	startTime = time.Now()
	hostname, _ = os.Hostname()
	configurationDir, ok := os.LookupEnv("CONfIG_DIR")
	var configFile string
	if !ok {
		log.Println("CONfIG_DIR not defined.")
		configFile = "config.yaml"
	} else {
		configFile = configurationDir + "/config.yaml"
	}
	log.Printf("Configuration File : %s", configFile)
	// read hostname name mapping to
	yfile, err := ioutil.ReadFile(configFile)

	if err != nil {
		log.Fatalf("Error opening config.yaml. Err: %s", err)
	}

	data := make(map[interface{}]interface{})

	err2 := yaml.Unmarshal(yfile, &data)

	if err2 != nil {
		log.Fatalf("Error happened in JSON marshal. Err: %s", err2)
	}
	for host, address := range data {
		config[fmt.Sprintf("%v", host)] = fmt.Sprintf("%v", address)
	}
}

func status(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Server", hostname)
	resp := make(map[string]string)
	resp["uptime"] = uptime().String()
	jsonResp, err := json.Marshal(resp)
	if err != nil {
		log.Fatalf("Error happened in JSON marshal. Err: %s", err)
	}

	w.Write(jsonResp)
}

func handleRequest(w http.ResponseWriter, req *http.Request) {
	log.Printf("Handling request : %s %s %s %s headers(%s)\n", hostname, req.Host, req.Method, req.URL.Path, req.Header)
	w.Header().Add("x-server", hostname)
	resp := make(map[string]string)

	address := config[req.Host]
	if address == "" {
		log.Printf("Unable to find host mapping for: %s", req.Host)
		resp["message"] = "failure"
		w.WriteHeader(503)
		jsonResp, _ := json.Marshal(resp)
		w.Write(jsonResp)
		return
	}
	log.Printf("Found host mapping for: %s - Address : %s", req.Host, address)
	requestURL := fmt.Sprintf("http://%s%s", address, req.URL.Path)
	client := &http.Client{}
	r, _ := http.NewRequest(req.Method, requestURL, req.Body)
	r.Host = req.Host
	//r.Header.Add("host", req.Host)
	r.Header.Add("x-server", hostname)
	log.Printf("Making http request: %s with host %sn", requestURL, r.Host)
	res, err := client.Do(r)
	if err != nil {
		log.Printf("Error making http request: %s\n", err)
		resp["message"] = "failure"
		resp["error"] = err.Error()
		w.WriteHeader(503)
		jsonResp, _ := json.Marshal(resp)
		w.Write(jsonResp)
		return
	}
	body, _ := ioutil.ReadAll(res.Body)
	log.Printf("Http request: %s - StatusCode: %d", requestURL, res.StatusCode)
	resp["message"] = "success"
	resp["upstream-response"] = string(body)
	jsonResp, _ := json.Marshal(resp)
	w.Write(jsonResp)
}

func Run(addr string, sslAddr string, ssl map[string]string) chan error {

	errs := make(chan error)

	// Starting HTTP server
	go func() {
		log.Printf("Staring HTTP service on %s ...", addr)

		if err := http.ListenAndServe(addr, nil); err != nil {
			errs <- err
		}

	}()

	// Starting HTTPS server
	go func() {
		log.Printf("Staring HTTPS service on %s ...", sslAddr)
		if err := http.ListenAndServeTLS(sslAddr, ssl["cert"], ssl["key"], nil); err != nil {
			errs <- err
		}
	}()

	return errs
}

func main() {
	httpPort, ok := os.LookupEnv("HTTP_PORT")
	if !ok {
		log.Println("HTTP_PORT not defined.  Defaulting to 8080")
		httpPort = ":8080"
	} else {
		httpPort = ":" + httpPort
	}

	httpsPort, ok := os.LookupEnv("HTTPS_PORT")
	if !ok {
		log.Println("HTTPS_PORT not defined.  Defaulting to 8443")
		httpsPort = ":8443"
	} else {
		httpsPort = ":" + httpsPort
	}

	http.HandleFunc("/status", status)
	http.HandleFunc("/", handleRequest)

	log.Println("Version 0.1")

	errs := Run(httpPort, httpsPort, map[string]string{
		"cert": "server.crt",
		"key":  "server.key",
	})

	// This will run forever until channel receives error
	select {
	case err := <-errs:
		log.Printf("Could not start serving service due to (error: %s)", err)
	}
}
