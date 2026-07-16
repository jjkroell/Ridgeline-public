// Command scrub is a one-off tool to remove specific nodes and their adverts
// from the database. Reads target pubkeys (one per line) from a file given as
// the first argument. Run with the daemon stopped.
package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"os"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/jjkroell/ridgeline/internal/meshcore"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("usage: scrub <pubkeys-file> <db-path>")
		os.Exit(1)
	}
	targets := loadTargets(os.Args[1])
	fmt.Println("targets:", len(targets))

	db, err := sql.Open("sqlite", os.Args[2])
	must(err)
	defer db.Close()

	var nodeDel int64
	for k := range targets {
		res, err := db.Exec(`DELETE FROM nodes WHERE UPPER(pubkey) = ?`, k)
		must(err)
		n, _ := res.RowsAffected()
		nodeDel += n
	}
	fmt.Println("deleted node rows:", nodeDel)

	var obsvDel int64
	for k := range targets {
		res, err := db.Exec(`DELETE FROM observers WHERE UPPER(pubkey) = ?`, k)
		must(err)
		n, _ := res.RowsAffected()
		obsvDel += n
	}
	fmt.Println("deleted observer rows:", obsvDel)

	rows, err := db.Query(`SELECT id, raw_hex FROM observations WHERE payload_type = 'Advert'`)
	must(err)
	var ids []int64
	scanned := 0
	for rows.Next() {
		var id int64
		var raw string
		must(rows.Scan(&id, &raw))
		scanned++
		pkt, err := meshcore.DecodeHex(raw)
		if err != nil || pkt == nil || pkt.Advert == nil {
			continue
		}
		if targets[strings.ToUpper(pkt.Advert.PublicKey)] {
			ids = append(ids, id)
		}
	}
	rows.Close()
	fmt.Printf("scanned %d adverts; %d belong to targets\n", scanned, len(ids))

	var obsDel int64
	for _, id := range ids {
		res, err := db.Exec(`DELETE FROM observations WHERE id = ?`, id)
		must(err)
		n, _ := res.RowsAffected()
		obsDel += n
	}
	fmt.Println("deleted advert observations:", obsDel)
}

func loadTargets(path string) map[string]bool {
	f, err := os.Open(path)
	must(err)
	defer f.Close()
	t := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k := strings.ToUpper(strings.TrimSpace(sc.Text()))
		if k != "" {
			t[k] = true
		}
	}
	return t
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
