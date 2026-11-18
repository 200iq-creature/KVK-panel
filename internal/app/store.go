package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ---------- data model ----------

type Admin struct {
	Username       string `json:"username"`
	PassHash       string `json:"passHash"`
	SessionVersion int    `json:"sessionVersion"`
	TOTPSecret     string `json:"totpSecret,omitempty"`
	TOTPEnabled    bool   `json:"totpEnabled"`
	TOTPPending    string `json:"totpPending,omitempty"`
	TgEnabled      bool   `json:"tgEnabled"`
}

type Settings struct {
	PanelPort      int    `json:"panelPort"`
	PanelListen    string `json:"panelListen"`
	BasePath       string `json:"basePath"`
	SubPath        string `json:"subPath"`
	SubPort        int    `json:"subPort"`
	SessionSecret  string `json:"sessionSecret"`
	APIToken       string `json:"apiToken"`
	NodeName       string `json:"nodeName"`
	CountryCode    string `json:"countryCode"`
	PublicHost     string `json:"publicHost"`
	PanelCertID    string `json:"panelCertId"`
	DefaultFP      string `json:"defaultFp"`
	AutoUpdate     bool   `json:"autoUpdate"`
	XrayAPIPort    int    `json:"xrayApiPort"`
	BridgePort     int    `json:"bridgePort,omitempty"` // local VLESS inbound for TUIC / AmneziaWG
	TimeZone       string `json:"timeZone"`
	ShowTrafficAll bool   `json:"showTrafficAll"`
	NewResetPeriod string `json:"newResetPeriod"`
	NewResetDays   int    `json:"newResetDays"`
	AutoCleanDays  int    `json:"autoCleanDays"`
	// telegram (2FA + notifications)
	TgBotToken string `json:"tgBotToken"`
	TgAdminID  string `json:"tgAdminId"`
	TgNotify   bool   `json:"tgNotify"`
	// geo
	GeoSource     string `json:"geoSource"`
	GeoAuto       bool   `json:"geoAuto"`
	GeoWeekly     bool   `json:"geoWeekly"`
	GeoHour       int    `json:"geoHour"`
	GeoLastUpdate int64  `json:"geoLastUpdate"`
	GeoLastSource string `json:"geoLastSource"`
	// logs
	LogUntil  int64  `json:"logUntil"`
	LogLevel  string `json:"logLevel"`
	LogAccess bool   `json:"logAccess"`
	// misc
	LastUpdateCheck int64  `json:"lastUpdateCheck"`
	LatestVersion   string `json:"latestVersion"`
	XrayPrevVersion string `json:"xrayPrevVersion"`
	// backups
	AutoBackup     bool  `json:"autoBackup"`
	BackupKeep     int   `json:"backupKeep"`
	BackupTg       bool  `json:"backupTg"`
	LastAutoBackup int64 `json:"lastAutoBackup"`
	// limits
	DeviceBlockMinutes int `json:"deviceBlockMinutes"`
	// shop bot
	Shop ShopSettings `json:"shop"`
	// telegram proxy
	TgProxy TgProxySettings `json:"tgProxy"`
	// VLESS auto-setup: decoy website + Reality/CDN inbounds
	Fake FakeSiteSettings `json:"fake"`
}

// FakeSiteSettings describes the decoy website that fronts the auto-configured
// VLESS Reality inbound (Reality "steals" the TLS handshake from it) and the
// XHTTP inbound reachable through a CDN such as Cloudflare.
type FakeSiteSettings struct {
	Enabled    bool     `json:"enabled"`
	Domains    []string `json:"domains"`    // all decoy domains (Reality serverNames)
	SNI        string   `json:"sni"`        // main SNI used in Reality links
	CDNDomains []string `json:"cdnDomains"` // domains used for XHTTP-over-CDN links
	Port       int      `json:"port"`       // public port of the Reality inbound (443)
	TLSPort    int      `json:"tlsPort"`    // internal 127.0.0.1 HTTPS port of the decoy site
	XHTTPPath  string   `json:"xhttpPath"`  // secret path proxied to the XHTTP inbound
	WSPath     string   `json:"wsPath"`     // secret path proxied to the WebSocket inbound (sing-box/Clash clients)
	CDN        bool     `json:"cdn"`        // XHTTP-over-CDN profile enabled
	CustomHTML string   `json:"customHtml"` // optional own index.html
	Email      string   `json:"email"`
	CFToken    string   `json:"cfToken"`
	SetupAt    int64    `json:"setupAt"`
}

