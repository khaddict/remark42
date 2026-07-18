package cmd

import (
	"context"
	"embed"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	log "github.com/go-pkgz/lgr"
	ntf "github.com/go-pkgz/notify"
	"github.com/golang-jwt/jwt/v5"
	"github.com/kyokomi/emoji/v2"
	bolt "go.etcd.io/bbolt"

	"github.com/go-pkgz/auth/v2"
	"github.com/go-pkgz/auth/v2/avatar"
	"github.com/go-pkgz/auth/v2/token"
	cache "github.com/go-pkgz/lcw/v2"

	"github.com/umputun/remark42/backend/app/migrator"
	"github.com/umputun/remark42/backend/app/notify"
	"github.com/umputun/remark42/backend/app/rest/api"
	"github.com/umputun/remark42/backend/app/rest/proxy"
	"github.com/umputun/remark42/backend/app/safehttp"
	"github.com/umputun/remark42/backend/app/store"
	"github.com/umputun/remark42/backend/app/store/admin"
	"github.com/umputun/remark42/backend/app/store/engine"
	"github.com/umputun/remark42/backend/app/store/image"
	"github.com/umputun/remark42/backend/app/store/service"
)

//go:embed web
var webFS embed.FS

// ServerCommand with command line flags and env
type ServerCommand struct {
	Store      StoreGroup      `group:"store" namespace:"store" env-namespace:"STORE"`
	Avatar     AvatarGroup     `group:"avatar" namespace:"avatar" env-namespace:"AVATAR"`
	Cache      CacheGroup      `group:"cache" namespace:"cache" env-namespace:"CACHE"`
	Admin      AdminGroup      `group:"admin" namespace:"admin" env-namespace:"ADMIN"`
	Notify     NotifyGroup     `group:"notify" namespace:"notify" env-namespace:"NOTIFY"`
	SMTP       SMTPGroup       `group:"smtp" namespace:"smtp" env-namespace:"SMTP"`
	Image      ImageGroup      `group:"image" namespace:"image" env-namespace:"IMAGE"`
	ImageProxy ImageProxyGroup `group:"image-proxy" namespace:"image-proxy" env-namespace:"IMAGE_PROXY"`

	Sites                      []string      `long:"site" env:"SITE" default:"remark" description:"site names" env-delim:","`
	AnonymousVote              bool          `long:"anon-vote" env:"ANON_VOTE" description:"enable anonymous votes (works only with VOTES_IP enabled)"`
	AdminPasswd                string        `long:"admin-passwd" env:"ADMIN_PASSWD" default:"" description:"admin basic auth password"`
	BackupLocation             string        `long:"backup" env:"BACKUP_PATH" default:"./var/backup" description:"backups location"`
	MaxBackupFiles             int           `long:"max-back" env:"MAX_BACKUP_FILES" default:"10" description:"max backups to keep"`
	LegacyImageProxy           bool          `long:"img-proxy" env:"IMG_PROXY" description:"[deprecated, use image-proxy.http2https] enable image proxy"`
	MinCommentSize             int           `long:"min-comment" env:"MIN_COMMENT_SIZE" default:"0" description:"min comment size"`
	MaxCommentSize             int           `long:"max-comment" env:"MAX_COMMENT_SIZE" default:"2048" description:"max comment size"`
	MaxVotes                   int           `long:"max-votes" env:"MAX_VOTES" default:"-1" description:"maximum number of votes per comment"`
	RestrictVoteIP             bool          `long:"votes-ip" env:"VOTES_IP" description:"restrict votes from the same ip"`
	DurationVoteIP             time.Duration `long:"votes-ip-time" env:"VOTES_IP_TIME" default:"5m" description:"same ip vote duration"`
	LowScore                   int           `long:"low-score" env:"LOW_SCORE" default:"-5" description:"low score threshold"`
	CriticalScore              int           `long:"critical-score" env:"CRITICAL_SCORE" default:"-10" description:"critical score threshold"`
	PositiveScore              bool          `long:"positive-score" env:"POSITIVE_SCORE" description:"enable positive score only"`
	ReadOnlyAge                int           `long:"read-age" env:"READONLY_AGE" default:"0" description:"read-only age of comments, days"`
	EditDuration               time.Duration `long:"edit-time" env:"EDIT_TIME" default:"5m" description:"edit window; set to 0 to disable comment editing and staged image cleanup"`
	AdminEdit                  bool          `long:"admin-edit" env:"ADMIN_EDIT" description:"unlimited edit for admins"`
	Port                       int           `long:"port" env:"REMARK_PORT" default:"8080" description:"port"`
	Address                    string        `long:"address" env:"REMARK_ADDRESS" default:"" description:"listening address"`
	WebRoot                    string        `long:"web-root" env:"REMARK_WEB_ROOT" default:"./web" description:"web root directory"`
	UpdateLimit                float64       `long:"update-limit" env:"UPDATE_LIMIT" default:"0.5" description:"updates/sec limit"`
	TrustedProxies             []string      `long:"trusted-proxy" env:"TRUSTED_PROXY" description:"reverse-proxy networks (CIDR or IP) trusted to set the client IP; if unset, trusted from any client (see docs)" env-delim:","`
	RestrictedWords            []string      `long:"restricted-words" env:"RESTRICTED_WORDS" description:"words prohibited to use in comments" env-delim:","`
	RestrictedNames            []string      `long:"restricted-names" env:"RESTRICTED_NAMES" description:"names prohibited to use by user" env-delim:","`
	EnableEmoji                bool          `long:"emoji" env:"EMOJI" description:"enable emoji"`
	SimpleView                 bool          `long:"simple-view" env:"SIMPLE_VIEW" description:"minimal comment editor mode"`
	ProxyCORS                  bool          `long:"proxy-cors" env:"PROXY_CORS" description:"disable internal CORS and delegate it to proxy"`
	AllowedHosts               []string      `long:"allowed-hosts" env:"ALLOWED_HOSTS" description:"limit hosts/sources allowed to embed comments via CSP 'frame-ancestors'" env-delim:","`
	SubscribersOnly            bool          `long:"subscribers-only" env:"SUBSCRIBERS_ONLY" description:"enable commenting only for Patreon subscribers"`
	DisableSignature           bool          `long:"disable-signature" env:"DISABLE_SIGNATURE" description:"disable server signature in headers"`
	DisableFancyTextFormatting bool          `long:"disable-fancy-text-formatting" env:"DISABLE_FANCY_TEXT_FORMATTING" description:"disable fancy comments text formatting (replacement of quotes, dashes, fractions, etc)"`

	Auth struct {
		TTL struct {
			JWT    time.Duration `long:"jwt" env:"JWT" default:"5m" description:"JWT TTL"`
			Cookie time.Duration `long:"cookie" env:"COOKIE" default:"200h" description:"auth cookie TTL"`
		} `group:"ttl" namespace:"ttl" env-namespace:"TTL"`

		SendJWTHeader bool   `long:"send-jwt-header" env:"SEND_JWT_HEADER" description:"send JWT as a header instead of server-set cookie; with this enabled, frontend stores the JWT in a client-side cookie (note: increases vulnerability to XSS attacks)"`
		SameSite      string `long:"same-site" env:"SAME_SITE" description:"set same site policy for cookies" choice:"default" choice:"none" choice:"lax" choice:"strict" default:"default"` // nolint

		Github AuthGroup `group:"github" namespace:"github" env-namespace:"GITHUB" description:"Github OAuth"`
	} `group:"auth" namespace:"auth" env-namespace:"AUTH"`

	CommonOpts

	emailMsgTemplatePath          string // used only in tests
	emailVerificationTemplatePath string // used only in tests
}

