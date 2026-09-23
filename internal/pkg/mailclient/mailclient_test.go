package mailclient

import "testing"

func TestDetectOpens(t *testing.T) {
	cases := []struct {
		name string
		ua   string
		want Reading
	}{
		{
			name: "gmail proxy hides the device and the network",
			ua:   "Mozilla/5.0 (Windows NT 5.1; rv:11.0) Gecko Firefox/11.0 (via ggpht.com GoogleImageProxy)",
			want: Reading{Client: "Gmail", DeviceHidden: true, Locate: LocateNone},
		},
		{
			name: "yahoo proxy",
			ua:   "YahooMailProxy; https://help.yahoo.com/kb/yahoo-mail-proxy-SLN28749.html",
			want: Reading{Client: "Yahoo Mail", DeviceHidden: true, Locate: LocateNone},
		},
		{
			name: "apple mail privacy protection is the bare product token",
			ua:   "Mozilla/5.0",
			want: Reading{Client: "Apple Mail", DeviceHidden: true, Locate: LocateRegion},
		},
		{
			name: "hey's proxy wears chrome on linux",
			ua:   "hey.com/imageproxy Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36",
			want: Reading{Client: "HEY", DeviceHidden: true, Locate: LocateNone},
		},
		{
			name: "fastmail's web proxy",
			ua:   "FastmailUA/1.0",
			want: Reading{Client: "Fastmail", DeviceHidden: true, Locate: LocateNone},
		},
		{
			// Issue #564: the new Outlook for Windows sent this from the
			// reader's own network, so it is a direct fetch on an unknown device.
			name: "bare webkit is mac mail or the new outlook, on no device it can name",
			ua:   "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko)",
			want: Reading{Client: BareWebKitClient, ClientType: App},
		},
		{
			name: "outlook for ios",
			ua:   "Outlook-iOS/2.0",
			want: Reading{Client: "Outlook", ClientType: App, DeviceType: "mobile", OS: "iOS"},
		},
		{
			name: "outlook for ios keeps an ipad a tablet",
			ua:   "Mozilla/5.0 (iPad; CPU OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148 Outlook-iOS/2.0",
			want: Reading{Client: "Outlook", ClientType: App, DeviceType: "tablet", OS: "iOS"},
		},
		{
			name: "mail on an iphone",
			ua:   "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148",
			want: Reading{Client: "Apple Mail", ClientType: App, DeviceType: "mobile", OS: "iOS"},
		},
		{
			name: "mail on an ipad",
			ua:   "Mozilla/5.0 (iPad; CPU OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148",
			want: Reading{Client: "Apple Mail", ClientType: App, DeviceType: "tablet", OS: "iOS"},
		},
		{
			name: "classic outlook for windows",
			ua:   "Microsoft Office/16.0 (Windows NT 10.0; Microsoft Outlook 16.0.17928; Pro)",
			want: Reading{Client: "Outlook", ClientType: App, DeviceType: "desktop", OS: "Windows"},
		},
		{
			name: "outlook's word renderer names no platform but is only on windows",
			ua:   "Mozilla/4.0 (compatible; ms-office; MSOffice 16)",
			want: Reading{Client: "Outlook", ClientType: App, DeviceType: "desktop", OS: "Windows"},
		},
		{
			name: "thunderbird",
			ua:   "Mozilla/5.0 (X11; Linux x86_64; rv:128.0) Gecko/20100101 Thunderbird/128.2.0",
			want: Reading{Client: "Thunderbird", ClientType: App, DeviceType: "desktop", OS: "Linux"},
		},
		{
			name: "an android mail app's webview",
			ua:   "Mozilla/5.0 (Linux; Android 14; SM-S911B Build/UP1A.231005.007; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/128.0.6613.127 Mobile Safari/537.36",
			want: Reading{ClientType: App, DeviceType: "mobile", OS: "Android"},
		},
		{
			name: "webmail in chrome on windows",
			ua:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36",
			want: Reading{ClientType: Webmail, DeviceType: "desktop", OS: "Windows", Browser: "Chrome", BrowserVersion: "128.0.0.0"},
		},
		{
			name: "webmail in safari on a mac is not the bare signature",
			ua:   "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Safari/605.1.15",
			want: Reading{ClientType: Webmail, DeviceType: "desktop", OS: "macOS", Browser: "Safari", BrowserVersion: "17.5"},
		},
		{
			name: "empty says nothing",
			ua:   "  ",
			want: Reading{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Detect(tc.ua, false); got != tc.want {
				t.Fatalf("Detect(%q)\n got  %+v\n want %+v", tc.ua, got, tc.want)
			}
		})
	}
}

func TestDetectClicks(t *testing.T) {
	// A link opened from Mail lands in Safari: the click is Safari's, not
	// webmail and not Apple Mail.
	safari := "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1"
	got := Detect(safari, true)
	want := Reading{DeviceType: "mobile", OS: "iOS", Browser: "Safari", BrowserVersion: "17.5"}
	if got != want {
		t.Fatalf("safari click\n got  %+v\n want %+v", got, want)
	}

	// A client that names itself on a link made the request (Outlook checks
	// a link before handing it to the browser), so the click is still its.
	outlook := "Microsoft Office/16.0 (Windows NT 10.0; Microsoft Outlook 16.0.17928; Pro)"
	if got := Detect(outlook, true); got.Client != "Outlook" || got.ClientType != App || got.DeviceType != "desktop" {
		t.Fatalf("outlook click = %+v", got)
	}

	// A browser click names the browser and never webmail.
	chrome := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36"
	if got := Detect(chrome, true); got.Client != "" || got.ClientType != "" || got.Browser != "Chrome" {
		t.Fatalf("browser click = %+v", got)
	}

	// The stripped signature on a link names no browser the parser could
	// read honestly (it takes the platform for one), so nothing is claimed.
	if got := Detect("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko)", true); got != (Reading{}) {
		t.Fatalf("bare webkit click = %+v", got)
	}
}

func TestIsBareWebKit(t *testing.T) {
	if !IsBareWebKit("  Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) ") {
		t.Fatal("the stripped signature is bare webkit")
	}
	if IsBareWebKit("Mozilla/5.0 (KHTML, like Gecko)") {
		t.Fatal("the compatibility suffix without an engine is not")
	}
	if IsBareWebKit("Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148") {
		t.Fatal("iOS Mail's webview carries a build token after the engine")
	}
}
