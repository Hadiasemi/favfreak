package main

import (
	"bufio"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/fatih/color"
	"golang.org/x/net/html"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/spaolacci/murmur3"

)

type Result struct {
	MMH3Hash uint32
	MD5Hash  string
	URL      string
}


func printFancyBanner() {
	
	green := color.New(color.FgGreen).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()
	purple := color.New(color.FgMagenta).SprintFunc()
	white := color.New(color.FgWhite).SprintFunc() 

	
	fmt.Println(green(`
  █████▒▄▄▄    ██▒   █▓  █████▒██▀███  ▓█████ ▄▄▄       ██ ▄█▀
▓██   ▒▒████▄ ▓██░   █▒▓██   ▒▓██ ▒ ██▒▓█   ▀▒████▄     ██▄█▒ 
▒████ ░▒██  ▀█▄▓██  █▒░▒████ ░▓██ ░▄█ ▒▒███  ▒██  ▀█▄  ▓███▄░ 
░▓█▒  ░░██▄▄▄▄██▒██ █░░░▓█▒  ░▒██▀▀█▄  ▒▓█  ▄░██▄▄▄▄██ ▓██ █▄ 
░▒█░    ▓█   ▓██▒▒▀█░  ░▒█░   ░██▓ ▒██▒░▒████▒▓█   ▓██▒▒██▒ █▄
 ▒ ░    ▒▒   ▓▒█░░ ▐░   ▒ ░   ░ ▒▓ ░▒▓░░░ ▒░ ░▒▒   ▓▒█░▒ ▒▒ ▓▒
 ░       ▒   ▒▒ ░░ ░░   ░       ░▒ ░ ▒░ ░ ░  ░ ▒   ▒▒ ░░ ░▒ ▒░
 ░ ░     ░   ▒     ░░   ░ ░     ░░   ░    ░    ░   ▒   ░ ░░ ░ 
             ░  ░   ░            ░        ░  ░     ░  ░░  ░   
                   ░                                          
	`))

	
	tagline := fmt.Sprintf("%s⚔ %s %s %s⚔",
		red(""),                         
		purple("FavFreak"),              
		white("- Favicon Hash Breaker"), 
		red(""),                         
	)

	
	credits := fmt.Sprintf("%s💀 %s%s%s %s %s",
		purple("- Coded by"),
		red("3>"), 
		green("Hadi Asemi"),
		red("<3"),  
		red(" 💀"),  
		purple(""), 
	)

	fmt.Println(tagline)
	fmt.Println(credits)
	fmt.Println()
}



func fetchFaviconFromHTML(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch HTML: %v", err)
	}
	defer resp.Body.Close()

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to parse HTML: %v", err)
	}

	var faviconURL string
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "link" {
			var rel, href string
			for _, attr := range n.Attr {
				if attr.Key == "rel" {
					rel = strings.ToLower(attr.Val)
				}
				if attr.Key == "href" {
					href = attr.Val
				}
			}
			if rel == "icon" || rel == "shortcut icon" || rel == "apple-touch-icon" {
				if href != "" {
					faviconURL = href
					return
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}

	f(doc)

	
	if faviconURL != "" {
		if !strings.HasPrefix(faviconURL, "http") {
			if strings.HasPrefix(faviconURL, "/") {
				baseURL := strings.TrimRight(url, "/")
				faviconURL = baseURL + faviconURL
			} else {
				faviconURL = url + "/" + faviconURL
			}
		}
		return faviconURL, nil
	}

	return "", fmt.Errorf("no favicon found")
}


func FaviconHashesFromURL(faviconURL string) (uint32, string, error) {
	resp, err := http.Get(faviconURL)
	if err != nil {
		return 0, "", fmt.Errorf("failed to get favicon: %v", err)
	}
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, "", fmt.Errorf("failed to read favicon data: %v", err)
	}

	b64 := base64.StdEncoding.EncodeToString(data)
	re := regexp.MustCompile(`.{1,76}`)
	withNewlines := re.ReplaceAllString(b64, "$0\n")

	// Hash the result using MMH3 (for Shodan/ZoomEye)
	mmh3Hash := murmur3.Sum32([]byte(withNewlines))

	// Hash the result using MD5 (for Censys)
	md5Hash := md5.Sum(data)
	md5HashString := hex.EncodeToString(md5Hash[:])

	return mmh3Hash, md5HashString, nil
}


