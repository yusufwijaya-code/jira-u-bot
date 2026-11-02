package sprint_report

import (
	"github.com/gin-gonic/gin"

	"jira-u-bot/modules/sprint_report/sprint_report_handler"
)

func Router(dashboard *gin.RouterGroup, api *gin.RouterGroup, handler sprint_report_handler.SprintReportHandler) {
	dashboard.GET("/report", handler.Report)
	api.GET("/report", handler.GetReport)
}