// ImageProxyGroup defines options group for image proxy
type ImageProxyGroup struct {
	HTTP2HTTPS    bool `long:"http2https" env:"HTTP2HTTPS" description:"enable HTTP->HTTPS proxy"`
	CacheExternal bool `long:"cache-external" env:"CACHE_EXTERNAL" description:"enable caching for external images"`
}

// AuthGroup defines options group for auth params
type AuthGroup struct {
	CID  string `long:"cid" env:"CID" description:"OAuth client ID"`
	CSEC string `long:"csec" env:"CSEC" description:"OAuth client secret"`
}

// StoreGroup defines options group for store params
type StoreGroup struct {
	Type string `long:"type" env:"TYPE" description:"type of storage" choice:"bolt" default:"bolt"` // nolint
	Bolt struct {
		Path    string        `long:"path" env:"PATH" default:"./var" description:"parent directory for the bolt files"`
		Timeout time.Duration `long:"timeout" env:"TIMEOUT" default:"30s" description:"bolt timeout"`
	} `group:"bolt" namespace:"bolt" env-namespace:"BOLT"`
}

// ImageGroup defines options group for store pictures
type ImageGroup struct {
	Type string `long:"type" env:"TYPE" description:"type of storage" choice:"fs" choice:"bolt" default:"fs"` // nolint
	FS   struct {
		Path       string `long:"path" env:"PATH" default:"./var/pictures" description:"images location"`
		Staging    string `long:"staging" env:"STAGING" default:"./var/pictures.staging" description:"staging location"`
		Partitions int    `long:"partitions" env:"PARTITIONS" default:"100" description:"partitions (subdirs)"`
	} `group:"fs" namespace:"fs" env-namespace:"FS"`
	Bolt struct {
		File string `long:"file" env:"FILE" default:"./var/pictures.db" description:"images bolt file location"`
	} `group:"bolt" namespace:"bolt" env-namespace:"BOLT"`
	MaxSize      int `long:"max-size" env:"MAX_SIZE" default:"5000000" description:"max size of image file"`
	ResizeWidth  int `long:"resize-width" env:"RESIZE_WIDTH" default:"2400" description:"width of a resized image"`
	ResizeHeight int `long:"resize-height" env:"RESIZE_HEIGHT" default:"900" description:"height of a resized image"`
}

// AvatarGroup defines options group for avatar params
type AvatarGroup struct {
	Type string `long:"type" env:"TYPE" description:"type of avatar storage" choice:"fs" choice:"uri" default:"fs"` //nolint
	FS   struct {
		Path string `long:"path" env:"PATH" default:"./var/avatars" description:"avatars location"`
	} `group:"fs" namespace:"fs" env-namespace:"FS"`
	URI    string `long:"uri" env:"URI" default:"./var/avatars" description:"avatars store URI"`
	RszLmt int    `long:"rsz-lmt" env:"RESIZE" default:"0" description:"max image size for resizing avatars on save"`
}

