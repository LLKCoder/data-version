package database

import (
	"data-vision/backend/internal/model"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func Open(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetMaxOpenConns(30)

	return db, nil
}

func MigrateAndSeed(db *gorm.DB) error {
	if err := db.AutoMigrate(&model.Dashboard{}, &model.Panel{}, &model.DataSource{}); err != nil {
		return err
	}

	var existing model.Dashboard
	findErr := db.Where("uid = ?", "ops-overview").First(&existing).Error
	if findErr == nil {
		var panelCount int64
		if err := db.Model(&model.Panel{}).Where("dashboard_id = ?", existing.ID).Count(&panelCount).Error; err != nil {
			return err
		}
		if panelCount == 0 {
			return db.Create(samplePanels(existing.ID)).Error
		}
		return nil
	}
	if findErr != gorm.ErrRecordNotFound {
		return findErr
	}

	dashboard := model.Dashboard{
		UID:             "ops-overview",
		Name:            "运营总览",
		Description:     "核心服务运行状态与业务趋势",
		TimeRange:       "最近 24 小时",
		RefreshInterval: 30,
	}
	if err := db.Create(&dashboard).Error; err != nil {
		return err
	}
	return db.Create(samplePanels(dashboard.ID)).Error
}

func samplePanels(dashboardID uint) []model.Panel {
	return []model.Panel{
		{DashboardID: dashboardID, UID: "requests", Title: "请求量", Type: "timeseries", PosX: 0, PosY: 0, PosW: 8, PosH: 4, QueryConfigJSON: "null", OptionsJSON: "{}", VisualizationJSON: "{}"},
		{DashboardID: dashboardID, UID: "success-rate", Title: "成功率", Type: "stat", PosX: 8, PosY: 0, PosW: 4, PosH: 4, QueryConfigJSON: "null", OptionsJSON: "{}", VisualizationJSON: "{}"},
		{DashboardID: dashboardID, UID: "latency", Title: "平均响应时间", Type: "timeseries", PosX: 0, PosY: 4, PosW: 6, PosH: 4, QueryConfigJSON: "null", OptionsJSON: "{}", VisualizationJSON: "{}"},
		{DashboardID: dashboardID, UID: "traffic-by-region", Title: "区域流量分布", Type: "bar", PosX: 6, PosY: 4, PosW: 6, PosH: 4, QueryConfigJSON: "null", OptionsJSON: "{}", VisualizationJSON: "{}"},
	}
}
