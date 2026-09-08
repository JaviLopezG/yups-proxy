package bot

import (
	"strings"
)

// socialBotSignatures holds lower-cased substring patterns for known social media link crawlers
// specified in section 10.1 of Quijote specification.
// Generic bots like Googlebot or Bingbot are intentionally excluded to keep the web open.
var socialBotSignatures = []string{
	"facebookexternalhit",
	"facebookplatform",
	"twitterbot",
	"linkedinbot",
	"discordbot",
	"whatsapp",
	"telegrambot",
	"mastodon",
	"slackbot-linkexpanding",
	"slackbot",
	"pinterestbot",
	"redditbot",
	"vkshare",
	"viberbot",
	"snapchat",
	"line-crawler",
	"bluesky",
}

// IsSocialBot checks if the provided User-Agent header matches a known social media preview crawler.
func IsSocialBot(userAgent string) bool {
	if userAgent == "" {
		return false
	}
	uaLower := strings.ToLower(userAgent)
	for _, sig := range socialBotSignatures {
		if strings.Contains(uaLower, sig) {
			return true
		}
	}
	return false
}
