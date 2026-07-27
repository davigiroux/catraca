// Command catracad runs the catraca HTTP API server.
package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/davigiroux/catraca/api"
	"github.com/davigiroux/catraca/store"
)

func main() {
	addr := flag.String("addr", ":8080", "address to listen on")
	dbPath := flag.String("db", "catraca.db", "path to the SQLite database file")
	merchantsPath := flag.String("merchants", "merchants.json", "path to the operator-managed merchants config file")
	flag.Parse()

	merchants, err := api.LoadMerchants(*merchantsPath)
	if err != nil {
		log.Fatalf("catracad: %v", err)
	}

	s, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("catracad: %v", err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			log.Printf("catracad: closing store: %v", err)
		}
	}()

	handler := api.NewHandler(s, merchants)
	log.Printf("catracad: listening on %s", *addr)
	if err := http.ListenAndServe(*addr, handler); err != nil {
		log.Fatalf("catracad: %v", err)
	}
}
