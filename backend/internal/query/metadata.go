package query

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"data-vision/backend/internal/datasource"
)

func (e *Executor) ListTables(ctx context.Context, uid, search string) ([]Table, error) {
	db, source, err := e.sources.DB(ctx, uid)
	if err != nil {
		return nil, err
	}
	var queryText string
	switch source.Type {
	case datasource.TypeMySQL:
		queryText = "SELECT table_schema, table_name, table_type FROM information_schema.tables WHERE table_schema = DATABASE() ORDER BY table_name"
	case datasource.TypePostgres:
		queryText = "SELECT table_schema, table_name, table_type FROM information_schema.tables WHERE table_schema NOT IN ('pg_catalog', 'information_schema') ORDER BY table_schema, table_name"
	case datasource.TypeSQLite:
		queryText = "SELECT 'main', name, type FROM sqlite_master WHERE type IN ('table', 'view') AND name NOT LIKE 'sqlite_%' ORDER BY name"
	default:
		return nil, errors.New("HTTP 数据源不支持数据库表浏览")
	}
	rows, err := db.QueryContext(ctx, queryText)
	if err != nil {
		return nil, fmt.Errorf("读取数据表失败: %w", err)
	}
	defer rows.Close()
	result := make([]Table, 0)
	search = strings.ToLower(strings.TrimSpace(search))
	for rows.Next() {
		var table Table
		if err := rows.Scan(&table.Schema, &table.Name, &table.Type); err != nil {
			return nil, err
		}
		if search == "" || strings.Contains(strings.ToLower(table.Schema+"."+table.Name), search) {
			result = append(result, table)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (e *Executor) DescribeTable(ctx context.Context, uid, schema, tableName string) ([]ColumnInfo, error) {
	db, source, err := e.sources.DB(ctx, uid)
	if err != nil {
		return nil, err
	}
	table, err := e.findTable(ctx, uid, schema, tableName)
	if err != nil {
		return nil, err
	}
	schema = table.Schema
	tableName = table.Name
	var rows *sql.Rows
	switch source.Type {
	case datasource.TypeMySQL:
		rows, err = db.QueryContext(ctx, "SELECT column_name, data_type, is_nullable, column_key FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? ORDER BY ordinal_position", tableName)
	case datasource.TypePostgres:
		rows, err = db.QueryContext(ctx, "SELECT c.column_name, c.data_type, c.is_nullable, CASE WHEN EXISTS (SELECT 1 FROM information_schema.key_column_usage k WHERE k.table_schema = c.table_schema AND k.table_name = c.table_name AND k.column_name = c.column_name) THEN 'PRI' ELSE '' END FROM information_schema.columns c WHERE c.table_schema = $1 AND c.table_name = $2 ORDER BY c.ordinal_position", schema, tableName)
	case datasource.TypeSQLite:
		rows, err = db.QueryContext(ctx, "PRAGMA table_info("+quoteIdent(tableName, source.Type)+")")
	default:
		return nil, errors.New("HTTP 数据源不支持字段浏览")
	}
	if err != nil {
		return nil, fmt.Errorf("读取字段信息失败: %w", err)
	}
	defer rows.Close()
	columns := make([]ColumnInfo, 0)
	for rows.Next() {
		if source.Type == datasource.TypeSQLite {
			var cid int
			var name, dataType string
			var notNull, primaryKey int
			var defaultValue any
			if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
				return nil, err
			}
			columns = append(columns, ColumnInfo{Name: name, Type: normalizeType(dataType, ""), Nullable: notNull == 0, PrimaryKey: primaryKey > 0})
			continue
		}
		var column ColumnInfo
		var nullable, key string
		if err := rows.Scan(&column.Name, &column.Type, &nullable, &key); err != nil {
			return nil, err
		}
		column.Type = normalizeType(column.Type, "")
		column.Nullable = strings.EqualFold(nullable, "YES")
		column.PrimaryKey = strings.EqualFold(key, "PRI")
		columns = append(columns, column)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return columns, nil
}

func (e *Executor) PreviewTable(ctx context.Context, uid, schema, tableName string, page, pageSize int, sortField, order string) (TablePreview, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 500 {
		pageSize = 500
	}
	db, source, err := e.sources.DB(ctx, uid)
	if err != nil {
		return TablePreview{}, err
	}
	table, err := e.findTable(ctx, uid, schema, tableName)
	if err != nil {
		return TablePreview{}, err
	}
	columns, err := e.DescribeTable(ctx, uid, table.Schema, table.Name)
	if err != nil {
		return TablePreview{}, err
	}
	allowedSort := ""
	for _, column := range columns {
		if column.Name == sortField {
			allowedSort = column.Name
			break
		}
	}
	if sortField != "" && allowedSort == "" {
		return TablePreview{}, errors.New("排序字段不存在")
	}
	if sortField == "" && len(columns) > 0 {
		allowedSort = columns[0].Name
	}
	direction := "ASC"
	if strings.EqualFold(order, "desc") {
		direction = "DESC"
	}
	qualified := quoteIdent(table.Schema, source.Type) + "." + quoteIdent(table.Name, source.Type)
	var total int64
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+qualified).Scan(&total); err != nil {
		return TablePreview{}, fmt.Errorf("读取表数据总数失败: %w", err)
	}
	placeholderLimit, placeholderOffset := "?", "?"
	if source.Type == datasource.TypePostgres {
		placeholderLimit, placeholderOffset = "$1", "$2"
	}
	queryText := "SELECT * FROM " + qualified
	if allowedSort != "" {
		queryText += " ORDER BY " + quoteIdent(allowedSort, source.Type) + " " + direction
	}
	queryText += " LIMIT " + placeholderLimit + " OFFSET " + placeholderOffset
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return TablePreview{}, err
	}
	rows, err := tx.QueryContext(ctx, queryText, pageSize, (page-1)*pageSize)
	if err != nil {
		_ = tx.Rollback()
		return TablePreview{}, fmt.Errorf("读取表数据失败: %w", err)
	}
	result, err := rowsResult(rows, pageSize)
	closeErr := rows.Close()
	commitErr := tx.Commit()
	if err != nil {
		return TablePreview{}, err
	}
	if closeErr != nil {
		return TablePreview{}, closeErr
	}
	if commitErr != nil {
		return TablePreview{}, commitErr
	}
	return TablePreview{Schema: table.Schema, Table: table.Name, Columns: result.Columns, Rows: result.Rows, Page: page, PageSize: pageSize, Total: total}, nil
}

func (e *Executor) findTable(ctx context.Context, uid, schema, tableName string) (Table, error) {
	schema = strings.TrimSpace(schema)
	tableName = strings.TrimSpace(tableName)
	if tableName == "" {
		return Table{}, errors.New("表名不能为空")
	}
	tables, err := e.ListTables(ctx, uid, tableName)
	if err != nil {
		return Table{}, err
	}
	for _, table := range tables {
		if table.Name == tableName && (schema == "" || table.Schema == schema) {
			return table, nil
		}
	}
	return Table{}, errors.New("数据表不存在")
}

func quoteIdent(value, driverType string) string {
	value = strings.ReplaceAll(value, "`", "``")
	if driverType == datasource.TypeMySQL {
		return "`" + value + "`"
	}
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
