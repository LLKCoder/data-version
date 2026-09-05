package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"data-vision/backend/internal/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type dashboardExport struct {
	SchemaVersion int                     `json:"schemaVersion"`
	Dashboard     dashboardExportMetadata `json:"dashboard"`
	DataSources   []datasourceExport      `json:"datasources"`
}

type dashboardExportMetadata struct {
	UID             string        `json:"uid,omitempty"`
	Name            string        `json:"name"`
	Description     string        `json:"description,omitempty"`
	TimeRange       string        `json:"timeRange"`
	RefreshInterval int           `json:"refreshInterval"`
	Panels          []panelExport `json:"panels"`
}

type panelExport struct {
	UID           string `json:"uid"`
	Title         string `json:"title"`
	Type          string `json:"type"`
	X             int    `json:"x"`
	Y             int    `json:"y"`
	W             int    `json:"w"`
	H             int    `json:"h"`
	Query         string `json:"query,omitempty"`
	QueryConfig   any    `json:"queryConfig,omitempty"`
	Visualization any    `json:"visualization,omitempty"`
	Options       any    `json:"options,omitempty"`
}

type datasourceExport struct {
	UID    string         `json:"uid"`
	Name   string         `json:"name"`
	Type   string         `json:"type"`
	Config map[string]any `json:"config"`
}

type dashboardImport struct {
	SchemaVersion int                     `json:"schemaVersion"`
	Dashboard     dashboardExportMetadata `json:"dashboard"`
	DataSources   []datasourceExport      `json:"datasources"`
	ReplaceUID    string                  `json:"replaceUid,omitempty"`
}

func (h *Handler) ExportDashboard(c *gin.Context) {
	var dashboard model.Dashboard
	if err := h.db.Preload("Panels").Where("uid = ?", c.Param("uid")).First(&dashboard).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "看板不存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "读取看板失败")
		return
	}
	var sources []model.DataSource
	if err := h.db.Order("name asc").Find(&sources).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "读取数据源失败")
		return
	}
	export := dashboardExport{SchemaVersion: 1, Dashboard: dashboardMetadataExport(dashboard), DataSources: make([]datasourceExport, 0, len(sources))}
	for _, source := range sources {
		export.DataSources = append(export.DataSources, datasourceExport{UID: source.UID, Name: source.Name, Type: source.Type, Config: parseConfigJSON(source.ConfigJSON)})
	}
	filename := strings.ReplaceAll(dashboard.Name, "\"", "")
	if filename == "" {
		filename = "dashboard"
	}
	c.Header("Content-Disposition", `attachment; filename="`+filename+`.json"`)
	c.JSON(http.StatusOK, export)
}

func (h *Handler) ImportDashboard(c *gin.Context) {
	var input dashboardImport
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, http.StatusBadRequest, "看板 JSON 不合法")
		return
	}
	if input.SchemaVersion == 0 {
		input.SchemaVersion = 1
	}
	if input.SchemaVersion != 1 {
		writeError(c, http.StatusBadRequest, "不支持的看板 JSON 版本")
		return
	}
	if strings.TrimSpace(input.Dashboard.Name) == "" {
		writeError(c, http.StatusBadRequest, "看板名称不能为空")
		return
	}
	if len(input.Dashboard.Panels) > 100 {
		writeError(c, http.StatusBadRequest, "面板数量超过限制")
		return
	}

	result, err := h.importDashboard(c, input)
	if err != nil {
		var conflict importConflict
		if errors.As(err, &conflict) {
			writeError(c, http.StatusConflict, conflict.Error())
			return
		}
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}
	c.JSON(http.StatusCreated, result)
}

