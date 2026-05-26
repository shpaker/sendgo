package usecase_test

import (
	"testing"

	"github.com/sendgo/sendgo/server/internal/config"
	"github.com/sendgo/sendgo/server/internal/usecase"
)

func TestBuildClientConstants(t *testing.T) {
	cfg := &config.CLI{
		MaxFileSize:        1024,
		MaxDownloads:       50,
		MaxExpireSeconds:   604800,
		MaxFilesPerArchive: 64,
		MaxArchivesPerUser: 16,

		FooterDonateURL:        "https://donate.example",
		FooterCLIURL:           "https://cli.example",
		FooterDMCAURL:          "https://dmca.example",
		FooterSourceURL:        "https://source.example",
		CustomFooterText:       "footer-text",
		CustomFooterURL:        "https://footer.example",
		MainNoticeHTML:         "<main/>",
		UploadAreaNoticeHTML:   "<upload/>",
		UploadsListNoticeHTML:  "<list/>",
		DownloadNoticeHTML:     "<dl/>",
		ShowThunderbirdSponsor: true,
		UIColorPrimary:         "#111111",
		UIColorAccent:          "#222222",

		UICustomAssetsAndroidChrome192: "a192",
		UICustomAssetsAndroidChrome512: "a512",
		UICustomAssetsAppleTouchIcon:   "apple",
		UICustomAssetsFavicon16:        "f16",
		UICustomAssetsFavicon32:        "f32",
		UICustomAssetsIcon:             "icon",
		UICustomAssetsSafariPinnedTab:  "safari",
		UICustomAssetsFacebook:         "fb",
		UICustomAssetsTwitter:          "tw",
		UICustomAssetsWordmark:         "wm",
		UICustomCSS:                    "/* css */",

		DefaultDownloads:     1,
		DownloadCounts:       []int{1, 2, 3},
		ExpireTimesSeconds:   []int{60, 300, 3600},
		DefaultExpireSeconds: 300,
	}

	got := usecase.BuildClientConstants(cfg)

	if got.Limits.MaxFileSize != 1024 || got.Limits.MaxDownloads != 50 ||
		got.Limits.MaxExpireSeconds != 604800 || got.Limits.MaxFilesPerArchive != 64 ||
		got.Limits.MaxArchivesPerUser != 16 {
		t.Errorf("Limits mismatch: %+v", got.Limits)
	}
	if got.WebUI.FooterDonateURL != "https://donate.example" ||
		got.WebUI.FooterCLIURL != "https://cli.example" ||
		got.WebUI.FooterDMCAURL != "https://dmca.example" ||
		got.WebUI.FooterSourceURL != "https://source.example" ||
		got.WebUI.CustomFooterText != "footer-text" ||
		got.WebUI.CustomFooterURL != "https://footer.example" ||
		got.WebUI.MainNoticeHTML != "<main/>" ||
		got.WebUI.UploadAreaNoticeHTML != "<upload/>" ||
		got.WebUI.UploadsListNoticeHTML != "<list/>" ||
		got.WebUI.DownloadNoticeHTML != "<dl/>" ||
		!got.WebUI.ShowThunderbirdSponsor {
		t.Errorf("WebUI mismatch: %+v", got.WebUI)
	}
	if got.WebUI.Colors.Primary != "#111111" || got.WebUI.Colors.Accent != "#222222" {
		t.Errorf("Colors mismatch: %+v", got.WebUI.Colors)
	}
	a := got.WebUI.CustomAssets
	if a.AndroidChrome192 != "a192" || a.AndroidChrome512 != "a512" ||
		a.AppleTouchIcon != "apple" || a.Favicon16 != "f16" || a.Favicon32 != "f32" ||
		a.Icon != "icon" || a.SafariPinnedTab != "safari" || a.Facebook != "fb" ||
		a.Twitter != "tw" || a.Wordmark != "wm" || a.CustomCSS != "/* css */" {
		t.Errorf("CustomAssets mismatch: %+v", a)
	}
	if got.Defaults.Downloads != 1 || got.Defaults.ExpireSeconds != 300 ||
		len(got.Defaults.DownloadCounts) != 3 || len(got.Defaults.ExpireTimesSeconds) != 3 {
		t.Errorf("Defaults mismatch: %+v", got.Defaults)
	}
}
