// Command whybridge explains why one node is (or isn't) flagged as an RF bridge
// candidate: it replays the detector's internals for a single pubkey and prints
// the foreign nodes routing through it, with paths, specificity, and distance.
// Throwaway diagnostic — not part of the daemon.
package main

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jjkroell/ridgeline/internal/meshcore"
	"github.com/jjkroell/ridgeline/internal/store"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Println("usage: whybridge <db> <pubkey> <sinceSec>")
		os.Exit(1)
	}
	dbPath, target := os.Args[1], strings.ToUpper(os.Args[2])
	var sinceSec int
	fmt.Sscan(os.Args[3], &sinceSec)

	st, err := store.Open(dbPath)
	must(err)
	defer st.Close()
	nodes, err := st.ListNodes()
	must(err)

	name := map[string]string{}
	lat := map[string]*float64{}
	lon := map[string]*float64{}
	for _, n := range nodes {
		k := strings.ToUpper(n.PublicKey)
		name[k] = n.Name
		lat[k], lon[k] = n.Latitude, n.Longitude
	}
	nm := func(k string) string {
		if name[k] != "" {
			return name[k]
		}
		return k[:12]
	}

	// prefix resolver (same as analytics)
	index := map[int]map[string][]string{2: {}, 4: {}, 6: {}}
	for _, n := range nodes {
		pk := strings.ToUpper(n.PublicKey)
		for _, l := range []int{2, 4, 6} {
			if len(pk) >= l {
				index[l][pk[:l]] = append(index[l][pk[:l]], pk)
			}
		}
	}
	resolve := func(hop string) string {
		h := strings.ToUpper(hop)
		if m := index[len(h)]; m != nil {
			if x := m[h]; len(x) == 1 {
				return x[0]
			}
		}
		return ""
	}

	cutoff := time.Now().Add(-time.Duration(sinceSec) * time.Second).UTC().Format(time.RFC3339Nano)
	raws, err := st.RawWindow(cutoff, 0)
	must(err)

	// FIND:<substr> mode: decode adverts and print pubkey+name for matches.
	if strings.HasPrefix(target, "FIND:") {
		sub := strings.ToLower(strings.TrimPrefix(target, "FIND:"))
		seen := map[string]string{}
		for _, ro := range raws {
			pkt, err := meshcore.DecodeHex(ro.RawHex)
			if err != nil || pkt == nil || pkt.Advert == nil || pkt.Advert.PublicKey == "" {
				continue
			}
			pk := strings.ToUpper(pkt.Advert.PublicKey)
			if pkt.Advert.Name != "" {
				seen[pk] = pkt.Advert.Name
			}
		}
		for pk, name := range seen {
			if sub == "" || strings.Contains(strings.ToLower(name), sub) || strings.HasPrefix(pk, strings.ToUpper(sub)) {
				fmt.Printf("  %s  %s\n", pk, name)
			}
		}
		return
	}

	// SCAN mode: list every relay's captive-foreign count (incl. sub-threshold),
	// plus the foreign nodes we've actually ingested — to see what's forming.
	if target == "SCAN" {
		dh := map[string]bool{}
		obsT := map[string]int{}
		advCount := map[string]int{}
		via := map[string]map[string]int{}
		for _, ro := range raws {
			pkt, err := meshcore.DecodeHex(ro.RawHex)
			if err != nil || pkt == nil || pkt.Advert == nil || pkt.Advert.PublicKey == "" {
				continue
			}
			o := strings.ToUpper(pkt.Advert.PublicKey)
			advCount[o]++
			if len(pkt.Path) == 0 {
				dh[o] = true
				continue
			}
			obsT[o]++
			seen := map[string]bool{}
			for _, h := range pkt.Path {
				if k := resolve(h); k != "" {
					ku := strings.ToUpper(k)
					if seen[ku] {
						continue
					}
					seen[ku] = true
					if via[ku] == nil {
						via[ku] = map[string]int{}
					}
					via[ku][o]++
				}
			}
		}
		type cand struct {
			relay            string
			captive, foreign int
		}
		var cands []cand
		for relay, origins := range via {
			cap_, fr := 0, 0
			for o, c := range origins {
				if dh[o] {
					continue
				}
				fr++
				if float64(c)/float64(max(1, obsT[o])) >= 0.95 {
					cap_++
				}
			}
			if fr > 0 {
				cands = append(cands, cand{relay, cap_, fr})
			}
		}
		sort.Slice(cands, func(i, j int) bool {
			if cands[i].captive != cands[j].captive {
				return cands[i].captive > cands[j].captive
			}
			return cands[i].foreign > cands[j].foreign
		})
		foreign := []string{}
		for o := range advCount {
			if !dh[o] {
				foreign = append(foreign, o)
			}
		}
		fmt.Printf("Window %ds: %d nodes adverted, %d never-zero-hop (foreign candidates)\n", sinceSec, len(advCount), len(foreign))
		fmt.Printf("Flag rule: captive≥3 AND captive/foreign≥0.6\n\n")
		fmt.Printf("Top relays by captive-foreign count:\n")
		for i, c := range cands {
			if i > 20 {
				break
			}
			flag := ""
			if c.captive >= 3 && float64(c.captive)/float64(max(1, c.foreign)) >= 0.6 {
				flag = "  <-- WOULD FLAG"
			}
			fmt.Printf("  %-26s captive=%d/%d%s\n", trunc(nm(c.relay), 26), c.captive, c.foreign, flag)
		}
		fmt.Printf("\nForeign (never-zero-hop) nodes seen, with advert count:\n")
		sort.Slice(foreign, func(i, j int) bool { return advCount[foreign[i]] > advCount[foreign[j]] })
		for _, o := range foreign {
			fmt.Printf("  %-26s adverts=%d\n", trunc(nm(o), 26), advCount[o])
		}
		return
	}

	directlyHeard := map[string]bool{}
	txHops := map[string]map[string]map[string]bool{} // origin -> msgHash -> resolved hops
	rawPath := map[string]map[string][]string{}       // origin -> msgHash -> example raw path
	// Per-origin: count of individual observed paths, and how many contained the
	// target. A true bridge is on ~100% of an origin's observed paths (no
	// alternative route); a legit relay has target-free observations (alternatives).
	obsTotal := map[string]int{}
	obsViaTarget := map[string]int{}
	for _, ro := range raws {
		pkt, err := meshcore.DecodeHex(ro.RawHex)
		if err != nil || pkt == nil || pkt.Advert == nil || pkt.Advert.PublicKey == "" {
			continue
		}
		o := strings.ToUpper(pkt.Advert.PublicKey)
		if len(pkt.Path) == 0 {
			directlyHeard[o] = true
			continue
		}
		obsTotal[o]++
		hasTarget := false
		if txHops[o] == nil {
			txHops[o] = map[string]map[string]bool{}
			rawPath[o] = map[string][]string{}
		}
		if txHops[o][pkt.MessageHash] == nil {
			txHops[o][pkt.MessageHash] = map[string]bool{}
			rawPath[o][pkt.MessageHash] = pkt.Path
		}
		for _, h := range pkt.Path {
			if k := resolve(h); k != "" {
				ku := strings.ToUpper(k)
				txHops[o][pkt.MessageHash][ku] = true
				if ku == target {
					hasTarget = true
				}
			}
		}
		if hasTarget {
			obsViaTarget[o]++
		}
	}

	// always-present relays per origin
	through := map[string]bool{} // origins with target in always-present set
	var foreign, local []string
	exampleHash := map[string]string{}
	for o, txs := range txHops {
		var always map[string]bool
		for _, hops := range txs {
			if always == nil {
				always = map[string]bool{}
				for k := range hops {
					always[k] = true
				}
				continue
			}
			for k := range always {
				if !hops[k] {
					delete(always, k)
				}
			}
		}
		if !always[target] {
			continue
		}
		through[o] = true
		if directlyHeard[o] {
			local = append(local, o)
		} else {
			foreign = append(foreign, o)
			for h := range txs {
				exampleHash[o] = h
				break
			}
		}
	}

	fmt.Printf("TARGET %s (%s)\n", nm(target), target[:16])
	fmt.Printf("  directly heard (zero-hop by an observer)? %v\n", directlyHeard[target])
	fmt.Printf("  origins routed through it: %d total  →  %d foreign (never zero-hop) + %d local\n\n",
		len(through), len(foreign), len(local))
	spec := float64(len(foreign)) / float64(max(1, len(through)))
	fmt.Printf("  foreignCount=%d  throughTotal=%d  specificity=%.2f  (thresholds: ≥3 and ≥0.80)\n\n",
		len(foreign), len(through), spec)

	// distances
	mlat, mlon := medianLoc(nodes)
	fmt.Printf("  FOREIGN nodes entering via %s (the ones that flag it):\n", nm(target))
	sort.Slice(foreign, func(i, j int) bool { return nm(foreign[i]) < nm(foreign[j]) })
	var flat, flon []float64
	for _, o := range foreign {
		d := "no-gps"
		if lat[o] != nil && lon[o] != nil && (*lat[o] != 0 || *lon[o] != 0) {
			km := haversine(mlat, mlon, *lat[o], *lon[o])
			d = fmt.Sprintf("%.0f km from mesh ctr", km)
			flat = append(flat, *lat[o])
			flon = append(flon, *lon[o])
		}
		frac := float64(obsViaTarget[o]) / float64(max(1, obsTotal[o]))
		alt := ""
		if obsViaTarget[o] < obsTotal[o] {
			alt = fmt.Sprintf("  ← %d alt route(s) NOT via target", obsTotal[o]-obsViaTarget[o])
		}
		fmt.Printf("    • %-26s %-20s via-target %d/%d=%.0f%%%s\n",
			trunc(nm(o), 26), d, obsViaTarget[o], obsTotal[o], frac*100, alt)
	}
	if len(flat) > 0 {
		fc, fcn := median(flat), median(flon)
		fmt.Printf("\n  foreign-cluster centroid = %.3f,%.3f   mesh centroid = %.3f,%.3f   →  foreignKm=%.0f\n",
			fc, fcn, mlat, mlon, haversine(mlat, mlon, fc, fcn))
	}
}

func renderPath(path []string, resolve func(string) string, nm func(string) string, target string) string {
	var parts []string
	for _, h := range path {
		k := resolve(h)
		if k == target {
			parts = append(parts, "["+h+":"+nm(k)+"]") // bracket the target
		} else if k != "" {
			parts = append(parts, h+":"+trunc(nm(k), 10))
		} else {
			parts = append(parts, h+":?")
		}
	}
	return strings.Join(parts, " → ")
}

func medianLoc(nodes []store.Node) (float64, float64) {
	var la, lo []float64
	for _, n := range nodes {
		if n.Latitude != nil && n.Longitude != nil && (*n.Latitude != 0 || *n.Longitude != 0) {
			la = append(la, *n.Latitude)
			lo = append(lo, *n.Longitude)
		}
	}
	return median(la), median(lo)
}
func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	return s[len(s)/2]
}
func haversine(a, b, c, d float64) float64 {
	const R = 6371.0
	r := math.Pi / 180
	dLat, dLon := (c-a)*r, (d-b)*r
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(a*r)*math.Cos(c*r)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return R * 2 * math.Atan2(math.Sqrt(h), math.Sqrt(1-h))
}
func trunc(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}
