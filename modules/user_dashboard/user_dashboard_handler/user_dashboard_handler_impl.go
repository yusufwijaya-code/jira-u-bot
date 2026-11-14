package user_dashboard_handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_middleware"
	"nudge/modules/chat_interactive/chat_interactive_service"
	"nudge/modules/jira_credential/jira_credential_service"
	"nudge/modules/user_dashboard/user_dashboard_dto"
	"nudge/modules/user_dashboard/user_dashboard_service"
)

// tierOptions is the fixed 6-tier menu (spec AC2.1) in display order.
var tierOptions = []struct{ Code, Label string }{
	{"2h", "T-2 jam"},
	{"1h", "T-1 jam"},
	{"30m", "T-30 menit"},
	{"15m", "T-15 menit"},
	{"5m", "T-5 menit"},
	{"1m", "T-1 menit"},
}

type userDashboardHandlerImpl struct {
	dashboardSvc    user_dashboard_service.UserDashboardService
	chatInteractive chat_interactive_service.ChatInteractiveService
	jiraCredential  jira_credential_service.JiraCredentialService
}

func NewUserDashboardHandler(
	dashboardSvc user_dashboard_service.UserDashboardService,
	chatInteractive chat_interactive_service.ChatInteractiveService,
	jiraCredential jira_credential_service.JiraCredentialService,
) UserDashboardHandler {
	return &userDashboardHandlerImpl{
		dashboardSvc:    dashboardSvc,
		chatInteractive: chatInteractive,
		jiraCredential:  jiraCredential,
	}
}

func currentUser(c *gin.Context) auth_dto.Session {
	return c.MustGet(auth_middleware.CurrentUserKey).(auth_dto.Session)
}

func (h *userDashboardHandlerImpl) GetSettings(c *gin.Context) {
	user := currentUser(c)
	ctx := c.Request.Context()

	savedPreference, err := h.dashboardSvc.GetReminderTierPreference(ctx, user.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load settings"})
		return
	}
	// nil (never set) means every tier is checked by default (spec AC5.2);
	// a non-nil CSV (possibly "") means only the listed codes are checked.
	preferredSet := map[string]bool{}
	if savedPreference != nil {
		for _, code := range strings.Split(*savedPreference, ",") {
			preferredSet[code] = true
		}
	}
	tiers := make([]string, 0, len(tierOptions))
	for _, opt := range tierOptions {
		if savedPreference == nil || preferredSet[opt.Code] {
			tiers = append(tiers, opt.Code)
		}
	}

	credentialMasked, hasCredential, _ := h.jiraCredential.MaskedPreview(ctx, user.UserID)

	c.JSON(http.StatusOK, gin.H{
		"tierPreference": gin.H{
			"tiers": tiers,
		},
		"credential": gin.H{
			"hasCredential": hasCredential,
			"maskedPreview": credentialMasked,
		},
	})
}

type updatePreferenceRequest struct {
	Tiers []string `json:"tiers"`
}

func (h *userDashboardHandlerImpl) UpdatePreference(c *gin.Context) {
	var req updatePreferenceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// req.Tiers == nil (key omitted) vs []string{} (explicitly empty) both
	// arrive here as a zero-length slice from encoding/json — the dashboard
	// form always submits the key once the user has interacted with it at
	// all, so "" (AC2.1 "deliberately unchecked all") is the correct
	// persisted value for an empty array, same as a NULL config/config.go.
	csv := strings.Join(req.Tiers, ",")
	if err := h.dashboardSvc.SetReminderTierPreference(c.Request.Context(), currentUser(c).Email, csv); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save preference"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
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
	board, err := h.dashboardSvc.GetMyBoard(c.Request.Context(), currentUser(c).Email)
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
