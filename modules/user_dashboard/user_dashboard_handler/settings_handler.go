package user_dashboard_handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"nudge/modules/jira_credential/jira_credential_service"
	"nudge/modules/user_dashboard/user_dashboard_service"
	"nudge/modules/user_setting/user_setting_dto"
	"nudge/modules/user_setting/user_setting_service"
)

// SettingsTarget is the user whose settings a request reads or changes.
type SettingsTarget struct {
	UserID int64
	Email  string
}

// SettingsChangeKind names a write the SettingsHandler performed.
type SettingsChangeKind string

const (
	ChangeNotification      SettingsChangeKind = "notification"
	ChangeReset             SettingsChangeKind = "reset"
	ChangeMute              SettingsChangeKind = "mute"
	ChangeUnmute            SettingsChangeKind = "unmute"
	ChangeCredentialUpdated SettingsChangeKind = "credential_updated"
	ChangeCredentialDeleted SettingsChangeKind = "credential_deleted"
)

// SettingsChange describes one successful write. BoardID 0 = the default
// scope / global mute (credential changes are always 0); Until is set for
// ChangeMute only.
type SettingsChange struct {
	Kind    SettingsChangeKind
	BoardID int64
	Until   *time.Time
}

// TargetResolver returns the request's target user. When it returns false
// it has already written the error response.
type TargetResolver func(c *gin.Context) (SettingsTarget, bool)

// ChangeListener is told about every successful write.
type ChangeListener func(c *gin.Context, target SettingsTarget, change SettingsChange)

// SettingsHandler serves the notification / mute / credential / board-list
// endpoints for ONE target user: the session's user on /api/dashboard, a
// path-selected user on /api/dashboard/admin/users/:userID (Tech Spec
// "Admin Panel Revamp"). Both mounts share this code so their request and
// response shapes can't drift apart.
type SettingsHandler struct {
	dashboardSvc   user_dashboard_service.UserDashboardService
	jiraCredential jira_credential_service.JiraCredentialService
	userSettings   user_setting_service.UserSettingService
	resolve        TargetResolver
	onChange       ChangeListener
}

// NewSettingsHandler builds a SettingsHandler; onChange may be nil.
func NewSettingsHandler(
	dashboardSvc user_dashboard_service.UserDashboardService,
	jiraCredential jira_credential_service.JiraCredentialService,
	userSettings user_setting_service.UserSettingService,
	resolve TargetResolver,
	onChange ChangeListener,
) *SettingsHandler {
	return &SettingsHandler{
		dashboardSvc:   dashboardSvc,
		jiraCredential: jiraCredential,
		userSettings:   userSettings,
		resolve:        resolve,
		onChange:       onChange,
	}
}

// SessionTarget resolves the logged-in user — identity only ever comes from
// the session on the user's own endpoints.
func SessionTarget(c *gin.Context) (SettingsTarget, bool) {
	u := currentUser(c)
	return SettingsTarget{UserID: u.UserID, Email: u.Email}, true
}

func (h *SettingsHandler) emit(c *gin.Context, target SettingsTarget, change SettingsChange) {
	if h.onChange != nil {
		h.onChange(c, target, change)
	}
}