type SubSettings struct {
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	NameFormat     string   `json:"nameFormat"`
	AutoSelect     bool     `json:"autoSelect"`
	AutoName       string   `json:"autoName"`
	AutoHosts      []string `json:"autoHosts"`
	HappRoutingRU  bool     `json:"happRoutingRu"`
	UpdateHours    int      `json:"updateHours"`
	RenewRemind    bool     `json:"renewRemind"`
	RenewURL       string   `json:"renewUrl"`
	Fragment       bool     `json:"fragment"`
	FragPackets    string   `json:"fragPackets"`
	FragLength     string   `json:"fragLength"`
	FragInterval   string   `json:"fragInterval"`
	JSONTemplate   string   `json:"jsonTemplate"`
	PageView       string   `json:"pageView"` // simple | classic
	PageTitle      string   `json:"pageTitle"`
	PageText       string   `json:"pageText"`
	PageSupportURL string   `json:"pageSupportUrl"`
	PageLogoURL    string   `json:"pageLogoUrl"`
	ExternalLinks  string   `json:"externalLinks"`
}

type FilterSettings struct {
	Enabled   bool     `json:"enabled"`
	Ads       bool     `json:"ads"`
	Security  bool     `json:"security"`
	Adult     bool     `json:"adult"`
	Mode      string   `json:"mode"` // dns | lists | custom
	CustomDoH string   `json:"customDoh"`
	Block     []string `json:"block"`
	Allow     []string `json:"allow"`
}

type WarpState struct {
	ID         string   `json:"id"`
	Token      string   `json:"token"`
	PrivateKey string   `json:"privateKey"`
	PeerPublic string   `json:"peerPublic"`
	Address4   string   `json:"address4"`
	Address6   string   `json:"address6"`
	Reserved   []int    `json:"reserved"`
	License    string   `json:"license"`
	Type       string   `json:"type"`
	Enabled    bool     `json:"enabled"`
	Mode       string   `json:"mode"` // all | sites | fallback
	Sites      []string `json:"sites"`
	Port       int      `json:"port"`
	IPv6First  bool     `json:"ipv6First"`
	FallbackOn bool     `json:"fallbackOn"`
	CreatedAt  int64    `json:"createdAt"`
}

type Inbound struct {
	ID       string `json:"id"`
	Tag      string `json:"tag"`
	Remark   string `json:"remark"`
	Enabled  bool   `json:"enabled"`
	Protocol string `json:"protocol"` // vless vmess trojan shadowsocks hysteria2 tuic amneziawg masque
	Listen   string `json:"listen"`
	Port     int    `json:"port"`
	Network  string `json:"network"`  // tcp ws grpc xhttp httpupgrade
	Security string `json:"security"` // none tls reality
	Flow     string `json:"flow"`
	// reality
	RealityTarget string   `json:"realityTarget"`
	RealitySNI    []string `json:"realitySni"`
	PrivateKey    string   `json:"privateKey"`
	PublicKey     string   `json:"publicKey"`
	ShortIDs      []string `json:"shortIds"`
	SpiderX       string   `json:"spiderX"`
	Fingerprint   string   `json:"fingerprint"`
	// tls
	CertID        string   `json:"certId"`
	TLSServerName string   `json:"tlsServerName"`
	ALPN          []string `json:"alpn"`
	AllowInsecure bool     `json:"allowInsecure"`
	// transport
	Path        string `json:"path"`
	Host        string `json:"host"`
	ServiceName string `json:"serviceName"`
	XHTTPMode   string `json:"xhttpMode"`
	HeaderType  string `json:"headerType"`
	// shadowsocks
	SSMethod   string `json:"ssMethod"`
	SSPassword string `json:"ssPassword"`
	// hysteria2
	ObfsPassword string `json:"obfsPassword"`
	// tuic (served by the panel itself)
	TUICCongestion string `json:"tuicCongestion,omitempty"` // bbr cubic new_reno
	TUICUDPMode    string `json:"tuicUdpMode,omitempty"`    // native quic
	TUICZeroRTT    bool   `json:"tuicZeroRtt,omitempty"`
	// masque (xray): tunnel address pool and MTU (MTU is also used by AmneziaWG)
	MasquePool []string `json:"masquePool,omitempty"`
	MTU        int      `json:"mtu,omitempty"`
	// amneziawg (served by the panel itself)
	AWG *AWGSettings `json:"awg,omitempty"`
	// links
	ExternalHost string `json:"externalHost"`
	ExternalPort int    `json:"externalPort"`
	// Front = "fake": the inbound listens on localhost behind the decoy website,
	// links use TLS with FrontHosts as address/SNI (one link per host).
	Front      string   `json:"front,omitempty"`
	FrontHosts []string `json:"frontHosts,omitempty"`
	Auto       string   `json:"auto,omitempty"` // reality | cdn — created by auto-setup
	Sniffing   bool     `json:"sniffing"`
	Up         int64    `json:"up"`
	Down       int64    `json:"down"`
	CreatedAt  int64    `json:"createdAt"`
}

