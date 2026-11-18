package app

import (
	"net/http"
	"strconv"
	"strings"
)

// Copying an inbound to other servers of the link group (or to the same one).
// What belongs to the source server is replaced by the target's own:
//   - Reality: new keys; if the mask was the source's own site (127.0.0.1:…),
//     it becomes the target's site and its domain (SNI);
//   - TLS: the certificate of the same domain on the target, else the target's
//     site domain, else any active certificate there;
//   - AmneziaWG: new server keys and a free subnet;
//   - the remark «<source> …» becomes «<target> …».

type copyReq struct {
	From  string   `json:"from"` // "" — this server, else a node id
	ID    string   `json:"id"`
	To    []string `json:"to"`    // "" — this server, else node ids
	Grant bool     `json:"grant"` // give the copy to the clients that have the original; false — to nobody
}

type copyResult struct {
	Server  string `json:"server"`
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	ID      string `json:"id,omitempty"`
	Remark  string `json:"remark,omitempty"`
	Port    int    `json:"port,omitempty"`
	Error   string `json:"error,omitempty"`
	Warning string `json:"warning,omitempty"`
}

type copySite struct {
	Settings FakeSiteSettings `json:"settings"`
}

func (a *App) apiInboundCopy(w http.ResponseWriter, r *http.Request) error {
	var req copyReq
	if err := readJSON(r, &req); err != nil {
		return err
	}
	if len(req.To) == 0 {
		return errf(400, "Выберите, на какие серверы скопировать")
	}
	src, okSrc := a.wizServer(req.From)
	if !okSrc {
		return errf(404, "Сервер не найден")
	}
	var list struct {
		Inbounds []Inbound `json:"inbounds"`
	}
	if err := a.srvCall(src, "GET", "inbounds", nil, &list); err != nil {
		return errf(502, "%s: %v", src.Name, err)
	}
	var in *Inbound
	for i := range list.Inbounds {
		if list.Inbounds[i].ID == req.ID {
			in = &list.Inbounds[i]
		}
	}
	if in == nil {
		return errf(404, "Инбаунд не найден")
	}
	if in.Front != "" || in.Auto == "cdn" {
		return errf(400, "«%s» работает за сайтом-прикрытием, поэтому его нельзя скопировать. На другом сервере запустите «Автонастройку»", in.Remark)
	}
	srcDoms, _ := a.wizListDomains(src)
	var srcSite copySite
	_ = a.srvCall(src, "GET", "autosetup", nil, &srcSite)

	results := []copyResult{}
	type made struct {
		server wizServer
		id     string
	}
	var created []made
	seenTo := map[string]bool{}
	for _, to := range req.To {
		to = strings.TrimSpace(to) // "" — this server
		if seenTo[to] {
			continue
		}
		seenTo[to] = true
		dst, okDst := a.wizServer(to)
		if !okDst {
			results = append(results, copyResult{Server: to, Error: "сервер не найден или выключен"})
			continue
		}
		res := a.copyInboundTo(*in, src, dst, srcDoms, srcSite.Settings)
		results = append(results, res)
		if res.OK {
			created = append(created, made{dst, res.ID})
		}
	}

	// access: the copy goes to the clients that have the original (or to nobody)
	if len(created) > 0 {
		srcKey := in.ID
		if !src.Main {
			srcKey = src.ID + ":" + in.ID
		}
		nodeCopy := false
		for _, m := range created {
			if !m.server.Main {
				nodeCopy = true
			}
		}
		if nodeCopy {
			// learn the new node inbounds now (a new one would otherwise go to everyone using that server)
			syncMu.Lock()
			a.syncNodesNow()
			syncMu.Unlock()
		}
		a.Store.Update(func(d *Data) error {
			for _, m := range created {
				key := m.id
				if !m.server.Main {
					key = m.server.ID + ":" + m.id
				}
				for _, c := range d.Clients {
					if c.Origin == "main" {
						continue
					}
					has := req.Grant && c.HasInbound(srcKey)
					if has && !c.HasInbound(key) {
						c.Inbounds = append(c.Inbounds, key)
					} else if !has && c.HasInbound(key) {
						c.Inbounds = removeStr(c.Inbounds, key)
					}
				}
			}
			return nil
		})
		a.ScheduleApply()
		go a.SyncNodes()
	}
	n := 0
	for _, x := range results {
		if x.OK {
			n++
		}
	}
	if n > 0 {
		a.Store.Update(func(d *Data) error {
			d.AddEvent("ok", "Инбаунд «"+in.Remark+"» скопирован: "+strconv.Itoa(n)+" "+plural(n, "сервер", "сервера", "серверов"), "")
			return nil
		})
	}
	ok(w, map[string]any{"results": results, "copied": n})
	return nil
}

