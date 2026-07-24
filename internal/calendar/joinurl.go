package calendar

import (
	"regexp"
	"strings"

	gcal "google.golang.org/api/calendar/v3"
)

var joinPatterns = []*regexp.Regexp{
	regexp.MustCompile(`https://meet\.google\.com/[a-zA-Z0-9\-_]+`),
	regexp.MustCompile(`https://[a-zA-Z0-9.-]*zoom\.us/j/[^\s<>"']+`),
	regexp.MustCompile(`https://teams\.microsoft\.com/l/meetup-join/[^\s<>"']+`),
	regexp.MustCompile(`https://[a-zA-Z0-9.-]*webex\.com/[^\s<>"']+`),
	regexp.MustCompile(`https://[a-zA-Z0-9.-]*gotomeeting\.com/[^\s<>"']+`),
}

func ExtractJoinURL(ev *gcal.Event) string {
	if ev == nil {
		return ""
	}
	if ev.HangoutLink != "" {
		return ev.HangoutLink
	}
	if ev.ConferenceData != nil {
		for _, ep := range ev.ConferenceData.EntryPoints {
			if ep == nil {
				continue
			}
			if ep.EntryPointType == "video" && ep.Uri != "" {
				return ep.Uri
			}
		}
		for _, ep := range ev.ConferenceData.EntryPoints {
			if ep != nil && ep.Uri != "" {
				return ep.Uri
			}
		}
	}
	if u := findJoinURL(ev.Location); u != "" {
		return u
	}
	if u := findJoinURL(ev.Description); u != "" {
		return u
	}
	if ev.HtmlLink != "" && strings.Contains(ev.HtmlLink, "meet.google.com") {
		return ev.HtmlLink
	}
	return ""
}

func findJoinURL(text string) string {
	if text == "" {
		return ""
	}
	for _, re := range joinPatterns {
		if m := re.FindString(text); m != "" {
			return strings.TrimRight(m, ".,);]")
		}
	}
	return ""
}