// AWGSettings describes an AmneziaWG server. Client keys and addresses live in
// Client.WGKey / Client.WGIndex so they stay the same on every linked server.
type AWGSettings struct {
	PrivateKey string   `json:"privateKey"` // base64
	PublicKey  string   `json:"publicKey"`
	Network    string   `json:"network"` // e.g. 10.66.0.0/16, the server takes .1
	DNS        []string `json:"dns"`
	Jc         int      `json:"jc"`
	Jmin       int      `json:"jmin"`
	Jmax       int      `json:"jmax"`
	S1         int      `json:"s1"`
	S2         int      `json:"s2"`
	S3         int      `json:"s3,omitempty"`
	S4         int      `json:"s4,omitempty"`
	H1         string   `json:"h1"`
	H2         string   `json:"h2"`
	H3         string   `json:"h3"`
	H4         string   `json:"h4"`
	I1         string   `json:"i1,omitempty"`
	I2         string   `json:"i2,omitempty"`
	I3         string   `json:"i3,omitempty"`
	I4         string   `json:"i4,omitempty"`
	I5         string   `json:"i5,omitempty"`
}

type Client struct {
	ID           string   `json:"id"`
	UUID         string   `json:"uuid"`
	Name         string   `json:"name"`
	SubID        string   `json:"subId"`
	Password     string   `json:"password"`
	Telegram     string   `json:"telegram"`
	Note         string   `json:"note"`
	Tags         []string `json:"tags"`
	Inbounds     []string `json:"inbounds"`
	ExpiresAt    int64    `json:"expiresAt"`
	TrafficLimit int64    `json:"trafficLimit"`
	Up           int64    `json:"up"`
	Down         int64    `json:"down"`
	TotalUp      int64    `json:"totalUp"`
	TotalDown    int64    `json:"totalDown"`
	DeviceLimit  int      `json:"deviceLimit"`
	Banned       bool     `json:"banned"`
	Frozen       bool     `json:"frozen"`
	FrozenAt     int64    `json:"frozenAt"`
	ResetPeriod  string   `json:"resetPeriod"`
	ResetDays    int      `json:"resetDays"`
	LastReset    int64    `json:"lastReset"`
	CreatedAt    int64    `json:"createdAt"`
	LastOnline   int64    `json:"lastOnline"`
	LastSubAt    int64    `json:"lastSubAt"`
	LastSubUA    string   `json:"lastSubUa"`
	LastSubIP    string   `json:"lastSubIp"`
	Origin       string   `json:"origin,omitempty"`
	// AmneziaWG credentials: private key (base64) and host number inside AWG networks
	WGKey   string `json:"wgKey,omitempty"`
	WGIndex int    `json:"wgIndex,omitempty"`
	// traffic already reported to main server (when this server is a node)
	ReportedUp   int64 `json:"reportedUp,omitempty"`
	ReportedDown int64 `json:"reportedDown,omitempty"`
}

