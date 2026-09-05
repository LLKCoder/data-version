package httpapi

import (
	"net/http"
	"time"

	"data-vision/backend/internal/config"
	"data-vision/backend/internal/datasource"
	"data-vision/backend/internal/model"
	"data-vision/backend/internal/query"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func NewRouter(db *gorm.DB, cfg config.Config) http.Handler {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery(), cors())

	sources := datasource.NewManager(cfg.DatasourceEncryptionKey)
	var configuredSources []model.DataSource
	if err := db.Find(&configuredSources).Error; err == nil {
		for _, source := range configuredSources {
			sources.Load(source)
		}
	}
	handler := &Handler{db: db, cfg: cfg, sources: sources, executor: query.NewExecutor(sources, time.Duration(cfg.QueryTimeoutSeconds)*time.Second, cfg.QueryMaxRows, cfg.HTTPMaxBodyBytes)}
	v1 := router.Group("/api/v1")
	v1.GET("/health", handler.Health)
	v1.GET("/dashboards", handler.ListDashboards)
	v1.POST("/dashboards", handler.CreateDashboard)
	v1.GET("/dashboards/:uid", handler.GetDashboard)
	v1.PUT("/dashboards/:uid", handler.UpdateDashboard)
	v1.DELETE("/dashboards/:uid", handler.DeleteDashboard)
	v1.POST("/dashboards/:uid/export/pdf", handler.ExportPDF)
	v1.GET("/dashboards/:uid/export", handler.ExportDashboard)
	v1.POST("/dashboards/import", handler.ImportDashboard)
	v1.GET("/datasources", handler.ListDataSources)
	v1.POST("/datasources", handler.CreateDataSource)
	v1.GET("/datasources/:uid", handler.GetDataSource)
	v1.PUT("/datasources/:uid", handler.UpdateDataSource)
	v1.DELETE("/datasources/:uid", handler.DeleteDataSource)
	v1.POST("/datasources/:uid/test", handler.TestDataSource)
	v1.GET("/datasources/:uid/tables", handler.ListTables)
	v1.GET("/datasources/:uid/tables/schema", handler.DescribeTable)
	v1.GET("/datasources/:uid/table-preview", handler.PreviewTable)
	v1.POST("/query/execute", handler.ExecuteQuery)

	return router
}

func cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