// CacheGroup defines options group for cache params
type CacheGroup struct {
	Type string `long:"type" env:"TYPE" description:"type of cache" choice:"mem" choice:"none" default:"mem"` // nolint
	Max  struct {
		Items int   `long:"items" env:"ITEMS" default:"1000" description:"max cached items"`
		Value int   `long:"value" env:"VALUE" default:"65536" description:"max size of the cached value"`
		Size  int64 `long:"size" env:"SIZE" default:"50000000" description:"max size of total cache"`
	} `group:"max" namespace:"max" env-namespace:"MAX"`
}

// AdminGroup defines options group for admin params
type AdminGroup struct {
	Type   string `long:"type" env:"TYPE" description:"type of admin store" choice:"shared" default:"shared"` //nolint
	Shared struct {
		Admins []string `long:"id" env:"ID" description:"admin(s) ids" env-delim:","`
		Email  []string `long:"email" env:"EMAIL" description:"admin emails" env-delim:","`
	} `group:"shared" namespace:"shared" env-namespace:"SHARED"`
}

// SMTPGroup defines options for SMTP server connection, used in auth and notify modules
type SMTPGroup struct {
	Host               string        `long:"host" env:"HOST" description:"SMTP host"`
	Port               int           `long:"port" env:"PORT" description:"SMTP port"`
	Username           string        `long:"username" env:"USERNAME" description:"SMTP user name"`
	Password           string        `long:"password" env:"PASSWORD" description:"SMTP password"`
	TLS                bool          `long:"tls" env:"TLS" description:"enable TLS"`
	InsecureSkipVerify bool          `long:"insecure_skip_verify" env:"INSECURE_SKIP_VERIFY" description:"skip certificate verification"`
	LoginAuth          bool          `long:"login_auth" env:"LOGIN_AUTH" description:"enable LOGIN auth instead of PLAIN"`
	StartTLS           bool          `long:"starttls" env:"STARTTLS" description:"enable StartTLS"`
	TimeOut            time.Duration `long:"timeout" env:"TIMEOUT" default:"10s" description:"SMTP TCP connection timeout"`
}

// NotifyGroup defines options for notification
type NotifyGroup struct {
	Type      []string `long:"type" env:"TYPE" description:"[deprecated, use user and admin types instead] types of notifications" choice:"none" choice:"email" default:"none" env-delim:","` //nolint
	Users     []string `long:"users" env:"USERS" description:"types of user notifications" choice:"none" choice:"email" default:"none" env-delim:","`                                        //nolint
	Admins    []string `long:"admins" env:"ADMINS" description:"types of admin notifications" choice:"none" choice:"email" default:"none" env-delim:","`                                     //nolint
	QueueSize int      `long:"queue" env:"QUEUE" description:"size of notification queue" default:"100"`
	Email struct {
		From                string `long:"from_address" env:"FROM" description:"from email address"`
		VerificationSubject string `long:"verification_subj" env:"VERIFICATION_SUBJ" description:"verification message subject"`
		AdminNotifications  bool   `long:"notify_admin" env:"ADMIN" description:"[deprecated, use --notify.admins=email] notify admin on new comments via ADMIN_SHARED_EMAIL"`
	} `group:"email" namespace:"email" env-namespace:"EMAIL"`
}

// LoadingCache defines interface for caching
type LoadingCache interface {
	Get(key cache.Key, fn func() ([]byte, error)) (data []byte, err error) // load from cache if found or put to cache and return
	Flush(req cache.FlusherRequest)                                        // evict matched records
	Close() error
}

// serverApp holds all active objects
type serverApp struct {
	*ServerCommand
	restSrv       *api.Rest
	migratorSrv   *api.Migrator
	exporter      migrator.Exporter
	dataService   *service.DataStore
	avatarStore   avatar.Store
	notifyService *notify.Service
	imageService  *image.Service
	authenticator *auth.Service
	terminated    chan struct{}

	authRefreshCache *authRefreshCache // stored only to close it properly on shutdown
}

// Execute is the entry point for "server" command, called by flag parser
func (s *ServerCommand) Execute(_ []string) error {
	log.Printf("[INFO] start server on port %s:%d", s.Address, s.Port)
	resetEnv(
		"SECRET",
		"AUTH_GITHUB_CSEC",
		"SMTP_PASSWORD",
		"ADMIN_PASSWD",
	)

	ctx, cancel := context.WithCancel(context.Background())
	go func() { // catch signal and invoke graceful termination
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
		<-stop
		log.Printf("[WARN] interrupt signal")
		cancel()
	}()

	app, err := s.newServerApp(ctx)
	if err != nil {
		log.Printf("[PANIC] failed to setup application, %+v", err)
		return err
	}
	if err = app.run(ctx); err != nil {
		log.Printf("[ERROR] remark terminated with error %+v", err)
		return err
	}
	log.Printf("[INFO] remark terminated")
	return nil
}