// copyInboundTo creates the copy of in on dst.
func (a *App) copyInboundTo(in Inbound, src, dst wizServer, srcDoms wizDomains, srcSite FakeSiteSettings) copyResult {
	res := copyResult{Server: dst.ID, Name: dst.Name}
	fail := func(msg string) copyResult { res.Error = msg; return res }
	cp := in
	cp.ID, cp.Tag, cp.Up, cp.Down, cp.CreatedAt, cp.Auto = "", "", 0, 0, 0, ""
	cp.Front, cp.FrontHosts = "", nil
	cp.Enabled = true
	same := src.ID == dst.ID && src.Main == dst.Main
	switch {
	case same:
		cp.Remark = in.Remark + " копия"
		cp.Port = 0 // the original holds its port
	case strings.HasPrefix(in.Remark, src.Name+" "):
		cp.Remark = dst.Name + strings.TrimPrefix(in.Remark, src.Name)
	}

	// names that belong to the source server: its domains and its site
	srcNames := map[string]bool{}
	for _, d := range srcDoms.Domains {
		srcNames[d.Name] = true
	}
	for _, d := range srcSite.Domains {
		srcNames[d] = true
	}
	var dstDoms wizDomains
	var dstSite copySite
	if !same {
		var err error
		if dstDoms, err = a.wizListDomains(dst); err != nil {
			return fail(err.Error())
		}
		_ = a.srvCall(dst, "GET", "autosetup", nil, &dstSite)
	} else {
		dstDoms, dstSite.Settings = srcDoms, srcSite
	}
	dstHost := "" // the target's own name for links (replaces the source's)
	warn := []string{}

	if cp.Security == "reality" {
		cp.PrivateKey, cp.PublicKey, cp.ShortIDs = "", "", nil
		if !same && strings.HasPrefix(cp.RealityTarget, "127.0.0.1:") {
			st := dstSite.Settings
			if !st.Enabled || st.TLSPort == 0 || st.SNI == "" {
				return fail("Reality смотрит на сайт-прикрытие своего сервера, а на «" + dst.Name + "» сайта нет. Запустите там «Сайт» или «Автонастройку» и повторите")
			}
			cp.RealityTarget = "127.0.0.1:" + strconv.Itoa(st.TLSPort)
			cp.RealitySNI = []string{st.SNI}
			dstHost = st.SNI
		} else if !same {
			// a foreign mask (www.microsoft.com …) works from anywhere; drop the source's own names
			sni := []string{}
			for _, s := range cp.RealitySNI {
				if !srcNames[s] {
					sni = append(sni, s)
				}
			}
			if len(sni) == 0 {
				return fail("маска Reality — домен сервера «" + src.Name + "». Укажите маску вручную после копирования")
			}
			cp.RealitySNI = sni
		}
	}

	certID := ""
	if cp.CertID != "" {
		certName := ""
		for _, d := range srcDoms.Domains {
			if d.ID == cp.CertID {
				certName = d.Name
			}
		}
		if same {
			certID = cp.CertID
		} else {
			pick := func(ok func(d wizDomain) bool) {
				for _, d := range dstDoms.Domains {
					if certID == "" && d.Status == "active" && ok(d) {
						certID, dstHost = d.ID, d.Name
					}
				}
			}
			if certName != "" {
				pick(func(d wizDomain) bool { return d.Name == certName }) // the same domain is on the target too
			}
			if sni := dstSite.Settings.SNI; certID == "" && sni != "" {
				pick(func(d wizDomain) bool { return d.Name == sni && d.Mode != "selfsigned" })
				pick(func(d wizDomain) bool { return d.Name == sni })
			}
			pick(func(d wizDomain) bool { return d.Mode != "selfsigned" && !d.Panel })
			pick(func(d wizDomain) bool { return d.Mode != "selfsigned" })
			pick(func(d wizDomain) bool { return true })
			if certID == "" {
				return fail("на «" + dst.Name + "» нет выпущенного сертификата. Добавьте домен в разделе «Домены» этого сервера")
			}
			if certName != "" && dstHost != certName {
				warn = append(warn, "сертификат: "+dstHost+" вместо "+certName)
			}
		}
		cp.CertID = certID
	}
	if !same {
		if cp.TLSServerName != "" && srcNames[cp.TLSServerName] {
			cp.TLSServerName = dstHost
		}
		if cp.ExternalHost != "" && (srcNames[cp.ExternalHost] || cp.ExternalHost == srcDoms.ServerIP) {
			cp.ExternalHost = dstHost // empty: the target's own address
		}
		if cp.Host != "" && srcNames[cp.Host] {
			cp.Host = dstHost
		}
	}

	if cp.Protocol == "amneziawg" && cp.AWG != nil {
		aw := *cp.AWG
		aw.PrivateKey, aw.PublicKey = "", ""
		cp.AWG = &aw
	}

	var out Inbound
	err := a.srvCall(dst, "POST", "inbounds", cp, &out)
	if err != nil && cp.Port != 0 && strings.Contains(err.Error(), "орт") {
		// the port is taken there — any free one
		warn = append(warn, "порт "+strconv.Itoa(cp.Port)+" занят — выбран другой")
		cp.Port = 0
		err = a.srvCall(dst, "POST", "inbounds", cp, &out)
	}
	for i := 66; err != nil && cp.Protocol == "amneziawg" && cp.AWG != nil && strings.Contains(err.Error(), "пересекается") && i < 100; i++ {
		aw := *cp.AWG
		aw.Network = "10." + strconv.Itoa(i) + ".0.0/16"
		cp.AWG = &aw
		err = a.srvCall(dst, "POST", "inbounds", cp, &out)
	}
	if err != nil {
		return fail(err.Error())
	}
	res.OK, res.ID, res.Remark, res.Port = true, out.ID, out.Remark, out.Port
	res.Warning = strings.Join(warn, "; ")
	return res
}
