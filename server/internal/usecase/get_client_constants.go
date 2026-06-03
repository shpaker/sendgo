package usecase

import "github.com/sendgo/sendgo/server/internal/config"

// ClientConstants is an exact copy of the GET /config response structure from
// the Node code (server/clientConstants.js). JSON field names and nesting
// match byte-for-byte so the frontend's `app/api.js:getConstants()` keeps
// working.
//
// Node returns three top-level keys: LIMITS, WEB_UI, DEFAULTS.
type ClientConstants struct {
	Limits   LimitsConstants   `json:"LIMITS"`
	WebUI    WebUIConstants    `json:"WEB_UI"`
	Defaults DefaultsConstants `json:"DEFAULTS"`
}

// LimitsConstants is a port of the `LIMITS` block from clientConstants.js.
type LimitsConstants struct {
	MaxFileSize        int64 `json:"MAX_FILE_SIZE"`
	MaxDownloads       int   `json:"MAX_DOWNLOADS"`
	MaxExpireSeconds   int   `json:"MAX_EXPIRE_SECONDS"`
	MaxFilesPerArchive int   `json:"MAX_FILES_PER_ARCHIVE"`
	MaxArchivesPerUser int   `json:"MAX_ARCHIVES_PER_USER"`
}

// WebUIConstants is a port of the `WEB_UI` block from clientConstants.js.
type WebUIConstants struct {
	FooterDonateURL        string            `json:"FOOTER_DONATE_URL"`
	FooterCLIURL           string            `json:"FOOTER_CLI_URL"`
	FooterDMCAURL          string            `json:"FOOTER_DMCA_URL"`
	FooterSourceURL        string            `json:"FOOTER_SOURCE_URL"`
	CustomFooterText       string            `json:"CUSTOM_FOOTER_TEXT"`
	CustomFooterURL        string            `json:"CUSTOM_FOOTER_URL"`
	MainNoticeHTML         string            `json:"MAIN_NOTICE_HTML"`
	UploadAreaNoticeHTML   string            `json:"UPLOAD_AREA_NOTICE_HTML"`
	UploadsListNoticeHTML  string            `json:"UPLOADS_LIST_NOTICE_HTML"`
	DownloadNoticeHTML     string            `json:"DOWNLOAD_NOTICE_HTML"`
	ShowThunderbirdSponsor bool              `json:"SHOW_THUNDERBIRD_SPONSOR"`
	Colors                 WebUIColors       `json:"COLORS"`
	CustomAssets           WebUICustomAssets `json:"CUSTOM_ASSETS"`
}

// WebUIColors is a port of the `WEB_UI.COLORS` block (nested object).
type WebUIColors struct {
	Primary string `json:"PRIMARY"`
	Accent  string `json:"ACCENT"`
}

// WebUICustomAssets is a port of the `WEB_UI.CUSTOM_ASSETS` block (nested
// object). Fields match ui_custom_assets in server/config.js.
type WebUICustomAssets struct {
	AndroidChrome192 string `json:"android_chrome_192px"`
	AndroidChrome512 string `json:"android_chrome_512px"`
	AppleTouchIcon   string `json:"apple_touch_icon"`
	Favicon16        string `json:"favicon_16px"`
	Favicon32        string `json:"favicon_32px"`
	Icon             string `json:"icon"`
	SafariPinnedTab  string `json:"safari_pinned_tab"`
	Facebook         string `json:"facebook"`
	Twitter          string `json:"twitter"`
	Wordmark         string `json:"wordmark"`
	CustomCSS        string `json:"custom_css"`
}

// DefaultsConstants is a port of the `DEFAULTS` block from clientConstants.js.
type DefaultsConstants struct {
	Downloads          int   `json:"DOWNLOADS"`
	DownloadCounts     []int `json:"DOWNLOAD_COUNTS"`
	ExpireTimesSeconds []int `json:"EXPIRE_TIMES_SECONDS"`
	ExpireSeconds      int   `json:"EXPIRE_SECONDS"`
}

// BuildClientConstants assembles the constants from the CLI config. The JSON
// format matches what Node returned byte-for-byte — the frontend's
// `app/api.js:getConstants()` sees no difference.
func BuildClientConstants(cfg *config.CLI) ClientConstants {
	return ClientConstants{
		Limits: LimitsConstants{
			MaxFileSize:        cfg.MaxFileSize,
			MaxDownloads:       cfg.MaxDownloads,
			MaxExpireSeconds:   cfg.MaxExpireSeconds,
			MaxFilesPerArchive: cfg.MaxFilesPerArchive,
			MaxArchivesPerUser: cfg.MaxArchivesPerUser,
		},
		WebUI: WebUIConstants{
			FooterDonateURL:        cfg.FooterDonateURL,
			FooterCLIURL:           cfg.FooterCLIURL,
			FooterDMCAURL:          cfg.FooterDMCAURL,
			FooterSourceURL:        cfg.FooterSourceURL,
			CustomFooterText:       cfg.CustomFooterText,
			CustomFooterURL:        cfg.CustomFooterURL,
			MainNoticeHTML:         cfg.MainNoticeHTML,
			UploadAreaNoticeHTML:   cfg.UploadAreaNoticeHTML,
			UploadsListNoticeHTML:  cfg.UploadsListNoticeHTML,
			DownloadNoticeHTML:     cfg.DownloadNoticeHTML,
			ShowThunderbirdSponsor: cfg.ShowThunderbirdSponsor,
			Colors: WebUIColors{
				Primary: cfg.UIColorPrimary,
				Accent:  cfg.UIColorAccent,
			},
			CustomAssets: WebUICustomAssets{
				AndroidChrome192: cfg.UICustomAssetsAndroidChrome192,
				AndroidChrome512: cfg.UICustomAssetsAndroidChrome512,
				AppleTouchIcon:   cfg.UICustomAssetsAppleTouchIcon,
				Favicon16:        cfg.UICustomAssetsFavicon16,
				Favicon32:        cfg.UICustomAssetsFavicon32,
				Icon:             cfg.UICustomAssetsIcon,
				SafariPinnedTab:  cfg.UICustomAssetsSafariPinnedTab,
				Facebook:         cfg.UICustomAssetsFacebook,
				Twitter:          cfg.UICustomAssetsTwitter,
				Wordmark:         cfg.UICustomAssetsWordmark,
				CustomCSS:        cfg.UICustomCSS,
			},
		},
		Defaults: DefaultsConstants{
			Downloads:          cfg.DefaultDownloads,
			DownloadCounts:     cfg.DownloadCounts,
			ExpireTimesSeconds: cfg.ExpireTimesSeconds,
			ExpireSeconds:      cfg.DefaultExpireSeconds,
		},
	}
}
