package query

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"data-vision/backend/internal/datasource"
)

type Config struct {
	Mode          string         `json:"mode"`
	DatasourceUID string         `json:"datasourceUid,omitempty"`
	SQL           string         `json:"sql,omitempty"`
	Params        map[string]any `json:"params,omitempty"`
	Request       *HTTPConfig    `json:"request,omitempty"`
	Nodes         []PipelineNode `json:"nodes,omitempty"`
	OutputNodeID  string         `json:"outputNodeId,omitempty"`
}

type HTTPConfig struct {
	Method   string            `json:"method"`
	Path     string            `json:"path"`
	Params   map[string]any    `json:"params,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
	Body     any               `json:"body,omitempty"`
	RowsPath string            `json:"rowsPath,omitempty"`
	FieldMap map[string]string `json:"fieldMap,omitempty"`
}

type PipelineNode struct {
	ID         string               `json:"id"`
	Kind       string               `json:"kind"`
	Alias      string               `json:"alias,omitempty"`
	Query      json.RawMessage      `json:"query,omitempty"`
	Input      string               `json:"input,omitempty"`
	Left       string               `json:"left,omitempty"`
	Right      string               `json:"right,omitempty"`
	JoinType   string               `json:"joinType,omitempty"`
	LeftKeys   []string             `json:"leftKeys,omitempty"`
	RightKeys  []string             `json:"rightKeys,omitempty"`
	Fields     map[string]string    `json:"fields,omitempty"`
	GroupBy    []string             `json:"groupBy,omitempty"`
	Aggregates map[string]Aggregate `json:"aggregates,omitempty"`
}

type Aggregate struct {
	Op    string `json:"op"`
	Field string `json:"field,omitempty"`
}

type Column struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type Result struct {
	Columns []Column         `json:"columns"`
	Rows    []map[string]any `json:"rows"`
	Meta    ResultMeta       `json:"meta"`
}

type ResultMeta struct {
	DurationMS int  `json:"durationMs"`
	RowCount   int  `json:"rowCount"`
	Truncated  bool `json:"truncated,omitempty"`
}

type Table struct {
	Schema string `json:"schema"`
	Name   string `json:"name"`
	Type   string `json:"type"`
}

type ColumnInfo struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Nullable   bool   `json:"nullable"`
	PrimaryKey bool   `json:"primaryKey"`
}

type TablePreview struct {
	Schema   string           `json:"schema"`
	Table    string           `json:"table"`
	Columns  []Column         `json:"columns"`
	Rows     []map[string]any `json:"rows"`
	Page     int              `json:"page"`
	PageSize int              `json:"pageSize"`
	Total    int64            `json:"total"`
}

type Executor struct {
	sources     *datasource.Manager
	timeout     time.Duration
	maxRows     int
	maxHTTPBody int64
}

func NewExecutor(sources *datasource.Manager, timeout time.Duration, maxRows int, maxHTTPBody int64) *Executor {
	if maxRows <= 0 {
		maxRows = 10000
	}
	if maxHTTPBody <= 0 {
		maxHTTPBody = 5 * 1024 * 1024
	}
	return &Executor{sources: sources, timeout: timeout, maxRows: maxRows, maxHTTPBody: maxHTTPBody}
}

func Decode(raw json.RawMessage) (Config, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return Config{Mode: "none"}, nil
	}
	var config Config
	if raw[0] == '"' {
		var sqlText string
		if err := json.Unmarshal(raw, &sqlText); err != nil {
			return Config{}, errors.New("旧版 SQL 查询格式无效")
		}
		return Config{Mode: "sql", SQL: sqlText}, nil
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return Config{}, errors.New("查询配置 JSON 无效")
	}
	if config.Mode == "" {
		config.Mode = "none"
	}
	return config, nil
}

func (e *Executor) Execute(ctx context.Context, config Config) (Result, error) {
	started := time.Now()
	queryCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	var result Result
	var err error
	switch strings.ToLower(config.Mode) {
	case "sql":
		result, err = e.executeSQL(queryCtx, config)
	case "http":
		result, err = e.executeHTTP(queryCtx, config)
	case "pipeline":
		result, err = e.executePipeline(queryCtx, config)
	case "", "none":
		return Result{}, errors.New("面板尚未配置查询")
	default:
		return Result{}, fmt.Errorf("不支持的查询模式: %s", config.Mode)
	}
	if err != nil {
		return Result{}, err
	}
	result.Meta.DurationMS = int(time.Since(started).Milliseconds())
	result.Meta.RowCount = len(result.Rows)
	return result, nil
}

func (e *Executor) executeSQL(ctx context.Context, config Config) (Result, error) {
	db, source, err := e.sources.DB(ctx, config.DatasourceUID)
	if err != nil {
		return Result{}, err
	}
	if err := validateSQL(config.SQL); err != nil {
		return Result{}, err
	}
	rewritten, args, err := rewriteParams(config.SQL, config.Params, source.Type)
	if err != nil {
		return Result{}, err
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Result{}, fmt.Errorf("开启只读事务失败: %w", err)
	}
	rows, err := tx.QueryContext(ctx, rewritten, args...)
	if err != nil {
		_ = tx.Rollback()
		return Result{}, fmt.Errorf("执行 SQL 失败: %w", err)
	}
	result, err := rowsResult(rows, e.maxRows)
	closeErr := rows.Close()
	commitErr := tx.Commit()
	if err != nil {
		return Result{}, err
	}
	if closeErr != nil {
		return Result{}, closeErr
	}
	if commitErr != nil {
		return Result{}, fmt.Errorf("提交只读事务失败: %w", commitErr)
	}
	return result, nil
}

func validateSQL(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return errors.New("SQL 不能为空")
	}
	if strings.Contains(trimmed, ";") || strings.Contains(trimmed, "--") || strings.Contains(trimmed, "/*") || strings.Contains(trimmed, "#") {
		return errors.New("SQL 只允许单条不带注释的查询")
	}
	first := strings.ToUpper(strings.Fields(trimmed)[0])
	if first != "SELECT" && first != "WITH" {
		return errors.New("只允许 SELECT 或只读 WITH 查询")
	}
	if regexp.MustCompile(`(?i)\b(insert|update|delete|drop|alter|create|truncate|grant|revoke|call|copy|merge)\b`).MatchString(trimmed) {
		return errors.New("SQL 包含禁止执行的写操作或管理操作")
	}
	return nil
}

func rewriteParams(sqlText string, params map[string]any, driverType string) (string, []any, error) {
	var builder strings.Builder
	args := make([]any, 0)
	postgres := driverType == datasource.TypePostgres
	for index := 0; index < len(sqlText); {
		char := sqlText[index]
		if char == '\'' || char == '"' || char == '`' {
			quote := char
			builder.WriteByte(char)
			index++
			for index < len(sqlText) {
				builder.WriteByte(sqlText[index])
				if sqlText[index] == quote {
					if index+1 < len(sqlText) && sqlText[index+1] == quote {
						builder.WriteByte(sqlText[index+1])
						index += 2
						continue
					}
					index++
					break
				}
				index++
			}
			continue
		}
		if char == ':' && (index == 0 || sqlText[index-1] != ':') && index+1 < len(sqlText) && (unicode.IsLetter(rune(sqlText[index+1])) || sqlText[index+1] == '_') {
			end := index + 2
			for end < len(sqlText) && (unicode.IsLetter(rune(sqlText[end])) || unicode.IsDigit(rune(sqlText[end])) || sqlText[end] == '_') {
				end++
			}
			name := sqlText[index+1 : end]
			value, ok := params[name]
			if !ok {
				return "", nil, fmt.Errorf("缺少 SQL 参数: %s", name)
			}
			if postgres {
				builder.WriteString("$")
				builder.WriteString(strconv.Itoa(len(args) + 1))
			} else {
				builder.WriteByte('?')
			}
			args = append(args, value)
			index = end
			continue
		}
		builder.WriteByte(char)
		index++
	}
	return builder.String(), args, nil
}

