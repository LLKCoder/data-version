package httpapi

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"data-vision/backend/internal/config"
	"data-vision/backend/internal/datasource"
	"data-vision/backend/internal/model"
	"data-vision/backend/internal/query"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Handler struct {
	db       *gorm.DB
	cfg      config.Config
	sources  *datasource.Manager
	executor *query.Executor
}

type dashboardInput struct {
	Name            string       `json:"name" binding:"required,max=120"`
	Description     string       `json:"description"`
	TimeRange       string       `json:"timeRange"`
	RefreshInterval int          `json:"refreshInterval"`
	Revision        int          `json:"revision"`
	Panels          []panelInput `json:"panels"`
}

type panelInput struct {
	UID           string          `json:"uid"`
	Title         string          `json:"title" binding:"required,max=120"`
	Type          string          `json:"type" binding:"required,max=40"`
	X             int             `json:"x"`
	Y             int             `json:"y"`
	W             int             `json:"w"`
	H             int             `json:"h"`
	Query         json.RawMessage `json:"query"`
	QueryConfig   json.RawMessage `json:"queryConfig"`
	Options       map[string]any  `json:"options"`
	Visualization map[string]any  `json:"visualization"`
}

type datasourceInput struct {
	Name        string                  `json:"name" binding:"required,max=120"`
	Type        string                  `json:"type" binding:"required,max=30"`
	Config      map[string]any          `json:"config"`
	Credentials *datasource.Credentials `json:"credentials"`
}

func (h *Handler) Health(c *gin.Context) {
	sqlDB, err := h.db.DB()
	if err != nil || sqlDB.Ping() != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "degraded", "database": "unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok", "database": "ok", "time": time.Now().UTC()})
}

func (h *Handler) ListDashboards(c *gin.Context) {
	var dashboards []model.Dashboard
	if err := h.db.Preload("Panels").Order("updated_at desc").Find(&dashboards).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "读取看板失败")
		return
	}
	response := make([]dashboardResponse, 0, len(dashboards))
	for _, dashboard := range dashboards {
		response = append(response, dashboardToResponse(dashboard))
	}
	c.JSON(http.StatusOK, response)
}

func (h *Handler) GetDashboard(c *gin.Context) {
	var dashboard model.Dashboard
	if err := h.db.Preload("Panels").Where("uid = ?", c.Param("uid")).First(&dashboard).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeError(c, http.StatusNotFound, "看板不存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "读取看板失败")
		return
	}
	c.JSON(http.StatusOK, dashboardToResponse(dashboard))
}

func (h *Handler) CreateDashboard(c *gin.Context) {
	var input dashboardInput
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, http.StatusBadRequest, "看板参数不合法")
		return
	}

	dashboard := dashboardFromInput(input, newUID())
	if err := h.db.Create(&dashboard).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "创建看板失败")
		return
	}
	c.JSON(http.StatusCreated, dashboardToResponse(dashboard))
}

func (h *Handler) UpdateDashboard(c *gin.Context) {
	var input dashboardInput
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, http.StatusBadRequest, "看板参数不合法")
		return
	}

	var dashboard model.Dashboard
	if err := h.db.Where("uid = ?", c.Param("uid")).First(&dashboard).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeError(c, http.StatusNotFound, "看板不存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "读取看板失败")
		return
	}
	if input.Revision > 0 && input.Revision != dashboard.Revision {
		writeError(c, http.StatusConflict, "看板已被其他窗口更新，请重新加载")
		return
	}

	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&dashboard).Updates(map[string]any{
			"name":             input.Name,
			"description":      input.Description,
			"time_range":       valueOrDefault(input.TimeRange, "最近 24 小时"),
			"refresh_interval": refreshInterval(input.RefreshInterval),
			"revision":         dashboard.Revision + 1,
		}).Error; err != nil {
			return err
		}
		if err := tx.Where("dashboard_id = ?", dashboard.ID).Delete(&model.Panel{}).Error; err != nil {
			return err
		}
		panels := panelsFromInput(input.Panels, dashboard.ID)
		if len(panels) > 0 {
			return tx.Create(&panels).Error
		}
		return nil
	})
	if err != nil {
		writeError(c, http.StatusInternalServerError, "更新看板失败")
		return
	}
	h.GetDashboard(c)
}

