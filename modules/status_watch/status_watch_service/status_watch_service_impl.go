package status_watch_service

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"nudge/modules/business_hours/business_hours_dto"
	"nudge/modules/business_hours/business_hours_service"
	"nudge/modules/chat_delivery"
	"nudge/modules/google_chat_notifier/google_chat_notifier_dto"
	"nudge/modules/google_chat_notifier/google_chat_notifier_service"
	"nudge/modules/issue_session/issue_session_dto"
	"nudge/modules/issue_session/issue_session_service"
	"nudge/modules/jira_client/jira_client_dto"
	"nudge/modules/registration/registration_dto"
	"nudge/modules/status_watch/status_watch_dto"
	"nudge/modules/status_watch/status_watch_repository"
	"nudge/modules/user_setting/user_setting_dto"
)

const (
	pollInterval     = 10 * time.Second
	cursorOverlap    = time.Minute
	fullSyncInterval = time.Hour
	userCacheTTL     = 5 * time.Minute
	cleanupInterval  = 24 * time.Hour
	snapshotMaxAge   = 30 * 24 * time.Hour
)

// IssueSource is the Jira slice the watcher needs.
type IssueSource interface {
	SearchSprintIssuesUpdatedWithin(ctx context.Context, accountIDs []string, within time.Duration) ([]jira_client_dto.Issue, error)
	GetRecentChangelog(ctx context.Context, issueKey string) ([]jira_client_dto.ChangelogEntry, error)
}

// UserDirectory lists the registered users.
type UserDirectory interface {
	FindAll(ctx context.Context) ([]registration_dto.User, error)
}

// SettingsSource is the assignee's effective settings for a board.
type SettingsSource interface {
	GetEffectiveForEmail(ctx context.Context, email string, boardID int64) user_setting_dto.UserSetting
}

// SessionSource finds a ticket's tracked session (for the budget line).
type SessionSource interface {
	GetSessionByIssueKey(ctx context.Context, issueKey string) (*issue_session_dto.IssueSession, bool, error)
}

// Deps wires the watcher. DM may be nil (no OAuth): everything then goes
// through Webhook, as for the bot account's own tickets (SelfEmail).
type Deps struct {
	Jira          IssueSource
	Users         UserDirectory
	Settings      SettingsSource
	Sessions      SessionSource
	Snapshots     status_watch_repository.StatusWatchRepository
	BusinessHours business_hours_service.BusinessHoursService
	Calendar      business_hours_dto.CalendarSource
	Webhook       google_chat_notifier_service.GoogleChatNotifierService
	DM            google_chat_notifier_service.GoogleChatNotifierService
	SelfEmail     string
	JiraBaseURL   string
}

type statusWatchServiceImpl struct {
	Deps
	now func() time.Time
	// lastSuccess is when the last successful cycle started; lastFullSync
	// the last one that read every ticket (refreshes every snapshot, so
	// the 30-day cleanup only drops tickets that left the sprints).
	lastSuccess  *time.Time
	lastFullSync time.Time

	users        map[string]registration_dto.User // by Jira accountId
	emails       map[string]bool
	accountIDs   []string
	usersExpires time.Time
}

func NewStatusWatchService(d Deps) StatusWatchService {
	d.JiraBaseURL = strings.TrimRight(d.JiraBaseURL, "/")
	return &statusWatchServiceImpl{Deps: d, now: time.Now}
}

// registeredUsers caches the active users with a Jira account for a while.
func (s *statusWatchServiceImpl) registeredUsers(ctx context.Context) error {
	if s.users != nil && s.now().Before(s.usersExpires) {
		return nil
	}
	all, err := s.Users.FindAll(ctx)
	if err != nil {
		return err
	}
	users := map[string]registration_dto.User{}
	emails := map[string]bool{}
	var ids []string
	for _, u := range all {
		if !u.FlagActive || u.JiraAccountID == "" || u.Email == "" {
			continue
		}
		users[u.JiraAccountID] = u
		emails[u.Email] = true
		ids = append(ids, u.JiraAccountID)
	}
	s.users, s.emails, s.accountIDs, s.usersExpires = users, emails, ids, s.now().Add(userCacheTTL)
	return nil
}

