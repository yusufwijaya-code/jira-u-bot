package chat_interactive_service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"nudge/modules/jira_client/jira_fields"
	"nudge/modules/move_fields"
	"nudge/modules/registration/registration_dto"
	"nudge/modules/user_dashboard/user_dashboard_dto"
	"nudge/modules/user_dashboard/user_dashboard_service"
	"nudge/modules/user_leave/user_leave_dto"
	"nudge/modules/user_leave/user_leave_service"
	"nudge/modules/user_setting/user_setting_dto"
)

const dev = "dev@esb.co.id"

// Monday 16 Nov 2026, 10:00 WIB.
var now = time.Date(2026, 11, 16, 10, 0, 0, 0, wib)

type fakeUsers map[string]*registration_dto.User

func (f fakeUsers) FindUserByEmail(_ context.Context, email string) (*registration_dto.User, bool, error) {
	u, ok := f[email]
	return u, ok, nil
}

type fakeDashboard struct {
	board       user_dashboard_dto.Board
	boardErr    error
	transitions map[string]user_dashboard_dto.TicketTransitions
	moved       []string
	moveErr     error
}

func (f *fakeDashboard) GetMyBoard(context.Context, string, int64) (user_dashboard_dto.Board, error) {
	return f.board, f.boardErr
}

func (f *fakeDashboard) DescribeBoard(_ context.Context, id int64) (string, string) {
	return map[int64]string{1: "CORE Board", 2: "OMS Board"}[id], ""
}

func (f *fakeDashboard) ListTransitions(_ context.Context, _ string, key string) (user_dashboard_dto.TicketTransitions, error) {
	t, ok := f.transitions[key]
	if !ok {
		return t, user_dashboard_service.ErrNotOwnTicket
	}
	return t, nil
}

func (f *fakeDashboard) Transition(_ context.Context, _ string, key, id string) (string, error) {
	if f.moveErr != nil {
		return "", f.moveErr
	}
	f.moved = append(f.moved, key+">"+id)
	for _, tr := range f.transitions[key].Transitions {
		if tr.ID == id {
			return tr.ToStatus, nil
		}
	}
	return "", user_dashboard_service.ErrTransitionUnavailable
}

type fakeLeaves struct {
	leaves  []user_leave_dto.Leave
	added   []string
	removed []string
	addErr  error
}

func (f *fakeLeaves) List(context.Context, string) ([]user_leave_dto.Leave, error) {
	return f.leaves, nil
}

func (f *fakeLeaves) Add(_ context.Context, _ string, from, to, note string) (user_leave_dto.AddResult, error) {
	if f.addErr != nil {
		return user_leave_dto.AddResult{}, f.addErr
	}
	f.added = append(f.added, from+".."+to+"|"+note)
	return user_leave_dto.AddResult{Added: []string{from}}, nil
}

func (f *fakeLeaves) Remove(_ context.Context, _ string, date string) (bool, error) {
	f.removed = append(f.removed, date)
	return false, nil
}

type fakeSettings struct {
	mutedUntil *time.Time
	mutedBoard int64
	switchOff  bool
	unmuted    int
}

func (f *fakeSettings) GetEffective(context.Context, int64, int64) (user_setting_dto.UserSetting, error) {
	return user_setting_dto.UserSetting{MutedUntil: f.mutedUntil}, nil
}

func (f *fakeSettings) MuteUntil(_ context.Context, _, board int64, until time.Time) error {
	f.mutedUntil, f.mutedBoard = &until, board
	return nil
}

func (f *fakeSettings) Unmute(context.Context, int64, int64) error {
	f.mutedUntil = nil
	f.unmuted++
	return nil
}

func (f *fakeSettings) NotificationSwitch(context.Context, int64) (user_setting_dto.NotificationSwitch, error) {
	return user_setting_dto.NotificationSwitch{Enabled: !f.switchOff, Chosen: true}, nil
}

type fixture struct {
	dash     *fakeDashboard
	leaves   *fakeLeaves
	settings *fakeSettings
	scope    *fakeScope
	svc      ChatInteractiveService
}

type fakeScope struct{ id int64 }

func (f *fakeScope) Get(context.Context, string) (int64, error) { return f.id, nil }
func (f *fakeScope) Set(_ context.Context, _ string, id int64) error {
	f.id = id
	return nil
}

func newFixture() fixture {
	f := fixture{dash: &fakeDashboard{transitions: map[string]user_dashboard_dto.TicketTransitions{}}, leaves: &fakeLeaves{}, settings: &fakeSettings{}, scope: &fakeScope{}}
	f.svc = New(Deps{
		Scope:     f.scope,
		Users:     fakeUsers{dev: {UserID: 7, UserName: "Yusuf Wijaya", Email: dev, FlagActive: true}},
		Dashboard: f.dash, Leaves: f.leaves, Settings: f.settings,
		WebURL: "https://nudge.test", Now: func() time.Time { return now },
	})
	return f
}

