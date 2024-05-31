package main

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"
)

var (
	hostFile            string
	domainFile          string
	threads             int
	proxyURL            string
	headerFile          string
	verbose             bool
	filterContentLength int
)

type Headers map[string]string

func init() {
	flag.StringVar(&hostFile, "host", "", "File containing the list of hosts")
	flag.StringVar(&domainFile, "domain", "", "File containing the list of domain headers")
	flag.IntVar(&threads, "t", 10, "Number of concurrent threads")
	flag.StringVar(&proxyURL, "proxy", "", "Proxy URL in the format https://IP:PORT")
	flag.StringVar(&headerFile, "header", "", "JSON file containing custom headers")
	flag.BoolVar(&verbose, "v", false, "Verbose mode: show all request details")
	flag.IntVar(&filterContentLength, "fcl", 0, "Filter responses by Content-Length")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage of %s:
  -host string
	File containing the list of hosts
  -domain string
	File containing the list of domain headers
  -t int
	Number of concurrent threads (default 10)
  -proxy string
	[OPTIONAL] Proxy URL in the format https://IP:PORT 
  -h string
	[OPTIONAL] JSON file containing custom headers
  -v 
        Verbose mode: show all request details

Filtering:
  -fcl int
        Filter responses by Content-Length (default 0)

Example:
  %s -host hosts.txt -domain domains.txt -t 5 -proxy https://127.0.0.1:8080 -h headers.json -v -fcl 100
`, os.Args[0], os.Args[0])
	}
}

func readLines(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

func readHeaders(filePath string) (Headers, error) {
	if filePath == "" {
		return Headers{
			"User-Agent":      "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
			"Accept-Language": "en-US,en;q=0.5",
		}, nil
	}

	file, err := os.Open(filePath)
	if err != nil {
		return Headers{
			"User-Agent":      "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
			"Accept-Language": "en-US,en;q=0.5",
		}, nil
	}
	defer file.Close()

	var headers Headers
	err = json.NewDecoder(file).Decode(&headers)
	if err != nil {
		return Headers{
			"User-Agent":      "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
			"Accept-Language": "en-US,en;q=0.5",
		}, nil
	}
	return headers, err
}

func createClient(proxyURL string) *http.Client {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // Ignore TLS certificate verification
	}

	if proxyURL != "" {
		proxy, err := url.Parse(proxyURL)
		if err != nil {
			fmt.Printf("Failed to parse proxy URL: %v\n", err)
			return &http.Client{Transport: transport}
		}
		transport.Proxy = http.ProxyURL(proxy)
	}

	return &http.Client{Transport: transport}
}

func makeRequest(client *http.Client, host, domain string, headers Headers, wg *sync.WaitGroup, ch chan<- string) {
	defer wg.Done()

	req, err := http.NewRequest("GET", host, nil)
	if err != nil {
		ch <- fmt.Sprintf("Failed to create request for %s: %v", host, err)
		return
	}

	req.Host = domain // Correctly set the Host header
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		ch <- fmt.Sprintf("Request to %s failed: %v", host, err)
		return
	}
	defer resp.Body.Close()

	contentLength := resp.Header.Get("Content-Length")
	contentLengthInt := 0
	if contentLength != "" {
		contentLengthInt, _ = strconv.Atoi(contentLength)
	}

	if filterContentLength > 0 && contentLengthInt < filterContentLength {
		return // Skip this response as it doesn't meet the filter criteria
	}

	if verbose {
		ch <- fmt.Sprintf("Host: %s Domain: %s Code: %s Content-Length: %s", host, domain, resp.Status, contentLength)
	} else if resp.StatusCode == http.StatusOK {
		ch <- fmt.Sprintf("Host: %s Domain: %s Code: %s Content-Length: %s", host, domain, resp.Status, contentLength)
	}
}

func printBanner() {
	fmt.Println(`
  _____  ____  _____   ____  _    _ _______ ______ 
 / ____|/ __ \|  __ \ / __ \| |  | |__   __|  ____|
| |  __| |  | | |__) | |  | | |  | |  | |  | |__   
| | |_ | |  | |  _  /| |  | | |  | |  | |  |  __|  
| |__| | |__| | | \ \| |__| | |__| |  | |  | |____ 
 \_____|\____/|_|  \_\\____/ \____/   |_|  |______| v0.1
													
					By: 0xdead4f 							  
`)
}

func main() {
	printBanner()
	flag.Parse()

	hosts, err := readLines(hostFile)
	if err != nil {
		fmt.Printf("Failed to read hosts file: %v\n", err)
		return
	}

	domains, err := readLines(domainFile)
	if err != nil {
		fmt.Printf("Failed to read domains file: %v\n", err)
		return
	}

	// Read headers
	headers, err := readHeaders(headerFile)
	if err != nil {
		fmt.Printf("Failed to read headers file: %v\n", err)
		return
	}

	// Create HTTP client
	client := createClient(proxyURL)

	var wg sync.WaitGroup
	ch := make(chan string, len(hosts)*len(domains))

	for _, host := range hosts {
		for _, domain := range domains {
			wg.Add(1)
			go makeRequest(client, host, domain, headers, &wg, ch)
		}
	}

	wg.Wait()
	close(ch)

	for msg := range ch {
		fmt.Println(msg)
	}
}