type NodeInbound struct {
	ID             string       `json:"id"`
	Tag            string       `json:"tag"`
	Remark         string       `json:"remark"`
	Protocol       string       `json:"protocol"`
	Host           string       `json:"host"`
	Port           int          `json:"port"`
	Network        string       `json:"network"`
	Security       string       `json:"security"`
	Flow           string       `json:"flow"`
	SNI            string       `json:"sni"`
	PublicKey      string       `json:"publicKey"`
	ShortID        string       `json:"shortId"`
	SpiderX        string       `json:"spiderX"`
	FP             string       `json:"fp"`
	ALPN           []string     `json:"alpn"`
	Path           string       `json:"path"`
	HostHdr        string       `json:"hostHdr"`
	ServiceName    string       `json:"serviceName"`
	XHTTPMode      string       `json:"xhttpMode"`
	HeaderType     string       `json:"headerType"`
	SSMethod       string       `json:"ssMethod"`
	SSPassword     string       `json:"ssPassword"`
	ObfsPassword   string       `json:"obfsPassword"`
	AllowInsecure  bool         `json:"allowInsecure"`
	CertSha256     string       `json:"certSha256,omitempty"`
	Front          bool         `json:"front,omitempty"` // reached through the decoy site / CDN
	TUICCongestion string       `json:"tuicCongestion,omitempty"`
	TUICUDPMode    string       `json:"tuicUdpMode,omitempty"`
	TUICZeroRTT    bool         `json:"tuicZeroRtt,omitempty"`
	MTU            int          `json:"mtu,omitempty"`
	AWG            *AWGSettings `json:"awg,omitempty"` // PrivateKey is always blank here
}

type Node struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	URL       string        `json:"url"`
	Token     string        `json:"token"`
	Enabled   bool          `json:"enabled"`
	Country   string        `json:"country"`
	Version   string        `json:"version"`
	Clients   int           `json:"clients"`
	CPU       float64       `json:"cpu"`
	PingMs    int           `json:"pingMs"`
	LastSync  int64         `json:"lastSync"`
	LastError string        `json:"lastError"`
	Inbounds  []NodeInbound `json:"inbounds"`
	AddedAt   int64         `json:"addedAt"`
	// Granted is set once the node's inbounds were handed out to the clients
	// (on the first sync); after that access is chosen per client.
	Granted bool `json:"granted,omitempty"`
	// Xray is the core version reported by the node
	Xray string `json:"xray,omitempty"`
}

// InboundIDs returns the unique (main-side) inbound ids of the node.
func (n *Node) InboundIDs() []string {
	out := []string{}
	for _, ep := range n.Inbounds {
		if !containsStr(out, ep.ID) {
			out = append(out, ep.ID)
		}
	}
	return out
}

type MainLink struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	LastSync int64  `json:"lastSync"`
}

type Domain struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`   // domain | ip
	Mode      string `json:"mode"`   // letsencrypt | custom | selfsigned
	Status    string `json:"status"` // pending | active | error
	Error     string `json:"error"`
	CertFile  string `json:"certFile"`
	KeyFile   string `json:"keyFile"`
	IssuedAt  int64  `json:"issuedAt"`
	ExpiresAt int64  `json:"expiresAt"`
	AutoRenew bool   `json:"autoRenew"`
	Email     string `json:"email"`
	CreatedAt int64  `json:"createdAt"`
	LastTry   int64  `json:"lastTry"`
}

type Tariff struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"` // vpn | proxy
	Days      int    `json:"days"`
	TrafficGB int    `json:"trafficGb"`
	Devices   int    `json:"devices"`
	Price     int    `json:"price"` // in roubles
	Stars     int    `json:"stars"`
	Enabled   bool   `json:"enabled"`
}

type ShopSettings struct {
	Enabled       bool     `json:"enabled"`
	BotToken      string   `json:"botToken"`
	BotUsername   string   `json:"botUsername"`
	Welcome       string   `json:"welcome"`
	Support       string   `json:"support"`
	Tariffs       []Tariff `json:"tariffs"`
	TrialDays     int      `json:"trialDays"`
	TrialGB       int      `json:"trialGb"`
	PayStars      bool     `json:"payStars"`
	ProviderToken string   `json:"providerToken"`
	CryptoToken   string   `json:"cryptoToken"`
	PayManual     bool     `json:"payManual"`
	ManualText    string   `json:"manualText"`
	Inbounds      []string `json:"inbounds"`
	Remind        bool     `json:"remind"`
}

type TgProxySettings struct {
	MTEnabled    bool   `json:"mtEnabled"`
	MTPort       int    `json:"mtPort"`
	MTSecret     string `json:"mtSecret"`
	MTDomain     string `json:"mtDomain"`
	SocksEnabled bool   `json:"socksEnabled"`
	SocksPort    int    `json:"socksPort"`
	SocksUDP     bool   `json:"socksUdp"`
}