func codeReviewCORE() user_dashboard_dto.Card {
	// CORE counts Code Review time too: the dashboard service attaches the
	// budget (HasBudgetDetail) to a CORE ticket in CODE REVIEW.
	return user_dashboard_dto.Card{IssueKey: "CORE-2", Summary: "Voucher API", Status: "CODE REVIEW", BoardID: 1,
		HasBudgetDetail: true, StoryPoints: 2, ElapsedSeconds: 3 * 3600, BudgetSeconds: 2 * 3600}
}

func sampleBoard() user_dashboard_dto.Board {
	return user_dashboard_dto.Board{Columns: []user_dashboard_dto.BoardColumn{
		{Status: "To Do", Cards: []user_dashboard_dto.Card{{IssueKey: "CORE-3", Summary: "Report", Status: "To Do", BoardID: 1}}},
		{Status: "Code Process", Cards: []user_dashboard_dto.Card{{IssueKey: "CORE-1", Summary: "Fix login", Status: "Code Process", BoardID: 1,
			HasBudgetDetail: true, StoryPoints: 3, ElapsedSeconds: 3600, BudgetSeconds: 3 * 3600, DeadlineAt: now.Add(4 * time.Hour).Format(time.RFC3339)}}},
		{Status: "CODE REVIEW", Cards: []user_dashboard_dto.Card{codeReviewCORE()}},
		{Status: "Done", Cards: []user_dashboard_dto.Card{{IssueKey: "OMS-9", Summary: "Old", Status: "Done", BoardID: 2}}},
	}}
}

func TestParseCommand_AcceptsNewOldAndBareNames(t *testing.T) {
	cases := map[string][2]string{
		"/n-dashboard":           {cmdDashboard, ""},
		"/N-TIKET core-1 review": {cmdTiket, "core-1 review"},
		"tiket":                  {cmdTiket, ""},
		"/n-mytickets":           {cmdTiket, ""},
		"/n-budget CORE-1":       {cmdTiket, "CORE-1"},
		"/n-overdue":             {cmdDashboard, ""},
		"/n-team":                {cmdDashboard, ""},
		"/n-geser CORE-1 todo":   {cmdTiket, "CORE-1 todo"},
		"/n-geser<CORE-1>:todo":  {cmdTiket, "CORE-1 todo"},
		"cuti besok":             {cmdCuti, "besok"},
		"/n-mute 1 jam":          {cmdMute, "1 jam"},
		"/n-unmute":              {cmdUnmute, ""},
		"menu":                   {cmdHelp, ""},
	}
	for in, want := range cases {
		name, args, ok := parseCommand(in)
		if !ok || name != want[0] || args != want[1] {
			t.Errorf("parseCommand(%q) = %q %q %v, want %q %q", in, name, args, ok, want[0], want[1])
		}
	}
	if _, _, ok := parseCommand("halo nudge"); ok {
		t.Error("plain chat must not be a command")
	}
}

func TestCommand_UnknownTextShowsTheMenuWithAButtonPerFeature(t *testing.T) {
	r := newFixture().svc.Command(context.Background(), dev, "halo")
	if !strings.Contains(r.Text, "Hai Yusuf") {
		t.Errorf("menu text = %q", r.Text)
	}
	if len(r.Cards) != 1 {
		t.Fatalf("cards = %d", len(r.Cards))
	}
	var fns []string
	for _, s := range r.Cards[0].Card.Sections {
		for _, w := range s.Widgets {
			if w.DecoratedText != nil && w.DecoratedText.Button != nil {
				for _, p := range w.DecoratedText.Button.OnClick.Action.Parameters {
					if p.Key == "cmd" {
						fns = append(fns, p.Value)
					}
				}
			}
		}
	}
	if strings.Join(fns, ",") != "/n-mytickets,/n-dashboard,/n-cuti,/n-mute" {
		t.Errorf("menu buttons = %v", fns)
	}
}

func TestCommand_UnregisteredUserIsAskedToSignIn(t *testing.T) {
	r := newFixture().svc.Command(context.Background(), "x@esb.co.id", "/n-dashboard")
	if !strings.Contains(r.Text, "belum terdaftar") || !strings.Contains(r.Text, "https://nudge.test/") {
		t.Errorf("reply = %q", r.Text)
	}
}

func TestDashboard_ShowsOnlyWorkingTicketsWithBudgetAndCountsCOREReviewAsWorking(t *testing.T) {
	f := newFixture()
	f.dash.board = sampleBoard()
	r := f.svc.Command(context.Background(), dev, "/n-dashboard")
	for _, want := range []string{
		"*4 tiket* di sprint aktif",
		"To Do *1* · Code Process *1* · CODE REVIEW *1* · Done *1*",
		"🔴 *1 tiket lewat budget*",
		"*CORE-2* Voucher API", // CORE in Code Review is still counted
		"🔴 Lewat 1 jam",
		"*CORE-1* Fix login",
		"⏱ Sisa 2 jam",
		"🔔 Notifikasi aktif",
	} {
		if !strings.Contains(r.Text, want) {
			t.Errorf("dashboard missing %q:\n%s", want, r.Text)
		}
	}
	if strings.Contains(r.Text, "CORE-3") || strings.Contains(r.Text, "OMS-9") {
		t.Errorf("tickets not being worked on must not be listed:\n%s", r.Text)
	}
	if strings.Index(r.Text, "CORE-2") > strings.Index(r.Text, "CORE-1") {
		t.Error("overdue ticket should come first")
	}
}

