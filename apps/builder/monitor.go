package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/bketelsen/bespoke/pkg/web"
)

// The monitor bridges the spool back into the app: while a run is building
// or deploying it tails runs/<run>/events.jsonl into run_events and advances
// the state machine on status.json / deploy.json. One goroutine per active
// run; restarted on app boot for runs that were mid-flight.
type monitor struct {
	db *sql.DB

	mu     sync.Mutex
	active map[string]bool
}

func newMonitor(db *sql.DB) *monitor {
	return &monitor{db: db, active: map[string]bool{}}
}

// resume restarts monitors after an app restart.
func (m *monitor) resume() {
	rows, err := m.db.Query("SELECT id FROM runs WHERE status IN ('building','deploying')")
	if err != nil {
		log.Println("monitor resume:", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			m.ensure(id)
		}
	}
}

func (m *monitor) ensure(runID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active[runID] {
		return
	}
	m.active[runID] = true
	go m.watch(runID)
}

func (m *monitor) watch(runID string) {
	defer func() {
		m.mu.Lock()
		delete(m.active, runID)
		m.mu.Unlock()
	}()
	for {
		time.Sleep(2 * time.Second)
		var status, login, slug, idea string
		err := m.db.QueryRow("SELECT status, login, slug, idea FROM runs WHERE id = ?", runID).
			Scan(&status, &login, &slug, &idea)
		if err != nil {
			log.Println("monitor:", runID, err)
			return
		}

		changed := m.tailEvents(runID)

		switch status {
		case "building":
			var st struct {
				OK     bool   `json:"ok"`
				Detail string `json:"detail"`
			}
			if readOutcome(runID, "status.json", &st) {
				if st.OK {
					if err := writeDeployRequest(runID, slug); err != nil {
						m.setStatus(runID, "failed", "deploy request: "+err.Error())
					} else {
						m.setStatus(runID, "deploying", st.Detail)
					}
				} else {
					m.setStatus(runID, "failed", st.Detail)
				}
				changed = true
			}
		case "deploying":
			var res struct {
				OK     bool   `json:"ok"`
				Detail string `json:"detail"`
			}
			if readOutcome(runID, "deploy.json", &res) {
				if res.OK {
					m.setStatus(runID, "live", res.Detail)
				} else {
					m.setStatus(runID, "failed", res.Detail)
				}
				changed = true
			}
		default:
			// Terminal (or rewound) — one last tail already happened above.
			if changed {
				notifyOwner(m.db, runID)
			}
			return
		}

		if changed {
			notifyOwner(m.db, runID)
		}
	}
}

func (m *monitor) setStatus(runID, status, detail string) {
	_, err := m.db.Exec(
		"UPDATE runs SET status = ?, detail = ?, updated_at = datetime('now') WHERE id = ?",
		status, detail, runID)
	if err != nil {
		log.Println("monitor setStatus:", err)
	}
}

// tailEvents mirrors new events.jsonl lines into run_events; the line number
// is the seq, so re-reads are idempotent. Returns true when rows were added.
func (m *monitor) tailEvents(runID string) bool {
	f, err := os.Open(filepath.Join(runDir(runID), "events.jsonl"))
	if err != nil {
		return false
	}
	defer f.Close()

	var have int
	_ = m.db.QueryRow("SELECT count(*) FROM run_events WHERE run_id = ?", runID).Scan(&have)

	added := false
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	seq := 0
	for sc.Scan() {
		seq++
		if seq <= have {
			continue
		}
		var ev struct{ TS, Kind, Text string }
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			ev = struct{ TS, Kind, Text string }{time.Now().UTC().Format(time.DateTime), "raw", sc.Text()}
		}
		_, err := m.db.Exec(
			"INSERT OR IGNORE INTO run_events (run_id, seq, ts, kind, body) VALUES (?,?,?,?,?)",
			runID, seq, ev.TS, ev.Kind, ev.Text)
		if err == nil {
			added = true
		}
	}
	return added
}

func notifyOwner(db *sql.DB, runID string) {
	var login string
	if err := db.QueryRow("SELECT login FROM runs WHERE id = ?", runID).Scan(&login); err == nil {
		web.Changed(login)
	}
}
