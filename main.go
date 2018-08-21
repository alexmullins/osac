package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

var (
	OPENSOURCE_APPLE_COM_URL = "https://opensource.apple.com"
	USAGE_STRING             = `
usage:
osac list                                   Prints available products (mac, devtools, ios, server)
osac list _product_                         Prints available releases for that product
osac list _product_ _release_               Prints available packages for that particular (product, release)

osac get _product_ _release_                Gets all packages for that particular (product, release)
osac get _product_ _release_ _package_      Gets that package for the particular (product, release)
	`
	availableProducts = map[string]string{
		"mac":      "macOS",
		"devtools": "Developer Tools",
		"ios":      "iOS",
		"server":   "OS X Server",
	}
)

func getDocument(url string) *goquery.Document {
	res, err := http.Get(url)
	if err != nil {
		log.Fatalln("http: couldn't get url: ", url)
	}
	if res.StatusCode != 200 {
		log.Fatalf("http: got non 200 status code: %s %d\n", url, res.StatusCode)
	}
	defer res.Body.Close()
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		log.Fatalln("document: couldn't create document from http response")
	}
	return doc
}

func doListReleases(product string) {
	releases := getReleaseListing(product)
	for _, r := range releases {
		fmt.Println(r.release)
	}
}

func doListPackages(product, release string) {
	packs := getPackageListing(product, release)
	for _, p := range packs {
		updatedStr := ""
		if p.updated {
			updatedStr = "*"
		}
		fmt.Printf("%s (%s)%s\n", p.name, p.version, updatedStr)
	}
}

func splitProjectName(s string) (string, string) {
	ss := strings.Split(s, "-")
	if len(ss) == 2 {
		return ss[0], ss[1]
	}
	return ss[0], "problem"
}

// product: optional
// release: optional
func doList(product, release string) {
	if product == "" {
		fmt.Println("Available products:")
		for p := range availableProducts {
			fmt.Println(p)
		}
	} else {
		if release == "" {
			// print releases
			doListReleases(product)
		} else {
			// print packages
			doListPackages(product, release)
		}
	}
}

func downloadPackages(product, release string, packs []Package) {
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalln("download: couldn't get cwd")
	}
	path := filepath.Join(cwd, product+"-"+release)
	err = os.Mkdir(path, os.ModeDir|os.ModePerm)
	if err != nil {
		log.Fatalln("download: couldn't create director: ", path)
	}
	for _, p := range packs {
		filename := filepath.Base(p.url)
		filename = filepath.Join(path, filename)
		fmt.Println(filename)
		resp, err := http.Get(p.url)
		if err != nil {
			log.Fatalln("download: couldn't download file: ", p.url)
		}
		defer resp.Body.Close()

		out, err := os.Create(filename)
		if err != nil {
			log.Fatalln("download: couldn't create file: ", filename, err)
		}

		_, err = io.Copy(out, resp.Body)
		if err != nil {
			log.Fatalln("download: couldn't copy bytes to file: ", filename)
		}
	}
}

// product: required
// release: required
// targetPackage: optional *TODO(alex): not supported*
func doGet(product, release, targetPackage string) {
	packs := getPackageListing(product, release)
	downloadPackages(product, release, packs)
}

func printUsage() {
	fmt.Println(USAGE_STRING)
	os.Exit(1)
}

type Release struct {
	product string
	release string
	url     string
}

type Package struct {
	name    string
	version string
	updated bool
	url     string
}

func getReleaseListing(product string) []Release {
	releases := make([]Release, 0)

	doc := getDocument(OPENSOURCE_APPLE_COM_URL)
	doc.Find(".product").EachWithBreak(func(i int, s *goquery.Selection) bool {
		if s.Find(".product-name").Text() == availableProducts[product] {
			s.Find("ul > li > a").Each(func(ii int, ss *goquery.Selection) {
				release := ss.Text()
				href, ok := ss.Attr("href")
				if !ok {
					log.Fatalf("document: couldn't get href from: (%s, %s)", product, release)
				}
				r := Release{
					product: product,
					release: release,
					url:     OPENSOURCE_APPLE_COM_URL + href,
				}
				releases = append(releases, r)
			})
			return false
		}
		return true
	})

	return releases
}

func getPackageListing(product, release string) []Package {
	packs := make([]Package, 0)

	releases := getReleaseListing(product)
	var theRelease Release
	found := false
	for _, r := range releases {
		if r.release == release {
			theRelease = r
			found = true
			break
		}
	}
	if !found {
		log.Fatalf("document: couldn't find the release: %s", release)
	}
	doc := getDocument(theRelease.url)
	doc.Find(".project-row").Each(func(i int, s *goquery.Selection) {
		nameNode := s.Find(".project-name")
		downloadNode := s.Find(".project-downloads")
		if aNode := nameNode.Find("a"); aNode != nil && aNode.Length() > 0 {
			// need project name, version, updated, url
			updated := nameNode.HasClass("newproject")
			projectName := strings.TrimSpace(nameNode.Find("a").Text())
			projectName, version := splitProjectName(projectName)
			url, ok := downloadNode.Find("a").Attr("href")
			if !ok {
				log.Fatalf("documnent: couldn't find href in downloadNode: %s", projectName)
			}
			p := Package{
				name:    projectName,
				version: version,
				updated: updated,
				url:     OPENSOURCE_APPLE_COM_URL + url,
			}
			packs = append(packs, p)
		}
	})
	return packs
}

func main() {
	// Parse args
	// Switch on command
	argc := len(os.Args)
	if argc < 2 {
		printUsage()
	}
	command := os.Args[1]
	switch command {
	case "list":
		if argc == 2 { // *nothing*
			doList("", "")
		} else if argc == 3 { // product
			doList(os.Args[2], "")
		} else if argc == 4 { // product, release
			doList(os.Args[2], os.Args[3])
		} else {
			printUsage()
		}
	case "get":
		if argc > 3 {
			if argc == 4 { // product, release
				doGet(os.Args[2], os.Args[3], "")
			} else if argc == 5 { // product, release, package
				doGet(os.Args[2], os.Args[3], os.Args[4])
			} else {
				printUsage()
			}
		} else {
			printUsage()
		}
	default:
		printUsage()
	}
}
