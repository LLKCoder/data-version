package datasource

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"data-vision/backend/internal/model"

	mysqlDriver "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/mattn/go-sqlite3"
)

const (
	TypeMySQL    = "mysql"
	TypePostgres = "postgres"
	TypeSQLite   = "sqlite"
	TypeHTTP     = "http"
)

type Credentials struct {
	Password string            `json:"password,omitempty"`
	Token    string            `json:"token,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
}

type Manager struct {
	mu      sync.RWMutex
	key     []byte
	sources map[string]model.DataSource
	pools   map[string]*sql.DB
}

func NewManager(encryptionKey string) *Manager {
	digest := sha256.Sum256([]byte(encryptionKey))
	return &Manager{key: digest[:], sources: make(map[string]model.DataSource), pools: make(map[string]*sql.DB)}
}

func (m *Manager) Load(source model.DataSource) {
	m.mu.Lock()
	m.sources[source.UID] = source
	m.mu.Unlock()
}

func (m *Manager) Register(ctx context.Context, source model.DataSource) error {
	m.mu.Lock()
	old := m.pools[source.UID]
	delete(m.pools, source.UID)
	m.sources[source.UID] = source
	m.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	if source.Type == TypeHTTP {
		return nil
	}

	db, err := m.open(source)
	if err != nil {
		return err
	}
	if err := ping(ctx, db); err != nil {
		_ = db.Close()
		return err
	}
	m.mu.Lock()
	m.pools[source.UID] = db
	m.mu.Unlock()
	return nil
}

func (m *Manager) Test(ctx context.Context, source model.DataSource) error {
	if source.Type == TypeHTTP {
		return testHTTP(ctx, source)
	}
	db, err := m.open(source)
	if err != nil {
		return err
	}
	defer db.Close()
	return ping(ctx, db)
}

func (m *Manager) DB(ctx context.Context, uid string) (*sql.DB, model.DataSource, error) {
	m.mu.RLock()
	source, ok := m.sources[uid]
	db := m.pools[uid]
	m.mu.RUnlock()
	if !ok {
		return nil, model.DataSource{}, fmt.Errorf("数据源不存在: %s", uid)
	}
	if source.Type == TypeHTTP {
		return nil, source, errors.New("HTTP 数据源没有数据库连接")
	}
	if db != nil {
		return db, source, nil
	}
	if err := m.Register(ctx, source); err != nil {
		return nil, source, err
	}
	m.mu.RLock()
	db = m.pools[uid]
	m.mu.RUnlock()
	if db == nil {
		return nil, source, errors.New("数据源连接不可用")
	}
	return db, source, nil
}

func (m *Manager) Source(uid string) (model.DataSource, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	source, ok := m.sources[uid]
	return source, ok
}

func (m *Manager) Remove(uid string) {
	m.mu.Lock()
	delete(m.sources, uid)
	old := m.pools[uid]
	delete(m.pools, uid)
	m.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
}

func (m *Manager) Close() error {
	m.mu.Lock()
	pools := make([]*sql.DB, 0, len(m.pools))
	for uid, pool := range m.pools {
		pools = append(pools, pool)
		delete(m.pools, uid)
	}
	m.mu.Unlock()
	var first error
	for _, pool := range pools {
		if err := pool.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (m *Manager) Encrypt(value Credentials) (string, error) {
	plain, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(m.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, plain, nil)
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (m *Manager) Decrypt(source model.DataSource) (Credentials, error) {
	if source.SecretJSON == "" {
		return Credentials{}, nil
	}
	sealed, err := base64.RawURLEncoding.DecodeString(source.SecretJSON)
	if err != nil {
		return Credentials{}, errors.New("数据源密钥格式无效")
	}
	block, err := aes.NewCipher(m.key)
	if err != nil {
		return Credentials{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return Credentials{}, err
	}
	if len(sealed) < gcm.NonceSize() {
		return Credentials{}, errors.New("数据源密钥内容无效")
	}
	plain, err := gcm.Open(nil, sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():], nil)
	if err != nil {
		return Credentials{}, errors.New("数据源密钥无法解密")
	}
	var credentials Credentials
	if err := json.Unmarshal(plain, &credentials); err != nil {
		return Credentials{}, errors.New("数据源密钥内容无效")
	}
	return credentials, nil
}

func (m *Manager) open(source model.DataSource) (*sql.DB, error) {
	var config map[string]any
	if err := json.Unmarshal([]byte(source.ConfigJSON), &config); err != nil {
		return nil, errors.New("数据源连接配置无效")
	}
	credentials, err := m.Decrypt(source)
	if err != nil {
		return nil, err
	}
	host := stringValue(config, "host", "127.0.0.1")
	port := intValue(config, "port", map[string]int{TypeMySQL: 3306, TypePostgres: 5432}[source.Type])
	database := stringValue(config, "database", "")
	username := stringValue(config, "username", "")

	var driverName, dsn string
	switch source.Type {
	case TypeMySQL:
		driverName = "mysql"
		mysqlConfig := mysqlDriver.Config{User: username, Passwd: credentials.Password, Net: "tcp", Addr: net.JoinHostPort(host, strconv.Itoa(port)), DBName: database, Params: map[string]string{"charset": "utf8mb4", "parseTime": "true", "loc": "Local"}}
		dsn = mysqlConfig.FormatDSN()
	case TypePostgres:
		driverName = "pgx"
		dsnURL := url.URL{Scheme: "postgres", User: url.UserPassword(username, credentials.Password), Host: net.JoinHostPort(host, strconv.Itoa(port)), Path: "/" + database}
		query := dsnURL.Query()
		query.Set("sslmode", stringValue(config, "sslMode", "disable"))
		dsnURL.RawQuery = query.Encode()
		dsn = dsnURL.String()
	case TypeSQLite:
		driverName = "sqlite3"
		dsn = stringValue(config, "path", "data-vision.sqlite")
	default:
		return nil, fmt.Errorf("不支持的数据源类型: %s", source.Type)
	}
	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxIdleConns(5)
	db.SetMaxOpenConns(20)
	db.SetConnMaxLifetime(30 * time.Minute)
	return db, nil
}

func ping(ctx context.Context, db *sql.DB) error {
	pingContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return db.PingContext(pingContext)
}

func testHTTP(ctx context.Context, source model.DataSource) error {
	baseURL, err := HTTPBaseURL(source)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL.String(), nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1024))
	if response.StatusCode >= 500 {
		return fmt.Errorf("HTTP 服务返回 %d", response.StatusCode)
	}
	return nil
}

func HTTPBaseURL(source model.DataSource) (*url.URL, error) {
	var config map[string]any
	if err := json.Unmarshal([]byte(source.ConfigJSON), &config); err != nil {
		return nil, errors.New("HTTP 数据源配置无效")
	}
	value := strings.TrimSpace(stringValue(config, "baseUrl", ""))
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("HTTP 数据源 Base URL 必须是 http/https 地址")
	}
	if parsed.User != nil {
		return nil, errors.New("HTTP 数据源 Base URL 不能包含用户名或密码")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed, nil
}

func stringValue(values map[string]any, key, fallback string) string {
	if value, ok := values[key].(string); ok && strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func intValue(values map[string]any, key string, fallback int) int {
	switch value := values[key].(type) {
	case float64:
		if value > 0 {
			return int(value)
		}
	case int:
		if value > 0 {
			return value
		}
	case string:
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}