// HandleDeprecatedFlags sets new flags from deprecated returns their list.
// Returned list has DeprecatedFlag.Old and DeprecatedFlag.Version set, and DeprecatedFlag.New is optional
// (as some entries are removed without substitute).
// Also it returns flags found by findDeprecatedFlagsCollisions, with DeprecatedFlag.Collision flag set.
func (s *ServerCommand) HandleDeprecatedFlags() (result []DeprecatedFlag) {
	if s.LegacyImageProxy && !s.ImageProxy.HTTP2HTTPS {
		s.ImageProxy.HTTP2HTTPS = s.LegacyImageProxy
		result = append(result, DeprecatedFlag{Old: "img-proxy", New: "image-proxy.http2https", Version: "1.5"})
	}
	if !contains("none", s.Notify.Type) &&
		contains("none", s.Notify.Users) &&
		contains("none", s.Notify.Admins) { // if new notify param(s) are used, safe to ignore the old one
		s.handleDeprecatedNotifications()
		result = append(result, DeprecatedFlag{Old: "notify.type", New: "notify.(users|admins)", Version: "1.9"})
	}
	if s.Notify.Email.AdminNotifications && !contains("email", s.Notify.Admins) {
		s.Notify.Admins = append(s.Notify.Admins, "email")
		result = append(result, DeprecatedFlag{Old: "notify.email.notify_admin", New: "notify.admins=email", Version: "1.9"})
	}
	return append(result, s.findDeprecatedFlagsCollisions()...)
}

// findDeprecatedFlagsCollisions returns flags which are set both old (deprecated) and new way,
// which means new ones are used and old ones are ignored by deprecated flag handler.
// It returns DeprecatedFlag list which always has only DeprecatedFlag.Old and DeprecatedFlag.New set,
// and DeprecatedFlag.Collision set to true.
func (s *ServerCommand) findDeprecatedFlagsCollisions() (result []DeprecatedFlag) {
	if !contains("none", s.Notify.Type) &&
		(!contains("none", s.Notify.Users) || !contains("none", s.Notify.Admins)) {
		result = append(result, DeprecatedFlag{Old: "notify.type", New: "notify.(users|admins)", Collision: true})
	}
	return result
}

func (s *ServerCommand) handleDeprecatedNotifications() {
	for _, t := range s.Notify.Type {
		if t == "email" && !contains(t, s.Notify.Users) {
			s.Notify.Users = append(s.Notify.Users, t)
		}
	}
}

func contains(s string, a []string) bool {
	return slices.Contains(a, s)
}

