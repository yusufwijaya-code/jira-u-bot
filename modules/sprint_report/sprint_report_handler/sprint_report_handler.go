package sprint_report_handler

import "github.com/gin-gonic/gin"

type SprintReportHandler interface {
	Report(c *gin.Context)
}
