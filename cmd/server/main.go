// Command server is the runnable entry point for the unitized curtain-wall
// silicone hoist-gate backend. It opens the SQLite store, runs the
// restart-recovery bootstrap, wires the application service and serves the HTTP
// API. Projections are rebuilt from the append-only event chain on demand, so
// no in-memory state survives a restart.
package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"unitized-curtainwall-silicone-hoist-gate/internal/api"
	"unitized-curtainwall-silicone-hoist-gate/internal/service"
	"unitized-curtainwall-silicone-hoist-gate/internal/store"
)

func main() {
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "hoist-gate.db"
	}

	st, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()

	if err := recover(st); err != nil {
		log.Fatalf("recovery: %v", err)
	}

	svc := service.New(st)
	srv := api.NewServer(svc)
	log.Printf("unitized curtain-wall hoist gate listening on %s (db=%s)", addr, dbPath)
	if err := http.ListenAndServe(addr, srv); err != nil {
		log.Fatalf("server: %v", err)
	}
}

// recover performs the restart-recovery bootstrap: it expires stale resource
// leases and reconciles any prepared-but-uncommitted transaction journal
// entries. Because every business change commits atomically in SQLite, an
// interrupted transaction is already rolled back; this step only clears
// transient lease holds so a crash cannot block new work.
//
// The purge watermark is the high-water mark of the business logical clock
// (the greatest logical_time across all committed evidence events), never
// wall-clock time: leases are acquired and expired in the logical-clock
// domain, so using a nanosecond wall-clock value here would delete every
// lease on restart, discarding holds that are still within their business
// validity window.
func recover(st *store.Store) error {
	return st.InTx(context.Background(), func(tx *store.Tx) error {
		watermark, err := tx.MaxLogicalTime()
		if err != nil {
			return err
		}
		if err := tx.ExpiredLeases(watermark); err != nil {
			return err
		}
		incomplete, err := tx.IncompleteJournal()
		if err != nil {
			return err
		}
		for _, j := range incomplete {
			log.Printf("recovery: discarding uncommitted journal batch %s (events %d..%d)", j.TxnID, j.EventFrom, j.EventTo)
		}
		return nil
	})
}