// newServerApp prepares application and return it with all active parts
// doesn't start anything
func (s *ServerCommand) newServerApp(_ context.Context) (*serverApp, error) {
	if err := makeDirs(s.BackupLocation); err != nil {
		return nil, fmt.Errorf("failed to create backup store: %w", err)
	}

	if !strings.HasPrefix(s.RemarkURL, "http://") && !strings.HasPrefix(s.RemarkURL, "https://") {
		return nil, fmt.Errorf("invalid remark42 url %s", s.RemarkURL)
	}
	log.Printf("[INFO] root url=%s", s.RemarkURL)

	// parse trusted proxies up front so a bad CIDR fails before any resource is allocated
	trustedProxies, err := api.ParseTrustedProxies(s.TrustedProxies)
	if err != nil {
		return nil, fmt.Errorf("invalid --trusted-proxy: %w", err)
	}
	switch {
	case len(trustedProxies) == 0:
		log.Printf("[WARN] --trusted-proxy not set: forwarding headers are trusted from any client and can be spoofed to bypass rate limiting / vote dedup; set it behind a reverse proxy (see docs)")
	case api.TrustsAnyPeer(trustedProxies):
		log.Printf("[WARN] --trusted-proxy has a catch-all (0.0.0.0/0 or ::/0): forwarding headers are trusted from any client, re-opening the spoofing bypass; scope it to your proxy network")
	}

	storeEngine, err := s.makeDataStore()
	if err != nil {
		return nil, fmt.Errorf("failed to make data store engine: %w", err)
	}

	adminStore, err := s.makeAdminStore()
	if err != nil {
		return nil, fmt.Errorf("failed to make admin store: %w", err)
	}

	imageService, err := s.makePicturesStore()
	if err != nil {
		return nil, fmt.Errorf("failed to make pictures store: %w", err)
	}
	log.Printf("[DEBUG] image service for url=%s, EditDuration=%v", imageService.ImageAPI, imageService.EditDuration)

	dataService := &service.DataStore{
		Engine:                 storeEngine,
		EditDuration:           s.EditDuration,
		AdminEdits:             s.AdminEdit,
		AdminStore:             adminStore,
		MinCommentSize:         s.MinCommentSize,
		MaxCommentSize:         s.MaxCommentSize,
		MaxVotes:               s.MaxVotes,
		PositiveScore:          s.PositiveScore,
		ImageService:           imageService,
		TitleExtractor:         service.NewTitleExtractor(http.Client{Timeout: time.Second * 5, Transport: safehttp.Transport()}, s.getAllowedDomains()),
		RestrictedWordsMatcher: service.NewRestrictedWordsMatcher(service.StaticRestrictedWordsLister{Words: s.RestrictedWords}),
	}
	dataService.RestrictSameIPVotes.Enabled = s.RestrictVoteIP
	dataService.RestrictSameIPVotes.Duration = s.DurationVoteIP

	loadingCache, err := s.makeCache()
	if err != nil {
		_ = dataService.Close()
		return nil, fmt.Errorf("failed to make cache: %w", err)
	}

	avatarStore, err := s.makeAvatarStore()
	if err != nil {
		_ = dataService.Close()
		return nil, fmt.Errorf("failed to make avatar store: %w", err)
	}
	authRefreshCache := newAuthRefreshCache()
	authenticator := s.getAuthenticator(dataService, avatarStore, adminStore, authRefreshCache)

	s.addAuthProviders(authenticator)

	exporter := &migrator.Native{DataStore: dataService}

	migr := &api.Migrator{
		Cache:          loadingCache,
		NativeImporter: &migrator.Native{DataStore: dataService},
		NativeExporter: &migrator.Native{DataStore: dataService},
		URLMapperMaker: migrator.NewURLMapper,
		KeyStore:       adminStore,
	}

	notifyDestinations, err := s.makeNotifyDestinations(authenticator)
	if err != nil {
		log.Printf("[WARN] failed to prepare notify destinations, %s", err)
	}

	notifyService := s.makeNotifyService(dataService, notifyDestinations)

	imgProxy := &proxy.Image{
		HTTP2HTTPS:    s.ImageProxy.HTTP2HTTPS,
		CacheExternal: s.ImageProxy.CacheExternal,
		RoutePath:     "/api/v1/img",
		RemarkURL:     s.RemarkURL,
		ImageService:  imageService,
	}
	emojiFmt := store.CommentConverterFunc(func(text string) string { return text })
	if s.EnableEmoji {
		emojiFmt = func(text string) string { return emoji.Sprint(text) }
	}
	commentFormatter := store.NewCommentFormatter(imgProxy, emojiFmt)

	srv := &api.Rest{
		Version:                    s.Revision,
		DataService:                dataService,
		WebRoot:                    s.WebRoot,
		WebFS:                      webFS,
		RemarkURL:                  s.RemarkURL,
		ImageProxy:                 imgProxy,
		CommentFormatter:           commentFormatter,
		Migrator:                   migr,
		ReadOnlyAge:                s.ReadOnlyAge,
		SharedSecret:               s.SharedSecret,
		TrustedProxies:             trustedProxies,
		Authenticator:              authenticator,
		Cache:                      loadingCache,
		NotifyService:              notifyService,
		UpdateLimiter:              s.UpdateLimit,
		ImageService:               imageService,
		EmailNotifications:         contains("email", s.Notify.Users),
		EmojiEnabled:               s.EnableEmoji,
		AnonVote:                   s.AnonymousVote && s.RestrictVoteIP,
		SimpleView:                 s.SimpleView,
		ProxyCORS:                  s.ProxyCORS,
		AllowedAncestors:           s.AllowedHosts,
		SendJWTHeader:              s.Auth.SendJWTHeader,
		SubscribersOnly:            s.SubscribersOnly,
		DisableSignature:           s.DisableSignature,
		DisableFancyTextFormatting: s.DisableFancyTextFormatting,
		ExternalImageProxy:         s.ImageProxy.CacheExternal,
	}

	srv.ScoreThresholds.Low, srv.ScoreThresholds.Critical = s.LowScore, s.CriticalScore

	return &serverApp{
		ServerCommand:    s,
		restSrv:          srv,
		migratorSrv:      migr,
		exporter:         exporter,
		dataService:      dataService,
		avatarStore:      avatarStore,
		notifyService:    notifyService,
		imageService:     imageService,
		authenticator:    authenticator,
		terminated:       make(chan struct{}),
		authRefreshCache: authRefreshCache,
	}, nil
}

// Extract domains from s.AllowedHosts and second level domain from s.RemarkURL.
// It can be and IP like http://127.0.0.1 in which case we need to use whole IP as domain
// Beware, if s.RemarkURL is in third-level domain like https://example.co.uk, co.uk will be returned.
func (s *ServerCommand) getAllowedDomains() []string {
	rawDomains := s.AllowedHosts
	rawDomains = append(rawDomains, s.RemarkURL)
	allowedDomains := []string{}
	for _, rawURL := range rawDomains {
		// case of 'self' AllowedHosts, which is not a valid rawURL name
		if rawURL == "self" || rawURL == "'self'" || rawURL == "\"self\"" {
			continue
		}
		// AllowedHosts usually don't have https:// prefix, so we're adding it just to make parsing below work the same way as for RemarkURL
		if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
			rawURL = "https://" + rawURL
		}
		parsedURL, err := url.Parse(rawURL)
		if err != nil {
			log.Printf("[WARN] failed to parse URL %s for TitleExtract whitelist: %v", rawURL, err)
			continue
		}
		domain := parsedURL.Hostname()

		if domain == "" || // don't add empty domain as it will allow everything to be extracted
			(len(strings.Split(domain, ".")) < 2 && // don't allow single-word domains like "com"
				domain != "localhost") { // localhost is an exceptional single-word domain which is allowed
			continue
		}

		// only for RemarkURL if domain is not IP and has more than two levels, extract second level domain.
		// for AllowedHosts we don't do this as they are exact list of domains which can host comments, but
		// remarkURL might be on a subdomain and we must allow parent domain to be used for TitleExtract.
		if rawURL == s.RemarkURL && net.ParseIP(domain) == nil && len(strings.Split(domain, ".")) > 2 {
			domain = strings.Join(strings.Split(domain, ".")[len(strings.Split(domain, "."))-2:], ".")
		}

		allowedDomains = append(allowedDomains, domain)
	}
	return allowedDomains
}