func TestDashboard_ShowsMuteAndNextLeave(t *testing.T) {
	f := newFixture()
	until := now.Add(time.Hour)
	f.settings.mutedUntil = &until
	f.leaves.leaves = []user_leave_dto.Leave{{Date: "2026-11-01"}, {Date: "2026-11-20"}}
	r := f.svc.Command(context.Background(), dev, "/n-dashboard")
	if !strings.Contains(r.Text, "🔕 Notifikasi di-mute sampai 11:00 WIB") {
		t.Errorf("mute line missing:\n%s", r.Text)
	}
	if !strings.Contains(r.Text, "Cuti berikutnya: Jum, 20 Nov 2026") {
		t.Errorf("next leave missing (past leave must be skipped):\n%s", r.Text)
	}
}

func TestDashboard_JiraDownIsAFriendlyNote(t *testing.T) {
	f := newFixture()
	f.dash.boardErr = errors.New("boom")
	if r := f.svc.Command(context.Background(), dev, "/n-dashboard"); !strings.Contains(r.Text, "Jira lagi nggak bisa dihubungi") {
		t.Errorf("reply = %q", r.Text)
	}
}

func TestMyTickets_FollowsTheWebBoardChoice(t *testing.T) {
	f := newFixture()
	f.dash.board = sampleBoard()
	f.scope.id = 2 // OMS picked in the web navbar
	r := f.svc.Command(context.Background(), dev, "/n-mytickets")
	if !strings.Contains(r.Text, "*OMS-9*") || strings.Contains(r.Text, "*CORE-1*") || !strings.Contains(r.Text, "📌 *OMS Board* — ikut pilihan board di web") {
		t.Errorf("OMS only:\n%s", r.Text)
	}
	f.scope.id = 0 // Semua board
	r = f.svc.Command(context.Background(), dev, "/n-mytickets")
	if !strings.Contains(r.Text, "*OMS-9*") || !strings.Contains(r.Text, "*CORE-1*") || !strings.Contains(r.Text, "📌 *Semua board*") {
		t.Errorf("all boards:\n%s", r.Text)
	}
	var switches []string
	for _, s := range r.Cards[0].Card.Sections {
		for _, w := range s.Widgets {
			if w.ButtonList == nil {
				continue
			}
			for _, b := range w.ButtonList.Buttons {
				if b.OnClick.Action != nil && b.OnClick.Action.Function == "nudge.scope" {
					switches = append(switches, b.Text)
				}
			}
		}
	}
	if strings.Join(switches, ",") != "CORE Board,OMS Board" {
		t.Errorf("switch buttons = %v", switches)
	}
}

func TestMyTickets_SwitchingBoardIsSavedForTheWeb(t *testing.T) {
	f := newFixture()
	f.dash.board = sampleBoard()
	a := f.svc.Action(context.Background(), dev, "nudge.scope", map[string]string{"board": "1", "then": "/n-mytickets"})
	if f.scope.id != 1 || !a.Update || !strings.Contains(a.Text, "*CORE-1*") || strings.Contains(a.Text, "*OMS-9*") {
		t.Errorf("scope=%d update=%v\n%s", f.scope.id, a.Update, a.Text)
	}
	f.svc.Command(context.Background(), dev, "/n-mytickets semua")
	if f.scope.id != 0 {
		t.Errorf("typed 'semua' must select every board, scope = %d", f.scope.id)
	}
	f.svc.Command(context.Background(), dev, "/n-mytickets oms")
	if f.scope.id != 2 {
		t.Errorf("typed board name must select it, scope = %d", f.scope.id)
	}
	if r := f.svc.Command(context.Background(), dev, "/n-mytickets hr"); !strings.Contains(r.Text, `Board "hr" nggak ketemu`) || f.scope.id != 2 {
		t.Errorf("unknown board: scope=%d\n%s", f.scope.id, r.Text)
	}
}

func TestMyTickets_StaleWebChoiceFallsBackToAllBoards(t *testing.T) {
	f := newFixture()
	f.dash.board = sampleBoard()
	f.scope.id = 99
	r := f.svc.Command(context.Background(), dev, "/n-mytickets")
	if !strings.Contains(r.Text, "nggak punya tiket kamu di sprint aktif") || !strings.Contains(r.Text, "*CORE-1*") {
		t.Errorf("reply:\n%s", r.Text)
	}
}

func TestDashboardAndMute_FollowTheWebBoardChoice(t *testing.T) {
	f := newFixture()
	f.dash.board = sampleBoard()
	f.scope.id = 2
	r := f.svc.Command(context.Background(), dev, "/n-dashboard")
	if !strings.Contains(r.Text, "*1 tiket* di sprint aktif") || strings.Contains(r.Text, "CORE-2") {
		t.Errorf("dashboard OMS only:\n%s", r.Text)
	}
	f.svc.Command(context.Background(), dev, "/n-mute 1 jam")
	if f.settings.mutedBoard != 2 {
		t.Errorf("mute must target the picked board, got %d", f.settings.mutedBoard)
	}
}

