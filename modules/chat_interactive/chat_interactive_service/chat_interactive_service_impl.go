package chat_interactive_service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"nudge/modules/move_fields"
	"nudge/modules/registration/registration_dto"
	"nudge/modules/user_dashboard/user_dashboard_dto"
	"nudge/modules/user_dashboard/user_dashboard_service"
	"nudge/modules/user_leave/user_leave_dto"
	"nudge/modules/user_leave/user_leave_service"
	"nudge/modules/user_setting/user_setting_dto"
)

// Action functions (sent as ActionPrefix + name).
const (
	actCommand     = "cmd"          // param cmd: run a command
	actTicket      = "ticket"       // param key: open one ticket
	actMove        = "move"         // params key, tid, to: move a ticket
	actLeaveAdd    = "leave.add"    // form from, to, note
	actLeaveRemove = "leave.remove" // param date
	actMute        = "mute"         // param for: minutes | tomorrow (an old card's "selamanya" is refused)
	actUnmute      = "unmute"
	actScope       = "scope" // params board (0 = all), then: switch board, show then again
)

// Form field names of the leave card.
const (
	fieldFrom = "from"
	fieldTo   = "to"
	fieldNote = "note"
)

// maxTicketRows bounds the Papan Tiket card (Chat cards have a size limit).
const maxTicketRows = 60

type UserFinder interface {
	FindUserByEmail(ctx context.Context, email string) (*registration_dto.User, bool, error)
}

// Dashboard is the user_dashboard_service slice the commands use — the same
// calls the web dashboard and Papan Tiket make.
type Dashboard interface {
	GetMyBoard(ctx context.Context, email string, boardID int64) (user_dashboard_dto.Board, error)
	DescribeBoard(ctx context.Context, boardID int64) (name, projectKey string)
	ListTransitions(ctx context.Context, email, issueKey string) (user_dashboard_dto.TicketTransitions, error)
	Transition(ctx context.Context, email, issueKey, transitionID string) (string, error)
}

type Leaves interface {
	List(ctx context.Context, email string) ([]user_leave_dto.Leave, error)
	Add(ctx context.Context, email, from, to, note string) (user_leave_dto.AddResult, error)
	Remove(ctx context.Context, email, date string) (bool, error)
}

type Settings interface {
	GetEffective(ctx context.Context, userID, boardID int64) (user_setting_dto.UserSetting, error)
	MuteUntil(ctx context.Context, userID, boardID int64, until time.Time) error
	Unmute(ctx context.Context, userID, boardID int64) error
	NotificationSwitch(ctx context.Context, userID int64) (user_setting_dto.NotificationSwitch, error)
}

// BoardScope is the board picked in the web navbar (0 = every board).
type BoardScope interface {
	Get(ctx context.Context, email string) (int64, error)
	Set(ctx context.Context, email string, boardID int64) error
}

type Deps struct {
	Users     UserFinder
	Dashboard Dashboard
	Leaves    Leaves
	Settings  Settings
	// Scope (optional) makes /n-mytickets, /n-dashboard and /n-mute follow
	// the web's board choice.
	Scope BoardScope
	// WebURL is the dashboard root, e.g. https://nudge.labcup.tech.
	WebURL string
	Now    func() time.Time
}

type service struct {
	d Deps
}

func New(d Deps) ChatInteractiveService {
	if d.Now == nil {
		d.Now = time.Now
	}
	d.WebURL = strings.TrimRight(d.WebURL, "/")
	if d.WebURL == "" {
		d.WebURL = defaultWebURL
	}
	return &service{d: d}
}

const (
	defaultWebURL = "https://nudge.labcup.tech"
	errorBody     = "Lagi ada gangguan, coba lagi sebentar lagi ya 🙏"
	jiraBody      = "Jira lagi nggak bisa dihubungi, coba lagi sebentar lagi ya 🙏"
)

func (s *service) RouteCommand(ctx context.Context, email, text string) string {
	return s.Command(ctx, email, text).Text
}

func (s *service) user(ctx context.Context, email string) (*registration_dto.User, *view) {
	u, found, err := s.d.Users.FindUserByEmail(ctx, email)
	switch {
	case err != nil:
		log.Printf("chat_interactive: FindUserByEmail(%s) failed: %v", email, err)
		v := note("Ups", errorBody)
		return nil, &v
	case !found:
		v := note("Belum terdaftar", fmt.Sprintf("Kamu belum terdaftar di Nudge. Login dulu di %s ya 🙏", s.d.WebURL+"/"))
		return nil, &v
	}
	return u, nil
}

func (s *service) Command(ctx context.Context, email, text string) Reply {
	u, fail := s.user(ctx, email)
	if fail != nil {
		return fail.reply()
	}
	name, args, known := parseCommand(text)
	if !known {
		return s.menu(u, strings.TrimSpace(text)).reply()
	}
	switch name {
	case cmdDashboard:
		return s.dashboard(ctx, u).reply()
	case cmdTiket:
		return s.tiketCommand(ctx, email, args).reply()
	case cmdCuti:
		return s.cutiCommand(ctx, email, args).reply()
	case cmdMute:
		return s.muteCommand(ctx, u, args).reply()
	case cmdUnmute:
		return s.unmute(ctx, u, "").reply()
	}
	return s.menu(u, "").reply()
}

