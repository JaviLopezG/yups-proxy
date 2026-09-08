package bot

import (
	"testing"
)

func TestIsSocialBot(t *testing.T) {
	tests := []struct {
		name      string
		userAgent string
		expected  bool
	}{
		{"TelegramBot crawler", "TelegramBot (like TwitterBot)", true},
		{"Twitterbot crawler", "Twitterbot/1.0", true},
		{"Facebook crawler 1", "facebookexternalhit/1.1 (+http://www.facebook.com/externalhit_uatext.php)", true},
		{"Facebook platform", "facebookplatform/1.0", true},
		{"LinkedInBot crawler", "LinkedInBot/1.0 (compatible; Mozilla/5.0; Apache-HttpClient +http://www.linkedin.com)", true},
		{"Discordbot crawler", "Mozilla/5.0 (compatible; Discordbot/2.0; +https://discordapp.com)", true},
		{"WhatsApp preview", "WhatsApp/2.21.12.21 A", true},
		{"Slackbot expander", "Slackbot-LinkExpanding 1.0 (+https://api.slack.com/robots)", true},
		{"Slackbot generic", "Slackbot 1.0 (+https://api.slack.com/robots)", true},
		{"Mastodon crawler", "Mastodon/4.2.0 (compatible; MastodonBot)", true},
		{"Pinterestbot", "Pinterestbot/1.0", true},
		{"Redditbot", "redditbot/1.0 (+http://www.reddit.com/feedback)", true},
		{"VK share", "vkShare; +http://vk.com/dev/Share", true},
		{"ViberBot", "ViberBot", true},
		{"Snapchat", "Snapchat/11.0", true},
		{"Line-Crawler", "Line-Crawler/0.1.0", true},
		{"Bluesky crawler", "Bluesky Web Crawler 1.0", true},
		{"Standard Chrome browser", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36", false},
		{"Standard Firefox browser", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/119.0", false},
		{"Mobile Safari browser", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1", false},
		{"Googlebot search crawler", "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)", false},
		{"Empty User-Agent", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSocialBot(tt.userAgent)
			if got != tt.expected {
				t.Errorf("for UA %q: got %v, want %v", tt.userAgent, got, tt.expected)
			}
		})
	}
}