// getAllowedRedirectHosts normalises s.AllowedHosts into the form that
// go-pkgz/auth's redirect validator expects. Strips http(s) schemes and
// paths; preserves explicit ports (the validator matches both host-only
// and host:port, so an entry without a port accepts any port while an
// entry with a port restricts to that port). Skips CSP sentinels
// ('self' / "self") and wildcard entries (*, *.example.com) that are
// valid CSP source expressions but not valid hostnames.
func (s *ServerCommand) getAllowedRedirectHosts() []string {
	out := make([]string, 0, len(s.AllowedHosts))
	for _, raw := range s.AllowedHosts {
		raw = strings.TrimSpace(raw)
		if raw == "" || raw == "self" || raw == "'self'" || raw == `"self"` {
			continue
		}
		if strings.ContainsRune(raw, '*') { // CSP wildcard, not a host
			continue
		}
		// add scheme so url.Parse populates Hostname()/Host consistently for bare hosts
		toParse := raw
		if !strings.HasPrefix(toParse, "http://") && !strings.HasPrefix(toParse, "https://") {
			toParse = "https://" + toParse
		}
		u, err := url.Parse(toParse)
		if err != nil || u.Hostname() == "" {
			log.Printf("[WARN] skipping invalid AllowedHosts entry %q for redirect allowlist: %v", raw, err)
			continue
		}
		if u.Port() != "" {
			out = append(out, u.Host) // preserve explicit host:port so allowlist is port-specific
			continue
		}
		out = append(out, u.Hostname())
	}
	return out
}

// Run all application objects
func (a *serverApp) run(ctx context.Context) error {
	if a.AdminPasswd != "" {
		log.Printf("[WARN] admin basic auth enabled")
	}

	go func() {
		// shutdown on context cancellation
		<-ctx.Done()
		log.Print("[INFO] shutdown initiated")
		a.restSrv.Shutdown()
	}()

	a.activateBackup(ctx) // runs in goroutine for each site

	// staging images resubmit after restart of the app
	if e := a.dataService.ResubmitStagingImages(a.Sites); e != nil {
		log.Printf("[WARN] failed to resubmit comments with staging images, %s", e)
	}

	go a.imageService.Cleanup(ctx) // pictures cleanup for staging images

	a.restSrv.Run(a.Address, a.Port)

	// shutdown procedures after HTTP server is stopped
	if e := a.dataService.Close(); e != nil {
		log.Printf("[WARN] failed to close data store, %s", e)
	}
	if e := a.avatarStore.Close(); e != nil {
		log.Printf("[WARN] failed to close avatar store, %s", e)
	}
	if e := a.restSrv.Cache.Close(); e != nil {
		log.Printf("[WARN] failed to close rest server cache, %s", e)
	}
	if e := a.authRefreshCache.Close(); e != nil {
		log.Printf("[WARN] failed to close auth authRefreshCache, %s", e)
	}
	a.notifyService.Close()
	// call potentially infinite loop with cancellation after a minute as a safeguard
	minuteCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	a.imageService.Close(minuteCtx)

	close(a.terminated)
	return nil
}

// Wait for application completion (termination)
func (a *serverApp) Wait() {
	<-a.terminated
}

// activateBackup runs background backups for each site
func (a *serverApp) activateBackup(ctx context.Context) {
	for _, siteID := range a.Sites {
		backup := migrator.AutoBackup{
			Exporter:       a.exporter,
			BackupLocation: a.BackupLocation,
			SiteID:         siteID,
			KeepMax:        a.MaxBackupFiles,
			Duration:       24 * time.Hour,
		}
		go backup.Do(ctx)
	}
}

// makeDataStore creates store for all sites
func (s *ServerCommand) makeDataStore() (result engine.Interface, err error) {
	log.Printf("[INFO] make data store, type=%s", s.Store.Type)

	switch s.Store.Type {
	case "bolt":
		if err = makeDirs(s.Store.Bolt.Path); err != nil {
			return nil, fmt.Errorf("failed to create bolt store: %w", err)
		}
		sites := []engine.BoltSite{}
		for _, site := range s.Sites {
			sites = append(sites, engine.BoltSite{SiteID: site, FileName: fmt.Sprintf("%s/%s.db", s.Store.Bolt.Path, site)})
		}
		result, err = engine.NewBoltDB(bolt.Options{Timeout: s.Store.Bolt.Timeout}, sites...)
	default:
		return nil, fmt.Errorf("unsupported store type %s", s.Store.Type)
	}
	if err != nil {
		return nil, fmt.Errorf("can't initialize data store: %w", err)
	}
	return result, nil
}

func (s *ServerCommand) makeAvatarStore() (avatar.Store, error) {
	log.Printf("[INFO] make avatar store, type=%s", s.Avatar.Type)

	switch s.Avatar.Type {
	case "fs":
		if err := makeDirs(s.Avatar.FS.Path); err != nil {
			return nil, fmt.Errorf("failed to create avatar store: %w", err)
		}
		return avatar.NewLocalFS(s.Avatar.FS.Path), nil
	case "uri":
		return avatar.NewStore(s.Avatar.URI)
	}
	return nil, fmt.Errorf("unsupported avatar store type %s", s.Avatar.Type)
}

