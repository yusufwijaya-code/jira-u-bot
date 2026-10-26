package registration_handler

import (
	"html/template"
	"net/http"

	"github.com/gin-gonic/gin"

	regtemplates "jira-u-bot/modules/registration/templates"
)

type registrationHandlerImpl struct {
	formTemplate *template.Template
}

func NewRegistrationHandler() RegistrationHandler {
	formTmpl := template.Must(template.ParseFS(regtemplates.FS, "register.html"))
	return &registrationHandlerImpl{formTemplate: formTmpl}
}

func (h *registrationHandlerImpl) ShowForm(c *gin.Context) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Status(http.StatusOK)
	if err := h.formTemplate.Execute(c.Writer, nil); err != nil {
		c.String(http.StatusInternalServerError, "failed to render form")
	}
}