func rowsResult(rows *sql.Rows, maxRows int) (Result, error) {
	columnTypes, err := rows.ColumnTypes()
	if err != nil {
		return Result{}, err
	}
	columns := make([]Column, len(columnTypes))
	for index, column := range columnTypes {
		scanType := ""
		if value := column.ScanType(); value != nil {
			scanType = value.String()
		}
		columns[index] = Column{Name: column.Name(), Type: normalizeType(column.DatabaseTypeName(), scanType)}
	}
	values := make([]any, len(columns))
	ptrs := make([]any, len(columns))
	result := Result{Columns: columns, Rows: make([]map[string]any, 0)}
	for rows.Next() {
		if len(result.Rows) >= maxRows {
			result.Meta.Truncated = true
			break
		}
		for index := range values {
			ptrs[index] = &values[index]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return Result{}, err
		}
		row := make(map[string]any, len(columns))
		for index, column := range columns {
			row[column.Name] = normalizeValue(values[index], column.Type)
		}
		result.Rows = append(result.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return Result{}, err
	}
	return result, nil
}

func normalizeType(databaseType, scanType string) string {
	value := strings.ToLower(databaseType + " " + scanType)
	if strings.Contains(value, "bool") {
		return "boolean"
	}
	if strings.Contains(value, "int") || strings.Contains(value, "numeric") || strings.Contains(value, "decimal") || strings.Contains(value, "real") || strings.Contains(value, "double") || strings.Contains(value, "float") {
		return "number"
	}
	if strings.Contains(value, "date") || strings.Contains(value, "time") {
		return "time"
	}
	return "string"
}

func normalizeValue(value any, valueType string) any {
	if value == nil {
		return nil
	}
	if bytes, ok := value.([]byte); ok {
		value = string(bytes)
	}
	if valueType == "number" {
		if number, ok := toFloat(value); ok {
			return number
		}
	}
	return value
}

func (e *Executor) executeHTTP(ctx context.Context, config Config) (Result, error) {
	source, ok := e.sources.Source(config.DatasourceUID)
	if !ok {
		return Result{}, errors.New("HTTP 数据源不存在")
	}
	if source.Type != datasource.TypeHTTP {
		return Result{}, errors.New("查询配置的数据源不是 HTTP 类型")
	}
	if config.Request == nil {
		return Result{}, errors.New("HTTP 请求配置不能为空")
	}
	requestConfig := config.Request
	if err := validateHTTPHeaders(requestConfig.Headers); err != nil {
		return Result{}, err
	}
	base, err := datasource.HTTPBaseURL(source)
	if err != nil {
		return Result{}, err
	}
	requestURL, err := resolveHTTPURL(base, requestConfig.Path)
	if err != nil {
		return Result{}, err
	}
	values := requestURL.Query()
	for key, value := range requestConfig.Params {
		values.Set(key, fmt.Sprint(value))
	}
	requestURL.RawQuery = values.Encode()
	method := strings.ToUpper(requestConfig.Method)
	if method == "" {
		method = http.MethodGet
	}
	var body io.Reader
	if requestConfig.Body != nil && method != http.MethodGet && method != http.MethodHead {
		encoded, err := json.Marshal(requestConfig.Body)
		if err != nil {
			return Result{}, fmt.Errorf("HTTP body 无效: %w", err)
		}
		body = strings.NewReader(string(encoded))
	}
	request, err := http.NewRequestWithContext(ctx, method, requestURL.String(), body)
	if err != nil {
		return Result{}, err
	}
	credentials, err := e.sources.Decrypt(source)
	if err != nil {
		return Result{}, err
	}
	for key, value := range credentials.Headers {
		request.Header.Set(key, value)
	}
	if credentials.Token != "" && request.Header.Get("Authorization") == "" {
		request.Header.Set("Authorization", "Bearer "+credentials.Token)
	}
	for key, value := range requestConfig.Headers {
		request.Header.Set(key, value)
	}
	if body != nil && request.Header.Get("Content-Type") == "" {
		request.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: e.timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return Result{}, fmt.Errorf("HTTP 请求失败: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Result{}, fmt.Errorf("HTTP 请求返回 %d", response.StatusCode)
	}
	limited := io.LimitReader(response.Body, e.maxHTTPBody+1)
	payload, err := io.ReadAll(limited)
	if err != nil {
		return Result{}, err
	}
	if int64(len(payload)) > e.maxHTTPBody {
		return Result{}, errors.New("HTTP 响应超过大小限制")
	}
	var decoded any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return Result{}, fmt.Errorf("HTTP 响应不是有效 JSON: %w", err)
	}
	rows, err := httpRows(decoded, requestConfig.RowsPath, requestConfig.FieldMap)
	if err != nil {
		return Result{}, err
	}
	return resultFromMaps(rows, e.maxRows), nil
}

func validateHTTPHeaders(headers map[string]string) error {
	for key := range headers {
		switch strings.ToLower(key) {
		case "authorization", "proxy-authorization", "cookie", "set-cookie", "x-api-key":
			return fmt.Errorf("敏感请求头 %s 必须配置在 HTTP 数据源凭据中", key)
		}
	}
	return nil
}

func resolveHTTPURL(base *url.URL, path string) (*url.URL, error) {
	if strings.Contains(path, "://") {
		return nil, errors.New("HTTP 面板只能访问数据源 Base URL 下的路径")
	}
	parsed, err := url.Parse(path)
	if err != nil || parsed.Host != "" || parsed.User != nil {
		return nil, errors.New("HTTP 请求路径无效")
	}
	resolved := base.ResolveReference(parsed)
	if resolved.Scheme != base.Scheme || resolved.Host != base.Host {
		return nil, errors.New("HTTP 请求不能离开数据源 Base URL")
	}
	return resolved, nil
}

func httpRows(payload any, rowsPath string, fieldMap map[string]string) ([]map[string]any, error) {
	if rowsPath != "" {
		var ok bool
		payload, ok = jsonPath(payload, rowsPath)
		if !ok {
			return nil, fmt.Errorf("找不到 HTTP 响应路径: %s", rowsPath)
		}
	}
	if list, ok := payload.([]any); ok {
		rows := make([]map[string]any, 0, len(list))
		for _, item := range list {
			if len(fieldMap) == 0 {
				if row, ok := item.(map[string]any); ok {
					rows = append(rows, row)
					continue
				}
				rows = append(rows, map[string]any{"value": item})
				continue
			}
			row := make(map[string]any, len(fieldMap))
			for field, path := range fieldMap {
				value, _ := jsonPath(item, path)
				row[field] = value
			}
			rows = append(rows, row)
		}
		return rows, nil
	}
	if row, ok := payload.(map[string]any); ok {
		return []map[string]any{row}, nil
	}
	return []map[string]any{{"value": payload}}, nil
}

func jsonPath(value any, path string) (any, bool) {
	if path == "" || path == "." {
		return value, true
	}
	for _, part := range strings.Split(strings.TrimPrefix(path, "."), ".") {
		switch current := value.(type) {
		case map[string]any:
			var ok bool
			value, ok = current[part]
			if !ok {
				return nil, false
			}
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(current) {
				return nil, false
			}
			value = current[index]
		default:
			return nil, false
		}
	}
	return value, true
}

func resultFromMaps(rows []map[string]any, maxRows int) Result {
	truncated := len(rows) > maxRows
	columnSet := make(map[string]bool)
	for _, row := range rows {
		for key := range row {
			columnSet[key] = true
		}
	}
	columnNames := make([]string, 0, len(columnSet))
	for key := range columnSet {
		columnNames = append(columnNames, key)
	}
	sort.Strings(columnNames)
	columns := make([]Column, len(columnNames))
	for index, name := range columnNames {
		columns[index] = Column{Name: name, Type: inferType(rows, name)}
	}
	if len(rows) > maxRows {
		rows = rows[:maxRows]
	}
	return Result{Columns: columns, Rows: rows, Meta: ResultMeta{Truncated: truncated}}
}

func inferType(rows []map[string]any, name string) string {
	for _, row := range rows {
		value := row[name]
		switch value.(type) {
		case bool:
			return "boolean"
		case float64, float32, int, int64, uint, uint64, json.Number:
			return "number"
		case time.Time:
			return "time"
		}
	}
	return "string"
}

func (e *Executor) executePipeline(ctx context.Context, config Config) (Result, error) {
	nodes := make(map[string]PipelineNode, len(config.Nodes))
	for _, node := range config.Nodes {
		if node.ID == "" || nodes[node.ID].ID != "" {
			return Result{}, errors.New("pipeline 节点 ID 必须唯一且不能为空")
		}
		nodes[node.ID] = node
	}
	if config.OutputNodeID == "" {
		return Result{}, errors.New("pipeline 缺少输出节点")
	}
	cache := make(map[string]Result)
	visiting := make(map[string]bool)
	var evaluate func(string) (Result, error)
	evaluate = func(id string) (Result, error) {
		if value, ok := cache[id]; ok {
			return value, nil
		}
		if visiting[id] {
			return Result{}, errors.New("pipeline 存在循环依赖")
		}
		node, ok := nodes[id]
		if !ok {
			return Result{}, fmt.Errorf("pipeline 节点不存在: %s", id)
		}
		visiting[id] = true
		defer delete(visiting, id)
		var result Result
		var err error
		switch node.Kind {
		case "source":
			queryConfig, decodeErr := Decode(node.Query)
			if decodeErr != nil {
				return Result{}, decodeErr
			}
			if queryConfig.Mode == "pipeline" || queryConfig.Mode == "none" {
				return Result{}, errors.New("source 节点只能使用 SQL 或 HTTP 查询")
			}
			result, err = e.Execute(ctx, queryConfig)
		case "join":
			left, leftErr := evaluate(node.Left)
			if leftErr != nil {
				return Result{}, leftErr
			}
			right, rightErr := evaluate(node.Right)
			if rightErr != nil {
				return Result{}, rightErr
			}
			result, err = join(left, right, node)
		case "calculate":
			input, inputErr := evaluate(node.Input)
			if inputErr != nil {
				return Result{}, inputErr
			}
			result, err = calculate(input, node.Fields)
		case "aggregate":
			input, inputErr := evaluate(node.Input)
			if inputErr != nil {
				return Result{}, inputErr
			}
			result, err = aggregate(input, node.GroupBy, node.Aggregates)
		default:
			return Result{}, fmt.Errorf("不支持的 pipeline 节点类型: %s", node.Kind)
		}
		if err != nil {
			return Result{}, err
		}
		cache[id] = result
		return result, nil
	}
	return evaluate(config.OutputNodeID)
}

func join(left, right Result, node PipelineNode) (Result, error) {
	if len(node.LeftKeys) == 0 || len(node.LeftKeys) != len(node.RightKeys) {
		return Result{}, errors.New("Join 必须配置数量相同的左右关联字段")
	}
	joinType := strings.ToLower(node.JoinType)
	if joinType == "" {
		joinType = "left"
	}
	if joinType != "left" && joinType != "inner" {
		return Result{}, errors.New("Join 类型只支持 left 或 inner")
	}
	rightIndex := make(map[string][]map[string]any)
	for _, row := range right.Rows {
		key, err := rowKey(row, node.RightKeys)
		if err != nil {
			return Result{}, err
		}
		rightIndex[key] = append(rightIndex[key], row)
	}
	rows := make([]map[string]any, 0)
	for _, leftRow := range left.Rows {
		key, err := rowKey(leftRow, node.LeftKeys)
		if err != nil {
			return Result{}, err
		}
		matches := rightIndex[key]
		if len(matches) == 0 {
			if joinType == "left" {
				rows = append(rows, mergeRows(leftRow, nil, node.Alias))
			}
			continue
		}
		for _, rightRow := range matches {
			rows = append(rows, mergeRows(leftRow, rightRow, node.Alias))
		}
	}
	return resultFromMaps(rows, max(len(rows), 1)), nil
}

func rowKey(row map[string]any, fields []string) (string, error) {
	parts := make([]string, len(fields))
	for index, field := range fields {
		if _, ok := row[field]; !ok {
			return "", fmt.Errorf("Join 字段不存在: %s", field)
		}
		parts[index] = fmt.Sprintf("%T:%v", row[field], row[field])
	}
	return strings.Join(parts, "\x00"), nil
}

func mergeRows(left, right map[string]any, alias string) map[string]any {
	merged := make(map[string]any, len(left)+len(right))
	for key, value := range left {
		merged[key] = value
	}
	for key, value := range right {
		outputKey := key
		if _, exists := merged[outputKey]; exists {
			prefix := alias
			if prefix == "" {
				prefix = "right"
			}
			outputKey = prefix + "_" + key
		}
		merged[outputKey] = value
	}
	return merged
}

func calculate(input Result, fields map[string]string) (Result, error) {
	for name, expression := range fields {
		if strings.TrimSpace(name) == "" || strings.TrimSpace(expression) == "" {
			return Result{}, errors.New("计算字段名称和表达式不能为空")
		}
		for index := range input.Rows {
			value, err := evalExpression(expression, input.Rows[index])
			if err != nil {
				return Result{}, fmt.Errorf("计算字段 %s 失败: %w", name, err)
			}
			input.Rows[index][name] = value
		}
	}
	return resultFromMaps(input.Rows, max(len(input.Rows), 1)), nil
}

func aggregate(input Result, groups []string, definitions map[string]Aggregate) (Result, error) {
	if len(definitions) == 0 {
		return input, nil
	}
	type bucket struct{ rows []map[string]any }
	buckets := make(map[string]*bucket)
	order := make([]string, 0)
	for _, row := range input.Rows {
		key, err := rowKey(row, groups)
		if err != nil {
			return Result{}, err
		}
		if buckets[key] == nil {
			buckets[key] = &bucket{}
			order = append(order, key)
		}
		buckets[key].rows = append(buckets[key].rows, row)
	}
	resultRows := make([]map[string]any, 0, len(order))
	for _, key := range order {
		rows := buckets[key].rows
		row := make(map[string]any, len(groups)+len(definitions))
		for _, group := range groups {
			row[group] = rows[0][group]
		}
		for name, definition := range definitions {
			value, err := aggregateValue(rows, definition)
			if err != nil {
				return Result{}, fmt.Errorf("聚合字段 %s 失败: %w", name, err)
			}
			row[name] = value
		}
		resultRows = append(resultRows, row)
	}
	return resultFromMaps(resultRows, max(len(resultRows), 1)), nil
}

func aggregateValue(rows []map[string]any, definition Aggregate) (any, error) {
	op := strings.ToLower(definition.Op)
	if op == "count" {
		return len(rows), nil
	}
	if definition.Field == "" {
		return nil, errors.New("聚合字段不能为空")
	}
	var total float64
	var count int
	var minValue, maxValue float64
	for _, row := range rows {
		value, ok := toFloat(row[definition.Field])
		if !ok {
			continue
		}
		if count == 0 || value < minValue {
			minValue = value
		}
		if count == 0 || value > maxValue {
			maxValue = value
		}
		total += value
		count++
	}
	if count == 0 {
		return nil, nil
	}
	switch op {
	case "sum":
		return total, nil
	case "avg":
		return total / float64(count), nil
	case "min":
		return minValue, nil
	case "max":
		return maxValue, nil
	default:
		return nil, fmt.Errorf("不支持的聚合函数: %s", definition.Op)
	}
}

func toFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int8:
		return float64(typed), true
	case int16:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case uint:
		return float64(typed), true
	case uint64:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(typed, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}