func TestMyTickets_SubtasksHangUnderTheirParentAndCanBeMoved(t *testing.T) {
	f := newFixture()
	b := sampleBoard()
	b.Related = []user_dashboard_dto.Card{
		{IssueKey: "CORE-50", Summary: "Sub", Status: "Done", BoardID: 1, ParentKey: "CORE-1", Relation: "subtask"},
		{IssueKey: "CORE-51", Summary: "Linked", Status: "To Do", BoardID: 1, ParentKey: "CORE-3", Relation: "relates to"},
	}
	f.dash.board = b
	r := f.svc.Command(context.Background(), dev, "/n-mytickets core")
	for _, want := range []string{
		"• *CORE-1* Fix login\n   Code Process · 3 SP\n   ⏱ Sisa 2 jam · berjalan 1 jam · deadline 14:00 WIB\n\n      ↳ *CORE-50* Sub\n         Done · subtask",
		"• *CORE-3* Report\n   To Do\n\n      ↳ *CORE-51* Linked\n         To Do · relates to",
		"*CORE Board · 3 tiket (+2 subtask/terkait)*",
	} {
		if !strings.Contains(r.Text, want) {
			t.Errorf("missing %q in:\n%s", want, r.Text)
		}
	}
	found := false
	for _, s := range r.Cards[0].Card.Sections {
		for _, w := range s.Widgets {
			if d := w.DecoratedText; d != nil && strings.Contains(d.Text, "CORE-50") {
				found = strings.HasPrefix(d.Text, "↳ ") && d.Button != nil && d.Button.OnClick.Action.Function == "nudge.ticket"
			}
		}
	}
	if !found {
		t.Error("the subtask row must be a ↳ branch with its own Geser button")
	}
}

func TestTiket_ListGroupsByBoardWithAGeserButtonPerTicket(t *testing.T) {
	f := newFixture()
	f.dash.board = sampleBoard()
	r := f.svc.Command(context.Background(), dev, "/n-tiket semua")
	if !strings.Contains(r.Text, "*CORE Board · 3 tiket*") || !strings.Contains(r.Text, "*OMS Board · 1 tiket*") {
		t.Errorf("board groups missing:\n%s", r.Text)
	}
	buttons := 0
	for _, s := range r.Cards[0].Card.Sections {
		for _, w := range s.Widgets {
			if w.DecoratedText != nil && w.DecoratedText.Button != nil && w.DecoratedText.Button.OnClick.Action.Function == "nudge.ticket" {
				buttons++
			}
		}
	}
	if buttons != 4 {
		t.Errorf("Geser buttons = %d, want one per ticket (4)", buttons)
	}
}

func TestTiket_EmptySprint(t *testing.T) {
	if r := newFixture().svc.Command(context.Background(), dev, "/n-tiket"); !strings.Contains(r.Text, "belum punya tiket") {
		t.Errorf("reply = %q", r.Text)
	}
}

func coreTransitions() user_dashboard_dto.TicketTransitions {
	return user_dashboard_dto.TicketTransitions{IssueKey: "CORE-1", CurrentStatus: "Code Process", Transitions: []user_dashboard_dto.TransitionOption{
		{ID: "11", Name: "Back", ToStatus: "To Do"},
		{ID: "21", Name: "Review", ToStatus: "CODE REVIEW"},
		{ID: "31", Name: "Stay", ToStatus: "Code Process"},
	}}
}

func TestTiket_DetailHasAButtonPerWorkflowStatusExceptTheCurrentOne(t *testing.T) {
	f := newFixture()
	f.dash.board = sampleBoard()
	f.dash.transitions["CORE-1"] = coreTransitions()
	r := f.svc.Command(context.Background(), dev, "/n-tiket core-1")
	var labels []string
	for _, s := range r.Cards[0].Card.Sections {
		for _, w := range s.Widgets {
			if w.ButtonList == nil {
				continue
			}
			for _, b := range w.ButtonList.Buttons {
				if b.OnClick.Action != nil && b.OnClick.Action.Function == "nudge.move" {
					labels = append(labels, b.Text)
				}
			}
		}
	}
	if strings.Join(labels, ",") != "→ To Do,→ CODE REVIEW" {
		t.Errorf("move buttons = %v", labels)
	}
	if !strings.Contains(r.Text, "*Sisa waktu:* 2 jam") || !strings.Contains(r.Text, "Fix login") {
		t.Errorf("detail text:\n%s", r.Text)
	}
}

func TestTiket_TypedStatusMovesRightAway(t *testing.T) {
	f := newFixture()
	f.dash.transitions["CORE-1"] = coreTransitions()
	r := f.svc.Command(context.Background(), dev, "/n-tiket CORE-1 review")
	if len(f.dash.moved) != 1 || f.dash.moved[0] != "CORE-1>21" {
		t.Fatalf("moved = %v", f.dash.moved)
	}
	if !strings.Contains(r.Text, "✅ *CORE-1* berhasil digeser ke *CODE REVIEW* · 10:00 WIB") {
		t.Errorf("reply = %q", r.Text)
	}
}

