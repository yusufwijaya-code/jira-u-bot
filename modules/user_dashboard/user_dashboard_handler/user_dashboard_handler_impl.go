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
	avatars         AvatarSource
	// settings serves the settings / credential / board-list endpoints for
	// the session's own user (shared with the admin mount, see
	// SettingsHandler).
	settings *SettingsHandler
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
		avatars:         avatars,
		settings:        NewSettingsHandler(dashboardSvc, jiraCredential, userSettings, SessionTarget, nil),
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
// filters the target's own data. Writes a 400 and returns ok=false when it
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

type muteRequest struct {
	Minutes *int   `json:"minutes"`
	Until   string `json:"until"`
}

type saveCredentialRequest struct {
	JiraEmail string `json:"jiraEmail"`
	APIToken  string `json:"apiToken"`
}

// The settings, mute, board-list and credential endpoints act on the
// session's own user — see SettingsHandler.

func (h *userDashboardHandlerImpl) GetSettings(c *gin.Context) { h.settings.GetSettings(c) }
func (h *userDashboardHandlerImpl) UpdateNotificationSettings(c *gin.Context) {
	h.settings.UpdateNotification(c)
}
func (h *userDashboardHandlerImpl) ResetNotificationSettings(c *gin.Context) {
	h.settings.ResetNotification(c)
}
func (h *userDashboardHandlerImpl) UpdateNotificationsSwitch(c *gin.Context) {
	h.settings.UpdateNotificationsSwitch(c)
}
func (h *userDashboardHandlerImpl) Mute(c *gin.Context)             { h.settings.Mute(c) }
func (h *userDashboardHandlerImpl) Unmute(c *gin.Context)           { h.settings.Unmute(c) }
func (h *userDashboardHandlerImpl) ListBoards(c *gin.Context)       { h.settings.ListBoards(c) }
func (h *userDashboardHandlerImpl) SaveCredential(c *gin.Context)   { h.settings.SaveCredential(c) }
func (h *userDashboardHandlerImpl) DeleteCredential(c *gin.Context) { h.settings.DeleteCredential(c) }

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
	related := board.Related
	if related == nil {
		related = []user_dashboard_dto.Card{}
	}
	c.JSON(http.StatusOK, gin.H{"columns": columns, "related": related})
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
