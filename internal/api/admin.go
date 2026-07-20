package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jjkroell/ridgeline/internal/analytics"
	"github.com/jjkroell/ridgeline/internal/store"
)

// The injection-detection / quarantine handlers below are gated by
// requireAdminUser (any is_admin account, session-authenticated), so they take
// the acting user like the other admin-console handlers. The user isn't used by
// the detection logic itself.

// adminDetect runs injection detection over the window.
func (s *Server) adminDetect(w http.ResponseWriter, r *http.Request, _ store.User) {
	sinceSec := queryInt(r, "since", 24*3600, 1, 7*86400)
	cutoff := time.Now().Add(-time.Duration(sinceSec) * time.Second).UTC().Format(time.RFC3339Nano)
	nodes, err := s.store.ListNodes()
	if err != nil {
		s.fail(w, err)
		return
	}
	report, err := analytics.DetectInjection(s.store, nodes, cutoff, 0)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, report)
}

func (s *Server) adminBlocklist(w http.ResponseWriter, _ *http.Request, _ store.User) {
	list, err := s.store.ListBlocks()
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, list)
}

// blockReq is the body for POST /api/admin/block (quarantine, reversible).
type blockReq struct {
	Kind   string `json:"kind"` // observer | bridge | node | allow | known
	Key    string `json:"key"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
	// Nodes optionally blocks additional node pubkeys as kind "node" alongside
	// the main entry — used to hide a bridge's whole foreign cluster at once.
	Nodes []string `json:"nodes,omitempty"`
}

func (s *Server) adminBlock(w http.ResponseWriter, r *http.Request, _ store.User) {
	var req blockReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request body")
		return
	}
	if !validKind(req.Kind) || req.Key == "" {
		writeErr(w, http.StatusBadRequest, "kind must be observer|bridge|node|allow|known and key required")
		return
	}
	if err := s.store.AddBlock(req.Kind, req.Key, req.Name, req.Reason); err != nil {
		s.fail(w, err)
		return
	}
	for _, n := range req.Nodes {
		if n != "" {
			s.store.AddBlock("node", n, "", "foreign node via "+req.Name)
		}
	}
	s.log.Info("admin quarantined", "kind", req.Kind, "key", req.Key, "extraNodes", len(req.Nodes), "reason", req.Reason)
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) adminUnblock(w http.ResponseWriter, r *http.Request, _ store.User) {
	kind := r.URL.Query().Get("kind")
	key := r.URL.Query().Get("key")
	if !validKind(kind) || key == "" {
		writeErr(w, http.StatusBadRequest, "kind and key required")
		return
	}
	if err := s.store.RemoveBlock(kind, key); err != nil {
		s.fail(w, err)
		return
	}
	s.log.Info("admin un-quarantined", "kind", kind, "key", key)
	writeJSON(w, map[string]bool{"ok": true})
}

// purgeReq is the body for POST /api/admin/purge (hard delete). Each list holds
// the targets to remove; the affected entries are also added to the blocklist so
// purged data does not re-ingest.
type purgeReq struct {
	Observers []string `json:"observers"`
	Bridges   []string `json:"bridges"`
	Nodes     []string `json:"nodes"`
}

func (s *Server) adminPurge(w http.ResponseWriter, r *http.Request, _ store.User) {
	var req purgeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request body")
		return
	}
	if len(req.Observers)+len(req.Bridges)+len(req.Nodes) == 0 {
		writeErr(w, http.StatusBadRequest, "nothing to purge")
		return
	}
	// Block only the INGRESS points (bridges + observers) so they can't re-ingest;
	// these remain on the blocklist. The nodes they brought in are deleted
	// permanently with NO block — once the bridge/observer is blocked their traffic
	// can't return anyway, so there's no need to keep an entry for each one.
	for _, o := range req.Observers {
		s.store.AddBlock("observer", o, o, "purged")
	}
	for _, b := range req.Bridges {
		s.store.AddBlock("bridge", b, "", "purged")
	}
	// Purge targets come from the captivity DETECTOR, which is documented to
	// over-flag in a sparse-observer mesh. A claimed node is evidence it misfired,
	// so everything still gets blocked (reversible, and it's what actually stops
	// the traffic) but claimed keys are held back from the delete, which is not
	// reversible. Deliberate removal is what adminDelete is for.
	nodes, skippedNodes, err := s.store.PartitionClaimed(req.Nodes)
	if err != nil {
		s.fail(w, err)
		return
	}
	bridges, skippedBridges, err := s.store.PartitionClaimed(req.Bridges)
	if err != nil {
		s.fail(w, err)
		return
	}
	res, err := s.store.ScrubNodes(req.Observers, bridges, nodes)
	if err != nil {
		s.fail(w, err)
		return
	}
	res.SkippedClaimed = append(skippedNodes, skippedBridges...)
	s.log.Info("admin purged", "observers", len(req.Observers), "bridges", len(req.Bridges),
		"nodes", len(req.Nodes), "observationsDeleted", res.Observations, "nodesDeleted", res.Nodes,
		"claimsDeleted", res.Claims, "notesDeleted", res.Notes,
		"locationsDeleted", res.Locations, "sharesDeleted", res.Shares,
		"skippedClaimed", res.SkippedClaimed)
	writeJSON(w, res)
}

// adminDelete permanently deletes nodes and/or observers (their rows + stored
// observations) with NO blocklist entry — a clean removal, distinct from purge
// which keeps the ingress blocked. A deleted observer/node that still transmits
// (or keeps publishing) re-appears on its next report; delete is for retiring
// stale/old entries, not for stopping active injectors (use purge for that).
func (s *Server) adminDelete(w http.ResponseWriter, r *http.Request, _ store.User) {
	var req struct {
		Nodes     []string `json:"nodes"`
		Observers []string `json:"observers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request body")
		return
	}
	if len(req.Nodes)+len(req.Observers) == 0 {
		writeErr(w, http.StatusBadRequest, "nothing to delete")
		return
	}
	res, err := s.store.ScrubNodes(req.Observers, nil, req.Nodes)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.log.Info("admin deleted", "nodes", len(req.Nodes), "observers", len(req.Observers),
		"observationsDeleted", res.Observations, "nodesDeleted", res.Nodes,
		"claimsDeleted", res.Claims, "notesDeleted", res.Notes,
		"locationsDeleted", res.Locations, "sharesDeleted", res.Shares)
	writeJSON(w, res)
}