type ShopUser struct {
	TgID       int64  `json:"tgId"`
	Username   string `json:"username"`
	FirstName  string `json:"firstName"`
	ClientID   string `json:"clientId"`
	ProxyID    string `json:"proxyId"`
	TrialUsed  bool   `json:"trialUsed"`
	Promo      string `json:"promo"`
	JoinedAt   int64  `json:"joinedAt"`
	LastSeen   int64  `json:"lastSeen"`
	Blocked    bool   `json:"blocked"`
	State      string `json:"state,omitempty"`
	RemindedAt int64  `json:"remindedAt,omitempty"`
}

type Order struct {
	ID        string `json:"id"`
	TgID      int64  `json:"tgId"`
	User      string `json:"user"`
	TariffID  string `json:"tariffId"`
	Tariff    string `json:"tariff"`
	Amount    int    `json:"amount"`
	Currency  string `json:"currency"` // RUB | XTR
	Method    string `json:"method"`   // stars | card | crypto | manual | panel
	Status    string `json:"status"`   // pending | review | paid | rejected | canceled
	Promo     string `json:"promo,omitempty"`
	ExtID     string `json:"extId,omitempty"`
	PayURL    string `json:"payUrl,omitempty"`
	ClientID  string `json:"clientId,omitempty"`
	CreatedAt int64  `json:"createdAt"`
	PaidAt    int64  `json:"paidAt,omitempty"`
}

type Promo struct {
	Code      string `json:"code"`
	Percent   int    `json:"percent"`
	Days      int    `json:"days"`
	MaxUses   int    `json:"maxUses"`
	Used      int    `json:"used"`
	ExpiresAt int64  `json:"expiresAt"`
	Enabled   bool   `json:"enabled"`
	CreatedAt int64  `json:"createdAt"`
}

type ProxyUser struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	ExpiresAt int64  `json:"expiresAt"`
	Enabled   bool   `json:"enabled"`
	TgID      int64  `json:"tgId,omitempty"`
	Note      string `json:"note"`
	CreatedAt int64  `json:"createdAt"`
}

func (p *ProxyUser) Active(now int64) bool {
	return p.Enabled && (p.ExpiresAt == 0 || p.ExpiresAt > now)
}

type Event struct {
	T    int64  `json:"t"`
	Kind string `json:"kind"` // ok info warn bad
	Text string `json:"text"`
	Ref  string `json:"ref,omitempty"`
}

type LoginEntry struct {
	T      int64  `json:"t"`
	OK     bool   `json:"ok"`
	User   string `json:"user"`
	Method string `json:"method"`
	IP     string `json:"ip"`
	UA     string `json:"ua"`
	Reason string `json:"reason,omitempty"`
}

type Data struct {
	Version    int              `json:"version"`
	Admin      Admin            `json:"admin"`
	Settings   Settings         `json:"settings"`
	Sub        SubSettings      `json:"sub"`
	Filter     FilterSettings   `json:"filter"`
	Warp       WarpState        `json:"warp"`
	Inbounds   []*Inbound       `json:"inbounds"`
	Clients    []*Client        `json:"clients"`
	Nodes      []*Node          `json:"nodes"`
	Main       *MainLink        `json:"main,omitempty"`
	Domains    []*Domain        `json:"domains"`
	Events     []Event          `json:"events"`
	Logins     []LoginEntry     `json:"logins"`
	Daily      map[string]int64 `json:"daily"`
	ACMEKey    string           `json:"acmeKey,omitempty"`
	ShopUsers  []*ShopUser      `json:"shopUsers"`
	Orders     []*Order         `json:"orders"`
	Promos     []*Promo         `json:"promos"`
	ProxyUsers []*ProxyUser     `json:"proxyUsers"`
	ACMEKid    string           `json:"acmeKid,omitempty"`
}

// ---------- store ----------

type Store struct {
	mu    sync.RWMutex
	path  string
	d     *Data
	dirty bool
}

func NewStore(path string) (*Store, error) {
	s := &Store{path: path}
	b, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		s.d = &Data{}
	} else {
		var d Data
		if err := json.Unmarshal(b, &d); err != nil {
			return nil, err
		}
		s.d = &d
	}
	s.fillDefaults()
	return s, s.saveLocked()
}