func (h *Handler) DeleteDashboard(c *gin.Context) {
	result := h.db.Where("uid = ?", c.Param("uid")).Delete(&model.Dashboard{})
	if result.Error != nil {
		writeError(c, http.StatusInternalServerError, "删除看板失败")
		return
	}
	if result.RowsAffected == 0 {
		writeError(c, http.StatusNotFound, "看板不存在")
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) ExportPDF(c *gin.Context) {
	var dashboard model.Dashboard
	if err := h.db.Where("uid = ?", c.Param("uid")).First(&dashboard).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeError(c, http.StatusNotFound, "看板不存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "读取看板失败")
		return
	}

	payload, err := json.Marshal(map[string]string{
		"url":      strings.TrimRight(h.cfg.FrontendRenderURL, "/") + "/dashboards/" + dashboard.UID + "?export=1",
		"filename": dashboard.Name + ".pdf",
	})
	if err != nil {
		writeError(c, http.StatusInternalServerError, "创建导出请求失败")
		return
	}

	request, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, strings.TrimRight(h.cfg.ExporterURL, "/")+"/render", bytes.NewReader(payload))
	if err != nil {
		writeError(c, http.StatusInternalServerError, "创建导出请求失败")
		return
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 90 * time.Second}).Do(request)
	if err != nil {
		writeError(c, http.StatusBadGateway, "PDF 服务暂不可用")
		return
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		writeError(c, http.StatusBadGateway, "PDF 生成失败")
		return
	}

	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, dashboard.Name+".pdf"))
	c.DataFromReader(http.StatusOK, response.ContentLength, "application/pdf", response.Body, nil)
}

func dashboardFromInput(input dashboardInput, uid string) model.Dashboard {
	return model.Dashboard{
		UID:             uid,
		Name:            input.Name,
		Description:     input.Description,
		TimeRange:       valueOrDefault(input.TimeRange, "最近 24 小时"),
		RefreshInterval: refreshInterval(input.RefreshInterval),
		Revision:        1,
		Panels:          panelsFromInput(input.Panels, 0),
	}
}

func panelsFromInput(inputs []panelInput, dashboardID uint) []model.Panel {
	panels := make([]model.Panel, 0, len(inputs))
	for _, input := range inputs {
		options, _ := json.Marshal(input.Options)
		visualization, _ := json.Marshal(input.Visualization)
		queryConfig := input.QueryConfig
		legacyQuery := ""
		if len(queryConfig) == 0 || string(queryConfig) == "null" {
			queryConfig = input.Query
		}
		if len(queryConfig) > 0 && queryConfig[0] == '"' {
			_ = json.Unmarshal(queryConfig, &legacyQuery)
			queryConfig = nil
		}
		if len(queryConfig) > 0 && !json.Valid(queryConfig) {
			queryConfig = nil
		}
		panelUID := input.UID
		if strings.TrimSpace(panelUID) == "" {
			panelUID = newPanelUID()
		}
		panels = append(panels, model.Panel{
			DashboardID:       dashboardID,
			UID:               panelUID,
			Title:             input.Title,
			Type:              input.Type,
			PosX:              input.X,
			PosY:              input.Y,
			PosW:              positiveOrDefault(input.W, 6),
			PosH:              positiveOrDefault(input.H, 4),
			Query:             legacyQuery,
			QueryConfigJSON:   jsonStringOrEmpty(queryConfig),
			OptionsJSON:       string(options),
			VisualizationJSON: string(visualization),
		})
	}
	return panels
}

func newUID() string {
	var buffer [6]byte
	if _, err := rand.Read(buffer[:]); err == nil {
		return "dashboard-" + hex.EncodeToString(buffer[:])
	}
	return fmt.Sprintf("dashboard-%d", time.Now().UnixNano())
}

func newPanelUID() string {
	var buffer [6]byte
	if _, err := rand.Read(buffer[:]); err == nil {
		return "panel-" + hex.EncodeToString(buffer[:])
	}
	return fmt.Sprintf("panel-%d", time.Now().UnixNano())
}

func jsonStringOrEmpty(value json.RawMessage) string {
	if len(value) == 0 || string(value) == "null" {
		return "null"
	}
	return string(value)
}

func valueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func refreshInterval(value int) int {
	if value < 0 {
		return 0
	}
	if value == 0 {
		return 30
	}
	return value
}

func positiveOrDefault(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

func writeError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"error": message})
}
