package user_dashboard_handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_middleware"
	"nudge/modules/board/board_service"
	"nudge/modules/chat_interactive/chat_interactive_service"
	"nudge/modules/jira_credential/jira_credential_service"
	"nudge/modules/user_dashboard/user_dashboard_dto"
	"nudge/modules/user_dashboard/user_dashboard_service"
	"nudge/modules/user_setting/user_setting_dto"
	"nudge/modules/user_setting/user_setting_service"
)

type userDashboardHandlerImpl struct {
	dashboardSvc    user_dashboard_service.UserDashboardService
	chatInteractive chat_interactive_service.ChatInteractiveService
	jiraCredential  jira_credential_service.JiraCredentialService
	userSettings    user_setting_service.UserSettingService
	avatars         AvatarSource
}

// AvatarSource serves a board's (project) avatar image — board_service.
type AvatarSource interface {
	Avatar(ctx context.Context, boardID int64) (body []byte, contentType string, err error)
}

func NewUserDashboardHandler(
	dashboardSvc user_dashboard_service.UserDashboardService,
	chatInteractive chat_interactive_service.ChatInteractiveService,
	jiraCredential jira_credential_service.JiraCredentialService,
	userSettings user_setting_service.UserSettingService,
	avatars AvatarSource,
) UserDashboardHandler {
	return &userDashboardHandlerImpl{
		dashboardSvc:    dashboardSvc,
		chatInteractive: chatInteractive,
		jiraCredential:  jiraCredential,
		userSettings:    userSettings,
		avatars:         avatars,
	}
}

func currentUser(c *gin.Context) auth_dto.Session {
	return c.MustGet(auth_middleware.CurrentUserKey).(auth_dto.Session)
}

// notificationJSON is the "notification" object of GET /settings and the
// response of PUT /settings/notification.
func notificationJSON(s user_setting_dto.UserSetting) gin.H {
	return gin.H{
		"reminderTiers":        nonNil(s.ReminderTiers),
		"overageTiers":         nonNil(s.OverageTiers),
		"overageRepeatMinutes": s.OverageRepeatMinutes,
		"notifyEarlyFinish":    s.NotifyEarlyFinish,
		"notifyReturnedToTodo": s.NotifyReturnedToTodo,
		"quietHours":           s.QuietHours,
	}
}

// muteStateJSON renders one mute level.
func muteStateJSON(until *time.Time, now time.Time) gin.H {
	if !user_setting_dto.MuteActive(until, now) {
		return gin.H{"active": false, "until": nil, "forever": false}
	}
	return muteActiveJSON(*until)
}

func muteActiveJSON(until time.Time) gin.H {
	return gin.H{
		"active":  true,
		"until":   until.In(user_setting_dto.Jakarta).Format(time.RFC3339),
		"forever": user_setting_dto.IsForever(&until),
	}
}

func nonNil(tiers []string) []string {
	if tiers == nil {
		return []string{}
	}
	return tiers
}

// boardParam reads ?board= (Tech Spec "Per-Board Settings & Active Sprint
// Dashboard"): empty or 0 = all boards / the default scope. It only ever
// filters the caller's own data. Writes a 400 and returns ok=false when it
// isn't a non-negative integer.
func boardParam(c *gin.Context) (int64, bool) {
	raw := c.Query("board")
	if raw == "" {
		return 0, true
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "board harus berupa angka"})
		return 0, false
	}
	return id, true
}

// GetSettings returns the caller's EFFECTIVE settings for the scope, both
// mute levels and the Jira credential status. Identity always comes from
// the session.
func (h *userDashboardHandlerImpl) GetSettings(c *gin.Context) {
	boardID, ok := boardParam(c)
	if !ok {
		return
	}
	user := currentUser(c)
	ctx := c.Request.Context()

	setting, err := h.userSettings.GetEffective(ctx, user.UserID, boardID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load settings"})
		return
	}

	credentialMasked, hasCredential, _ := h.jiraCredential.MaskedPreview(ctx, user.UserID)

	now := time.Now()
	scope := "board"
	var boardMute any
	if boardID == user_setting_dto.DefaultBoardID {
		scope = "default"
	} else {
		boardMute = muteStateJSON(setting.BoardMutedUntil, now)
	}

	c.JSON(http.StatusOK, gin.H{
		"scope":        scope,
		"boardId":      boardID,
		"useDefault":   setting.UseDefault,
		"notification": notificationJSON(setting),
		"mute": gin.H{
			"global": muteStateJSON(setting.MutedUntil, now),
			"board":  boardMute,
		},
		// Kept for older clients: the pre-budget tiers as before.
		"tierPreference": gin.H{"tiers": nonNil(setting.ReminderTiers)},
		"credential": gin.H{
			"hasCredential": hasCredential,
			"maskedPreview": credentialMasked,
		},
	})
}