func ensureURLScheme(url string) string {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return "https://" + url
	}
	return url
}

func worker(urls <-chan string, results chan<- Result, wg *sync.WaitGroup) {
	defer wg.Done()
	for url := range urls {
		url = ensureURLScheme(url)

		faviconURL := url + "/favicon.ico"
		mmh3Hash, md5Hash, err := FaviconHashesFromURL(faviconURL)
		if err != nil {
			faviconURL, err = fetchFaviconFromHTML(url)
			if err != nil {
				log.Printf("Failed to find favicon for URL '%s': %v\n", url, err)
				continue
			}
			mmh3Hash, md5Hash, err = FaviconHashesFromURL(faviconURL)
			if err != nil {
				log.Printf("Failed to calculate hashes for URL '%s': %v", faviconURL, err)
				continue
			}
		}

		results <- Result{MMH3Hash: mmh3Hash, MD5Hash: md5Hash, URL: url}
	}
}

func main() {
	
	printFancyBanner()

	
	shodanFlag := flag.Bool("shodan", false, "Generate a Shodan search dork")
	zoomeyeFlag := flag.Bool("zoomeye", false, "Generate a ZoomEye search dork")
	censysFlag := flag.Bool("censys", false, "Generate a Censys search dork")
	allFlag := flag.Bool("all", false, "Generate search dorks for all platforms")

	
	fingerprintInput := flag.String("fingerprint", "", "A JSON string or file path with favicon hashes and names")

	
	flag.Parse()

	
	var fingerprintDict map[string]string

	
	if *fingerprintInput != "" {
		
		if _, err := os.Stat(*fingerprintInput); err == nil {
			
			fileContent, err := ioutil.ReadFile(*fingerprintInput)
			if err != nil {
				log.Fatalf("Failed to read fingerprint file: %v", err)
			}

			
			err = json.Unmarshal(fileContent, &fingerprintDict)
			if err != nil {
				log.Fatalf("Failed to parse fingerprint file: %v", err)
			}
		} else {
			
			err := json.Unmarshal([]byte(*fingerprintInput), &fingerprintDict)
			if err != nil {
				log.Fatalf("Failed to parse fingerprint JSON: %v", err)
			}
		}
	}

	
	scanner := bufio.NewScanner(os.Stdin)

	urls := make(chan string, 10)
	results := make(chan Result, 10)

	processedURLs := make(map[string]bool)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go worker(urls, results, &wg)
	}

	go func() {
		for scanner.Scan() {
			url := scanner.Text()
			if url != "" {
				mu.Lock()
				if _, exists := processedURLs[url]; !exists {
					processedURLs[url] = true
					urls <- url
				}
				mu.Unlock()
			}
		}
		close(urls)
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	hashGroups := make(map[uint32][]Result)

	for result := range results {
		hashGroups[result.MMH3Hash] = append(hashGroups[result.MMH3Hash], result)
	}

	
	fmt.Println("\n================= Favicon Hash Results =================")
	for mmh3Hash, resultList := range hashGroups {
		color.Yellow("[MMH3 Hash] %d", mmh3Hash)
		for _, res := range resultList {
			color.Green(res.URL)

			if *shodanFlag || *allFlag {
				color.Cyan("Shodan: https://www.shodan.io/search?query=http.favicon.hash:%d", mmh3Hash)
			}
			if *zoomeyeFlag || *allFlag {
				color.Cyan("ZoomEye: https://www.zoomeye.org/searchResult?q=iconhash:%d", mmh3Hash)
			}
			if *censysFlag || *allFlag {
				color.Cyan("Censys: https://search.censys.io/search?resource=hosts&sort=RELEVANCE&per_page=25&virtual_hosts=EXCLUDE&q=services.http.response.favicons.md5_hash:%s", res.MD5Hash)
			}
		}
		fmt.Println() 
	}

	
	if len(fingerprintDict) > 0 {
		fmt.Println("\n================= [FingerPrint Based Detection Results] =================")

		fingerprintCounts := make(map[string]int)

		
		for mmh3Hash, resultList := range hashGroups {
			hashStr := fmt.Sprintf("%d", mmh3Hash)
			if name, exists := fingerprintDict[hashStr]; exists {
				fingerprintCounts[name] += len(resultList)
			}
		}

		
		for name, count := range fingerprintCounts {
			color.Red("[%s] - count: %d", name, count)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Fatalf("Error reading input: %v", err)
	}
}
