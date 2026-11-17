package app

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sagernet/sing-quic/tuic"
	"github.com/sagernet/sing/common/auth"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/logger"
	sM "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	aTLS "github.com/sagernet/sing/common/tls"
)

// TUIC v5 server built on sing-quic (the implementation used by sing-box).

type tuicRunner struct {
	a      *App
	key    string
	id     string
	pc     net.PacketConn
	svc    *tuic.Service[string]
	ctx    context.Context
	cancel context.CancelFunc
	ctr    *trafficCounter
	users  atomic.Pointer[map[string]embUser]
	actMu  sync.Mutex
	active map[[2]string]int // {name, ip} -> open streams
}

func (r *tuicRunner) track(name, ip string, d int) {
	r.actMu.Lock()
	defer r.actMu.Unlock()
	if r.active == nil {
		r.active = map[[2]string]int{}
	}
	k := [2]string{name, ip}
	r.active[k] += d
	if r.active[k] <= 0 {
		delete(r.active, k)
	}
}

func (a *App) startTUIC(s embSpec) (embRunner, error) {
	if s.CertFile == "" {
		return nil, errors.New("не выбран сертификат")
	}
	cl := &certLoader{certFile: s.CertFile, keyFile: s.KeyFile}
	if _, err := cl.load(); err != nil {
		return nil, errors.New("сертификат не загружается: " + err.Error())
	}
	alpn := s.In.ALPN
	if len(alpn) == 0 {
		alpn = []string{"h3"}
	}
	tc := &tls.Config{MinVersion: tls.VersionTLS13, NextProtos: alpn, GetCertificate: cl.get}
	pc, err := net.ListenPacket("udp", udpListenAddr(s.In.Listen, s.In.Port))
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(a.ctxOrBackground())
	r := &tuicRunner{a: a, key: s.coreKey(), id: s.In.ID, pc: pc, ctx: ctx, cancel: cancel, ctr: a.emb.counter(s.In.ID)}
	cc := s.In.TUICCongestion
	if cc == "" {
		cc = "bbr"
	}
	svc, err := tuic.NewService[string](tuic.ServiceOptions{
		Context:           ctx,
		Logger:            logger.NOP(),
		TLSConfig:         &stdServerTLS{cfg: tc},
		CongestionControl: cc,
		AuthTimeout:       5 * time.Second,
		ZeroRTTHandshake:  s.In.TUICZeroRTT,
		Heartbeat:         10 * time.Second,
		UDPTimeout:        2 * time.Minute,
		Handler:           r,
	})
	if err != nil {
		cancel()
		pc.Close()
		return nil, err
	}
	r.svc = svc
	r.setUsers(s.Users)
	if err := svc.Start(pc); err != nil {
		cancel()
		pc.Close()
		return nil, err
	}
	return r, nil
}

func (a *App) ctxOrBackground() context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}

func (r *tuicRunner) setUsers(list []embUser) {
	m := map[string]embUser{}
	names := []string{}
	ids := [][16]byte{}
	pass := []string{}
	for _, u := range list {
		id, err := parseUUID(u.UUID)
		if err != nil || u.Password == "" {
			continue
		}
		m[u.Name] = u
		names = append(names, u.Name)
		ids = append(ids, id)
		pass = append(pass, u.Password)
	}
	r.users.Store(&m)
	r.svc.UpdateUsers(names, ids, pass)
}

func (r *tuicRunner) Update(s embSpec) error { r.setUsers(s.Users); return nil }
func (r *tuicRunner) CoreKey() string        { return r.key }
func (r *tuicRunner) Online() map[string]string {
	r.actMu.Lock()
	defer r.actMu.Unlock()
	out := map[string]string{}
	for k := range r.active {
		out[k[0]] = k[1]
	}
	return out
}

func (r *tuicRunner) Close() {
	r.cancel()
	if r.svc != nil {
		_ = r.svc.Close()
	}
	_ = r.pc.Close()
}

func (r *tuicRunner) user(ctx context.Context, source sM.Socksaddr) (embUser, string, bool) {
	name, _ := auth.UserFromContext[string](ctx)
	m := r.users.Load()
	if m == nil {
		return embUser{}, "", false
	}
	u, ok := (*m)[name]
	if !ok {
		return u, "", false
	}
	ip := source.Addr.Unmap().String()
	if r.a.ipBlocked(ip) {
		return u, ip, false
	}
	r.a.emb.markOnline(name, ip)
	return u, ip, true
}

