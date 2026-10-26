package registration_handler

import (
	"errors"
	"html/template"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"

	"jira-u-bot/modules/registration/registration_service"
	regtemplates "jira-u-bot/modules/registration/templates"
)

var emailPattern = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

type formData struct {
	Error string
}

type successData struct {
	Name  string
	Email string
}

type registrationHandlerImpl struct {
	formTemplate        *template.Template
	successTemplate     *template.Template
	registrationService registration_service.RegistrationService
}

func NewRegistrationHandler(registrationService registration_service.RegistrationService) RegistrationHandler {
	formTmpl := template.Must(template.ParseFS(regtemplates.FS, "register.html"))
	successTmpl := template.Must(template.ParseFS(regtemplates.FS, "register_success.html"))
	return &registrationHandlerImpl{
		formTemplate:        formTmpl,
		successTemplate:     successTmpl,
		registrationService: registrationService,
	}
}

func (h *registrationHandlerImpl) ShowForm(c *gin.Context) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Status(http.StatusOK)
	if err := h.formTemplate.Execute(c.Writer, nil); err != nil {
		c.String(http.StatusInternalServerError, "failed to render form")
	}
}

func (h *registrationHandlerImpl) Submit(c *gin.Context) {
	name := strings.TrimSpace(c.PostForm("name"))
	email := strings.ToLower(strings.TrimSpace(c.PostForm("email")))

	if name == "" || email == "" {
		h.renderFormError(c, http.StatusBadRequest, "Nama dan email wajib diisi")
		return
	}
	if !emailPattern.MatchString(email) {
		h.renderFormError(c, http.StatusBadRequest, "Format email tidak valid")
		return
	}

	err := h.registrationService.Register(c.Request.Context(), name, email)
	switch {
	case err == nil:
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.Status(http.StatusOK)
		h.successTemplate.Execute(c.Writer, successData{Name: name, Email: email})
	case errors.Is(err, registration_service.ErrJiraAccountNotFound):
		h.renderFormError(c, http.StatusNotFound, "Email ini tidak terdaftar sebagai account Jira esb-id — gunakan email kantor yang sama dengan akun Jira kamu")
	case errors.Is(err, registration_service.ErrJiraLookupUnavailable):
		h.renderFormError(c, http.StatusServiceUnavailable, "Gagal memverifikasi email ke Jira, coba lagi sebentar lagi")
	default:
		h.renderFormError(c, http.StatusInternalServerError, "Terjadi kesalahan, coba lagi")
	}
}

func (h *registrationHandlerImpl) renderFormError(c *gin.Context, status int, message string) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Status(status)
	h.formTemplate.Execute(c.Writer, formData{Error: message})
}
