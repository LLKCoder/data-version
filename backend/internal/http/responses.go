package httpapi

import (
	"encoding/json"
	"fmt"

	"data-vision/backend/internal/model"
)

type dashboardResponse struct {
	ID              uint            `json:"id"`
	UID             string          `json:"uid"`
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	TimeRange       string          `json:"timeRange"`
	RefreshInterval int             `json:"refreshInterval"`
	Revision        int             `json:"revision"`
	Panels          []panelResponse `json:"panels"`
	CreatedAt       string          `json:"createdAt,omitempty"`
	UpdatedAt       string          `json:"updatedAt,omitempty"`
}

type panelResponse struct {
	ID            uint   `json:"id,omitempty"`
	UID           string `json:"uid"`
	Title         string `json:"title"`
	Type          string `json:"type"`
	X             int    `json:"x"`
	Y             int    `json:"y"`
	W             int    `json:"w"`
	H             int    `json:"h"`
	Query         string `json:"query,omitempty"`
	QueryConfig   any    `json:"queryConfig,omitempty"`
	Options       any    `json:"options,omitempty"`
	Visualization any    `json:"visualization,omitempty"`
}

type datasourceResponse struct {
	ID        uint           `json:"id"`
	UID       string         `json:"uid"`
	Name      string         `json:"name"`
	Type      string         `json:"type"`
	Config    map[string]any `json:"config"`
	Status    string         `json:"status"`
	LastError string         `json:"lastError,omitempty"`
	CreatedAt string         `json:"createdAt,omitempty"`
	UpdatedAt string         `json:"updatedAt,omitempty"`
}

func dashboardToResponse(dashboard model.Dashboard) dashboardResponse {
	response := dashboardResponse{ID: dashboard.ID, UID: dashboard.UID, Name: dashboard.Name, Description: dashboard.Description, TimeRange: dashboard.TimeRange, RefreshInterval: dashboard.RefreshInterval, Revision: dashboard.Revision, Panels: make([]panelResponse, 0, len(dashboard.Panels))}
	if response.Revision == 0 {
		response.Revision = 1
	}
	if !dashboard.CreatedAt.IsZero() {
		response.CreatedAt = dashboard.CreatedAt.Format("2006-01-02T15:04:05.000Z07:00")
	}
	if !dashboard.UpdatedAt.IsZero() {
		response.UpdatedAt = dashboard.UpdatedAt.Format("2006-01-02T15:04:05.000Z07:00")
	}
	for index, panel := range dashboard.Panels {
		uid := panel.UID
		if uid == "" {
			uid = fmt.Sprintf("panel-%d", panel.ID)
		}
		queryConfig := parseJSON(panel.QueryConfigJSON)
		if queryConfig == nil && panel.Query != "" {
			queryConfig = map[string]any{"mode": "sql", "sql": panel.Query}
		}
		options := parseJSON(panel.OptionsJSON)
		visualization := parseJSON(panel.VisualizationJSON)
		if visualization == nil {
			visualization = options
		}
		response.Panels = append(response.Panels, panelResponse{ID: panel.ID, UID: uid, Title: panel.Title, Type: panel.Type, X: panel.PosX, Y: panel.PosY, W: panel.PosW, H: panel.PosH, Query: panel.Query, QueryConfig: queryConfig, Options: options, Visualization: visualization})
		if response.Panels[index].W <= 0 {
			response.Panels[index].W = 6
		}
		if response.Panels[index].H <= 0 {
			response.Panels[index].H = 4
		}
	}
	return response
}

func datasourceToResponse(source model.DataSource) datasourceResponse {
	response := datasourceResponse{ID: source.ID, UID: source.UID, Name: source.Name, Type: source.Type, Config: map[string]any{}, Status: source.Status, LastError: source.LastError}
	if response.Status == "" {
		response.Status = "unknown"
	}
	_ = json.Unmarshal([]byte(source.ConfigJSON), &response.Config)
	if !source.CreatedAt.IsZero() {
		response.CreatedAt = source.CreatedAt.Format("2006-01-02T15:04:05.000Z07:00")
	}
	if !source.UpdatedAt.IsZero() {
		response.UpdatedAt = source.UpdatedAt.Format("2006-01-02T15:04:05.000Z07:00")
	}
	return response
}

func parseJSON(value string) any {
	if value == "" || value == "null" {
		return nil
	}
	var parsed any
	if json.Unmarshal([]byte(value), &parsed) != nil {
		return nil
	}
	return parsed
}