func TestTiket_UnknownStatusShowsTheChoices(t *testing.T) {
	f := newFixture()
	f.dash.transitions["CORE-1"] = coreTransitions()
	r := f.svc.Command(context.Background(), dev, "/n-tiket CORE-1 deploy")
	if len(f.dash.moved) != 0 || !strings.Contains(r.Text, `Status "deploy" nggak ketemu`) {
		t.Errorf("moved=%v reply=%q", f.dash.moved, r.Text)
	}
}

func TestTiket_SomeoneElsesTicketIsRefused(t *testing.T) {
	r := newFixture().svc.Command(context.Background(), dev, "/n-tiket OMS-5")
	if !strings.Contains(r.Text, "bukan tiket kamu") {
		t.Errorf("reply = %q", r.Text)
	}
}

func TestAction_MoveUpdatesTheCard(t *testing.T) {
	f := newFixture()
	f.dash.transitions["CORE-1"] = coreTransitions()
	a := f.svc.Action(context.Background(), dev, "nudge.move", map[string]string{"key": "CORE-1", "tid": "11", "to": "To Do"})
	if !a.Update || !strings.Contains(a.Text, "digeser ke *To Do*") {
		t.Errorf("action = %+v", a)
	}
}

func TestAction_MoveThatIsNoLongerAvailableShowsFreshChoices(t *testing.T) {
	f := newFixture()
	f.dash.transitions["CORE-1"] = coreTransitions()
	a := f.svc.Action(context.Background(), dev, "nudge.move", map[string]string{"key": "CORE-1", "tid": "99", "to": "Done"})
	if !strings.Contains(a.Text, "pindah ke Done nggak tersedia lagi") || !strings.Contains(a.Text, "*Status:*") {
		t.Errorf("reply = %q", a.Text)
	}
}

func TestGeser_ReminderButtonsUseTheWorkflowCaseInsensitively(t *testing.T) {
	f := newFixture()
	f.dash.transitions["CORE-1"] = coreTransitions()
	res := f.svc.Geser(context.Background(), dev, "CORE-1", "code-review")
	if !res.OK || res.StatusName != "CODE REVIEW" || f.dash.moved[0] != "CORE-1>21" {
		t.Errorf("res=%+v moved=%v", res, f.dash.moved)
	}
	if res := f.svc.Geser(context.Background(), dev, "OMS-5", "todo"); !res.NotOwner {
		t.Errorf("someone else's ticket: %+v", res)
	}
	if res := f.svc.Geser(context.Background(), dev, "CORE-1", "deploy"); res.OK || !strings.Contains(res.Message, "nggak dikenali") {
		t.Errorf("unknown target: %+v", res)
	}
}

func TestGeser_CodeCompletedFollowsEachProjectsDoneStatus(t *testing.T) {
	f := newFixture()
	f.dash.transitions["CORE-1"] = user_dashboard_dto.TicketTransitions{IssueKey: "CORE-1", CurrentStatus: "CODE REVIEW", Transitions: []user_dashboard_dto.TransitionOption{
		{ID: "11", ToStatus: "To Do"},
		{ID: "41", ToStatus: "Code Completed"},
	}}
	res := f.svc.Geser(context.Background(), dev, "CORE-1", "code-completed")
	if !res.OK || res.StatusName != "Code Completed" || f.dash.moved[0] != "CORE-1>41" {
		t.Errorf("CORE: res=%+v moved=%v", res, f.dash.moved)
	}

	// OMS names the same step "Done".
	f.dash.transitions["CORE-1"] = user_dashboard_dto.TicketTransitions{IssueKey: "CORE-1", CurrentStatus: "Code Review", Transitions: []user_dashboard_dto.TransitionOption{
		{ID: "11", ToStatus: "To Do"},
		{ID: "51", ToStatus: "Done"},
	}}
	res = f.svc.Geser(context.Background(), dev, "CORE-1", "code-completed")
	if !res.OK || res.StatusName != "Done" || f.dash.moved[1] != "CORE-1>51" {
		t.Errorf("OMS-style: res=%+v moved=%v", res, f.dash.moved)
	}
}

func TestCuti_ListShowsUpcomingWithRemoveButtonsAndAForm(t *testing.T) {
	f := newFixture()
	f.leaves.leaves = []user_leave_dto.Leave{{Date: "2026-11-20", Note: "Liburan"}, {Date: "2026-11-02"}}
	r := f.svc.Command(context.Background(), dev, "/n-cuti")
	if !strings.Contains(r.Text, "• Jum, 20 Nov 2026") || strings.Contains(r.Text, "2 Nov") {
		t.Errorf("list:\n%s", r.Text)
	}
	var pickers, removes int
	for _, s := range r.Cards[0].Card.Sections {
		for _, w := range s.Widgets {
			if w.DateTimePicker != nil {
				pickers++
			}
			if w.DecoratedText != nil && w.DecoratedText.Button != nil && w.DecoratedText.Button.OnClick.Action.Function == "nudge.leave.remove" {
				removes++
			}
		}
	}
	if pickers != 2 || removes != 1 {
		t.Errorf("pickers=%d removes=%d", pickers, removes)
	}
}