func (h *userDashboardHandlerImpl) UpdateNotificationSettings(c *gin.Context) {
	boardID, ok := boardParam(c)
	if !ok {
		return
	}
	var in user_setting_dto.NotificationInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	saved, err := h.userSettings.SaveNotification(c.Request.Context(), currentUser(c).UserID, boardID, in)
	if err != nil {
		var verr *user_setting_dto.ValidationError
		if errors.As(err, &verr) {
			c.JSON(http.StatusBadRequest, gin.H{"error": verr.Message, "fields": gin.H{verr.Field: verr.Message}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save settings"})
		return
	}
	c.JSON(http.StatusOK, notificationJSON(saved))
}

// ResetNotificationSettings makes a board follow the default again (AC3.4).
func (h *userDashboardHandlerImpl) ResetNotificationSettings(c *gin.Context) {
	boardID, ok := boardParam(c)
	if !ok {
		return
	}
	if boardID == user_setting_dto.DefaultBoardID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pilih board yang mau dikembalikan ke default"})
		return
	}
	user := currentUser(c)
	if err := h.userSettings.ResetToDefault(c.Request.Context(), user.UserID, boardID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reset settings"})
		return
	}
	setting, err := h.userSettings.GetEffective(c.Request.Context(), user.UserID, boardID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load settings"})
		return
	}
	c.JSON(http.StatusOK, notificationJSON(setting))
}

type muteRequest struct {
	Minutes *int   `json:"minutes"`
	Until   string `json:"until"`
}

// Mute mutes globally (no board) or one board (AC4.1, AC4.2).
func (h *userDashboardHandlerImpl) Mute(c *gin.Context) {
	boardID, ok := boardParam(c)
	if !ok {
		return
	}
	var req muteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	now := time.Now()
	var until time.Time
	switch {
	case req.Minutes != nil && req.Until == "":
		if *req.Minutes < 1 || *req.Minutes > user_setting_dto.MaxMuteMinutes {
			c.JSON(http.StatusBadRequest, gin.H{"error": "durasi mute harus 1 sampai 1440 menit"})
			return
		}
		until = now.Add(time.Duration(*req.Minutes) * time.Minute)
	case req.Minutes == nil && req.Until == "tomorrow":
		until = user_setting_dto.TomorrowMorning(now)
	case req.Minutes == nil && req.Until == "forever":
		// Off until the user unmutes (the "matikan notif" switch).
		until = user_setting_dto.MuteForeverUntil
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "isi salah satu: minutes, until=\"tomorrow\", atau until=\"forever\""})
		return
	}

	if err := h.userSettings.MuteUntil(c.Request.Context(), currentUser(c).UserID, boardID, until); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to mute"})
		return
	}
	c.JSON(http.StatusOK, muteActiveJSON(until))
}