// GetSettings returns the target's EFFECTIVE settings for the scope, both
// mute levels and the Jira credential status.
func (h *SettingsHandler) GetSettings(c *gin.Context) {
	boardID, ok := boardParam(c)
	if !ok {
		return
	}
	target, ok := h.resolve(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	setting, err := h.userSettings.GetEffective(ctx, target.UserID, boardID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load settings"})
		return
	}

	credentialMasked, hasCredential, _ := h.jiraCredential.MaskedPreview(ctx, target.UserID)

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

func (h *SettingsHandler) UpdateNotification(c *gin.Context) {
	boardID, ok := boardParam(c)
	if !ok {
		return
	}
	target, ok := h.resolve(c)
	if !ok {
		return
	}
	var in user_setting_dto.NotificationInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	saved, err := h.userSettings.SaveNotification(c.Request.Context(), target.UserID, boardID, in)
	if err != nil {
		var verr *user_setting_dto.ValidationError
		if errors.As(err, &verr) {
			c.JSON(http.StatusBadRequest, gin.H{"error": verr.Message, "fields": gin.H{verr.Field: verr.Message}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save settings"})
		return
	}
	h.emit(c, target, SettingsChange{Kind: ChangeNotification, BoardID: boardID})
	c.JSON(http.StatusOK, notificationJSON(saved))
}

// ResetNotification makes a board follow the default again (AC3.4).
func (h *SettingsHandler) ResetNotification(c *gin.Context) {
	boardID, ok := boardParam(c)
	if !ok {
		return
	}
	if boardID == user_setting_dto.DefaultBoardID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pilih board yang mau dikembalikan ke default"})
		return
	}
	target, ok := h.resolve(c)
	if !ok {
		return
	}
	if err := h.userSettings.ResetToDefault(c.Request.Context(), target.UserID, boardID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reset settings"})
		return
	}
	h.emit(c, target, SettingsChange{Kind: ChangeReset, BoardID: boardID})
	setting, err := h.userSettings.GetEffective(c.Request.Context(), target.UserID, boardID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load settings"})
		return
	}
	c.JSON(http.StatusOK, notificationJSON(setting))
}

// Mute mutes globally (no board) or one board (AC4.1, AC4.2).
func (h *SettingsHandler) Mute(c *gin.Context) {
	boardID, ok := boardParam(c)
	if !ok {
		return
	}
	target, ok := h.resolve(c)
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

	if err := h.userSettings.MuteUntil(c.Request.Context(), target.UserID, boardID, until); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to mute"})
		return
	}
	h.emit(c, target, SettingsChange{Kind: ChangeMute, BoardID: boardID, Until: &until})
	c.JSON(http.StatusOK, muteActiveJSON(until))
}

func (h *SettingsHandler) Unmute(c *gin.Context) {
	boardID, ok := boardParam(c)
	if !ok {
		return
	}
	target, ok := h.resolve(c)
	if !ok {
		return
	}
	if err := h.userSettings.Unmute(c.Request.Context(), target.UserID, boardID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to unmute"})
		return
	}
	h.emit(c, target, SettingsChange{Kind: ChangeUnmute, BoardID: boardID})
	c.JSON(http.StatusOK, gin.H{"active": false, "until": nil, "forever": false})
}

// ListBoards returns the board picker entries (AC1.1, AC1.2).
func (h *SettingsHandler) ListBoards(c *gin.Context) {
	target, ok := h.resolve(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	boards, err := h.dashboardSvc.ListBoards(ctx, target.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load boards"})
		return
	}
	states, err := h.userSettings.BoardStates(ctx, target.UserID)
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

func (h *SettingsHandler) SaveCredential(c *gin.Context) {
	target, ok := h.resolve(c)
	if !ok {
		return
	}
	var req saveCredentialRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.JiraEmail == "" || req.APIToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "jiraEmail and apiToken are required"})
		return
	}

	if err := h.jiraCredential.UpsertCredential(c.Request.Context(), target.UserID, req.JiraEmail, req.APIToken); err != nil {
		if err == jira_credential_service.ErrEncryptionKeyNotConfigured {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "fitur ini belum dikonfigurasi server, hubungi admin"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save credential"})
		return
	}
	h.emit(c, target, SettingsChange{Kind: ChangeCredentialUpdated})
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *SettingsHandler) DeleteCredential(c *gin.Context) {
	target, ok := h.resolve(c)
	if !ok {
		return
	}
	if err := h.jiraCredential.DeleteCredential(c.Request.Context(), target.UserID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete credential"})
		return
	}
	h.emit(c, target, SettingsChange{Kind: ChangeCredentialDeleted})
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