func TestCuti_TypedDatesAreFlexible(t *testing.T) {
	cases := map[string]string{
		"/n-cuti besok":                     "2026-11-17..2026-11-17|",
		"/n-cuti 20/11":                     "2026-11-20..2026-11-20|",
		"/n-cuti 20/11 - 23/11 Liburan":     "2026-11-20..2026-11-23|Liburan",
		"/n-cuti 2026-12-01 s/d 2026-12-02": "2026-12-01..2026-12-02|",
		"/n-cuti tambah 5 jan":              "2027-01-05..2027-01-05|",
		"/n-cuti 24 des 2026 natal":         "2026-12-24..2026-12-24|natal",
	}
	for in, want := range cases {
		f := newFixture()
		r := f.svc.Command(context.Background(), dev, in)
		if len(f.leaves.added) != 1 || f.leaves.added[0] != want {
			t.Errorf("%q added %v, want %q", in, f.leaves.added, want)
		}
		if !strings.Contains(r.Text, "✅ Cuti disimpan") {
			t.Errorf("%q reply = %q", in, r.Text)
		}
	}
}

func TestCuti_UnreadableDateShowsExamples(t *testing.T) {
	f := newFixture()
	r := f.svc.Command(context.Background(), dev, "/n-cuti kapan2")
	if len(f.leaves.added) != 0 || !strings.Contains(r.Text, "Tanggalnya nggak kebaca") {
		t.Errorf("added=%v reply=%q", f.leaves.added, r.Text)
	}
}

func TestCuti_ServiceRulesBecomePlainMessages(t *testing.T) {
	f := newFixture()
	f.leaves.addErr = user_leave_service.ErrNoWorkingDay
	if r := f.svc.Command(context.Background(), dev, "/n-cuti 21/11"); !strings.Contains(r.Text, "Sabtu/Minggu") {
		t.Errorf("reply = %q", r.Text)
	}
	f.leaves.addErr = errors.Join(user_leave_service.ErrInvalidRange, errors.New("more than 31 days"))
	if r := f.svc.Command(context.Background(), dev, "/n-cuti 1/12 - 31/1"); !strings.Contains(r.Text, "kepanjangan") {
		t.Errorf("reply = %q", r.Text)
	}
}

func TestCuti_FormAndRemoveButtons(t *testing.T) {
	f := newFixture()
	a := f.svc.Action(context.Background(), dev, "nudge.leave.add", map[string]string{"from": "2026-11-20", "note": "Cuti"})
	if !a.Update || len(f.leaves.added) != 1 || f.leaves.added[0] != "2026-11-20..2026-11-20|Cuti" {
		t.Errorf("add: %+v %v", a.Update, f.leaves.added)
	}
	a = f.svc.Action(context.Background(), dev, "nudge.leave.add", map[string]string{})
	if !strings.Contains(a.Text, "Pilih *Dari tanggal*") {
		t.Errorf("empty form reply = %q", a.Text)
	}
	a = f.svc.Action(context.Background(), dev, "nudge.leave.remove", map[string]string{"date": "2026-11-20"})
	if len(f.leaves.removed) != 1 || !strings.Contains(a.Text, "Cuti Jum, 20 Nov 2026 dihapus") {
		t.Errorf("remove: %v %q", f.leaves.removed, a.Text)
	}
	if r := f.svc.Command(context.Background(), dev, "/n-cuti hapus 20/11"); len(f.leaves.removed) != 2 || !strings.Contains(r.Text, "dihapus") {
		t.Errorf("typed remove: %v %q", f.leaves.removed, r.Text)
	}
}

func TestMute_TypedDurations(t *testing.T) {
	cases := map[string]time.Time{
		"/n-mute 30m":       now.Add(30 * time.Minute),
		"/n-mute 2 jam":     now.Add(2 * time.Hour),
		"/n-mute 90":        now.Add(90 * time.Minute),
		"/n-mute besok":     user_setting_dto.TomorrowMorning(now),
	}
	for in, want := range cases {
		f := newFixture()
		r := f.svc.Command(context.Background(), dev, in)
		if f.settings.mutedUntil == nil || !f.settings.mutedUntil.Equal(want) {
			t.Errorf("%q muted until %v, want %v", in, f.settings.mutedUntil, want)
		}
		if !strings.Contains(r.Text, "🔕 Oke, notifikasi") {
			t.Errorf("%q reply = %q", in, r.Text)
		}
	}
	// The forever mute was removed: typed or via an old card's "Matikan" button.
	for _, in := range []string{"/n-mute selamanya", "/n-mute matikan"} {
		f := newFixture()
		if r := f.svc.Command(context.Background(), dev, in); f.settings.mutedUntil != nil || !strings.Contains(r.Text, "Mute selamanya sudah tidak tersedia") {
			t.Errorf("%q: muted=%v reply=%q, want a refusal", in, f.settings.mutedUntil, r.Text)
		}
	}
	f := newFixture()
	if r := f.svc.Command(context.Background(), dev, "/n-mute 3 hari"); f.settings.mutedUntil != nil || !strings.Contains(r.Text, "nggak kebaca") {
		t.Errorf("bad duration: %v %q", f.settings.mutedUntil, r.Text)
	}
	if r := f.svc.Command(context.Background(), dev, "/n-mute 25 jam"); f.settings.mutedUntil != nil || !strings.Contains(r.Text, "nggak kebaca") {
		t.Errorf("over the 24h cap: %v %q", f.settings.mutedUntil, r.Text)
	}
}

