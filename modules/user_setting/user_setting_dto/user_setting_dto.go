// Package user_setting_dto holds a user's notification settings (Tech Spec
// "Dynamic User Notification Settings", Fase 1) plus the pure rules around
// them: defaults, tier code parsing/validation, and the mute / quiet-hours
// check. Everything here is side-effect free so both the HTTP layer and the
// reminder engine share exactly the same rules.
package user_setting_dto

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "time/tzdata"
)

const (
	// MaxTiers caps both reminderTiers and overageTiers (AC1.1).
	MaxTiers = 10
	// MinTierDuration / MaxTierDuration bound a single tier (AC1.1, AC2.2).
	MinTierDuration = time.Minute
	MaxTierDuration = 8 * time.Hour
	// DefaultOverageRepeatMinutes reproduces the legacy fixed 5-minute
	// overage repeat (AC2.4).
	DefaultOverageRepeatMinutes = 5
	// MaxMuteMinutes bounds a "mute for N minutes" request.
	MaxMuteMinutes = 24 * 60
)

// DefaultReminderTiers is the legacy fixed tier set — a user without a
// settings row behaves exactly like before this feature (AC6.1).
var DefaultReminderTiers = []string{"2h", "1h", "30m", "15m", "5m", "1m"}

// AllowedOverageRepeatMinutes are the only accepted repeat intervals (AC2.5).
var AllowedOverageRepeatMinutes = []int{5, 10, 15, 30, 60}

// Jakarta is the zone mute / quiet hours are evaluated in — the same zone as
// business hours.
var Jakarta = loadJakarta()

func loadJakarta() *time.Location {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return time.FixedZone("WIB", 7*60*60)
	}
	return loc
}

// QuietHours is a daily window (WIB) with no notifications. Start/End are
// "HH:MM"; End before Start means the window crosses midnight.
type QuietHours struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// DefaultBoardID is the boardID of a user's "default for every board" row.
const DefaultBoardID int64 = 0

// UserSetting is one user's notification settings for one scope. Tier
// slices hold canonical tier codes (see CanonicalTierCode), ordered
// largest-first.
//
// As a stored row (repository), MutedUntil is that row's own mute: the
// global mute on the default row (BoardID 0), the board mute on a board row.
// As an EFFECTIVE setting (service GetEffective*), the notification fields
// are the ones that apply to the board, MutedUntil is the global mute and
// BoardMutedUntil the board's own mute (Tech Spec "Per-Board Settings &
// Active Sprint Dashboard").
type UserSetting struct {
	UserID int64
	// BoardID is the Jira Agile board (DefaultBoardID = every board).
	BoardID int64
	// UseDefault (board rows only): notification fields follow the default
	// row; the row may still carry a board mute.
	UseDefault           bool
	ReminderTiers        []string
	OverageTiers         []string
	OverageRepeatMinutes *int
	NotifyEarlyFinish    bool
	NotifyReturnedToTodo bool
	MutedUntil           *time.Time
	BoardMutedUntil      *time.Time
	QuietHours           *QuietHours
	// NotificationsOff: the user switched automatic notifications off (or
	// hasn't switched them on yet) — Tech Spec "Notification On-Off
	// Consent". Only GetEffectiveForEmail (the reminder engine) sets it.
	NotificationsOff bool
}

// NotificationSwitch is the user's master switch for automatic
// notifications; Chosen is false until they picked on/off themselves.
type NotificationSwitch struct {
	Enabled bool
	Chosen  bool
}

// Default returns the settings every user has until they save their own.
func Default() UserSetting {
	repeat := DefaultOverageRepeatMinutes
	return UserSetting{
		ReminderTiers:        append([]string(nil), DefaultReminderTiers...),
		OverageTiers:         []string{},
		OverageRepeatMinutes: &repeat,
		NotifyEarlyFinish:    true,
		NotifyReturnedToTodo: true,
	}
}

// MuteForeverUntil is the sentinel stored for "mute selamanya" — notifications
// stay off until the user switches them back on. Using the max DATETIME keeps
// a single mutedUntil column instead of a separate flag.
var MuteForeverUntil = time.Date(9999, 12, 31, 23, 59, 59, 0, Jakarta)

// IsMutedForever reports whether the (global / row) mute is the "selamanya"
// sentinel.
func (s UserSetting) IsMutedForever() bool {
	return IsForever(s.MutedUntil)
}

// IsForever reports whether a mute timestamp is the "selamanya" sentinel.
func IsForever(until *time.Time) bool {
	return until != nil && until.Year() >= 9999
}