func (h *userDashboardHandlerImpl) Unmute(c *gin.Context) {
	boardID, ok := boardParam(c)
	if !ok {
		return
	}
	if err := h.userSettings.Unmute(c.Request.Context(), currentUser(c).UserID, boardID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to unmute"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"active": false, "until": nil, "forever": false})
}

// ListBoards returns the board picker entries (AC1.1, AC1.2).
func (h *userDashboardHandlerImpl) ListBoards(c *gin.Context) {
	user := currentUser(c)
	ctx := c.Request.Context()

	boards, err := h.dashboardSvc.ListBoards(ctx, user.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load boards"})
		return
	}
	states, err := h.userSettings.BoardStates(ctx, user.UserID)
	if err != nil {
		states = map[int64]user_setting_service.BoardState{}
	}

	now := time.Now()
	out := make([]gin.H, 0, len(boards))
	for _, b := range boards {
		state := states[b.BoardID]
		out = append(out, gin.H{
			"boardId":           b.BoardID,
			"name":              b.Name,
			"projectKey":        b.ProjectKey,
			"activeSprint":      b.ActiveSprint,
			"ticketCount":       b.TicketCount,
			"reviewTracked":     b.ReviewTracked,
			"hasCustomSettings": state.Custom,
			"muted":             muteStateJSON(state.MutedUntil, now),
		})
	}
	c.JSON(http.StatusOK, gin.H{"boards": out})
}

// BoardAvatar proxies the board's project avatar (AC1.3, AC1.5).
func (h *userDashboardHandlerImpl) BoardAvatar(c *gin.Context) {
	boardID, err := strconv.ParseInt(c.Param("boardId"), 10, 64)
	if err != nil || boardID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "board harus berupa angka"})
		return
	}
	if h.avatars == nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	body, contentType, err := h.avatars.Avatar(c.Request.Context(), boardID)
	if err != nil {
		if errors.Is(err, board_service.ErrAvatarNotFound) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		c.AbortWithStatus(http.StatusBadGateway)
		return
	}
	c.Header("Cache-Control", "private, max-age=86400")
	c.Data(http.StatusOK, contentType, body)
}

type saveCredentialRequest struct {
	JiraEmail string `json:"jiraEmail"`
	APIToken  string `json:"apiToken"`
}

func (h *userDashboardHandlerImpl) SaveCredential(c *gin.Context) {
	var req saveCredentialRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.JiraEmail == "" || req.APIToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "jiraEmail and apiToken are required"})
		return
	}

	user := currentUser(c)
	if err := h.jiraCredential.UpsertCredential(c.Request.Context(), user.UserID, req.JiraEmail, req.APIToken); err != nil {
		if err == jira_credential_service.ErrEncryptionKeyNotConfigured {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "fitur ini belum dikonfigurasi server, hubungi admin"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save credential"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *userDashboardHandlerImpl) DeleteCredential(c *gin.Context) {
	user := currentUser(c)
	if err := h.jiraCredential.DeleteCredential(c.Request.Context(), user.UserID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete credential"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *userDashboardHandlerImpl) GetBoard(c *gin.Context) {
	boardID, ok := boardParam(c)
	if !ok {
		return
	}
	board, err := h.dashboardSvc.GetMyBoard(c.Request.Context(), currentUser(c).Email, boardID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load board"})
		return
	}
	columns := board.Columns
	if columns == nil {
		columns = []user_dashboard_dto.BoardColumn{} // JSON [] not null
	}
	for i := range columns {
		if columns[i].Cards == nil {
			columns[i].Cards = []user_dashboard_dto.Card{}
		}
	}
	c.JSON(http.StatusOK, gin.H{"columns": columns})
}

func (h *userDashboardHandlerImpl) GetTeam(c *gin.Context) {
	view, err := h.dashboardSvc.GetTeamView(c.Request.Context(), currentUser(c).Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load team data"})
		return
	}
	groups := view.Groups
	if groups == nil {
		groups = []user_dashboard_dto.TeamGroup{} // JSON [] not null
	}
	for i := range groups {
		if groups[i].Cards == nil {
			groups[i].Cards = []user_dashboard_dto.Card{}
		}
	}
	c.JSON(http.StatusOK, gin.H{"groups": groups})
}

func (h *userDashboardHandlerImpl) GetOverdue(c *gin.Context) {
	items, err := h.dashboardSvc.GetOverdueView(c.Request.Context(), currentUser(c).Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load overdue data"})
		return
	}
	if items == nil {
		items = []user_dashboard_dto.OverdueItem{} // JSON [] not null
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

type transitionRequest struct {
	Target string `json:"target"`
}

func (h *userDashboardHandlerImpl) TransitionTicket(c *gin.Context) {
	issueKey := c.Param("issueKey")
	var req transitionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Delegates straight to RouteCommand (spec AC4.1) - the exact same
	// validation/transition/credential-resolution path as the bot's
	// /u-geser, including its "own tickets only" ownership check.
	command := "/u-geser<" + issueKey + ">:" + req.Target
	message := h.chatInteractive.RouteCommand(c.Request.Context(), currentUser(c).Email, command)
	c.JSON(http.StatusOK, gin.H{"message": message})
}
