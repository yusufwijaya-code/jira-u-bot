package chat_interactive_service

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"nudge/modules/user_dashboard/user_dashboard_dto"
)

// --- Tiket Saya (/n-mytickets, alias /n-tiket) ---------------------------------
//
//	/n-mytickets              board picker (skipped when there is one board)
//	/n-mytickets semua        every board
//	/n-mytickets board 26     one board (the picker's buttons)
//	/n-mytickets core         one board, by name or project key
//	/n-mytickets CORE-1 [st]  one ticket (see ticketDetail)

func (s *service) tiketCommand(ctx context.Context, email, args string) view {
	if args == "" {
		return s.ticketList(ctx, email, "")
	}
	keyWord, target, _ := strings.Cut(args, " ")
	if key, ok := normalizeKey(keyWord); ok {
		return s.ticketDetail(ctx, email, key, strings.TrimSpace(target))
	}
	return s.ticketList(ctx, email, args)
}

// boardGroup is one board's tickets: sprint tickets plus the caller's
// subtasks / linked items outside the sprint (branches).
type boardGroup struct {
	id       int64
	name     string
	project  string
	cards    []user_dashboard_dto.Card
	branches []user_dashboard_dto.Card
}

func (g *boardGroup) all() []user_dashboard_dto.Card {
	return append(append([]user_dashboard_dto.Card{}, g.cards...), g.branches...)
}

func (s *service) groupByBoard(ctx context.Context, b user_dashboard_dto.Board) []*boardGroup {
	byID := map[int64]*boardGroup{}
	var order []*boardGroup
	get := func(id int64) *boardGroup {
		if g, ok := byID[id]; ok {
			return g
		}
		g := &boardGroup{id: id, name: "Tanpa board"}
		if id > 0 {
			g.name, g.project = s.d.Dashboard.DescribeBoard(ctx, id)
		}
		byID[id] = g
		order = append(order, g)
		return g
	}
	for _, col := range b.Columns {
		for _, c := range col.Cards {
			g := get(c.BoardID)
			g.cards = append(g.cards, c)
		}
	}
	for _, c := range b.Related {
		g := get(c.BoardID)
		g.branches = append(g.branches, c)
	}
	sort.SliceStable(order, func(i, j int) bool { return order[i].name < order[j].name })
	return order
}

// pickBoards resolves the board choice; ok is false for an unknown one.
func pickBoards(groups []*boardGroup, choice string) ([]*boardGroup, bool) {
	c := strings.ToLower(strings.TrimSpace(choice))
	switch c {
	case "semua", "all", "semua board":
		return groups, true
	}
	if rest, found := strings.CutPrefix(c, "board "); found {
		if id, err := strconv.ParseInt(strings.TrimSpace(rest), 10, 64); err == nil {
			for _, g := range groups {
				if g.id == id {
					return []*boardGroup{g}, true
				}
			}
		}
		return nil, false
	}
	want := squash(c)
	var out []*boardGroup
	for _, g := range groups {
		if want != "" && (squash(g.project) == want || strings.Contains(squash(g.name), want)) {
			out = append(out, g)
		}
	}
	return out, len(out) > 0
}

type ticketStats struct{ sprint, branches, working, late int }

func statsOf(groups []*boardGroup) ticketStats {
	var st ticketStats
	for _, g := range groups {
		st.sprint += len(g.cards)
		st.branches += len(g.branches)
		for _, c := range g.all() {
			if c.HasBudgetDetail {
				st.working++
			}
			if overdue(c) {
				st.late++
			}
		}
	}
	return st
}

func (st ticketStats) line() string {
	parts := []string{fmt.Sprintf("%d tiket", st.sprint)}
	if st.branches > 0 {
		parts = append(parts, fmt.Sprintf("+%d subtask/terkait", st.branches))
	}
	if st.working > 0 {
		parts = append(parts, fmt.Sprintf("⏱ %d dikerjakan", st.working))
	}
	if st.late > 0 {
		parts = append(parts, fmt.Sprintf("🔴 %d lewat budget", st.late))
	}
	return strings.Join(parts, " · ")
}