func socksDest(d sM.Socksaddr) bridgeDest {
	if d.IsFqdn() {
		return bridgeDest{Domain: d.Fqdn, Port: d.Port}
	}
	return bridgeDest{IP: d.Addr, Port: d.Port}
}

func (r *tuicRunner) NewConnectionEx(ctx context.Context, conn net.Conn, source sM.Socksaddr, destination sM.Socksaddr, onClose N.CloseHandlerFunc) {
	done := func(err error) {
		if onClose != nil {
			onClose(err)
		}
	}
	u, ip, ok := r.user(ctx, source)
	if !ok {
		conn.Close()
		done(errors.New("rejected"))
		return
	}
	r.track(u.Name, ip, 1)
	defer r.track(u.Name, ip, -1)
	bc, err := r.a.dialBridge(u.UUID, false, socksDest(destination), r.ctr)
	if err != nil {
		conn.Close()
		done(err)
		return
	}
	pipe(conn, bc)
	done(nil)
}

func (r *tuicRunner) NewPacketConnectionEx(ctx context.Context, conn N.PacketConn, source sM.Socksaddr, destination sM.Socksaddr, onClose N.CloseHandlerFunc) {
	defer func() {
		conn.Close()
		if onClose != nil {
			onClose(nil)
		}
	}()
	u, ip, ok := r.user(ctx, source)
	if !ok {
		return
	}
	r.track(u.Name, ip, 1)
	defer r.track(u.Name, ip, -1)
	var mu sync.Mutex
	sess := map[string]*bridgeConn{}
	defer func() {
		mu.Lock()
		for _, bc := range sess {
			bc.Close()
		}
		mu.Unlock()
	}()
	open := func(dst sM.Socksaddr) (*bridgeConn, error) {
		key := dst.String()
		mu.Lock()
		bc := sess[key]
		mu.Unlock()
		if bc != nil {
			return bc, nil
		}
		bc, err := r.a.dialBridge(u.UUID, true, socksDest(dst), r.ctr)
		if err != nil {
			return nil, err
		}
		mu.Lock()
		sess[key] = bc
		mu.Unlock()
		go func() {
			defer func() {
				bc.Close()
				mu.Lock()
				if sess[key] == bc {
					delete(sess, key)
				}
				mu.Unlock()
			}()
			p := make([]byte, 65535)
			for {
				_ = bc.SetReadDeadline(time.Now().Add(3 * time.Minute))
				n, err := bc.ReadPacket(p)
				if err != nil {
					return
				}
				if n == 0 {
					continue
				}
				out := buf.NewSize(n)
				_, _ = out.Write(p[:n])
				if err := conn.WritePacket(out, dst); err != nil {
					return
				}
			}
		}()
		return bc, nil
	}
	for {
		b := buf.NewPacket()
		dst, err := conn.ReadPacket(b)
		if err != nil {
			b.Release()
			return
		}
		if !dst.IsValid() {
			dst = destination
		}
		bc, err := open(dst)
		if err == nil {
			if werr := bc.WritePacket(b.Bytes()); werr != nil {
				bc.Close()
			}
		}
		b.Release()
	}
}

// stdServerTLS adapts a crypto/tls config to sing's TLS server interface.
type stdServerTLS struct{ cfg *tls.Config }

func (s *stdServerTLS) ServerName() string                   { return s.cfg.ServerName }
func (s *stdServerTLS) SetServerName(n string)               { s.cfg.ServerName = n }
func (s *stdServerTLS) NextProtos() []string                 { return s.cfg.NextProtos }
func (s *stdServerTLS) SetNextProtos(p []string)             { s.cfg.NextProtos = p }
func (s *stdServerTLS) HandshakeTimeout() time.Duration      { return 10 * time.Second }
func (s *stdServerTLS) SetHandshakeTimeout(time.Duration)    {}
func (s *stdServerTLS) STDConfig() (*aTLS.STDConfig, error)  { return s.cfg, nil }
func (s *stdServerTLS) Client(net.Conn) (aTLS.Conn, error)   { return nil, errors.New("server only") }
func (s *stdServerTLS) Clone() aTLS.Config                   { return &stdServerTLS{cfg: s.cfg.Clone()} }
func (s *stdServerTLS) Start() error                         { return nil }
func (s *stdServerTLS) Close() error                         { return nil }
func (s *stdServerTLS) Server(c net.Conn) (aTLS.Conn, error) { return tls.Server(c, s.cfg), nil }
