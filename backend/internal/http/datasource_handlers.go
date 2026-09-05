package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"data-vision/backend/internal/datasource"
	"data-vision/backend/internal/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handler) ListDataSources(c *gin.Context) {
	var sources []model.DataSource
	if err := h.db.Order("name asc").Find(&sources).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "读取数据源失败")
		return
	}
	for _, source := range sources {
		h.sources.Load(source)
	}
	response := make([]datasourceResponse, 0, len(sources))
	for _, source := range sources {
		response = append(response, datasourceToResponse(source))
	}
	c.JSON(http.StatusOK, response)
}

func (h *Handler) GetDataSource(c *gin.Context) {
	source, err := h.findDataSource(c.Param("uid"))
	if err != nil {
		writeDataSourceError(c, err)
		return
	}
	h.sources.Load(source)
	c.JSON(http.StatusOK, datasourceToResponse(source))
}

func (h *Handler) CreateDataSource(c *gin.Context) {
	var input datasourceInput
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, http.StatusBadRequest, "数据源参数不合法")
		return
	}
	source, err := h.sourceFromInput(input, datasourceUID(), model.DataSource{})
	if err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.db.Create(&source).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "保存数据源失败")
		return
	}
	h.sources.Load(source)
	source = h.updateDataSourceConnection(c, source)
	c.JSON(http.StatusCreated, datasourceToResponse(source))
}

func (h *Handler) UpdateDataSource(c *gin.Context) {
	old, err := h.findDataSource(c.Param("uid"))
	if err != nil {
		writeDataSourceError(c, err)
		return
	}
	var input datasourceInput
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, http.StatusBadRequest, "数据源参数不合法")
		return
	}
	source, err := h.sourceFromInput(input, old.UID, old)
	if err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.db.Model(&old).Updates(map[string]any{"name": source.Name, "type": source.Type, "config_json": source.ConfigJSON, "secret_json": source.SecretJSON, "status": "unknown", "last_error": ""}).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "保存数据源失败")
		return
	}
	h.sources.Load(source)
	source = h.updateDataSourceConnection(c, source)
	c.JSON(http.StatusOK, datasourceToResponse(source))
}

func (h *Handler) DeleteDataSource(c *gin.Context) {
	source, err := h.findDataSource(c.Param("uid"))
	if err != nil {
		writeDataSourceError(c, err)
		return
	}
	pattern := "%" + source.UID + "%"
	var references int64
	if err := h.db.Model(&model.Panel{}).Where("query_config_json LIKE ? OR query LIKE ?", pattern, pattern).Count(&references).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "检查数据源引用失败")
		return
	}
	if references > 0 {
		writeError(c, http.StatusConflict, "数据源仍被看板面板引用")
		return
	}
	if err := h.db.Delete(&source).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "删除数据源失败")
		return
	}
	h.sources.Remove(source.UID)
	c.Status(http.StatusNoContent)
}

func (h *Handler) TestDataSource(c *gin.Context) {
	var input datasourceInput
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, http.StatusBadRequest, "数据源参数不合法")
		return
	}
	old, _ := h.findDataSource(c.Param("uid"))
	uid := c.Param("uid")
	if uid == "" {
		uid = datasourceUID()
	}
	source, err := h.sourceFromInput(input, uid, old)
	if err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.sources.Test(c.Request.Context(), source); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "连接测试成功"})
}

func (h *Handler) ListTables(c *gin.Context) {
	tables, err := h.executor.ListTables(c.Request.Context(), c.Param("uid"), c.Query("search"))
	if err != nil {
		writeError(c, http.StatusBadGateway, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"tables": tables})
}

func (h *Handler) DescribeTable(c *gin.Context) {
	columns, err := h.executor.DescribeTable(c.Request.Context(), c.Param("uid"), c.Query("schema"), c.Query("table"))
	if err != nil {
		writeError(c, http.StatusBadGateway, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"columns": columns})
}

func (h *Handler) PreviewTable(c *gin.Context) {
	page := queryInt(c.Query("page"), 1)
	pageSize := queryInt(c.Query("pageSize"), 50)
	preview, err := h.executor.PreviewTable(c.Request.Context(), c.Param("uid"), c.Query("schema"), c.Query("table"), page, pageSize, c.Query("sort"), c.Query("order"))
	if err != nil {
		writeError(c, http.StatusBadGateway, err.Error())
		return
	}
	c.JSON(http.StatusOK, preview)
}

func (h *Handler) findDataSource(uid string) (model.DataSource, error) {
	var source model.DataSource
	if err := h.db.Where("uid = ?", uid).First(&source).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return source, errNotFound("数据源不存在")
		}
		return source, err
	}
	return source, nil
}

func (h *Handler) sourceFromInput(input datasourceInput, uid string, old model.DataSource) (model.DataSource, error) {
	input.Type = strings.ToLower(strings.TrimSpace(input.Type))
	if input.Type != datasource.TypeMySQL && input.Type != datasource.TypePostgres && input.Type != datasource.TypeSQLite && input.Type != datasource.TypeHTTP {
		return model.DataSource{}, fmt.Errorf("不支持的数据源类型: %s", input.Type)
	}
	if strings.TrimSpace(input.Name) == "" {
		return model.DataSource{}, errors.New("数据源名称不能为空")
	}
	config := input.Config
	if config == nil {
		config = map[string]any{}
	}
	configJSON, err := json.Marshal(config)
	if err != nil {
		return model.DataSource{}, errors.New("数据源连接配置无效")
	}
	secretJSON := old.SecretJSON
	if old.UID != "" && old.Type != input.Type {
		secretJSON = ""
	}
	if input.Credentials != nil {
		secretJSON, err = h.sources.Encrypt(*input.Credentials)
		if err != nil {
			return model.DataSource{}, errors.New("数据源密钥保存失败")
		}
	}
	source := model.DataSource{ID: old.ID, UID: uid, Name: strings.TrimSpace(input.Name), Type: input.Type, ConfigJSON: string(configJSON), SecretJSON: secretJSON, Status: "unknown"}
	if source.Type == datasource.TypeHTTP {
		if _, err := datasource.HTTPBaseURL(source); err != nil {
			return model.DataSource{}, err
		}
	}
	return source, nil
}

func (h *Handler) updateDataSourceConnection(c *gin.Context, source model.DataSource) model.DataSource {
	err := h.sources.Register(c.Request.Context(), source)
	if err == nil {
		source.Status, source.LastError = "connected", ""
	} else {
		source.Status, source.LastError = "error", truncateError(err.Error())
	}
	_ = h.db.Model(&model.DataSource{}).Where("uid = ?", source.UID).Updates(map[string]any{"status": source.Status, "last_error": source.LastError}).Error
	return source
}

func datasourceUID() string { return "datasource-" + newUID()[len("dashboard-"):] }
func queryInt(value string, fallback int) int {
	parsed := 0
	if _, err := fmt.Sscan(value, &parsed); err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
func truncateError(value string) string {
	if len(value) > 500 {
		return value[:500]
	}
	return value
}

type notFoundError string

func (e notFoundError) Error() string  { return string(e) }
func errNotFound(message string) error { return notFoundError(message) }
func writeDataSourceError(c *gin.Context, err error) {
	if _, ok := err.(notFoundError); ok {
		writeError(c, http.StatusNotFound, err.Error())
		return
	}
	writeError(c, http.StatusInternalServerError, err.Error())
}