// scope is the board the commands show: the one picked in the web navbar
// (shared through Deps.Scope), or every board the user has tickets on.
type scope struct {
	groups []*boardGroup // every board of the user
	picked []*boardGroup // the boards shown
	id     int64         // picked board, 0 = all
	note   string        // why the stored choice wasn't used
}

func (sc scope) label() string {
	if sc.id > 0 && len(sc.picked) == 1 {
		return sc.picked[0].name
	}
	return "Semua board"
}

func (s *service) storedBoard(ctx context.Context, email string) int64 {
	if s.d.Scope == nil {
		return 0
	}
	id, err := s.d.Scope.Get(ctx, email)
	if err != nil {
		log.Printf("chat_interactive: board scope of %s failed: %v", email, err)
		return 0
	}
	return id
}

func (s *service) saveBoard(ctx context.Context, email string, id int64) {
	if s.d.Scope == nil {
		return
	}
	if err := s.d.Scope.Set(ctx, email, id); err != nil {
		log.Printf("chat_interactive: saving board scope of %s failed: %v", email, err)
	}
}

// scoped loads the user's tickets and narrows them to the board choice:
// choice (typed) when given — it also becomes the web's choice — else the
// stored one.
func (s *service) scoped(ctx context.Context, email, choice string) (scope, error) {
	board, err := s.d.Dashboard.GetMyBoard(ctx, email, 0)
	if err != nil {
		return scope{}, err
	}
	sc := scope{groups: s.groupByBoard(ctx, board)}
	sc.picked = sc.groups
	if choice != "" {
		picked, ok := pickBoards(sc.groups, choice)
		if !ok {
			sc.note = fmt.Sprintf("⚠️ Board %q nggak ketemu, jadi tampil sesuai pilihan board kamu.", choice)
		} else {
			id := int64(0)
			if len(picked) == 1 && len(sc.groups) > 1 {
				id = picked[0].id
			}
			s.saveBoard(ctx, email, id)
		}
	}
	id := s.storedBoard(ctx, email)
	if id > 0 {
		for _, g := range sc.groups {
			if g.id == id {
				sc.picked, sc.id = []*boardGroup{g}, id
			}
		}
		if sc.id == 0 && sc.note == "" && len(sc.groups) > 0 {
			sc.note = "_Board pilihanmu di web nggak punya tiket kamu di sprint aktif, jadi tampil semua board._"
		}
	}
	return sc, nil
}

// scopeSection shows the board in use and buttons to switch it (saved for
// the web too); then is the command to show again after switching.
func (s *service) scopeSection(sc scope, then string) section {
	var bs []button
	if sc.id > 0 {
		bs = append(bs, action("Semua board", actScope, "board", "0", "then", then))
	}
	for _, g := range sc.groups {
		if g.id != sc.id && g.id > 0 {
			bs = append(bs, action(g.name, actScope, "board", strconv.FormatInt(g.id, 10), "then", then))
		}
	}
	items := []item{paragraph("📌 *" + sc.label() + "* — ikut pilihan board di web")}
	if len(bs) > 0 && len(sc.groups) > 1 {
		items = append(items, buttons(bs...))
	}
	return section{header: "Board", items: items}
}

func (s *service) ticketList(ctx context.Context, email, choice string) view {
	sc, err := s.scoped(ctx, email, choice)
	if err != nil {
		log.Printf("chat_interactive: GetMyBoard(%s) failed: %v", email, err)
		return note("📋 Tiket Saya", jiraBody)
	}
	if len(sc.groups) == 0 {
		return view{
			id: "nudge-tiket", title: "📋 Tiket Saya",
			sections: []section{{items: []item{
				paragraph("Kamu belum punya tiket di sprint aktif 👍"),
				buttons(action("📊 Dashboard", actCommand, "cmd", "/n-dashboard")),
			}}},
		}
	}
	return s.ticketBoards(sc)
}

