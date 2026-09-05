package httpapi

import (
	"encoding/json"
	"net/http"

	"data-vision/backend/internal/query"

	"github.com/gin-gonic/gin"
)

func (h *Handler) ExecuteQuery(c *gin.Context) {
	var payload map[string]json.RawMessage
	if err := c.ShouldBindJSON(&payload); err != nil {
		writeError(c, http.StatusBadRequest, "查询配置 JSON 不合法")
		return
	}
	raw := json.RawMessage{}
	if nested, ok := payload["config"]; ok {
		raw = nested
	} else {
		encoded, err := json.Marshal(payload)
		if err != nil {
			writeError(c, http.StatusBadRequest, "查询配置 JSON 不合法")
			return
		}
		raw = encoded
	}
	config, err := query.Decode(raw)
	if err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}
	result, err := h.executor.Execute(c.Request.Context(), config)
	if err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}
