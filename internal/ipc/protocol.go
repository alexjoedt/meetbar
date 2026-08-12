package ipc

import "encoding/json"

type Request struct {
	ID     int             `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	ID     int             `json:"id"`
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

type StatusResult struct {
	Daemon   string `json:"daemon"`
	Auth     string `json:"auth"` // disconnected | connected
	Email    string `json:"email,omitempty"`
	LastSync string `json:"last_sync,omitempty"`
	SyncErr  string `json:"sync_error,omitempty"`
}

type UpcomingParams struct {
	Hours int `json:"hours"`
}

type Event struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Start        string `json:"start"`
	End          string `json:"end"`
	JoinURL      string `json:"join_url,omitempty"`
	Location     string `json:"location,omitempty"`
	CalendarID   string `json:"calendar_id"`
	AllDay       bool   `json:"all_day"`
	MinutesUntil *int   `json:"minutes_until,omitempty"`
}

type UpcomingResult struct {
	Events []Event `json:"events"`
}

// TodayResult mirrors UpcomingResult: today returns the same event shape, scoped
// to the current local day.
type TodayResult = UpcomingResult

type Alert struct {
	Key          string `json:"key"`
	EventID      string `json:"event_id"`
	ThresholdMin int    `json:"threshold_min"`
	Title        string `json:"title"`
	Start        string `json:"start"`
	JoinURL      string `json:"join_url,omitempty"`
}

type AlertsPollResult struct {
	Alerts []Alert `json:"alerts"`
}

type AlertsAckParams struct {
	Keys []string `json:"keys"`
}

type CalendarInfo struct {
	ID      string `json:"id"`
	Summary string `json:"summary"`
	Primary bool   `json:"primary"`
}

type CalendarsResult struct {
	Calendars []CalendarInfo `json:"calendars"`
}

type LoginResult struct {
	Email string `json:"email"`
}

// SyncResult is returned by a forced calendar pull.
type SyncResult struct {
	LastSync string `json:"last_sync,omitempty"`
	Events   int    `json:"events"`
}