// ticketBoards lists the tickets of the chosen boards, subtasks and linked
// items hanging under their parent.
func (s *service) ticketBoards(sc scope) view {
	groups := sc.picked
	now := s.d.Now()
	st := statsOf(groups)

	statusCount := map[string]int{}
	var statuses []string
	for _, g := range groups {
		for _, c := range g.cards {
			if _, seen := statusCount[c.Status]; !seen {
				statuses = append(statuses, c.Status)
			}
			statusCount[c.Status]++
		}
	}
	var counts []string
	for _, status := range statuses {
		counts = append(counts, fmt.Sprintf("%s *%d*", status, statusCount[status]))
	}
	overview := strings.Join(counts, " · ")
	if st.working > 0 {
		overview += fmt.Sprintf("\n⏱ *%d* sedang dikerjakan", st.working)
		if st.late > 0 {
			overview += fmt.Sprintf(" · 🔴 *%d* lewat budget", st.late)
		}
	}
	overview += "\n_Klik *Geser* untuk pindah status. ↳ = subtask / tiket terkait._"

	sections := []section{{items: []item{paragraph(overview)}}}
	if sc.note != "" {
		sections = append([]section{{items: []item{paragraph(sc.note)}}}, sections...)
	}
	shown, total := 0, 0
	for _, g := range groups {
		rows := treeRows(g, now)
		total += len(rows)
		if room := maxTicketRows - shown; len(rows) > room {
			rows = rows[:max(room, 0)]
		}
		shown += len(rows)
		if len(rows) == 0 {
			continue
		}
		header := fmt.Sprintf("%s · %d tiket", g.name, len(g.cards))
		if len(g.branches) > 0 {
			header += fmt.Sprintf(" (+%d subtask/terkait)", len(g.branches))
		}
		sections = append(sections, section{header: header, collapseAfter: 8, items: rows})
	}
	if shown < total {
		sections = append(sections, section{items: []item{paragraph(fmt.Sprintf("_+%d tiket lain — lihat semua di web._", total-shown))}})
	}

	sections = append(sections, s.scopeSection(sc, "/n-mytickets"))
	sections = append(sections, section{items: []item{buttons(
		action("📊 Dashboard", actCommand, "cmd", "/n-dashboard"),
		link("Buka Papan Tiket di web", s.d.WebURL+"/dashboard/board"),
	)}})

	return view{
		id: "nudge-tiket", title: "📋 Tiket Saya", subtitle: sc.label() + " · " + st.line(),
		sections: sections,
		tip:      "Buka 1 tiket: */n-tiket CORE-123* · geser langsung: */n-tiket CORE-123 review*",
	}
}

// treeRows orders a board's tickets as a tree: each ticket followed by its
// subtasks / linked items (depth 1, 2, ...). A branch whose parent isn't on
// this board stands on its own.
func treeRows(g *boardGroup, now time.Time) []item {
	all := g.all()
	present := map[string]bool{}
	for _, c := range all {
		present[strings.ToUpper(c.IssueKey)] = true
	}
	children := map[string][]user_dashboard_dto.Card{}
	var roots []user_dashboard_dto.Card
	for _, c := range all {
		parent := strings.ToUpper(c.ParentKey)
		if parent != "" && present[parent] && parent != strings.ToUpper(c.IssueKey) {
			children[parent] = append(children[parent], c)
			continue
		}
		roots = append(roots, c)
	}

	var out []item
	visited := map[string]bool{}
	var walk func(c user_dashboard_dto.Card, depth int)
	walk = func(c user_dashboard_dto.Card, depth int) {
		key := strings.ToUpper(c.IssueKey)
		if visited[key] {
			return
		}
		visited[key] = true
		it := ticketRow(c, now, true)
		if depth > 0 {
			it.row.depth = depth
			it.row.top = branchTop(c)
		}
		out = append(out, it)
		for _, child := range children[key] {
			walk(child, depth+1)
		}
	}
	for _, c := range roots {
		walk(c, 0)
	}
	return out
}

// branchTop is a branch's top label: its relation instead of "dari <parent>"
// (the tree already shows the parent).
func branchTop(c user_dashboard_dto.Card) string {
	rel := c.Relation
	if rel == "" || rel == "subtask" {
		rel = "subtask"
	}
	top := c.Status
	if c.StoryPoints > 0 {
		top += fmt.Sprintf(" · %s SP", spLabel(c.StoryPoints))
	}
	return top + " · " + rel
}