func (s *ServerCommand) makePicturesStore() (*image.Service, error) {
	imageServiceParams := image.ServiceParams{
		ImageAPI:     s.RemarkURL + "/api/v1/picture/",
		ProxyAPI:     s.RemarkURL + "/api/v1/img",
		EditDuration: s.EditDuration,
		MaxSize:      s.Image.MaxSize,
		MaxHeight:    s.Image.ResizeHeight,
		MaxWidth:     s.Image.ResizeWidth,
	}
	switch s.Image.Type {
	case "bolt":
		boltImageStore, err := image.NewBoltStorage(s.Image.Bolt.File, bolt.Options{})
		if err != nil {
			return nil, err
		}
		return image.NewService(boltImageStore, imageServiceParams), nil
	case "fs":
		if err := makeDirs(s.Image.FS.Path); err != nil {
			return nil, fmt.Errorf("failed to create pictures store: %w", err)
		}
		return image.NewService(&image.FileSystem{
			Location:   s.Image.FS.Path,
			Staging:    s.Image.FS.Staging,
			Partitions: s.Image.FS.Partitions,
		}, imageServiceParams), nil
	}
	return nil, fmt.Errorf("unsupported pictures store type %s", s.Image.Type)
}

func (s *ServerCommand) makeAdminStore() (admin.Store, error) {
	log.Printf("[INFO] make admin store, type=%s", s.Admin.Type)

	switch s.Admin.Type {
	case "shared":
		sharedAdminEmail := ""
		if len(s.Admin.Shared.Email) == 0 { // no admin email, use admin@domain
			if u, err := url.Parse(s.RemarkURL); err == nil {
				sharedAdminEmail = "admin@" + u.Host
			}
		} else {
			sharedAdminEmail = s.Admin.Shared.Email[0]
		}
		return admin.NewStaticStore(s.SharedSecret, s.Sites, s.Admin.Shared.Admins, sharedAdminEmail), nil
	default:
		return nil, fmt.Errorf("unsupported admin store type %s", s.Admin.Type)
	}
}

func (s *ServerCommand) makeCache() (LoadingCache, error) {
	log.Printf("[INFO] make cache, type=%s", s.Cache.Type)
	o := cache.NewOpts[[]byte]()
	switch s.Cache.Type {
	case "mem":
		backend, err := cache.NewLruCache(o.MaxCacheSize(s.Cache.Max.Size), o.MaxValSize(s.Cache.Max.Value),
			o.MaxKeys(s.Cache.Max.Items))
		if err != nil {
			return nil, fmt.Errorf("cache backend initialization: %w", err)
		}
		return cache.NewScache[[]byte](backend), nil
	case "none":
		return cache.NewScache[[]byte](&cache.Nop[[]byte]{}), nil
	}
	return nil, fmt.Errorf("unsupported cache type %s", s.Cache.Type)
}

func (s *ServerCommand) addAuthProviders(authenticator *auth.Service) {
	if s.Auth.Github.CID != "" && s.Auth.Github.CSEC != "" {
		authenticator.AddProvider("github", s.Auth.Github.CID, s.Auth.Github.CSEC)
	} else {
		log.Printf("[WARN] no auth providers defined")
	}
}

func (s *ServerCommand) makeNotifyService(dataStore *service.DataStore, destinations []notify.Destination) *notify.Service {
	if destinations == nil {
		destinations = []notify.Destination{}
	}

	if len(destinations) > 0 {
		log.Printf("[INFO] make notify, for users: %s, for admins: %s", s.Notify.Users, s.Notify.Admins)
		return notify.NewService(dataStore, s.Notify.QueueSize, destinations...)
	}
	return notify.NopService
}

// constructs list of notify destinations, returns empty list in case of error
func (s *ServerCommand) makeNotifyDestinations(authenticator *auth.Service) ([]notify.Destination, error) {
	destinations := make([]notify.Destination, 0)

	// with logic below admin notifications enable notifications for users on the backend even if they
	// are not enabled explicitly, however they won't be visible to the users in the frontend
	// because api.Rest.EmailNotifications would be set to false.
	if contains("email", s.Notify.Users) || contains("email", s.Notify.Admins) {
		emailParams := notify.EmailParams{
			MsgTemplatePath:          s.emailMsgTemplatePath,
			VerificationTemplatePath: s.emailVerificationTemplatePath, From: s.Notify.Email.From,
			VerificationSubject: s.Notify.Email.VerificationSubject,
			UnsubscribeURL:      s.RemarkURL + "/email/unsubscribe.html",
			TokenGenFn: func(userID, email, site string) (string, error) {
				claims := token.Claims{
					Handshake: &token.Handshake{ID: userID + "::" + email},
					RegisteredClaims: jwt.RegisteredClaims{
						Audience:  jwt.ClaimStrings{site},
						ExpiresAt: jwt.NewNumericDate(time.Now().Add(100 * 365 * 24 * time.Hour)),
						NotBefore: jwt.NewNumericDate(time.Now().Add(-1 * time.Minute)),
						Issuer:    "remark42",
					},
				}
				tkn, err := authenticator.TokenService().Token(claims)
				if err != nil {
					return "", fmt.Errorf("failed to make unsubscription token: %w", err)
				}
				return tkn, nil
			},
		}
		if contains("email", s.Notify.Admins) {
			emailParams.AdminEmails = s.Admin.Shared.Email
		}
		smtpParams := ntf.SMTPParams{
			Host:               s.SMTP.Host,
			Port:               s.SMTP.Port,
			TLS:                s.SMTP.TLS,
			StartTLS:           s.SMTP.StartTLS,
			InsecureSkipVerify: s.SMTP.InsecureSkipVerify,
			LoginAuth:          s.SMTP.LoginAuth,
			Username:           s.SMTP.Username,
			Password:           s.SMTP.Password,
			TimeOut:            s.SMTP.TimeOut,
			ContentType:        "text/html",
			Charset:            "UTF-8",
		}
		emailService, err := notify.NewEmail(emailParams, smtpParams)
		if err != nil {
			return destinations, fmt.Errorf("failed to create email notification destination: %w", err)
		}
		destinations = append(destinations, emailService)
	}

	return destinations, nil
}