// MuteActive reports whether a mute timestamp is still running at now.
func MuteActive(until *time.Time, now time.Time) bool {
	return until != nil && until.After(now)
}

// IsMuted reports whether the global mute OR the board mute is running.
func (s UserSetting) IsMuted(now time.Time) bool {
	return MuteActive(s.MutedUntil, now) || MuteActive(s.BoardMutedUntil, now)
}

// InQuietHours reports whether now (evaluated in WIB) falls inside the
// daily quiet window. The window is [start, end) and may wrap midnight.
func (s UserSetting) InQuietHours(now time.Time) bool {
	if s.QuietHours == nil {
		return false
	}
	start, okStart := parseClock(s.QuietHours.Start)
	end, okEnd := parseClock(s.QuietHours.End)
	if !okStart || !okEnd || start == end {
		return false
	}
	local := now.In(Jakarta)
	minute := local.Hour()*60 + local.Minute()
	if start < end {
		return minute >= start && minute < end
	}
	return minute >= start || minute < end
}

// IsSilenced is the single check the reminder engine uses (AC4.1–AC4.3):
// notifications switched off behave like an endless mute.
func (s UserSetting) IsSilenced(now time.Time) bool {
	return s.NotificationsOff || s.IsMuted(now) || s.InQuietHours(now)
}

// tomorrowCutoffHour: before this hour (WIB) "besok" still means the coming
// morning — someone muting at 00:30 means a few hours, not ~32.
const tomorrowCutoffHour = 4

// TomorrowMorning is the "mute until tomorrow" target: 08:30 WIB on the next
// calendar day (AC4.4), or the same day when it's still before 04:00.
func TomorrowMorning(now time.Time) time.Time {
	local := now.In(Jakarta)
	next := local.AddDate(0, 0, 1)
	if local.Hour() < tomorrowCutoffHour {
		next = local
	}
	return time.Date(next.Year(), next.Month(), next.Day(), 8, 30, 0, 0, Jakarta)
}

var tierCodePattern = regexp.MustCompile(`^(\d{1,4})([mh])$`)

// ParseTierCode turns a tier code ("45m", "2h") into its duration. It does
// not enforce the allowed range — see ValidateTierCode.
func ParseTierCode(code string) (time.Duration, error) {
	m := tierCodePattern.FindStringSubmatch(strings.TrimSpace(strings.ToLower(code)))
	if m == nil {
		return 0, fmt.Errorf("format tier %q tidak valid, gunakan contoh 45m atau 2h", code)
	}
	n, _ := strconv.Atoi(m[1])
	if m[2] == "h" {
		return time.Duration(n) * time.Hour, nil
	}
	return time.Duration(n) * time.Minute, nil
}

// CanonicalTierCode renders a duration as the shortest code: whole hours as
// "<n>h", everything else as "<n>m". This keeps codes stable for the
// reminder log dedup ("60m" and "1h" are the same tier).
func CanonicalTierCode(d time.Duration) string {
	minutes := int(d / time.Minute)
	if minutes%60 == 0 {
		return fmt.Sprintf("%dh", minutes/60)
	}
	return fmt.Sprintf("%dm", minutes)
}

// TierDuration parses an already-validated canonical code, returning 0 for
// anything unparseable (the engine treats that as "never due").
func TierDuration(code string) time.Duration {
	d, err := ParseTierCode(code)
	if err != nil {
		return 0
	}
	return d
}

// NormalizeTiers validates codes (format, range, count, duplicates) and
// returns them canonicalized and sorted largest-first.
func NormalizeTiers(codes []string) ([]string, error) {
	if len(codes) > MaxTiers {
		return nil, fmt.Errorf("maksimal %d tier", MaxTiers)
	}
	seen := map[time.Duration]bool{}
	durations := make([]time.Duration, 0, len(codes))
	for _, code := range codes {
		d, err := ParseTierCode(code)
		if err != nil {
			return nil, err
		}
		if d < MinTierDuration || d > MaxTierDuration {
			return nil, fmt.Errorf("tier %q di luar rentang 1 menit sampai 8 jam", code)
		}
		if d%time.Minute != 0 {
			return nil, fmt.Errorf("tier %q harus kelipatan menit", code)
		}
		if seen[d] {
			return nil, fmt.Errorf("tier %q duplikat", code)
		}
		seen[d] = true
		durations = append(durations, d)
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] > durations[j] })
	out := make([]string, len(durations))
	for i, d := range durations {
		out[i] = CanonicalTierCode(d)
	}
	return out, nil
}