func TestMute_ViewButtonsAndUnmute(t *testing.T) {
	f := newFixture()
	f.settings.switchOff = true
	r := f.svc.Command(context.Background(), dev, "/n-mute")
	if !strings.Contains(r.Text, "Notifikasi semua board *aktif*") || !strings.Contains(r.Text, "sedang OFF di Pengaturan") {
		t.Errorf("view:\n%s", r.Text)
	}
	a := f.svc.Action(context.Background(), dev, "nudge.mute", map[string]string{"for": "1j"})
	if !a.Update || f.settings.mutedUntil == nil || !f.settings.mutedUntil.Equal(now.Add(time.Hour)) {
		t.Errorf("mute button: %+v %v", a.Update, f.settings.mutedUntil)
	}
	r = f.svc.Command(context.Background(), dev, "/n-unmute")
	if f.settings.unmuted != 1 || !strings.Contains(r.Text, "🔔 Notifikasi semua board nyala lagi") {
		t.Errorf("unmute: %d %q", f.settings.unmuted, r.Text)
	}
}

func TestView_CardEscapesHTMLAndRendersMarkup(t *testing.T) {
	if got := cardHTML("*KEY* <b>x</b> _hint_\nnext"); got != "<b>KEY</b> &lt;b&gt;x&lt;/b&gt; <i>hint</i><br>next" {
		t.Errorf("cardHTML = %q", got)
	}
}

func TestRouteCommand_IsTheTextRendering(t *testing.T) {
	f := newFixture()
	if got := f.svc.RouteCommand(context.Background(), dev, "/n-mute"); !strings.HasPrefix(got, "*🔕 Mute notifikasi*") || !strings.Contains(got, "💡 Ketik */n-mute 30m*") {
		t.Errorf("text = %q", got)
	}
}

func TestMyTickets_ListsEveryTicketWithItsTimeInfo(t *testing.T) {
	f := newFixture()
	b := sampleBoard()
	b.Columns[0].Cards[0].StoryPoints = 2 // To Do ticket: budget it will get
	b.Columns[3].Cards[0].TimeRecorded = true
	b.Columns[3].Cards[0].ElapsedSeconds, b.Columns[3].Cards[0].BudgetSeconds = 1800, 3600
	b.Related = []user_dashboard_dto.Card{{IssueKey: "CORE-50", Summary: "Sub", Status: "To Do", BoardID: 1, ParentKey: "CORE-1", Relation: "subtask"}}
	f.dash.board = b
	r := f.svc.Command(context.Background(), dev, "/n-mytickets semua")
	for _, want := range []string{
		"*📋 Tiket Saya*",
		"To Do *1* · Code Process *1* · CODE REVIEW *1* · Done *1*",
		"⏱ *2* sedang dikerjakan · 🔴 *1* lewat budget",
		"• *CORE-1* Fix login\n   Code Process · 3 SP\n   ⏱ Sisa 2 jam · berjalan 1 jam · deadline 14:00 WIB",
		"• *CORE-2* Voucher API\n   CODE REVIEW · 2 SP\n   🔴 Lewat 1 jam · berjalan 3 jam dari budget 2 jam",
		"• *CORE-3* Report\n   To Do · 2 SP\n   Budget 2 jam · dihitung saat masuk Code Process",
		"✔ Tercatat 30 menit dari budget 1 jam",
		"↳ *CORE-50* Sub\n         To Do · subtask",
	} {
		if !strings.Contains(r.Text, want) {
			t.Errorf("missing %q in:\n%s", want, r.Text)
		}
	}
}

func TestMatchStatus_FollowsEachProjectsStatusNames(t *testing.T) {
	core := []string{"To Do", "Code Process", "CODE REVIEW", "Code Completed"}
	oms := []string{"To Do", "Code Process", "Code Review", "Done"}
	cases := []struct {
		typed      string
		candidates []string
		want       string
	}{
		{"review", core, "CODE REVIEW"},
		{"cr", oms, "Code Review"},
		{"code-review", core, "CODE REVIEW"},
		{"done", core, "Code Completed"},
		{"selesai", oms, "Done"},
		{"completed", core, "Code Completed"},
		{"process", oms, "Code Process"},
		{"TODO", core, "To Do"},
	}
	for _, c := range cases {
		i, ok := matchStatus(c.typed, c.candidates)
		if !ok || c.candidates[i] != c.want {
			t.Errorf("matchStatus(%q) = %v %v, want %q", c.typed, i, ok, c.want)
		}
	}
	if _, ok := matchStatus("code", core); ok {
		t.Error("an ambiguous word must not pick a status")
	}
}