func (s *statusWatchServiceImpl) RunOnce(ctx context.Context) error {
	started := s.now()
	if err := s.registeredUsers(ctx); err != nil {
		return err
	}
	if len(s.accountIDs) == 0 {
		return nil
	}
	// The first cycle and one an hour read every ticket; the others only
	// the ones updated since the last success (+ overlap).
	var within time.Duration
	full := s.lastSuccess == nil || started.Sub(s.lastFullSync) >= fullSyncInterval
	if !full {
		within = started.Sub(*s.lastSuccess) + cursorOverlap
	}
	issues, err := s.Jira.SearchSprintIssuesUpdatedWithin(ctx, s.accountIDs, within)
	if err != nil {
		return err // the window stays, the next cycle retries it
	}
	keys := make([]string, 0, len(issues))
	for _, issue := range issues {
		keys = append(keys, issue.Key)
	}
	known, err := s.Snapshots.Find(ctx, keys)
	if err != nil {
		return err
	}

	for _, issue := range issues {
		if issue.Fields.Assignee == nil {
			continue
		}
		user, ok := s.users[issue.Fields.Assignee.AccountID]
		if !ok {
			continue
		}
		status := issue.Fields.Status.Name
		prev, seen := known[issue.Key]
		// Snapshot first: a failed DM is never retried (no double notice).
		snap := status_watch_dto.Snapshot{IssueKey: issue.Key, Status: status, AssigneeEmail: user.Email, LastSeen: started}
		if err := s.Snapshots.Upsert(ctx, snap); err != nil {
			log.Printf("status_watch: snapshot of %s failed: %v", issue.Key, err)
			continue
		}
		if !seen || strings.EqualFold(prev.Status, status) {
			continue
		}
		// The current assignee, plus the previous one on a hand-off (e.g.
		// a developer moving a ticket to review and assigning the reviewer).
		recipients := []string{user.Email}
		if prev.AssigneeEmail != user.Email && s.emails[prev.AssigneeEmail] {
			recipients = append(recipients, prev.AssigneeEmail)
		}
		s.notifyChange(ctx, issue, recipients, prev.Status, status)
	}

	at := started
	s.lastSuccess = &at
	if full {
		s.lastFullSync = started
	}
	return nil
}

// notifyChange sends one status-change notice to each recipient who wants it.
func (s *statusWatchServiceImpl) notifyChange(ctx context.Context, issue jira_client_dto.Issue, recipients []string, from, to string) {
	now := s.now()
	var boardID int64
	if sprint, ok := issue.Fields.ActiveSprint(); ok {
		boardID = sprint.BoardID
	}
	var wanting []string
	for _, email := range recipients {
		setting := s.Settings.GetEffectiveForEmail(ctx, email, boardID)
		if !setting.WantsStatusChange(to) || setting.IsSilenced(now) || s.Calendar.ForEmail(ctx, email).IsHoliday(now) {
			continue
		}
		wanting = append(wanting, email)
	}
	if len(wanting) == 0 {
		return
	}

	actor, at := "", now
	if histories, err := s.Jira.GetRecentChangelog(ctx, issue.Key); err == nil {
		if who, when := lastStatusAuthor(histories); !when.IsZero() {
			actor, at = who, when
		}
	} else {
		log.Printf("status_watch: changelog of %s failed, sending without the author: %v", issue.Key, err)
	}

	timing := ""
	rule := jira_client_dto.RuleForIssue(issue.Key)
	if rule.IsTracked(from) && !rule.IsTracked(to) {
		if session, found, err := s.Sessions.GetSessionByIssueKey(ctx, issue.Key); err == nil && found && session.StoryPoints > 0 {
			// The session owner's calendar (their leave days), as for reminders.
			cal := s.Calendar.ForEmail(ctx, session.AssigneeEmail)
			elapsed := issue_session_service.CalculateElapsedSeconds(*session, s.BusinessHours, cal)
			timing = timingLine(rule, to, elapsed, int64(session.StoryPoints*3600))
		}
	}

	url := ""
	if s.JiraBaseURL != "" {
		url = s.JiraBaseURL + "/browse/" + issue.Key
	}
	text := statusChangeText(issue.Key, issue.Fields.Summary, url, from, to, actor, at, timing)
	for _, email := range wanting {
		msg := google_chat_notifier_dto.ReminderMessage{
			IssueKey: issue.Key, IssueURL: url, IssueSummary: issue.Fields.Summary, AssigneeEmail: email, Text: text,
		}
		notifier := google_chat_notifier_service.PickNotifier(email, s.SelfEmail, s.Webhook, s.DM)
		if err := notifier.SendReminder(ctx, msg); err != nil && !errors.Is(err, chat_delivery.ErrRecipientNotActivated) {
			log.Printf("status_watch: notice for %s to %s failed: %v", issue.Key, email, err)
		}
	}
}

func (s *statusWatchServiceImpl) Run(ctx context.Context) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	lastCleanup := time.Time{}
	for {
		if err := s.RunOnce(ctx); err != nil && ctx.Err() == nil {
			log.Printf("status_watch: cycle failed: %v", err)
		}
		if s.now().Sub(lastCleanup) >= cleanupInterval {
			lastCleanup = s.now()
			if n, err := s.Snapshots.DeleteNotSeenSince(ctx, lastCleanup.Add(-snapshotMaxAge)); err != nil {
				log.Printf("status_watch: cleanup failed: %v", err)
			} else if n > 0 {
				log.Printf("status_watch: dropped %d stale snapshot(s)", n)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
