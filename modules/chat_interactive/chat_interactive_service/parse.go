package chat_interactive_service

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"nudge/modules/user_setting/user_setting_dto"
)

var wib = user_setting_dto.Jakarta

var (
	issueKeyPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*-\d+$`)
	// legacyGeserPattern: the old /n-geser<KEY>:target form.
	legacyGeserPattern = regexp.MustCompile(`^/n-geser<([A-Za-z][A-Za-z0-9]*-\d+)>:(\S+)$`)
)

// Command names and the words (typed with or without "/n-") that reach
// them. Old command names stay as aliases so nobody's habit breaks.
const (
	cmdDashboard = "dashboard"
	cmdTiket     = "tiket"
	cmdCuti      = "cuti"
	cmdMute      = "mute"
	cmdUnmute    = "unmute"
	cmdHelp      = "help"
)

var commandAliases = map[string]string{
	"dashboard": cmdDashboard, "home": cmdDashboard, "ringkasan": cmdDashboard,
	"overdue": cmdDashboard, "team": cmdDashboard,
	"tiket": cmdTiket, "ticket": cmdTiket, "tickets": cmdTiket, "papan": cmdTiket, "board": cmdTiket,
	"mytickets": cmdTiket, "budget": cmdTiket, "geser": cmdTiket,
	"cuti": cmdCuti, "leave": cmdCuti, "libur": cmdCuti,
	"mute": cmdMute, "snooze": cmdMute, "diam": cmdMute,
	"unmute": cmdUnmute, "nyalakan": cmdUnmute,
	"help": cmdHelp, "menu": cmdHelp, "bantuan": cmdHelp, "start": cmdHelp,
}

// parseCommand splits "/n-tiket CORE-1 review" into ("tiket", "CORE-1
// review"). known is false for text that isn't a command (the menu answers).
func parseCommand(text string) (name, args string, known bool) {
	text = strings.TrimSpace(text)
	if m := legacyGeserPattern.FindStringSubmatch(text); m != nil {
		return cmdTiket, m[1] + " " + m[2], true
	}
	word, rest, _ := strings.Cut(text, " ")
	word = strings.ToLower(word)
	word = strings.TrimPrefix(word, "/")
	word = strings.TrimPrefix(word, "n-")
	name, known = commandAliases[word]
	return name, strings.TrimSpace(rest), known
}

// normalizeKey upper-cases an issue key ("core-12" → "CORE-12").
func normalizeKey(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if !issueKeyPattern.MatchString(s) {
		return "", false
	}
	return strings.ToUpper(s), true
}

// squash keeps only lower-case letters and digits: "Code Review" → "codereview".
func squash(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// statusShortcuts expand short status words people type. Projects name
// statuses differently (CORE "CODE REVIEW" / "Code Completed", OMS "Code
// Review" / "Done"), so a word can stand for several names, tried in order.
var statusShortcuts = map[string][]string{
	"cr": {"codereview"}, "review": {"codereview"},
	"cp": {"codeprocess"}, "process": {"codeprocess"}, "progress": {"inprogress", "codeprocess"},
	"todo": {"todo"},
	"done": {"done", "codecompleted"}, "selesai": {"done", "codecompleted"},
	"completed": {"codecompleted", "done"},
}

// matchStatus picks the one candidate status the typed target means:
// exact match first (shortcut names in order), then a unique partial
// match. ok is false when nothing or more than one candidate matches.
func matchStatus(target string, candidates []string) (int, bool) {
	t := squash(target)
	if t == "" {
		return 0, false
	}
	exact := []string{t}
	if s, ok := statusShortcuts[t]; ok {
		exact = s
		t = s[0]
	}
	for _, want := range exact {
		for i, c := range candidates {
			if squash(c) == want {
				return i, true
			}
		}
	}
	found := -1
	for i, c := range candidates {
		if strings.Contains(squash(c), t) {
			if found >= 0 {
				return 0, false
			}
			found = i
		}
	}
	return found, found >= 0
}

// --- dates -----------------------------------------------------------------

func today(now time.Time) time.Time {
	n := now.In(wib)
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, wib)
}

var monthWords = map[string]time.Month{
	"jan": 1, "januari": 1, "feb": 2, "februari": 2, "mar": 3, "maret": 3,
	"apr": 4, "april": 4, "mei": 5, "may": 5, "jun": 6, "juni": 6,
	"jul": 7, "juli": 7, "agu": 8, "agt": 8, "agustus": 8, "aug": 8,
	"sep": 9, "september": 9, "okt": 10, "oktober": 10, "oct": 10,
	"nov": 11, "november": 11, "des": 12, "desember": 12, "dec": 12,
}

var numericDate = regexp.MustCompile(`^(\d{1,2})[/.-](\d{1,2})(?:[/.-](\d{2,4}))?$`)

// parseDate reads one date token; tokens may be two words ("20 nov").
// A date without a year is the next such date from today.
func parseDate(tokens []string, now time.Time) (time.Time, int, bool) {
	if len(tokens) == 0 {
		return time.Time{}, 0, false
	}
	base := today(now)
	first := strings.ToLower(tokens[0])
	switch first {
	case "hari":
		if len(tokens) > 1 && strings.EqualFold(tokens[1], "ini") {
			return base, 2, true
		}
	case "today", "hariini":
		return base, 1, true
	case "besok", "tomorrow":
		return base.AddDate(0, 0, 1), 1, true
	case "lusa":
		return base.AddDate(0, 0, 2), 1, true
	}
	if t, err := time.ParseInLocation("2006-01-02", first, wib); err == nil {
		return t, 1, true
	}
	if m := numericDate.FindStringSubmatch(first); m != nil {
		day, _ := strconv.Atoi(m[1])
		month, _ := strconv.Atoi(m[2])
		year := 0
		if m[3] != "" {
			year, _ = strconv.Atoi(m[3])
			if year < 100 {
				year += 2000
			}
		}
		if t, ok := buildDate(day, time.Month(month), year, base); ok {
			return t, 1, true
		}
		return time.Time{}, 0, false
	}
	if day, err := strconv.Atoi(first); err == nil && len(tokens) > 1 {
		if month, ok := monthWords[strings.ToLower(tokens[1])]; ok {
			used, year := 2, 0
			if len(tokens) > 2 {
				if y, err := strconv.Atoi(tokens[2]); err == nil && y >= 2000 && y < 2100 {
					used, year = 3, y
				}
			}
			if t, ok := buildDate(day, month, year, base); ok {
				return t, used, true
			}
		}
	}
	return time.Time{}, 0, false
}

func buildDate(day int, month time.Month, year int, base time.Time) (time.Time, bool) {
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return time.Time{}, false
	}
	y := year
	if y == 0 {
		y = base.Year()
	}
	t := time.Date(y, month, day, 0, 0, 0, 0, wib)
	if t.Day() != day { // 31/02 rolls over
		return time.Time{}, false
	}
	if year == 0 && t.Before(base.AddDate(0, 0, -7)) {
		t = t.AddDate(1, 0, 0) // "5/1" typed in December means next January
	}
	return t, true
}

var rangeWords = map[string]bool{"-": true, "s/d": true, "sd": true, "sampai": true, "hingga": true, "to": true, "until": true, "–": true}

// parseLeaveArgs reads "<date> [- <date>] [note]".
func parseLeaveArgs(args string, now time.Time) (from, to time.Time, note string, ok bool) {
	tokens := strings.Fields(args)
	from, n, ok := parseDate(tokens, now)
	if !ok {
		return time.Time{}, time.Time{}, "", false
	}
	tokens = tokens[n:]
	to = from
	if len(tokens) > 0 && rangeWords[strings.ToLower(tokens[0])] {
		end, n2, ok2 := parseDate(tokens[1:], now)
		if !ok2 {
			return time.Time{}, time.Time{}, "", false
		}
		to, tokens = end, tokens[1+n2:]
	} else if end, n2, ok2 := parseDate(tokens, now); ok2 {
		to, tokens = end, tokens[n2:]
	}
	return from, to, strings.Join(tokens, " "), true
}

// --- mute durations ----------------------------------------------------------

// muteChoice is a mute target (always time-limited: the forever mute was
// removed on 2026-11-17, web and chat alike).
type muteChoice struct {
	until time.Time
}

var durationPattern = regexp.MustCompile(`^(\d{1,4})\s*(m|mnt|menit|min|minute|minutes|j|jam|h|hr|hour|hours)?$`)

// isForeverMute: the words the removed "mute forever" option used to accept
// (typed, or sent by the "Matikan" button of cards still in chats).
func isForeverMute(arg string) bool {
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "selamanya", "forever", "off", "mati", "matikan":
		return true
	}
	return false
}

// parseMute reads "30m", "1 jam", "90", "besok".
func parseMute(arg string, now time.Time) (muteChoice, bool) {
	a := strings.ToLower(strings.TrimSpace(arg))
	switch a {
	case "besok", "tomorrow", "sampai besok", "besok pagi":
		return muteChoice{until: user_setting_dto.TomorrowMorning(now)}, true
	}
	m := durationPattern.FindStringSubmatch(a)
	if m == nil {
		return muteChoice{}, false
	}
	n, _ := strconv.Atoi(m[1])
	minutes := n
	switch m[2] {
	case "j", "jam", "h", "hr", "hour", "hours":
		minutes = n * 60
	}
	if minutes < 1 || minutes > user_setting_dto.MaxMuteMinutes {
		return muteChoice{}, false
	}
	return muteChoice{until: now.Add(time.Duration(minutes) * time.Minute)}, true
}

// --- labels ------------------------------------------------------------------

var (
	weekdayShort = [...]string{"Min", "Sen", "Sel", "Rab", "Kam", "Jum", "Sab"}
	monthShort   = [...]string{"", "Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}
)

// dayLabel: "Jum, 20 Nov 2026".
func dayLabel(t time.Time) string {
	t = t.In(wib)
	return fmt.Sprintf("%s, %d %s %d", weekdayShort[t.Weekday()], t.Day(), monthShort[t.Month()], t.Year())
}

// whenLabel: "15:30" today, "besok 08:30", or "Jum, 20 Nov 08:30".
func whenLabel(t, now time.Time) string {
	t, base := t.In(wib), today(now)
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, wib)
	clock := t.Format("15:04")
	switch {
	case day.Equal(base):
		return clock
	case day.Equal(base.AddDate(0, 0, 1)):
		return "besok " + clock
	}
	return fmt.Sprintf("%s, %d %s %s", weekdayShort[t.Weekday()], t.Day(), monthShort[t.Month()], clock)
}

// humanDuration: "2 jam 10 menit", "45 menit", "1 hari 3 jam".
func humanDuration(seconds int64) string {
	if seconds < 0 {
		seconds = -seconds
	}
	d, h, m := seconds/86400, (seconds%86400)/3600, (seconds%3600)/60
	var parts []string
	if d > 0 {
		parts = append(parts, fmt.Sprintf("%d hari", d))
	}
	if h > 0 {
		parts = append(parts, fmt.Sprintf("%d jam", h))
	}
	if m > 0 && d == 0 {
		parts = append(parts, fmt.Sprintf("%d menit", m))
	}
	if len(parts) == 0 {
		return "< 1 menit"
	}
	return strings.Join(parts, " ")
}

// muteLabel describes a running mute.
func muteLabel(until *time.Time, now time.Time) string {
	if user_setting_dto.IsForever(until) {
		return "dimatikan sampai kamu nyalakan lagi"
	}
	return "di-mute sampai " + whenLabel(*until, now) + " WIB"
}
