package main

import (
	"flag"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"

	rice "github.com/GeertJohan/go.rice"
	zim "github.com/akhenakh/gozim"
	"github.com/blevesearch/bleve"
	lru "github.com/hashicorp/golang-lru"

	_ "github.com/blevesearch/bleve/analysis/lang/ar"
	_ "github.com/blevesearch/bleve/analysis/lang/cjk"
	_ "github.com/blevesearch/bleve/analysis/lang/ckb"
	_ "github.com/blevesearch/bleve/analysis/lang/en"
	_ "github.com/blevesearch/bleve/analysis/lang/fa"
	_ "github.com/blevesearch/bleve/analysis/lang/fr"
	_ "github.com/blevesearch/bleve/analysis/lang/hi"
	_ "github.com/blevesearch/bleve/analysis/lang/it"
	_ "github.com/blevesearch/bleve/analysis/lang/pt"

	_ "github.com/blevesearch/bleve/index/store/goleveldb"
)

type ResponseType int8

const (
	RedirectResponse ResponseType = iota
	DataResponse
	NoResponse
)

// CachedResponse cache the answer to an URL in the zim
type CachedResponse struct {
	ResponseType ResponseType
	Data         []byte
	MimeType     string
}

var (
	port      = flag.Int("port", -1, "port to listen to, read HOST env if not specified, default to 8080 otherwise")
	zimPath   = flag.String("path", "", "path for the zim file")
	indexPath = flag.String("index", "", "path for the index file")
	mmap      = flag.Bool("mmap", false, "use mmap")

	Z *zim.ZimReader
	// Cache is filled with CachedResponse to avoid hitting the zim file for a zim URL
	cache *lru.ARCCache
	idx   bool
	index bleve.Index

	templates map[string]*template.Template
)

func init() {
	templates = make(map[string]*template.Template)

	tplBox := rice.MustFindBox("templates")

	registerTemplate("index", tplBox)
	registerTemplate("searchResult", tplBox)
}

// registerTemplate load template from rice box and add them to a map[string] call templates
func registerTemplate(name string, tplBox *rice.Box) {
	tplString, err := tplBox.String(name + ".html")
	if err != nil {
		log.Fatal(err)
	}
	templates[name] = template.Must(template.New(name).Parse(tplString))
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	flag.Parse()
	if *zimPath == "" {
		log.Fatal("provide a zim file path")
	}

	if *mmap {
		log.Println("Using mmap")
	}

	// Do we have an index ?
	if indexPath != nil && *indexPath != "" || *indexPath == "" {

		if *indexPath == "" {
			log.Fatal("provide an index path")
		}

		if _, err := os.Stat(*indexPath); err != nil {
			log.Fatal(err)
		}

		idx = true

		// open the db
		var err error
		index, err = bleve.Open(*indexPath)
		if err != nil {
			log.Fatal(err)
		}
	}

	// assets
	box := rice.MustFindBox("static")
	fileServer := http.StripPrefix("/static/", http.FileServer(box.HTTPBox()))
	http.Handle("/static/", fileServer)

	// crompress wiki pages
	http.HandleFunc("/zim/", makeGzipHandler(zimHandler))
	z, err := zim.NewReader(*zimPath, *mmap)
	Z = z
	if err != nil {
		log.Fatal(err)
	}

	// tpl)
	http.HandleFunc("/robots.txt", robotHandler)
	http.HandleFunc("/", makeGzipHandler(searchHandler))

	// the need for a cache is absolute
	// a lot of the same urls will be called repeatedly, css, js ...
	// avoid to look for those one
	cache, _ = lru.NewARC(40)

	// default listening to port 8080
	listenPath := ":8080"

	if len(os.Getenv("PORT")) > 0 {
		listenPath = ":" + os.Getenv("PORT")
	}

	if port != nil && *port > 0 {
		listenPath = ":" + strconv.Itoa(*port)
	}

	// Opening large indexes could takes minutes on raspberry
	log.Println("Listening on", listenPath)

	err = http.ListenAndServe(listenPath, nil)
	if err != nil {
		log.Fatal(err)
	}

}