func (s *Store) fillDefaults() {
	d := s.d
	if d.Version == 0 {
		d.Version = 1
	}
	st := &d.Settings
	if st.PanelPort == 0 {
		st.PanelPort = 2053
	}
	if st.BasePath == "" {
		st.BasePath = RandomString(12)
	}
	if st.SubPath == "" {
		st.SubPath = "sub"
	}
	if st.SessionSecret == "" {
		st.SessionSecret = RandomHex(32)
	}
	if st.APIToken == "" {
		st.APIToken = RandomHex(32)
	}
	if st.NodeName == "" {
		st.NodeName = "KVK"
	}
	if st.DefaultFP == "" {
		st.DefaultFP = "chrome"
	}
	if st.XrayAPIPort == 0 {
		st.XrayAPIPort = 62789
	}
	if st.TimeZone == "" {
		st.TimeZone = "Europe/Moscow"
	}
	if st.GeoSource == "" {
		st.GeoSource = "loyalsoldier"
	}
	if st.GeoHour == 0 {
		st.GeoHour = 4
	}
	if st.LogLevel == "" {
		st.LogLevel = "warning"
	}
	if d.Admin.Username == "" {
		d.Admin.Username = "admin"
		d.Admin.PassHash = HashPassword("admin")
	}
	if d.Sub.NameFormat == "" {
		d.Sub.NameFormat = "{name} · {label}"
	}
	if d.Sub.FragPackets == "" {
		d.Sub.FragPackets = "tlshello"
	}
	if d.Sub.FragLength == "" {
		d.Sub.FragLength = "100-200"
	}
	if d.Sub.FragInterval == "" {
		d.Sub.FragInterval = "10-20"
	}
	if d.Sub.PageView == "" {
		d.Sub.PageView = "simple"
	}
	if d.Filter.Mode == "" {
		d.Filter.Mode = "dns"
		d.Filter.Ads = true
	}
	if d.Warp.Mode == "" {
		d.Warp.Mode = "all"
	}
	if d.Warp.Port == 0 {
		d.Warp.Port = 2408
	}
	if st.Shop.Welcome == "" {
		st.Shop.Welcome = "Привет! Здесь можно купить быстрый VPN без ограничений. Выберите действие ниже."
	}
	if st.Shop.Tariffs == nil {
		st.Shop.Tariffs = []Tariff{
			{ID: RandomHex(4), Name: "1 месяц", Kind: "vpn", Days: 30, Devices: 3, Price: 199, Stars: 120, Enabled: true},
			{ID: RandomHex(4), Name: "3 месяца", Kind: "vpn", Days: 90, Devices: 3, Price: 499, Stars: 300, Enabled: true},
		}
	}
	if st.TgProxy.MTPort == 0 {
		st.TgProxy.MTPort = 8443
	}
	if st.TgProxy.SocksPort == 0 {
		st.TgProxy.SocksPort = 1080
	}
	if st.TgProxy.MTDomain == "" {
		st.TgProxy.MTDomain = "www.google.com"
	}
	if st.BackupKeep == 0 {
		st.BackupKeep = 7
	}
	if d.Daily == nil {
		d.Daily = map[string]int64{}
	}
	for _, c := range d.Clients {
		if c.SubID == "" {
			c.SubID = RandomHex(8)
		}
		if c.Password == "" {
			c.Password = RandomString(16)
		}
		if c.Tags == nil {
			c.Tags = []string{}
		}
		if c.Inbounds == nil {
			c.Inbounds = []string{}
		}
	}
}

// Read runs fn under a read lock.
func (s *Store) Read(fn func(d *Data)) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	fn(s.d)
}

// Update runs fn under a write lock and persists the result when fn returns nil.
func (s *Store) Update(fn func(d *Data) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := fn(s.d); err != nil {
		return err
	}
	ensureWGCreds(s.d) // new clients get AmneziaWG credentials right away
	return s.saveLocked()
}

// UpdateLazy mutates without immediate fsync (used by frequent stats updates).
func (s *Store) UpdateLazy(fn func(d *Data)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(s.d)
	s.dirty = true
}

func (s *Store) Flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dirty {
		_ = s.saveLocked()
	}
}

func (s *Store) Replace(d *Data) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.d = d
	s.fillDefaults()
	ensureWGCreds(s.d)
	return s.saveLocked()
}

func (s *Store) Snapshot() []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, _ := json.MarshalIndent(s.d, "", "  ")
	return b
}

func (s *Store) saveLocked() error {
	b, err := json.MarshalIndent(s.d, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}
	s.dirty = false
	return nil
}