func (h *Handler) importDashboard(c *gin.Context, input dashboardImport) (gin.H, error) {
	datasourceMap := make(map[string]string)
	createdSources := make([]model.DataSource, 0)
	err := h.db.Transaction(func(tx *gorm.DB) error {
		for _, exported := range input.DataSources {
			if exported.Type == "" || exported.Name == "" {
				return errors.New("导入数据源缺少名称或类型")
			}
			if exported.UID != "" && !validUID(exported.UID) {
				return errors.New("导入数据源 UID 格式无效")
			}
			var existing model.DataSource
			uidErr := tx.Where("uid = ?", exported.UID).First(&existing).Error
			if uidErr == nil {
				if existing.Type != exported.Type {
					return importConflict("数据源 UID 类型冲突: " + exported.UID)
				}
				datasourceMap[exported.UID] = existing.UID
				continue
			}
			if !errors.Is(uidErr, gorm.ErrRecordNotFound) {
				return uidErr
			}
			nameErr := tx.Where("name = ? AND type = ?", exported.Name, exported.Type).First(&existing).Error
			if nameErr == nil {
				datasourceMap[exported.UID] = existing.UID
				continue
			}
			if !errors.Is(nameErr, gorm.ErrRecordNotFound) {
				return nameErr
			}
			uid := exported.UID
			if uid == "" {
				uid = datasourceUID()
			}
			placeholder := model.DataSource{UID: uid, Name: exported.Name, Type: exported.Type, ConfigJSON: mustJSON(exported.Config), Status: "needs_configuration", LastError: "导入文件未包含数据源密钥，请重新配置"}
			if err := tx.Create(&placeholder).Error; err != nil {
				return err
			}
			createdSources = append(createdSources, placeholder)
			datasourceMap[exported.UID] = uid
		}

		metadata := input.Dashboard
		for _, panel := range metadata.Panels {
			if strings.TrimSpace(panel.Title) == "" || strings.TrimSpace(panel.Type) == "" {
				return errors.New("导入面板缺少标题或类型")
			}
		}
		uid := input.ReplaceUID
		if uid == "" {
			uid = newUID()
		}
		var existingDashboard model.Dashboard
		findErr := tx.Where("uid = ?", uid).First(&existingDashboard).Error
		if input.ReplaceUID != "" {
			if errors.Is(findErr, gorm.ErrRecordNotFound) {
				return importConflict("要替换的看板不存在")
			}
			if findErr != nil {
				return findErr
			}
			metadataRevision := existingDashboard.Revision + 1
			if metadataRevision <= 0 {
				metadataRevision = 1
			}
			existingDashboard.Name, existingDashboard.Description, existingDashboard.TimeRange, existingDashboard.RefreshInterval, existingDashboard.Revision = metadata.Name, metadata.Description, valueOrDefault(metadata.TimeRange, "最近 24 小时"), refreshInterval(metadata.RefreshInterval), metadataRevision
			if err := tx.Save(&existingDashboard).Error; err != nil {
				return err
			}
			if err := tx.Where("dashboard_id = ?", existingDashboard.ID).Delete(&model.Panel{}).Error; err != nil {
				return err
			}
			metadataDashboard := existingDashboard
			metadataDashboard.Panels = panelsFromExport(metadata.Panels, existingDashboard.ID, datasourceMap)
			if len(metadataDashboard.Panels) > 0 {
				if err := tx.Create(&metadataDashboard.Panels).Error; err != nil {
					return err
				}
			}
			input.Dashboard.UID = uid
			return nil
		}
		dashboard := model.Dashboard{UID: uid, Name: metadata.Name, Description: metadata.Description, TimeRange: valueOrDefault(metadata.TimeRange, "最近 24 小时"), RefreshInterval: refreshInterval(metadata.RefreshInterval), Revision: 1}
		if err := tx.Create(&dashboard).Error; err != nil {
			return err
		}
		panels := panelsFromExport(metadata.Panels, dashboard.ID, datasourceMap)
		if len(panels) > 0 {
			if err := tx.Create(&panels).Error; err != nil {
				return err
			}
		}
		input.Dashboard.UID = uid
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, source := range createdSources {
		h.sources.Load(source)
	}
	var dashboard model.Dashboard
	if err := h.db.Preload("Panels").Where("uid = ?", input.Dashboard.UID).First(&dashboard).Error; err != nil {
		return nil, err
	}
	return gin.H{"dashboard": dashboardToResponse(dashboard), "datasourceMap": datasourceMap}, nil
}

func dashboardMetadataExport(dashboard model.Dashboard) dashboardExportMetadata {
	response := dashboardExportMetadata{UID: dashboard.UID, Name: dashboard.Name, Description: dashboard.Description, TimeRange: dashboard.TimeRange, RefreshInterval: dashboard.RefreshInterval, Panels: make([]panelExport, 0, len(dashboard.Panels))}
	for _, panel := range dashboard.Panels {
		uid := panel.UID
		if uid == "" {
			uid = fmt.Sprintf("panel-%d", panel.ID)
		}
		response.Panels = append(response.Panels, panelExport{UID: uid, Title: panel.Title, Type: panel.Type, X: panel.PosX, Y: panel.PosY, W: panel.PosW, H: panel.PosH, Query: panel.Query, QueryConfig: parseJSON(panel.QueryConfigJSON), Visualization: parseJSON(panel.VisualizationJSON), Options: parseJSON(panel.OptionsJSON)})
	}
	return response
}

func panelsFromExport(inputs []panelExport, dashboardID uint, datasourceMap map[string]string) []model.Panel {
	panels := make([]model.Panel, 0, len(inputs))
	for _, input := range inputs {
		queryConfig := input.QueryConfig
		if queryConfig == nil && input.Query != "" {
			queryConfig = map[string]any{"mode": "sql", "sql": input.Query}
		}
		rewriteDatasourceRefs(queryConfig, datasourceMap)
		visualization := input.Visualization
		if visualization == nil {
			visualization = input.Options
		}
		panels = append(panels, model.Panel{DashboardID: dashboardID, UID: valueOrDefault(input.UID, newPanelUID()), Title: input.Title, Type: input.Type, PosX: input.X, PosY: input.Y, PosW: positiveOrDefault(input.W, 6), PosH: positiveOrDefault(input.H, 4), Query: input.Query, QueryConfigJSON: mustJSON(queryConfig), VisualizationJSON: mustJSON(visualization), OptionsJSON: mustJSON(input.Options)})
	}
	return panels
}

func rewriteDatasourceRefs(value any, mappings map[string]string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == "datasourceUid" {
				if uid, ok := child.(string); ok {
					if mapped, exists := mappings[uid]; exists {
						typed[key] = mapped
					}
				}
			} else {
				rewriteDatasourceRefs(child, mappings)
			}
		}
	case []any:
		for _, child := range typed {
			rewriteDatasourceRefs(child, mappings)
		}
	}
}

func parseConfigJSON(value string) map[string]any {
	parsed := parseJSON(value)
	if result, ok := parsed.(map[string]any); ok {
		return result
	}
	return map[string]any{}
}
func mustJSON(value any) string {
	encoded, _ := json.Marshal(value)
	if value == nil {
		return "null"
	}
	return string(encoded)
}

type importConflict string

func (e importConflict) Error() string { return string(e) }

func validUID(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '-' && char != '_' {
			return false
		}
	}
	return true
}