func (s *service) Action(ctx context.Context, email, function string, params map[string]string) ActionReply {
	u, fail := s.user(ctx, email)
	if fail != nil {
		return ActionReply{Reply: fail.reply()}
	}
	switch strings.TrimPrefix(function, ActionPrefix) {
	case actCommand:
		return ActionReply{Reply: s.Command(ctx, email, params["cmd"])}
	case actTicket:
		return ActionReply{Reply: s.ticketDetail(ctx, email, params["key"], "").reply()}
	case actMove:
		return ActionReply{Reply: s.move(ctx, email, params["key"], params["tid"], params["to"]).reply(), Update: true}
	case actLeaveAdd:
		return ActionReply{Reply: s.addLeave(ctx, email, params[fieldFrom], params[fieldTo], params[fieldNote]).reply(), Update: true}
	case actLeaveRemove:
		return ActionReply{Reply: s.removeLeave(ctx, email, params["date"]).reply(), Update: true}
	case actMute:
		return ActionReply{Reply: s.mute(ctx, u, params["for"]).reply(), Update: true}
	case actUnmute:
		return ActionReply{Reply: s.unmute(ctx, u, "").reply(), Update: true}
	case actScope:
		id, _ := strconv.ParseInt(params["board"], 10, 64)
		s.saveBoard(ctx, email, max(id, 0))
		then := params["then"]
		if then != "/n-dashboard" && then != "/n-mute" {
			then = "/n-mytickets"
		}
		return ActionReply{Reply: s.Command(ctx, email, then), Update: true}
	}
	return ActionReply{Reply: s.menu(u, "").reply()}
}

// --- menu ----------------------------------------------------------------------

func (s *service) menu(u *registration_dto.User, typed string) view {
	greeting := "Hai! Mau ngapain hari ini?"
	if name := firstName(u.UserName); name != "" {
		greeting = "Hai " + name + "! Mau ngapain hari ini?"
	}
	if typed != "" && strings.HasPrefix(typed, "/") {
		greeting = fmt.Sprintf("Command %q nggak dikenal. Pilih menu di bawah ya 👇", typed)
	}
	entry := func(main, bottom, cmd string) item {
		b := action("Buka", actCommand, "cmd", cmd)
		return item{row: &row{main: main, bottom: bottom, button: &b}}
	}
	return view{
		id: "nudge-menu", title: "Nudge", subtitle: "Pengingat budget waktu tiket Jira",
		sections: []section{
			{items: []item{paragraph(greeting)}},
			{items: []item{
				entry("📋 *Tiket Saya*", "Semua tiket sprint aktif + deadline — geser status dari sini", "/n-mytickets"),
				entry("📊 *Dashboard*", "Tiket yang lagi dikerjakan, sisa budget & overdue", "/n-dashboard"),
				entry("🌴 *Cuti*", "Lihat, tambah atau hapus hari cuti", "/n-cuti"),
				entry("🔕 *Mute notifikasi*", "Diamkan pengingat sementara", "/n-mute"),
			}},
			{header: "Bisa juga diketik", collapseAfter: 1, items: []item{
				paragraph("*/n-mytickets* — semua tiket kamu + deadline · */n-dashboard* — ringkasan\n" +
					"*/n-tiket CORE-123* — buka 1 tiket\n" +
					"*/n-tiket CORE-123 review* — langsung geser ke Code Review\n" +
					"*/n-cuti besok* · */n-cuti 20/11* · */n-cuti 20/11 - 22/11 Liburan*\n" +
					"*/n-mute 1 jam* · */n-mute besok* · */n-unmute*"),
			}},
			{items: []item{buttons(link("Buka Nudge Web", s.d.WebURL+"/dashboard"))}},
		},
		tip: "Ketik */n-mytickets*, */n-dashboard*, */n-cuti*, */n-mute* atau */n-unmute*.",
	}
}

