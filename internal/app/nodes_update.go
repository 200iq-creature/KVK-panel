package app

import (
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Updating the panels of the whole link group from the main server: every
// server updates itself from its own branch (the same «Обновить» as in its
// «Настройках»), this server goes last because it restarts.

type updRow struct {
	ID       string `json:"id"` // "" — this server
	Name     string `json:"name"`
	Country  string `json:"country"`
	Main     bool   `json:"main"`
	Current  string `json:"current"`
	Latest   string `json:"latest"`
	Branch   string `json:"branch"`
	Update   bool   `json:"update"`
	Updating bool   `json:"updating"`
	Done     bool   `json:"done"` // updated a moment ago by this button
	Error    string `json:"error,omitempty"`
}

type updMark struct {
	from string
	at   int64
}

var (
	updMu      sync.Mutex
	updMarks   = map[string]updMark{} // server id → update started
	updLatest  = map[string]string{}  // server id → latest version from the last full check
	updBranchs = map[string]string{}
)

const updWindow = 6 * 60 * 1000 // ms: an update that hasn't changed the version by then has failed

// GET /api/nodes/updates — versions of every server; ?quick=1 only asks the
// current versions (for polling while updating), the latest ones come from
// the last full check.
func (a *App) apiNodesUpdates(w http.ResponseWriter, r *http.Request) error {
	quick := r.URL.Query().Get("quick") == "1"
	servers := a.wizServers()
	countries := map[string]string{}
	a.Store.Read(func(d *Data) {
		countries[""] = d.Settings.CountryCode
		for _, n := range d.Nodes {
			countries[n.ID] = n.Country
		}
	})
	rows := make([]updRow, len(servers))
	var wg sync.WaitGroup
	for i, s := range servers {
		wg.Add(1)
		go func(i int, s wizServer) {
			defer wg.Done()
			row := updRow{ID: s.ID, Name: s.Name, Country: countries[s.ID], Main: s.Main}
			updMu.Lock()
			row.Latest, row.Branch = updLatest[s.ID], updBranchs[s.ID]
			updMu.Unlock()
			switch {
			case s.Main && quick:
				row.Current = Version
			case s.Main:
				row.Current, row.Branch = Version, a.UpdateBranch()
				latest, err := fetchLatestPanelVersion(row.Branch)
				if err != nil {
					row.Error = "не удалось проверить обновления: " + err.Error()
				} else {
					row.Latest = latest
				}
			case quick:
				var ni nodeInfo
				if err := nodeCall("GET", s.URL, s.Token, "api/node/info", nil, &ni); err != nil {
					row.Error = err.Error()
				}
				row.Current = ni.Version
			default:
				var res struct {
					Current string `json:"current"`
					Latest  string `json:"latest"`
					Branch  string `json:"branch"`
				}
				if err := nodeCall("GET", s.URL, s.Token, "api/panel/update", nil, &res); err != nil {
					row.Error = err.Error()
					var ni nodeInfo
					if nodeCall("GET", s.URL, s.Token, "api/node/info", nil, &ni) == nil {
						row.Current = ni.Version
					}
				} else {
					row.Current, row.Latest, row.Branch = res.Current, res.Latest, res.Branch
				}
			}
			if row.Current == "" {
				row.Current = s.Version
			}
			now := nowMs()
			updMu.Lock()
			if !quick && row.Latest != "" {
				updLatest[s.ID], updBranchs[s.ID] = row.Latest, row.Branch
			}
			if m, ok := updMarks[s.ID]; ok {
				switch {
				case row.Current != "" && row.Current != m.from:
					row.Done = true
					if now-m.at > updWindow {
						delete(updMarks, s.ID)
					}
				case now-m.at < updWindow:
					row.Updating = true
					row.Error = "" // it restarts — no answer is expected for a while
				default:
					delete(updMarks, s.ID)
					if row.Error == "" {
						row.Error = "обновление не завершилось — версия осталась " + m.from + ". Запустите «kvk update» на сервере"
					}
				}
			}
			updMu.Unlock()
			row.Update = row.Latest != "" && versionNewer(row.Latest, row.Current)
			rows[i] = row
		}(i, s)
	}
	wg.Wait()
	ok(w, map[string]any{"servers": rows, "root": os.Geteuid() == 0})
	return nil
}

// POST /api/nodes/update {ids, self} — start the update on the chosen servers.
func (a *App) apiNodesUpdate(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		IDs  []string `json:"ids"`  // linked servers
		Self bool     `json:"self"` // this server too (last: it restarts)
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	if len(req.IDs) == 0 && !req.Self {
		return errf(400, "Не выбраны серверы")
	}
	if req.Self && os.Geteuid() != 0 {
		return errf(400, "Обновление этого сервера из панели требует запуска от root")
	}
	type res struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
	out := []res{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	seen := map[string]bool{}
	for _, id := range req.IDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		s, found := a.wizServer(id)
		if !found || s.Main {
			out = append(out, res{ID: id, Error: "сервер не найден или выключен"})
			continue
		}
		wg.Add(1)
		go func(s wizServer) {
			defer wg.Done()
			var ni nodeInfo
			from := s.Version
			if nodeCall("GET", s.URL, s.Token, "api/node/info", nil, &ni) == nil && ni.Version != "" {
				from = ni.Version
			}
			err := nodeCall("POST", s.URL, s.Token, "api/panel/update", map[string]any{}, nil)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				out = append(out, res{ID: s.ID, Name: s.Name, Error: err.Error()})
				return
			}
			updMu.Lock()
			updMarks[s.ID] = updMark{from: from, at: nowMs()}
			updMu.Unlock()
			out = append(out, res{ID: s.ID, Name: s.Name, OK: true})
		}(s)
	}
	wg.Wait()
	names := []string{}
	for _, x := range out {
		if x.OK {
			names = append(names, x.Name)
		}
	}
	if req.Self {
		updMu.Lock()
		updMarks[""] = updMark{from: Version, at: nowMs()}
		updMu.Unlock()
		names = append(names, "этот сервер")
		go func() {
			time.Sleep(3 * time.Second) // let the answer and the node calls go out first
			if err := a.RunSelfUpdate(); err != nil {
				a.Store.Update(func(d *Data) error {
					d.AddEvent("bad", "Обновление этого сервера не запустилось: "+err.Error(), "")
					return nil
				})
			}
		}()
		out = append(out, res{Name: "этот сервер", OK: true})
	}
	if len(names) > 0 {
		a.Store.Update(func(d *Data) error {
			d.AddEvent("info", "Запущено обновление панели: "+strings.Join(names, ", "), "")
			return nil
		})
	}
	ok(w, map[string]any{"results": out, "self": req.Self})
	return nil
}
