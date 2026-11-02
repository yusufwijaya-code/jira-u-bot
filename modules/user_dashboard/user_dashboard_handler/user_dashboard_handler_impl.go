package user_dashboard_handler

import (
	"html/template"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"jira-u-bot/config"
	"jira-u-bot/modules/auth/auth_dto"
	"jira-u-bot/modules/auth/auth_middleware"
	"jira-u-bot/modules/chat_interactive/chat_interactive_service"
	"jira-u-bot/modules/jira_credential/jira_credential_service"
	userdashboardtemplates "jira-u-bot/modules/user_dashboard/templates"
	"jira-u-bot/modules/user_dashboard/user_dashboard_dto"
	"jira-u-bot/modules/user_dashboard/user_dashboard_service"
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

type teamPageData struct {
	BasePath string
	Email    string
	View     user_dashboard_dto.TeamView
}

type overduePageData struct {
	BasePath string
	Email    string
	Items    []user_dashboard_dto.OverdueItem
}

type userDashboardHandlerImpl struct {
	dashboardSvc    user_dashboard_service.UserDashboardService
	chatInteractive chat_interactive_service.ChatInteractiveService
	jiraCredential  jira_credential_service.JiraCredentialService
	basePath        string

	teamTemplate    *template.Template
	overdueTemplate *template.Template
}

func NewUserDashboardHandler(
	dashboardSvc user_dashboard_service.UserDashboardService,
	chatInteractive chat_interactive_service.ChatInteractiveService,
	jiraCredential jira_credential_service.JiraCredentialService,
	cfg *config.AppConfig,
) UserDashboardHandler {
	return &userDashboardHandlerImpl{
		dashboardSvc:    dashboardSvc,
		chatInteractive: chatInteractive,
		jiraCredential:  jiraCredential,
		basePath:        cfg.BasePath,
		teamTemplate:    template.Must(template.ParseFS(userdashboardtemplates.FS, "team.html")),
		overdueTemplate: template.Must(template.ParseFS(userdashboardtemplates.FS, "overdue.html")),
	}
}

func currentUser(c *gin.Context) auth_dto.Session {
	return c.MustGet(auth_middleware.CurrentUserKey).(auth_dto.Session)
}

func (h *userDashboardHandlerImpl) GetBoard(c *gin.Context) {
	user := currentUser(c)
	ctx := c.Request.Context()

	board, err := h.dashboardSvc.GetMyBoard(ctx, user.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load board"})
		return
	}

	savedPreference, err := h.dashboardSvc.GetReminderTierPreference(ctx, user.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load board"})
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
		"columns": board.Columns,
		"tierPreference": gin.H{
			"tiers": tiers,
		},
		"credential": gin.H{
			"hasCredential": hasCredential,
			"maskedPreview": credentialMasked,
		},
	})
}

func (h *userDashboardHandlerImpl) TeamView(c *gin.Context) {
	view, err := h.dashboardSvc.GetTeamView(c.Request.Context())
	if err != nil {
		c.String(http.StatusInternalServerError, "Gagal memuat data team, coba lagi")
		return
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Status(http.StatusOK)
	h.teamTemplate.Execute(c.Writer, teamPageData{BasePath: h.basePath, Email: currentUser(c).Email, View: view})
}

func (h *userDashboardHandlerImpl) OverdueView(c *gin.Context) {
	items, err := h.dashboardSvc.GetOverdueView(c.Request.Context())
	if err != nil {
		c.String(http.StatusInternalServerError, "Gagal memuat data overdue, coba lagi")
		return
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Status(http.StatusOK)
	h.overdueTemplate.Execute(c.Writer, overduePageData{BasePath: h.basePath, Email: currentUser(c).Email, Items: items})
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

	// Delegates straight to RouteCommand (spec AC4.1) — the exact same
	// validation/transition/credential-resolution path as the bot's
	// /u-geser, zero duplicated logic.
	command := "/u-geser<" + issueKey + ">:" + req.Target
	message := h.chatInteractive.RouteCommand(c.Request.Context(), currentUser(c).Email, command)
	c.JSON(http.StatusOK, gin.H{"message": message})
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
