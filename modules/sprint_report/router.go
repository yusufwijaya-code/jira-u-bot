package sprint_report

import (
	"github.com/gin-gonic/gin"

	"nudge/modules/sprint_report/sprint_report_handler"
)

func Router(api *gin.RouterGroup, handler sprint_report_handler.SprintReportHandler) {
	api.GET("/report", handler.GetReport)
}