func TestView_RowsAreSeparatedByDividersAndFoldCountsThem(t *testing.T) {
	var items []item
	for i := 0; i < 5; i++ {
		items = append(items, item{row: &row{main: "T"}})
	}
	c := view{title: "x", sections: []section{{collapseAfter: 2, items: items}}}.card()
	s := c.Card.Sections[0]
	if len(s.Widgets) != 9 {
		t.Fatalf("widgets = %d, want 5 rows + 4 dividers", len(s.Widgets))
	}
	if s.Widgets[1].Divider == nil || s.Widgets[2].DecoratedText == nil {
		t.Errorf("expected row, divider, row: %+v", s.Widgets[:3])
	}
	if !s.Collapsible || s.UncollapsibleWidgetsCount != 3 {
		t.Errorf("collapsible=%v visible=%d, want the first 2 rows (3 widgets)", s.Collapsible, s.UncollapsibleWidgetsCount)
	}
}

func TestView_TextPutsABlankLineBetweenTickets(t *testing.T) {
	v := view{title: "x", sections: []section{{items: []item{
		{row: &row{main: "A", bottom: "a"}}, {row: &row{main: "B"}},
	}}}}
	if got := v.text(); !strings.Contains(got, "• A\n   a\n\n• B") {
		t.Errorf("text = %q", got)
	}
}

// Every command card comes with a title line (sent as message text, the
// Home / notification preview) and no card header repeating it.
func TestReply_EveryCardHasATitleLineAndNoRepeatedHeader(t *testing.T) {
	f := newFixture()
	f.dash.board = sampleBoard()
	f.dash.transitions["CORE-1"] = coreTransitions()
	for _, cmd := range []string{"/n-help", "/n-dashboard", "/n-mytickets", "/n-tiket CORE-1", "/n-cuti", "/n-mute", "/n-unmute", "halo", "/n-tiket OMS-5"} {
		r := f.svc.Command(context.Background(), dev, cmd)
		if r.Title == "" || !strings.HasPrefix(r.Title, "*") {
			t.Errorf("%s: title line = %q", cmd, r.Title)
		}
		if len(r.Cards) != 1 || r.Cards[0].Card.Header != nil {
			t.Errorf("%s: card header must be nil, cards = %+v", cmd, r.Cards)
		}
	}
	if r := f.svc.Command(context.Background(), dev, "/n-mytickets"); r.Title != "*📋 Tiket Saya* — Semua board · 4 tiket · ⏱ 2 dikerjakan · 🔴 1 lewat budget" {
		t.Errorf("tiket title line = %q", r.Title)
	}
}

// --- move-fields ---

func mergeRequestError() *move_fields.FieldsRequiredError {
	return &move_fields.FieldsRequiredError{Required: move_fields.Required{
		Transition: move_fields.Transition{ID: "21", Name: "Merge Request to Snapshot Branch", To: "Code Review"},
		Message:    "Fill in the Merge Request Fields Before Code Review",
		Fields:     []jira_fields.Field{{ID: "customfield_10095", Name: "Merge Request", Kind: "textarea", Required: true}},
		IssueKey:   "CORE-1", IssueURL: "https://jira/browse/CORE-1",
	}}
}

func TestAction_MoveNeedingFieldsLinksTheWebDialog(t *testing.T) {
	f := newFixture()
	f.dash.board = sampleBoard()
	f.dash.transitions["CORE-1"] = coreTransitions()
	f.dash.moveErr = mergeRequestError()
	a := f.svc.Action(context.Background(), dev, "nudge.move", map[string]string{"key": "CORE-1", "tid": "21", "to": "Code Review"})
	if !strings.Contains(a.Text, "⚠️ CORE-1 belum bisa dipindah ke *Code Review*: isi dulu *Merge Request*.") {
		t.Errorf("text = %q", a.Text)
	}
	b, _ := json.Marshal(a)
	var decoded any
	json.Unmarshal(b, &decoded)
	raw := fmt.Sprint(decoded)
	for _, want := range []string{
		"Isi \u0026 pindahkan di web", "https://nudge.test/dashboard/board?board=1\u0026move=CORE-1\u0026to=21",
		"Buka di Jira", "https://jira/browse/CORE-1",
	} {
		if !strings.Contains(raw, want) {
			t.Errorf("reply lacks %q: %s", want, raw)
		}
	}
}

func TestGeser_FieldsRequiredCarriesTheLink(t *testing.T) {
	f := newFixture()
	f.dash.board = sampleBoard()
	f.dash.transitions["CORE-1"] = coreTransitions()
	f.dash.moveErr = mergeRequestError()
	res := f.svc.Geser(context.Background(), dev, "CORE-1", "code-review")
	if res.OK || res.FieldsRequired == nil || res.FieldsRequired.Transition.ID != "21" {
		t.Fatalf("res = %+v", res)
	}
	if !strings.Contains(res.Message, "isi dulu *Merge Request*") || !strings.Contains(res.Message, "https://nudge.test/dashboard/board?board=1&move=CORE-1&to=21") {
		t.Errorf("message = %q", res.Message)
	}
}