func firstName(name string) string {
	f := strings.Fields(name)
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

// --- dashboard -----------------------------------------------------------------

// working: the ticket's clock is running (Code Process; for CORE also
// Code Review) — the same rule the web dashboard uses (HasBudgetDetail).
func working(c user_dashboard_dto.Card) bool { return c.HasBudgetDetail }

func overdue(c user_dashboard_dto.Card) bool {
	return c.HasBudgetDetail && c.ElapsedSeconds > c.BudgetSeconds
}

func (s *service) dashboard(ctx context.Context, u *registration_dto.User) view {
	sc, err := s.scoped(ctx, u.Email, "")
	if err != nil {
		log.Printf("chat_interactive: GetMyBoard(%s) failed: %v", u.Email, err)
		return note("Dashboard", jiraBody)
	}
	now := s.d.Now()

	var all, active, branches []user_dashboard_dto.Card
	counts := map[string]int{}
	var order []string
	for _, g := range sc.picked {
		for _, c := range g.cards {
			if _, seen := counts[c.Status]; !seen {
				order = append(order, c.Status)
			}
			counts[c.Status]++
			all = append(all, c)
		}
		branches = append(branches, g.branches...)
	}
	for _, c := range append(append([]user_dashboard_dto.Card{}, all...), branches...) {
		if working(c) {
			active = append(active, c)
		}
	}
	sort.SliceStable(active, func(i, j int) bool { return remaining(active[i]) < remaining(active[j]) })
	late := 0
	for _, c := range active {
		if overdue(c) {
			late++
		}
	}

	var summary []string
	for _, st := range order {
		summary = append(summary, fmt.Sprintf("%s *%d*", st, counts[st]))
	}
	overview := fmt.Sprintf("*%d tiket* di sprint aktif", len(all))
	if len(summary) > 0 {
		overview += "\n" + strings.Join(summary, " · ")
	}
	if late > 0 {
		overview += fmt.Sprintf("\n🔴 *%d tiket lewat budget*", late)
	} else if len(active) > 0 {
		overview += "\n🟢 Semua tiket yang dikerjakan masih dalam budget"
	}

	sections := []section{{items: []item{paragraph(overview)}}}
	if sc.note != "" {
		sections = append([]section{{items: []item{paragraph(sc.note)}}}, sections...)
	}
	if len(active) > 0 {
		var items []item
		for _, c := range active {
			items = append(items, ticketRow(c, now, true))
		}
		sections = append(sections, section{header: "⏱ Sedang dikerjakan", items: items})
	} else if len(all) > 0 {
		sections = append(sections, section{items: []item{paragraph("_Belum ada tiket yang sedang dikerjakan._")}})
	}

	var status []item
	if until := s.activeMute(ctx, u.UserID, sc.id, now); until != nil {
		b := action("Nyalakan", actUnmute)
		status = append(status, item{row: &row{main: "🔕 Notifikasi " + muteLabel(until, now), button: &b}})
	} else {
		b := action("Mute", actCommand, "cmd", "/n-mute")
		status = append(status, item{row: &row{main: "🔔 Notifikasi aktif", button: &b}})
	}
	if next := s.upcomingLeaves(ctx, u.Email, now); len(next) > 0 {
		b := action("Atur", actCommand, "cmd", "/n-cuti")
		status = append(status, item{row: &row{main: "🌴 Cuti berikutnya: " + dayLabel(mustDate(next[0].Date)), button: &b}})
	}
	sections = append(sections, section{header: "Status", items: status})
	sections = append(sections, s.scopeSection(sc, "/n-dashboard"))
	sections = append(sections, section{items: []item{buttons(
		action("📋 Tiket Saya", actCommand, "cmd", "/n-mytickets"),
		action("🌴 Cuti", actCommand, "cmd", "/n-cuti"),
		link("Buka di web", s.d.WebURL+"/dashboard"),
	)}})

	return view{
		id: "nudge-dashboard", title: "📊 Dashboard", subtitle: sc.label() + " · " + dayLabel(now),
		sections: sections,
		tip:      "Buka satu tiket: */n-tiket CORE-123*",
	}
}

func remaining(c user_dashboard_dto.Card) int64 { return c.BudgetSeconds - c.ElapsedSeconds }

// ticketRow is one ticket line; withButton adds the "Geser" button.
func ticketRow(c user_dashboard_dto.Card, now time.Time, withButton bool) item {
	r := &row{top: c.Status, main: "*" + c.IssueKey + "* " + c.Summary, bottom: budgetLine(c, now)}
	if c.StoryPoints > 0 {
		r.top += fmt.Sprintf(" · %s SP", spLabel(c.StoryPoints))
	}
	if c.ParentKey != "" {
		r.top += " · " + relationLabel(c) + " " + c.ParentKey
	}
	if withButton {
		b := action("Geser", actTicket, "key", c.IssueKey)
		r.button = &b
	}
	return item{row: r}
}

func relationLabel(c user_dashboard_dto.Card) string {
	if c.Relation == "subtask" {
		return "subtask dari"
	}
	return c.Relation
}

// spLabel: "3", "1.5".
func spLabel(sp float64) string {
	return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.1f", sp), "0"), ".")
}

// budgetLine is a ticket's time info: running clock (sisa / lewat, waktu
// berjalan, deadline), recorded time, or the budget it will get.
func budgetLine(c user_dashboard_dto.Card, now time.Time) string {
	deadline := ""
	if t, err := time.Parse(time.RFC3339, c.DeadlineAt); err == nil {
		deadline = whenLabel(t, now) + " WIB"
	}
	switch {
	case c.HasBudgetDetail && overdue(c):
		return fmt.Sprintf("🔴 Lewat %s · berjalan %s dari budget %s", humanDuration(-remaining(c)),
			humanDuration(c.ElapsedSeconds), humanDuration(c.BudgetSeconds))
	case c.HasBudgetDetail:
		line := fmt.Sprintf("⏱ Sisa %s · berjalan %s", humanDuration(remaining(c)), humanDuration(c.ElapsedSeconds))
		if deadline != "" {
			line += " · deadline " + deadline
		}
		return line
	case c.TimeRecorded:
		return fmt.Sprintf("✔ Tercatat %s dari budget %s", humanDuration(c.ElapsedSeconds), humanDuration(c.BudgetSeconds))
	case c.StoryPoints > 0:
		return "Budget " + humanDuration(int64(c.StoryPoints*3600)) + " · dihitung saat masuk Code Process"
	}
	return ""
}

func cardsOf(b user_dashboard_dto.Board) []user_dashboard_dto.Card {
	var out []user_dashboard_dto.Card
	for _, col := range b.Columns {
		out = append(out, col.Cards...)
	}
	return out
}

// findCard looks key up among the caller's active-sprint tickets.
func findCard(b user_dashboard_dto.Board, key string) (user_dashboard_dto.Card, bool) {
	for _, col := range b.Columns {
		for _, c := range col.Cards {
			if strings.EqualFold(c.IssueKey, key) {
				return c, true
			}
		}
	}
	for _, c := range b.Related {
		if strings.EqualFold(c.IssueKey, key) {
			return c, true
		}
	}
	return user_dashboard_dto.Card{}, false
}

func notOwnView(key string) view {
	return note("📋 "+key, fmt.Sprintf("*%s* bukan tiket kamu di sprint aktif, jadi nggak bisa dibuka atau digeser dari sini 🙏", key))
}

// ticketDetail shows one ticket with a button per status its workflow
// allows. A typed target moves it right away when it names one status.
func (s *service) ticketDetail(ctx context.Context, email, key, target string) view {
	key, ok := normalizeKey(key)
	if !ok {
		return note("📋 Tiket Saya", "Kode tiketnya nggak valid. Contoh: */n-tiket CORE-123*")
	}
	trs, err := s.d.Dashboard.ListTransitions(ctx, email, key)
	if err != nil {
		if errors.Is(err, user_dashboard_service.ErrNotOwnTicket) {
			return notOwnView(key)
		}
		log.Printf("chat_interactive: ListTransitions(%s, %s) failed: %v", email, key, err)
		return note("📋 "+key, jiraBody)
	}

	var moves []user_dashboard_dto.TransitionOption
	for _, tr := range trs.Transitions {
		if !strings.EqualFold(tr.ToStatus, trs.CurrentStatus) {
			moves = append(moves, tr)
		}
	}
	hint := ""
	if target != "" {
		names := make([]string, len(moves))
		for i, m := range moves {
			names[i] = m.ToStatus
		}
		if i, ok := matchStatus(target, names); ok {
			return s.move(ctx, email, key, moves[i].ID, moves[i].ToStatus)
		}
		hint = fmt.Sprintf("Status %q nggak ketemu dari status sekarang. Pilih salah satu di bawah 👇", target)
	}

	now := s.d.Now()
	card := user_dashboard_dto.Card{IssueKey: key, Status: trs.CurrentStatus}
	if board, err := s.d.Dashboard.GetMyBoard(ctx, email, 0); err == nil {
		if c, found := findCard(board, key); found {
			card = c
		}
	}

	info := []string{"*Status:* " + card.Status}
	if card.ParentKey != "" {
		info = append(info, fmt.Sprintf("*Induk:* %s %s — %s", relationLabel(card), card.ParentKey, card.ParentSummary))
	}
	if card.StoryPoints > 0 {
		info = append(info, fmt.Sprintf("*Story Point:* %.1f SP (budget %s)", card.StoryPoints, humanDuration(int64(card.StoryPoints*3600))))
	}
	if card.HasBudgetDetail {
		info = append(info, "*Waktu berjalan:* "+humanDuration(card.ElapsedSeconds))
		if overdue(card) {
			info = append(info, "*Lewat budget:* 🔴 "+humanDuration(-remaining(card)))
		} else {
			info = append(info, "*Sisa waktu:* "+humanDuration(remaining(card)))
		}
		if t, err := time.Parse(time.RFC3339, card.DeadlineAt); err == nil {
			info = append(info, "*Harus selesai:* "+whenLabel(t, now)+" WIB")
		}
	} else if card.TimeRecorded {
		info = append(info, "*Waktu tercatat:* "+humanDuration(card.ElapsedSeconds))
	}

	sections := []section{}
	if hint != "" {
		sections = append(sections, section{items: []item{paragraph(hint)}})
	}
	sections = append(sections, section{items: []item{paragraph(strings.Join(info, "\n"))}})

	var bs []button
	for _, m := range moves {
		bs = append(bs, action("→ "+m.ToStatus, actMove, "key", key, "tid", m.ID, "to", m.ToStatus))
	}
	if len(bs) > 0 {
		sections = append(sections, section{header: "Geser ke", items: []item{buttons(bs...)}})
	} else {
		sections = append(sections, section{items: []item{paragraph("_Workflow Jira nggak punya jalur pindah dari status ini._")}})
	}
	nav := []button{action("⬅ Tiket Saya", actCommand, "cmd", "/n-mytickets")}
	if card.IssueURL != "" {
		nav = append(nav, link("Buka di Jira", card.IssueURL))
	}
	sections = append(sections, section{items: []item{buttons(nav...)}})

	var names []string
	for _, m := range moves {
		names = append(names, m.ToStatus)
	}
	tip := ""
	if len(names) > 0 {
		tip = fmt.Sprintf("Geser: */n-tiket %s <status>* — pilihan: %s", key, strings.Join(names, ", "))
	}
	return view{id: "nudge-ticket-" + key, title: "📋 " + key, subtitle: card.Summary, sections: sections, tip: tip}
}

func (s *service) move(ctx context.Context, email, key, transitionID, toStatus string) view {
	key, ok := normalizeKey(key)
	if !ok || transitionID == "" {
		return note("Papan Tiket", errorBody)
	}
	status, err := s.d.Dashboard.Transition(ctx, email, key, transitionID)
	var fr *move_fields.FieldsRequiredError
	switch {
	case errors.As(err, &fr):
		return s.fieldsRequiredView(ctx, email, fr)
	case errors.Is(err, user_dashboard_service.ErrNotOwnTicket):
		return notOwnView(key)
	case errors.Is(err, user_dashboard_service.ErrTransitionUnavailable):
		return s.withNote(s.ticketDetail(ctx, email, key, ""),
			fmt.Sprintf("⚠️ Status tiketnya sudah berubah, jadi pindah ke %s nggak tersedia lagi. Ini pilihan terbarunya 👇", toStatus))
	case err != nil:
		log.Printf("chat_interactive: Transition(%s, %s) failed: %v", email, key, err)
		return note("📋 "+key, jiraBody)
	}
	now := s.d.Now()
	return view{
		id: "nudge-ticket-" + key, title: "📋 " + key,
		sections: []section{
			{items: []item{paragraph(fmt.Sprintf("✅ *%s* berhasil digeser ke *%s* · %s WIB", key, status, now.In(wib).Format("15:04")))}},
			{items: []item{buttons(
				action("Geser lagi", actTicket, "key", key),
				action("⬅ Tiket Saya", actCommand, "cmd", "/n-mytickets"),
			)}},
		},
	}
}

// fieldsRequiredText: "⚠️ CORE-123 belum bisa dipindah ke *Code Review*: isi dulu *Merge Request*."
func fieldsRequiredText(fr *move_fields.FieldsRequiredError) string {
	r := fr.Required
	if names := fr.FieldNames(); names != "" {
		return fmt.Sprintf("⚠️ %s belum bisa dipindah ke *%s*: isi dulu *%s*.", r.IssueKey, r.Transition.To, names)
	}
	return fmt.Sprintf("⚠️ %s belum bisa dipindah ke *%s*: %s", r.IssueKey, r.Transition.To, r.Message)
}

// moveLink opens Papan Tiket on the ticket's board with the move dialog.
func (s *service) moveLink(ctx context.Context, email string, r move_fields.Required) string {
	q := url.Values{}
	if board, err := s.d.Dashboard.GetMyBoard(ctx, email, 0); err == nil {
		if c, found := findCard(board, r.IssueKey); found && c.BoardID > 0 {
			q.Set("board", strconv.FormatInt(c.BoardID, 10))
		}
	}
	q.Set("move", r.IssueKey)
	q.Set("to", r.Transition.ID)
	return s.d.WebURL + "/dashboard/board?" + q.Encode()
}

// fieldsRequiredView answers a move that needs fields filled first: what to
// fill, a link to the web dialog and one to Jira. No dialog in Chat.
func (s *service) fieldsRequiredView(ctx context.Context, email string, fr *move_fields.FieldsRequiredError) view {
	r := fr.Required
	body := fieldsRequiredText(fr)
	if fr.FieldNames() != "" && r.Message != "" && !strings.HasPrefix(r.Message, "Isi dulu field") {
		body += "\n_" + r.Message + "_"
	}
	bs := []button{link("Isi & pindahkan di web", s.moveLink(ctx, email, r))}
	if r.IssueURL != "" {
		bs = append(bs, link("Buka di Jira", r.IssueURL))
	}
	bs = append(bs, action("⬅ Tiket Saya", actCommand, "cmd", "/n-mytickets"))
	return view{
		id: "nudge-ticket-" + r.IssueKey, title: "📋 " + r.IssueKey,
		sections: []section{{items: []item{paragraph(body)}}, {items: []item{buttons(bs...)}}},
	}
}

// withNote puts a message above a view's first section.
func (s *service) withNote(v view, msg string) view {
	v.sections = append([]section{{items: []item{paragraph(msg)}}}, v.sections...)
	return v
}

// Geser keeps the reminder card buttons ("todo" / "code-review") and the
// web's "code-completed" (review tickets) working on top of the Papan Tiket
// rules: the sender's own active-sprint tickets. A target lists every status
// name it means, in order — CORE ends at "Code Completed", OMS at "Done".
var geserTargets = map[string][]string{
	"todo":           {"To Do"},
	"code-review":    {"Code Review"},
	"code-completed": {"Code Completed", "Done"},
}

func (s *service) Geser(ctx context.Context, email, issueKey, targetSlug string) GeserResult {
	names, ok := geserTargets[strings.ToLower(targetSlug)]
	if !ok {
		return GeserResult{Message: fmt.Sprintf("Target %q nggak dikenali. Target yang didukung: todo, code-review, code-completed", targetSlug)}
	}
	target := names[0]
	key, ok := normalizeKey(issueKey)
	if !ok {
		return GeserResult{Message: fmt.Sprintf("Tiket %s nggak valid", issueKey)}
	}
	trs, err := s.d.Dashboard.ListTransitions(ctx, email, key)
	if errors.Is(err, user_dashboard_service.ErrNotOwnTicket) {
		return GeserResult{NotOwner: true, Message: fmt.Sprintf("Tiket %s bukan tiket kamu di sprint aktif — cuma assignee tiket yang bisa geser statusnya 🙏", key)}
	}
	if err != nil {
		return GeserResult{Message: fmt.Sprintf("Gagal ambil daftar transisi buat %s dari Jira, coba lagi beberapa saat 🙏", key)}
	}
	for _, name := range names {
		for _, tr := range trs.Transitions {
			if !strings.EqualFold(tr.ToStatus, name) {
				continue
			}
			status, err := s.d.Dashboard.Transition(ctx, email, key, tr.ID)
			var fr *move_fields.FieldsRequiredError
			if errors.As(err, &fr) {
				req := fr.Required
				return GeserResult{FieldsRequired: &req, Message: fieldsRequiredText(fr) + "\nIsi & pindahkan di web: " + s.moveLink(ctx, email, fr.Required)}
			}
			if err != nil {
				return GeserResult{Message: fmt.Sprintf("Gagal geser %s ke %s, coba lagi beberapa saat 🙏", key, name)}
			}
			return GeserResult{OK: true, StatusName: status, Message: fmt.Sprintf("✅ *%s* berhasil digeser ke *%s*", key, status)}
		}
	}
	return GeserResult{Message: fmt.Sprintf("Nggak ada jalur langsung dari status tiket %s sekarang ke %q di workflow project ini 🤔", key, target)}
}

// --- Cuti -----------------------------------------------------------------------

func mustDate(s string) time.Time {
	t, _ := time.ParseInLocation("2006-01-02", s, wib)
	return t
}

func (s *service) upcomingLeaves(ctx context.Context, email string, now time.Time) []user_leave_dto.Leave {
	all, err := s.d.Leaves.List(ctx, email)
	if err != nil {
		log.Printf("chat_interactive: leave List(%s) failed: %v", email, err)
		return nil
	}
	base := today(now)
	var out []user_leave_dto.Leave
	for _, l := range all {
		if !mustDate(l.Date).Before(base) {
			out = append(out, l)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out
}

func (s *service) cutiCommand(ctx context.Context, email, args string) view {
	if args == "" {
		return s.leaveView(ctx, email, "")
	}
	word, rest, _ := strings.Cut(args, " ")
	switch strings.ToLower(word) {
	case "hapus", "batal", "remove", "delete":
		day, _, ok := parseDate(strings.Fields(rest), s.d.Now())
		if !ok {
			return s.leaveView(ctx, email, "⚠️ Tanggalnya nggak kebaca. Contoh: */n-cuti hapus 20/11*")
		}
		return s.removeLeave(ctx, email, day.Format("2006-01-02"))
	case "tambah", "add":
		args = rest
	}
	from, to, noteText, ok := parseLeaveArgs(args, s.d.Now())
	if !ok {
		return s.leaveView(ctx, email, "⚠️ Tanggalnya nggak kebaca. Contoh: */n-cuti besok*, */n-cuti 20/11*, */n-cuti 20/11 - 22/11 Liburan*")
	}
	return s.addLeave(ctx, email, from.Format("2006-01-02"), to.Format("2006-01-02"), noteText)
}

func (s *service) leaveView(ctx context.Context, email, msg string) view {
	now := s.d.Now()
	leaves := s.upcomingLeaves(ctx, email, now)

	sections := []section{}
	if msg != "" {
		sections = append(sections, section{items: []item{paragraph(msg)}})
	}
	var rows []item
	for _, l := range leaves {
		b := action("Hapus", actLeaveRemove, "date", l.Date)
		r := &row{main: dayLabel(mustDate(l.Date)), bottom: l.Note, button: &b}
		rows = append(rows, item{row: r})
	}
	if len(rows) == 0 {
		rows = []item{paragraph("_Belum ada cuti yang akan datang._")}
	}
	sections = append(sections, section{header: "Cuti mendatang", collapseAfter: 10, items: rows})
	sections = append(sections, section{header: "Tambah cuti", items: []item{
		{date: &dateField{name: fieldFrom, label: "Dari tanggal"}},
		{date: &dateField{name: fieldTo, label: "Sampai tanggal (kosongkan kalau 1 hari)"}},
		{input: &inputField{name: fieldNote, label: "Catatan (opsional)", hint: "mis. Liburan keluarga"}},
		buttons(action("Simpan cuti", actLeaveAdd)),
		paragraph("_Atau ketik: /n-cuti besok · /n-cuti 20/11 - 22/11 Liburan_"),
	}})
	sections = append(sections, section{items: []item{paragraph("_Hari cuti nggak dihitung ke waktu tiket. Sabtu, Minggu & libur nasional dilewati otomatis._")}})
	return view{
		id: "nudge-cuti", title: "🌴 Cuti", subtitle: fmt.Sprintf("%d hari cuti mendatang", len(leaves)),
		sections: sections,
		tip:      "Tambah: */n-cuti 20/11* atau */n-cuti 20/11 - 22/11 catatan* · Hapus: */n-cuti hapus 20/11*",
	}
}

func (s *service) addLeave(ctx context.Context, email, from, to, noteText string) view {
	if from == "" {
		return s.leaveView(ctx, email, "⚠️ Pilih *Dari tanggal* dulu ya.")
	}
	if to == "" {
		to = from
	}
	res, err := s.d.Leaves.Add(ctx, email, from, to, strings.TrimSpace(noteText))
	switch {
	case errors.Is(err, user_leave_service.ErrNoWorkingDay):
		return s.leaveView(ctx, email, "⚠️ Tanggal itu Sabtu/Minggu atau libur nasional, jadi nggak perlu cuti 🙂")
	case errors.Is(err, user_leave_service.ErrInvalidRange):
		return s.leaveView(ctx, email, "⚠️ "+leaveRangeMessage(err))
	case err != nil:
		log.Printf("chat_interactive: leave Add(%s) failed: %v", email, err)
		return s.leaveView(ctx, email, "⚠️ "+errorBody)
	}
	msg := "✅ Cuti disimpan"
	if len(res.Added) > 0 {
		var days []string
		for _, d := range res.Added {
			days = append(days, dayLabel(mustDate(d)))
		}
		msg = "✅ Cuti disimpan: " + strings.Join(days, ", ")
	} else {
		msg = "Tanggal itu sudah tercatat sebagai cuti 👍"
	}
	if len(res.Skipped) > 0 {
		msg += fmt.Sprintf("\n_%d hari dilewati (akhir pekan, libur nasional, atau sudah ada)._", len(res.Skipped))
	}
	if res.Recalculating {
		msg += "\n_Waktu tiket kamu sedang dihitung ulang._"
	}
	return s.leaveView(ctx, email, msg)
}

func leaveRangeMessage(err error) string {
	e := err.Error()
	switch {
	case strings.Contains(e, "end before start"):
		return "Tanggal akhir lebih awal dari tanggal mulai."
	case strings.Contains(e, "more than") && strings.Contains(e, "days ago"):
		return "Tanggalnya terlalu jauh ke belakang (maksimal 60 hari lalu)."
	case strings.Contains(e, "more than"):
		return "Rentangnya kepanjangan (maksimal 31 hari sekali simpan)."
	case strings.Contains(e, "note too long"):
		return "Catatannya kepanjangan (maksimal 200 karakter)."
	}
	return "Tanggalnya nggak valid."
}

func (s *service) removeLeave(ctx context.Context, email, date string) view {
	t, err := time.ParseInLocation("2006-01-02", date, wib)
	if err != nil {
		return s.leaveView(ctx, email, "⚠️ Tanggalnya nggak valid.")
	}
	_, err = s.d.Leaves.Remove(ctx, email, date)
	switch {
	case errors.Is(err, user_leave_service.ErrNotFound):
		return s.leaveView(ctx, email, fmt.Sprintf("%s memang bukan hari cuti kamu.", dayLabel(t)))
	case err != nil:
		log.Printf("chat_interactive: leave Remove(%s, %s) failed: %v", email, date, err)
		return s.leaveView(ctx, email, "⚠️ "+errorBody)
	}
	return s.leaveView(ctx, email, "🗑 Cuti "+dayLabel(t)+" dihapus.")
}

// --- Mute ----------------------------------------------------------------------

func (s *service) muteCommand(ctx context.Context, u *registration_dto.User, args string) view {
	if args == "" {
		return s.muteView(ctx, u, "")
	}
	switch strings.ToLower(args) {
	case "nyalakan", "on", "stop", "batal":
		return s.unmute(ctx, u, "")
	}
	if isForeverMute(args) {
		return s.muteView(ctx, u, noForeverMute)
	}
	choice, ok := parseMute(args, s.d.Now())
	if !ok {
		return s.muteView(ctx, u, "⚠️ Durasinya nggak kebaca. Contoh: */n-mute 30m*, */n-mute 2 jam*, */n-mute besok*")
	}
	return s.applyMute(ctx, u, choice)
}

// noForeverMute answers the removed "mute forever" (typed or an old card's button).
const noForeverMute = "⚠️ Mute selamanya sudah tidak tersedia. Pilih durasi (maks. sampai besok pagi), " +
	"atau matikan semua notifikasi lewat *Pengaturan* di web."

func (s *service) mute(ctx context.Context, u *registration_dto.User, param string) view {
	if isForeverMute(param) {
		return s.muteView(ctx, u, noForeverMute)
	}
	choice, ok := parseMute(param, s.d.Now())
	if !ok {
		return s.muteView(ctx, u, "⚠️ Pilihan durasinya nggak dikenal.")
	}
	return s.applyMute(ctx, u, choice)
}

func (s *service) applyMute(ctx context.Context, u *registration_dto.User, c muteChoice) view {
	until := c.until
	boardID, name := s.muteBoard(ctx, u.Email)
	if err := s.d.Settings.MuteUntil(ctx, u.UserID, boardID, until); err != nil {
		log.Printf("chat_interactive: MuteUntil(%d, %d) failed: %v", u.UserID, boardID, err)
		return s.muteView(ctx, u, "⚠️ "+errorBody)
	}
	return s.muteView(ctx, u, "🔕 Oke, notifikasi "+name+" "+muteLabel(&until, s.d.Now())+".")
}

// unmute switches the notifications of the chosen board back on — its own
// mute and, when one is running, the mute of every board too.
func (s *service) unmute(ctx context.Context, u *registration_dto.User, _ string) view {
	boardID, name := s.muteBoard(ctx, u.Email)
	scopes := []int64{0}
	if boardID > 0 {
		scopes = []int64{boardID, 0}
	}
	for _, id := range scopes {
		if err := s.d.Settings.Unmute(ctx, u.UserID, id); err != nil {
			log.Printf("chat_interactive: Unmute(%d, %d) failed: %v", u.UserID, id, err)
			return s.muteView(ctx, u, "⚠️ "+errorBody)
		}
	}
	return s.muteView(ctx, u, "🔔 Notifikasi "+name+" nyala lagi.")
}

// muteBoard is the board /n-mute acts on: the web's board choice (0 = every
// board, the global mute) and its label.
func (s *service) muteBoard(ctx context.Context, email string) (int64, string) {
	id := s.storedBoard(ctx, email)
	if id <= 0 {
		return 0, "semua board"
	}
	name, _ := s.d.Dashboard.DescribeBoard(ctx, id)
	if name == "" {
		name = fmt.Sprintf("Board #%d", id)
	}
	return id, name
}

// activeMute is the running mute that silences boardID (the global one, or
// that board's own), nil when notifications are on.
func (s *service) activeMute(ctx context.Context, userID, boardID int64, now time.Time) *time.Time {
	st, err := s.d.Settings.GetEffective(ctx, userID, boardID)
	if err != nil {
		log.Printf("chat_interactive: GetEffective(%d, %d) failed: %v", userID, boardID, err)
		return nil
	}
	if user_setting_dto.MuteActive(st.MutedUntil, now) {
		return st.MutedUntil
	}
	if boardID > 0 && user_setting_dto.MuteActive(st.BoardMutedUntil, now) {
		return st.BoardMutedUntil
	}
	return nil
}

func (s *service) muteView(ctx context.Context, u *registration_dto.User, msg string) view {
	now := s.d.Now()
	sections := []section{}
	if msg != "" {
		sections = append(sections, section{items: []item{paragraph(msg)}})
	}

	boardID, name := s.muteBoard(ctx, u.Email)
	muted := false
	state := "🔔 Notifikasi " + name + " *aktif*"
	if until := s.activeMute(ctx, u.UserID, boardID, now); until != nil {
		muted = true
		state = "🔕 Notifikasi " + name + " *" + muteLabel(until, now) + "*"
	}
	state += "\n📌 _Board ikut pilihan di web (" + name + ")._"
	if sw, err := s.d.Settings.NotificationSwitch(ctx, u.UserID); err == nil && !sw.Enabled {
		state += "\n⚠️ _Notifikasi otomatis kamu sedang OFF di Pengaturan — nyalakan dulu di web supaya pengingat terkirim._"
	}
	sections = append(sections, section{items: []item{paragraph(state)}})

	if muted {
		sections = append(sections, section{items: []item{buttons(action("🔔 Nyalakan sekarang", actUnmute))}})
	}
	header := "Mute selama"
	if muted {
		header = "Ganti durasi mute"
	}
	sections = append(sections, section{header: header, items: []item{buttons(
		action("30 menit", actMute, "for", "30m"),
		action("1 jam", actMute, "for", "1j"),
		action("2 jam", actMute, "for", "2j"),
		action("4 jam", actMute, "for", "4j"),
		action("Sampai besok pagi", actMute, "for", "besok"),
	)}})
	sections = append(sections, section{items: []item{buttons(link("Pengaturan notifikasi", s.d.WebURL+"/dashboard/settings"))}})
	return view{
		id: "nudge-mute", title: "🔕 Mute notifikasi", sections: sections,
		tip: "Ketik */n-mute 30m*, */n-mute 2 jam*, */n-mute besok* atau */n-unmute*",
	}
}