// getAuthenticator creates new authenticator service, which doesn't have any auth providers enabled
func (s *ServerCommand) getAuthenticator(ds *service.DataStore, avas avatar.Store, admns admin.Store, authRefreshCache *authRefreshCache) *auth.Service {
	return auth.NewService(auth.Opts{
		URL:            strings.TrimSuffix(s.RemarkURL, "/"),
		Issuer:         "remark42",
		TokenDuration:  s.Auth.TTL.JWT,
		CookieDuration: s.Auth.TTL.Cookie,
		SendJWTHeader:  s.Auth.SendJWTHeader,
		SameSiteCookie: s.parseSameSite(s.Auth.SameSite),
		SecureCookies:  strings.HasPrefix(s.RemarkURL, "https://"),
		// enable the `from` redirect allowlist in go-pkgz/auth v2.1.2+ — limits
		// post-auth redirects to RemarkURL's own host plus any configured
		// AllowedHosts. Prevents the OAuth open-redirect / phishing vector.
		AllowedRedirectHosts: token.AllowedHostsFunc(func() ([]string, error) {
			return s.getAllowedRedirectHosts(), nil
		}),
		SecretReader: token.SecretFunc(func(aud string) (string, error) { // get secret per site
			return admns.Key(aud)
		}),
		ClaimsUpd: token.ClaimsUpdFunc(func(c token.Claims) token.Claims { // set attributes, on new token or refresh
			if c.User == nil {
				return c
			}
			// audience is a slice but we set it to a single element, and situation when there is no audience or there are more than one is unexpected
			if len(c.Audience) != 1 {
				return c
			}
			audience := c.Audience[0]

			c.User.SetAdmin(ds.IsAdmin(audience, c.User.ID))
			c.User.SetBoolAttr("blocked", ds.IsBlocked(audience, c.User.ID))
			var err error
			c.User.Email, err = ds.GetUserEmail(audience, c.User.ID)
			if err != nil {
				log.Printf("[WARN] can't read email for %s, %v", c.User.ID, err)
			}

			// don't allow anonymous and email with admins names
			// exclude admin from impersonation detection over email, it prevents a valid admin to login with RestrictedNames
			if strings.HasPrefix(c.User.ID, "anonymous_") || (strings.HasPrefix(c.User.ID, "email_") && !c.User.IsAdmin()) {
				for _, a := range s.RestrictedNames {
					if strings.EqualFold(strings.TrimSpace(c.User.Name), a) {
						c.User.SetBoolAttr("blocked", true)
						log.Printf("[INFO] blocked %+v, attempt to impersonate (restricted names)", c.User)
						break
					}
				}
			}

			return c
		}),
		AdminPasswd: s.AdminPasswd,
		Validator: token.ValidatorFunc(func(_ string, claims token.Claims) bool { // check on each auth call (in middleware)
			if claims.User == nil {
				return false
			}
			if claims.User.Audience == "" { // reject empty aud, made with old (pre 0.8.x) version of auth package
				return false
			}
			return !claims.User.BoolAttr("blocked")
		}),
		JWTQuery:          "jwt", // change default from "token" as it used for deleteme
		AvatarStore:       avas,
		AvatarResizeLimit: s.Avatar.RszLmt,
		AvatarRoutePath:   "/api/v1/avatar",
		Logger:            log.Default(),
		RefreshCache:      authRefreshCache,
		UseGravatar:       true,
		AudSecrets:        false,
	})
}

func (s *ServerCommand) parseSameSite(ss string) http.SameSite {
	switch strings.ToLower(ss) {
	case "default":
		return http.SameSiteDefaultMode
	case "none":
		return http.SameSiteNoneMode
	case "lax":
		return http.SameSiteLaxMode
	case "strict":
		return http.SameSiteStrictMode
	default:
		return http.SameSiteDefaultMode
	}
}

// authRefreshCache used by authenticator to minimize repeatable token refreshes
type authRefreshCache struct {
	cache.LoadingCache[token.Claims]
}

func newAuthRefreshCache() *authRefreshCache {
	o := cache.NewOpts[token.Claims]()
	expirableCache, _ := cache.NewExpirableCache(o.TTL(5 * time.Minute))
	return &authRefreshCache{LoadingCache: expirableCache}
}

// Get implements cache getter with key converted to string
func (c *authRefreshCache) Get(key string) (token.Claims, bool) {
	return c.Peek(key)
}

// Set implements cache setter with key converted to string
func (c *authRefreshCache) Set(key string, value token.Claims) {
	_, _ = c.LoadingCache.Get(key, func() (token.Claims, error) { return value, nil })
}