// SplitTierCSV parses a stored CSV leniently: invalid entries are dropped so
// a bad row can never stop reminders. Result is canonical, largest-first.
func SplitTierCSV(csv string) []string {
	var durations []time.Duration
	seen := map[time.Duration]bool{}
	for _, part := range strings.Split(csv, ",") {
		if strings.TrimSpace(part) == "" {
			continue
		}
		d, err := ParseTierCode(part)
		if err != nil || d < MinTierDuration || d > MaxTierDuration || seen[d] {
			continue
		}
		seen[d] = true
		durations = append(durations, d)
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] > durations[j] })
	out := make([]string, 0, len(durations))
	for _, d := range durations {
		out = append(out, CanonicalTierCode(d))
	}
	return out
}

// ValidateOverageRepeat accepts nil (stop repeating) or one of
// AllowedOverageRepeatMinutes.
func ValidateOverageRepeat(minutes *int) error {
	if minutes == nil {
		return nil
	}
	for _, allowed := range AllowedOverageRepeatMinutes {
		if *minutes == allowed {
			return nil
		}
	}
	return fmt.Errorf("interval ulang harus 5, 10, 15, 30, atau 60 menit")
}

// NormalizeQuietHours validates a quiet window (both ends "HH:MM", not
// equal) and returns it zero-padded; nil means disabled (AC4.5).
func NormalizeQuietHours(q *QuietHours) (*QuietHours, error) {
	if q == nil {
		return nil, nil
	}
	start, okStart := parseClock(q.Start)
	end, okEnd := parseClock(q.End)
	if !okStart || !okEnd {
		return nil, fmt.Errorf("jam tenang harus diisi lengkap dengan format HH:MM")
	}
	if start == end {
		return nil, fmt.Errorf("jam mulai dan selesai tidak boleh sama")
	}
	return &QuietHours{Start: formatClock(start), End: formatClock(end)}, nil
}

// parseClock parses "HH:MM" (also tolerating MySQL TIME's "HH:MM:SS") into
// minutes after midnight.
func parseClock(s string) (int, bool) {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	h, errH := strconv.Atoi(parts[0])
	m, errM := strconv.Atoi(parts[1])
	if errH != nil || errM != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

func formatClock(minutes int) string {
	return fmt.Sprintf("%02d:%02d", minutes/60, minutes%60)
}

// ClockFromDB converts a MySQL TIME value ("HH:MM:SS") to "HH:MM"; empty
// or unparseable input returns "".
func ClockFromDB(s string) string {
	minutes, ok := parseClock(s)
	if !ok {
		return ""
	}
	return formatClock(minutes)
}

// ValidationError carries a per-field message for a 400 response.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}

// NotificationInput is what the dashboard submits (everything except mute,
// which has its own instant endpoints).
type NotificationInput struct {
	ReminderTiers        []string    `json:"reminderTiers"`
	OverageTiers         []string    `json:"overageTiers"`
	OverageRepeatMinutes *int        `json:"overageRepeatMinutes"`
	NotifyEarlyFinish    bool        `json:"notifyEarlyFinish"`
	NotifyReturnedToTodo bool        `json:"notifyReturnedToTodo"`
	QuietHours           *QuietHours `json:"quietHours"`
}

// Apply validates in and returns the resulting settings for base (keeping
// base's UserID and mute). The first invalid field is reported.
func (in NotificationInput) Apply(base UserSetting) (UserSetting, error) {
	reminderTiers, err := NormalizeTiers(in.ReminderTiers)
	if err != nil {
		return UserSetting{}, &ValidationError{Field: "reminderTiers", Message: err.Error()}
	}
	overageTiers, err := NormalizeTiers(in.OverageTiers)
	if err != nil {
		return UserSetting{}, &ValidationError{Field: "overageTiers", Message: err.Error()}
	}
	if err := ValidateOverageRepeat(in.OverageRepeatMinutes); err != nil {
		return UserSetting{}, &ValidationError{Field: "overageRepeatMinutes", Message: err.Error()}
	}
	quiet, err := NormalizeQuietHours(in.QuietHours)
	if err != nil {
		return UserSetting{}, &ValidationError{Field: "quietHours", Message: err.Error()}
	}

	out := base
	out.ReminderTiers = reminderTiers
	out.OverageTiers = overageTiers
	out.OverageRepeatMinutes = in.OverageRepeatMinutes
	out.NotifyEarlyFinish = in.NotifyEarlyFinish
	out.NotifyReturnedToTodo = in.NotifyReturnedToTodo
	out.QuietHours = quiet
	return out, nil
}
