package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"portinaia/internal/server"
	"portinaia/internal/version"
)

const listenAddress = ":8080"

func main() {
	if len(os.Args) != 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "server":
		log.Printf("portinaia listening on %s", listenAddress)
		if err := http.ListenAndServe(listenAddress, server.NewHandler()); err != nil {
			log.Fatal(err)
		}
	case "version":
		fmt.Printf("portinaia %s\n", version.String())
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: portinaia {server|version}")
}
