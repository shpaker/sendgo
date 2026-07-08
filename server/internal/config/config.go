// Package config describes the sendgo CLI/env configuration via
// alecthomas/kong.
//
// All env names mirror those used by the current Node code (server/config.js)
// 1:1, so existing .env files and K8s manifests do not need rewriting.
// Branding flags live in a separate group for a tidy --help output.
//
// Value precedence: CLI flag > env var > default.
package config

import (
	"time"

	"github.com/alecthomas/kong"
)

// Version is injected via -ldflags at build time.
var Version = "dev"

// Commit holds the git SHA, also injected via ldflags.
var Commit = "unknown"

// CLI is the single config struct. Passed as *CLI to nearly every adapter
// that needs parameters. Each field is tagged with `help` (for --help),
// `default` (for kong), and `env` (env var name; matches the Node variant
// so existing deploy scripts keep working).
type CLI struct {
	// --- server ---
	Port          int    `help:"HTTP listen port." default:"1443" env:"PORT"`
	IPAddress     string `help:"Bind address." default:"0.0.0.0" env:"IP_ADDRESS"`
	BaseURL       string `help:"Public base URL (used in WS upload response). Defaults to scheme://host:port from request if --detect-base-url is set." env:"BASE_URL"`
	DetectBaseURL bool   `help:"Derive base URL from request headers." env:"DETECT_BASE_URL"`

	// --- limits (1:1 with server/config.js) ---
	MaxFileSize          int64 `help:"Max plaintext bytes per upload. The WS limit compares against the encrypted size." default:"2684354560" env:"MAX_FILE_SIZE"`
	ExpireTimesSeconds   []int `help:"Allowed TTLs (comma-separated)." default:"300,3600,86400,604800" env:"EXPIRE_TIMES_SECONDS"`
	DefaultExpireSeconds int   `help:"Default TTL in seconds." default:"86400" env:"DEFAULT_EXPIRE_SECONDS"`
	MaxExpireSeconds     int   `help:"Max allowed TTL in seconds." default:"604800" env:"MAX_EXPIRE_SECONDS"`
	DownloadCounts       []int `help:"Allowed dlimit values." default:"1,2,3,4,5,20,50,100" env:"DOWNLOAD_COUNTS"`
	DefaultDownloads     int   `help:"Default dlimit." default:"1" env:"DEFAULT_DOWNLOADS"`
	MaxDownloads         int   `help:"Max dlimit." default:"100" env:"MAX_DOWNLOADS"`
	MaxFilesPerArchive   int   `help:"Max files per multi-file share." default:"64" env:"MAX_FILES_PER_ARCHIVE"`
	MaxArchivesPerUser   int   `help:"Max shares retained per FxA user (legacy field, FXA dropped — kept for /config parity)." default:"16" env:"MAX_ARCHIVES_PER_USER"`

	// --- meta store (Redis if DSN given, else in-memory) ---
	RedisDSN string `help:"Redis DSN (redis://[user:pass@]host:port[/db]). Empty → in-memory meta store (state is lost on restart)." env:"REDIS_DSN"`

	// --- blob storage (S3 if bucket set, else FS) ---
	S3Bucket       string `name:"s3-bucket" help:"S3 bucket name. If set, S3 backend is used (compatible with Yandex Object Storage)." env:"S3_BUCKET"`
	S3Endpoint     string `name:"s3-endpoint" help:"S3 endpoint URL (Yandex: https://storage.yandexcloud.net)." env:"S3_ENDPOINT"`
	S3Region       string `name:"s3-region" help:"S3/AWS region." default:"us-east-1" env:"AWS_REGION"`
	S3UsePathStyle bool   `name:"s3-use-path-style" help:"Use path-style S3 URLs (required for MinIO)." env:"S3_USE_PATH_STYLE_ENDPOINT"`
	FileDir        string `help:"FS storage dir; used if --s3-bucket is empty. If left empty, a temp dir under os.TempDir is created at startup." env:"FILE_DIR"`

	// --- cleanup ---
	CleanupEnabled   bool          `help:"Run in-process cleanup goroutine to delete blobs of expired shares (fixes orphaned-blob bug from Node version)." env:"CLEANUP_ENABLED"`
	CleanupInterval  time.Duration `help:"Cleanup tick interval." default:"1m" env:"CLEANUP_INTERVAL"`
	CleanupBatchSize int           `help:"Max items processed per cleanup tick." default:"100" env:"CLEANUP_BATCH_SIZE"`

	// --- OIDC auth (optional) ---
	// All three of issuer/client-id/client-secret must be set together to
	// enable auth; with all of them empty the server runs open, exactly as
	// before. Validation lives in oidcauth.New (factory idiom, like meta.New).
	OIDCIssuerURL    string        `name:"oidc-issuer-url" help:"OIDC issuer URL (e.g. https://auth.example.com/application/o/sendgo/). Empty → auth disabled, uploads are public." group:"oidc" env:"OIDC_ISSUER_URL"`
	OIDCClientID     string        `name:"oidc-client-id" help:"OAuth2 client ID." group:"oidc" env:"OIDC_CLIENT_ID"`
	OIDCClientSecret string        `name:"oidc-client-secret" help:"OAuth2 client secret." group:"oidc" env:"OIDC_CLIENT_SECRET"`
	OIDCRedirectURL  string        `name:"oidc-redirect-url" help:"Explicit OAuth2 redirect URL. Empty → derived per request: <base-url>/oidc/callback." group:"oidc" env:"OIDC_REDIRECT_URL"`
	OIDCScopes       []string      `name:"oidc-scopes" help:"Requested OIDC scopes (comma-separated)." group:"oidc" default:"openid,profile,email" env:"OIDC_SCOPES"`
	OIDCSessionTTL   time.Duration `name:"oidc-session-ttl" help:"Session cookie lifetime." group:"oidc" default:"12h" env:"OIDC_SESSION_TTL"`
	OIDCCookieSecret string        `name:"oidc-cookie-secret" help:"HMAC key for session cookies (min 32 chars). Empty → random per start: sessions drop on restart and multi-replica setups break." group:"oidc" env:"OIDC_COOKIE_SECRET"`

	// Optional allow-lists on top of authentication. Matters for public IdPs
	// (Google): without a filter, "authenticated" means anyone with an
	// account there.
	OIDCAllowedEmails  []string `name:"oidc-allowed-emails" help:"Allow uploads only for these emails (comma-separated, case-insensitive). Empty → any authenticated user." group:"oidc" env:"OIDC_ALLOWED_EMAILS"`
	OIDCAllowedDomains []string `name:"oidc-allowed-domains" help:"Allow uploads only for these email domains (comma-separated, case-insensitive). Empty → any authenticated user." group:"oidc" env:"OIDC_ALLOWED_DOMAINS"`

	// --- observability ---
	SentryDSN string `help:"Sentry DSN. Empty → Sentry disabled." env:"SENTRY_DSN"`
	LogLevel  string `help:"Log level." default:"info" enum:"debug,info,warn,error" env:"LOG_LEVEL"`
	LogFormat string `help:"Log format. 'auto' picks text on TTY, json otherwise." default:"auto" enum:"json,text,auto" env:"LOG_FORMAT"`
	LogFile   string `help:"Log file path. Empty → stdout. Reopened on SIGHUP for logrotate." env:"LOG_FILE"`

	// --- UI / branding: 1:1 with the Node config (server/config.js + server/clientConstants.js).
	//     Split into its own --help group via kong.ExplicitGroups to keep the main
	//     list uncluttered. Env names and defaults match Node so existing .env
	//     files keep working.

	// SSR-only: consumed by pages.go when rendering /, not exposed via /config.
	CustomTitle       string `help:"UI title override (page <title>)." group:"branding" env:"CUSTOM_TITLE"`
	CustomDescription string `help:"UI description (meta description)." group:"branding" env:"CUSTOM_DESCRIPTION"`

	// Footer links — the original uses a SEND_* env prefix.
	FooterDonateURL  string `help:"Footer donate link URL." group:"branding" env:"SEND_FOOTER_DONATE_URL"`
	FooterCLIURL     string `help:"Footer CLI link URL." group:"branding" default:"https://github.com/timvisee/ffsend" env:"SEND_FOOTER_CLI_URL"`
	FooterDMCAURL    string `help:"Footer DMCA link URL." group:"branding" env:"SEND_FOOTER_DMCA_URL"`
	FooterSourceURL  string `help:"Footer source link URL." group:"branding" default:"https://github.com/timvisee/send" env:"SEND_FOOTER_SOURCE_URL"`
	CustomFooterText string `help:"Custom footer text." group:"branding" env:"CUSTOM_FOOTER_TEXT"`
	CustomFooterURL  string `help:"Custom footer link URL." group:"branding" env:"CUSTOM_FOOTER_URL"`

	// HTML notices — the original uses a SEND_* env prefix.
	MainNoticeHTML        string `help:"HTML banner on main page." group:"branding" env:"SEND_MAIN_NOTICE_HTML"`
	UploadAreaNoticeHTML  string `help:"HTML banner above upload area." group:"branding" env:"SEND_UPLOAD_AREA_NOTICE_HTML"`
	UploadsListNoticeHTML string `help:"HTML banner above uploads list." group:"branding" env:"SEND_UPLOADS_LIST_NOTICE_HTML"`
	DownloadNoticeHTML    string `help:"HTML banner on download page." group:"branding" env:"SEND_DOWNLOAD_NOTICE_HTML"`

	// Misc UI toggles.
	ShowThunderbirdSponsor bool   `help:"Show Thunderbird sponsor block." group:"branding" env:"SHOW_THUNDERBIRD_SPONSOR"`
	CustomLocale           string `help:"Force a specific locale (overrides Accept-Language negotiation)." group:"branding" env:"CUSTOM_LOCALE"`

	// Colors — defaults match the original Send.
	UIColorPrimary string `help:"UI primary color." group:"branding" default:"#0a84ff" env:"UI_COLOR_PRIMARY"`
	UIColorAccent  string `help:"UI accent color." group:"branding" default:"#003eaa" env:"UI_COLOR_ACCENT"`

	// Custom assets — URLs for custom icons / fonts / css. Surfaced via the
	// WEB_UI.CUSTOM_ASSETS.* JSON.
	UICustomAssetsAndroidChrome192 string `name:"ui-custom-assets-android-chrome-192px" help:"Custom Android Chrome icon 192px." group:"branding" env:"UI_CUSTOM_ASSETS_ANDROID_CHROME_192PX"`
	UICustomAssetsAndroidChrome512 string `name:"ui-custom-assets-android-chrome-512px" help:"Custom Android Chrome icon 512px." group:"branding" env:"UI_CUSTOM_ASSETS_ANDROID_CHROME_512PX"`
	UICustomAssetsAppleTouchIcon   string `help:"Custom Apple touch icon." group:"branding" env:"UI_CUSTOM_ASSETS_APPLE_TOUCH_ICON"`
	UICustomAssetsFavicon16        string `name:"ui-custom-assets-favicon-16px" help:"Custom favicon 16px." group:"branding" env:"UI_CUSTOM_ASSETS_FAVICON_16PX"`
	UICustomAssetsFavicon32        string `name:"ui-custom-assets-favicon-32px" help:"Custom favicon 32px." group:"branding" env:"UI_CUSTOM_ASSETS_FAVICON_32PX"`
	UICustomAssetsIcon             string `help:"Custom main icon." group:"branding" env:"UI_CUSTOM_ASSETS_ICON"`
	UICustomAssetsSafariPinnedTab  string `help:"Custom Safari pinned-tab icon." group:"branding" env:"UI_CUSTOM_ASSETS_SAFARI_PINNED_TAB"`
	UICustomAssetsFacebook         string `help:"Custom Facebook OG image." group:"branding" env:"UI_CUSTOM_ASSETS_FACEBOOK"`
	UICustomAssetsTwitter          string `help:"Custom Twitter card image." group:"branding" env:"UI_CUSTOM_ASSETS_TWITTER"`
	UICustomAssetsWordmark         string `help:"Custom wordmark image URL." group:"branding" env:"UI_CUSTOM_ASSETS_WORDMARK"`
	UICustomCSS                    string `help:"Inline custom CSS injected into <head>." group:"branding" env:"UI_CUSTOM_CSS"`

	Version kong.VersionFlag `help:"Show version and exit." short:"V"`
}

// Load parses argv + env via kong and returns a populated CLI struct.
func Load(args []string) *CLI {
	cli := &CLI{}
	parser := kong.Must(cli,
		kong.Name("sendgo"),
		kong.Description("Encrypted file sharing service (Send fork). Single binary; defaults to in-memory metadata + FS blob storage."),
		kong.UsageOnError(),
		kong.ExplicitGroups([]kong.Group{
			{Key: "branding", Title: "Branding flags (UI customization, mirrors server/clientConstants.js)"},
			{Key: "oidc", Title: "OIDC authentication (optional; gates uploads when configured)"},
		}),
		kong.Vars{"version": Version + " (" + Commit + ")"},
	)
	if _, err := parser.Parse(args); err != nil {
		parser.FatalIfErrorf(err)
	}
	return cli
}
