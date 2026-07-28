// Command catracad runs the catraca HTTP API server and the RPC watch/notify
// scan loop.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/davigiroux/catraca/api"
	"github.com/davigiroux/catraca/notify"
	"github.com/davigiroux/catraca/store"
	"github.com/davigiroux/catraca/watch"
	"github.com/gagliardetto/solana-go/rpc"
)

func main() {
	addr := flag.String("addr", ":8080", "address to listen on")
	dbPath := flag.String("db", "catraca.db", "path to the SQLite database file")
	merchantsPath := flag.String("merchants", "merchants.json", "path to the operator-managed merchants config file")
	rpcEndpoint := flag.String("rpc-endpoint", rpc.DevNet_RPC, "Solana RPC endpoint the watcher reads from")
	scanInterval := flag.Duration("scan-interval", 10*time.Second, "interval between watch/notify scan passes")
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

	// Deliveries share the intents SQLite file; the tables are independent
	// (see notify.Open's doc comment).
	deliveries, err := notify.Open(*dbPath)
	if err != nil {
		log.Fatalf("catracad: %v", err)
	}
	defer func() {
		if err := deliveries.Close(); err != nil {
			log.Printf("catracad: closing delivery store: %v", err)
		}
	}()

	chain := watch.NewRPCChainReader(*rpcEndpoint)

	watcher := &watch.Watcher{
		Store:             s,
		Chain:             chain,
		Merchants:         watch.APIMerchants{Merchants: merchants},
		DefaultCommitment: watch.CommitmentFinalized, // ADR 0001
	}

	notifier := &notify.Notifier{
		Intents:    s,
		Deliveries: deliveries,
		Merchants:  notify.APIMerchants{Merchants: merchants},
	}

	// Run the scan loop alongside the HTTP server. Graceful shutdown /
	// context cancellation on process exit is a future concern, not
	// required for this ticket.
	go runScanLoop(context.Background(), watcher, notifier, *scanInterval)

	handler := api.NewHandler(s, merchants)
	log.Printf("catracad: listening on %s", *addr)
	if err := http.ListenAndServe(*addr, handler); err != nil {
		log.Fatalf("catracad: %v", err)
	}
}

// runScanLoop runs watch and notify scan passes on a fixed interval,
// logging (not fataling) on error so a single bad tick doesn't crash the
// daemon.
func runScanLoop(ctx context.Context, watcher *watch.Watcher, notifier *notify.Notifier, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		if err := watcher.ScanPending(ctx); err != nil {
			log.Printf("catracad: scan pending: %v", err)
		}
		if err := watcher.ScanDetected(ctx); err != nil {
			log.Printf("catracad: scan detected: %v", err)
		}
		if err := watcher.ScanForExpiry(ctx); err != nil {
			log.Printf("catracad: scan expiry: %v", err)
		}
		if err := notifier.Scan(ctx); err != nil {
			log.Printf("catracad: notify scan: %v", err)
		}
	}
}