// adminRetiredObservers lists the observers currently withdrawn from the
// observers page.
func (s *Server) adminRetiredObservers(w http.ResponseWriter, _ *http.Request, _ store.User) {
	obs, err := s.store.ListRetiredObservers()
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, obs)
}

// adminRetireObserver withdraws a decommissioned observer from the observers
// page while keeping every packet it reported.
//
// This is the non-destructive counterpart to POST /api/admin/delete, which runs
// ScrubNodes and deletes the observer's observations outright. Retiring is the
// right action for a receiver that has simply left the network: its history
// stays attributable to it, and the retirement survives the broker replaying
// its retained /status message.
func (s *Server) adminRetireObserver(w http.ResponseWriter, r *http.Request, _ store.User) {
	s.setObserverRetired(w, r, true)
}

// adminUnretireObserver returns a retired observer to the observers page.
func (s *Server) adminUnretireObserver(w http.ResponseWriter, r *http.Request, _ store.User) {
	s.setObserverRetired(w, r, false)
}

func (s *Server) setObserverRetired(w http.ResponseWriter, r *http.Request, retire bool) {
	var req struct {
		Observer string `json:"observer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request body")
		return
	}
	if strings.TrimSpace(req.Observer) == "" {
		writeErr(w, http.StatusBadRequest, "observer required")
		return
	}
	var err error
	if retire {
		err = s.store.RetireObserver(req.Observer, time.Now().UTC().Format(time.RFC3339))
	} else {
		err = s.store.UnretireObserver(req.Observer)
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	s.log.Info("admin observer retirement", "observer", req.Observer, "retired", retire)
	writeJSON(w, map[string]any{"observer": req.Observer, "retired": retire})
}

func validKind(k string) bool {
	return k == "observer" || k == "bridge" || k == "node" || k == "allow" || k == store.BlockKnown
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// writeJSONStatus writes an arbitrary JSON body with an explicit status code.
func writeJSONStatus(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
