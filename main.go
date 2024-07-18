package main

import (
	"flag"
	"fmt"
	"os"
	"log"
	"net/http"
	"path/filepath"
)


func main(){
	// ganti sendiri2 ye
	defaultDirectory := "/home/riq/src/test"

	// directory nya
	dir := flag.String("dir", defaultDirectory, "the directory of static file to host")
	flag.Parse()

	// port nya
	port := flag.Int("port", 9000, "port to serve on")
	flag.Parse()

	// working directory
	absDir, err := filepath.Abs(*dir)
	if err != nil {
		log.Fatal("Could not determine the absolute path of directory %s", *dir)

	}

	// file server handler
	fileServer := http.FileServer(http.Dir(absDir))
	http.Handle("/", fileServer)

	// start server
	fmt.Printf("Serving directory %s on HTTP port: %d\n", absDir, *port)
	err = http.ListenAndServe(fmt.Sprintf(":%d", *port), nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
		os.Exit(1)
	}
}