// ---------- helpers on Data ----------

func (d *Data) AddEvent(kind, text, ref string) {
	d.Events = append(d.Events, Event{T: time.Now().UnixMilli(), Kind: kind, Text: text, Ref: ref})
	if len(d.Events) > 500 {
		d.Events = d.Events[len(d.Events)-500:]
	}
}

func (d *Data) AddLogin(e LoginEntry) {
	d.Logins = append(d.Logins, e)
	if len(d.Logins) > 300 {
		d.Logins = d.Logins[len(d.Logins)-300:]
	}
}

func (d *Data) ClientByID(id string) *Client {
	for _, c := range d.Clients {
		if c.ID == id {
			return c
		}
	}
	return nil
}

func (d *Data) ClientByName(name string) *Client {
	for _, c := range d.Clients {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func (d *Data) ClientBySub(sub string) *Client {
	for _, c := range d.Clients {
		if c.SubID == sub || c.UUID == sub {
			return c
		}
	}
	return nil
}

func (d *Data) InboundByID(id string) *Inbound {
	for _, in := range d.Inbounds {
		if in.ID == id {
			return in
		}
	}
	return nil
}

// NodeInboundByID resolves a linked-server inbound id ("<nodeID>:<inboundID>").
func (d *Data) NodeInboundByID(id string) (*Node, *NodeInbound) {
	nid, _, found := strings.Cut(id, ":")
	if !found {
		return nil, nil
	}
	n := d.NodeByID(nid)
	if n == nil {
		return nil, nil
	}
	for i := range n.Inbounds {
		if n.Inbounds[i].ID == id {
			return n, &n.Inbounds[i]
		}
	}
	return n, nil
}

// ValidInboundID reports whether id is a local inbound or an inbound of a linked server.
func (d *Data) ValidInboundID(id string) bool {
	if d.InboundByID(id) != nil {
		return true
	}
	_, ep := d.NodeInboundByID(id)
	return ep != nil
}

// DefaultGrant lists what a new client gets by default: every enabled local
// inbound plus every inbound of the enabled linked servers.
func (d *Data) DefaultGrant() []string {
	out := []string{}
	for _, in := range d.Inbounds {
		if in.Enabled {
			out = append(out, in.ID)
		}
	}
	for _, n := range d.Nodes {
		if n.Enabled {
			out = append(out, n.InboundIDs()...)
		}
	}
	return out
}

// dropNodeInbounds removes grants to inbounds of node nid that are not in keep (nil = all).
func (d *Data) dropNodeInbounds(nid string, keep []string) {
	prefix := nid + ":"
	for _, c := range d.Clients {
		kept := c.Inbounds[:0]
		for _, id := range c.Inbounds {
			if strings.HasPrefix(id, prefix) && !containsStr(keep, id) {
				continue
			}
			kept = append(kept, id)
		}
		c.Inbounds = kept
	}
}

func (d *Data) InboundByTag(tag string) *Inbound {
	for _, in := range d.Inbounds {
		if in.Tag == tag {
			return in
		}
	}
	return nil
}

func (d *Data) DomainByID(id string) *Domain {
	for _, x := range d.Domains {
		if x.ID == id {
			return x
		}
	}
	return nil
}

func (d *Data) NodeByID(id string) *Node {
	for _, n := range d.Nodes {
		if n.ID == id {
			return n
		}
	}
	return nil
}

func (d *Data) SortClients() {
	sort.SliceStable(d.Clients, func(i, j int) bool { return d.Clients[i].CreatedAt < d.Clients[j].CreatedAt })
}

// ---------- client status ----------

const (
	StActive  = "active"
	StExpired = "expired"
	StBanned  = "banned"
	StFrozen  = "frozen"
	StLimited = "limited"
)

func (c *Client) Status(now int64) string {
	switch {
	case c.Banned:
		return StBanned
	case c.Frozen:
		return StFrozen
	case c.ExpiresAt > 0 && now >= c.ExpiresAt:
		return StExpired
	case c.TrafficLimit > 0 && c.Up+c.Down >= c.TrafficLimit:
		return StLimited
	}
	return StActive
}

func (c *Client) HasInbound(id string) bool {
	for _, x := range c.Inbounds {
		if x == id {
			return true
		}
	}
	return false
}

func (c *Client) Used() int64 { return c.Up + c.Down }
