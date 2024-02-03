package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
)

func main() {
	// Define command-line flags
	singleSubdomain := flag.String("u", "", "Specify a single subdomain")
	subdomainFile := flag.String("l", "", "Specify a file containing multiple subdomains")
	outputFile := flag.String("o", "", "Specify an output file for the results")
	threads := flag.Int("t", 1, "Specify the number of threads (goroutines)")
	flag.Parse()

	// Check flag combinations
	if (*singleSubdomain == "" && *subdomainFile == "") || (*singleSubdomain != "" && *subdomainFile != "") {
		fmt.Println("Specify either a single subdomain with -u or a subdomain file with -l")
		return
	}

	// Read subdomains based on flags
	var subdomains []string
	if *singleSubdomain != "" {
		subdomains = append(subdomains, *singleSubdomain)
	} else {
		subdomainsFromFile, err := readSubdomains(*subdomainFile)
		if err != nil {
			fmt.Println("Error reading subdomains:", err)
			return
		}
		subdomains = subdomainsFromFile
	}

	// Use a WaitGroup to wait for all goroutines to finish
	var wg sync.WaitGroup

	// Create a buffered channel to limit the number of concurrent goroutines
	ch := make(chan struct{}, *threads)

	// Iterate through subdomains
	var allURLs []string
	seenURLs := make(map[string]bool)
	for _, subdomain := range subdomains {
		// Increment the WaitGroup counter
		wg.Add(1)

		// Launch a goroutine to process each subdomain concurrently
		go func(subdomain string) {
			// Defer the WaitGroup's Done method to decrement the counter when the goroutine completes
			defer wg.Done()

			// Acquire a token from the channel (limiting concurrency)
			ch <- struct{}{}
			defer func() { <-ch }()

			// Make a request to the AlienVault OTX API
			urls, err := getOTXURLs(subdomain)
			if err != nil {
				fmt.Printf("Error getting URLs for %s: %v\n", subdomain, err)
				return
			}

			// Print the URLs
			fmt.Printf("URLs for %s:\n", subdomain)
			for _, url := range urls {
				if !seenURLs[url] {
					fmt.Println(url)
					allURLs = append(allURLs, fmt.Sprintf("%s - %s", subdomain, url))
					seenURLs[url] = true
				}
			}
			fmt.Println()
		}(subdomain)
	}

	// Wait for all goroutines to finish
	wg.Wait()

	// Write results to the output file if specified
	if *outputFile != "" {
		err := writeResultsToFile(*outputFile, allURLs)
		if err != nil {
			fmt.Println("Error writing results to the output file:", err)
		} else {
			fmt.Printf("Results written to %s\n", *outputFile)
		}
	}
}

func readSubdomains(filename string) ([]string, error) {
	content, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	subdomains := strings.Fields(string(content))
	return subdomains, nil
}

func getOTXURLs(subdomain string) ([]string, error) {
	url := fmt.Sprintf("https://otx.alienvault.com/otxapi/indicator/hostname/url_list/%s?limit=100&page=1", subdomain)

	// Make HTTP GET request
	response, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	// Read response body
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	// Parse JSON response
	var data map[string]interface{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, err
	}

	// Extract URLs from the response
	urls := []string{}
	urlList, ok := data["url_list"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("url_list not found in the response")
	}

	for _, entry := range urlList {
		urlEntry, ok := entry.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("malformed response entry")
		}
		url, ok := urlEntry["url"].(string)
		if ok {
			urls = append(urls, url)
		}
	}

	return urls, nil
}

func writeResultsToFile(filename string, results []string) error {
	content := []byte(strings.Join(results, "\n"))
	err := ioutil.WriteFile(filename, content, 0644)
	return err
}
