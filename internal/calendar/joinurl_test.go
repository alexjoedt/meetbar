package calendar

import (
	"testing"

	gcal "google.golang.org/api/calendar/v3"
)

func TestExtractJoinURL(t *testing.T) {
	tests := []struct {
		name string
		ev   *gcal.Event
		want string
	}{
		{
			name: "hangout",
			ev:   &gcal.Event{HangoutLink: "https://meet.google.com/abc-defg-hij"},
			want: "https://meet.google.com/abc-defg-hij",
		},
		{
			name: "zoom in description",
			ev:   &gcal.Event{Description: "Join: https://zoom.us/j/123456789?pwd=abc thanks"},
			want: "https://zoom.us/j/123456789?pwd=abc",
		},
		{
			name: "teams in location",
			ev:   &gcal.Event{Location: "https://teams.microsoft.com/l/meetup-join/19%3ameeting"},
			want: "https://teams.microsoft.com/l/meetup-join/19%3ameeting",
		},
		{
			name: "conference data video",
			ev: &gcal.Event{
				ConferenceData: &gcal.ConferenceData{
					EntryPoints: []*gcal.EntryPoint{
						{EntryPointType: "phone", Uri: "tel:+1"},
						{EntryPointType: "video", Uri: "https://meet.google.com/xyz-uvwx-rst"},
					},
				},
			},
			want: "https://meet.google.com/xyz-uvwx-rst",
		},
		{
			name: "empty",
			ev:   &gcal.Event{Summary: "Focus time"},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractJoinURL(tt.ev)
			if got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}
