package storagemysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"time"

	"github.com/caddyserver/caddy/caddyfile"
	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/certmagic"
	_ "github.com/go-sql-driver/mysql"
)

type MysqlStorage struct {
	QueryTimeout time.Duration `json:"query_timeout,omitempty"`
	LockTimeout  time.Duration `json:"lock_timeout,omitempty"`
	Dsn          string        `json:"dsn,omitempty"`
	Database     *sql.DB       `json:"-"`
}

func init() {
	caddy.RegisterModule(MysqlStorage{})
}

func (c *MysqlStorage) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	for d.Next() {
		var value string

		key := d.Val()

		if !d.Args(&value) {
			continue
		}

		switch key {
		case "query_timeout":
			QueryTimeout, err := strconv.Atoi(value)
			if err == nil {
				c.QueryTimeout = time.Duration(QueryTimeout)
			}
		case "lock_timeout":
			LockTimeout, err := strconv.Atoi(value)
			if err == nil {
				c.LockTimeout = time.Duration(LockTimeout)
			}
		case "dsn":
			c.Dsn = value
		}
	}

	return nil
}

func (c *MysqlStorage) Provision(ctx caddy.Context) error {

	// Load Environment
	if c.Dsn == "" {
		c.Dsn = os.Getenv("MYSQL_DSN")
	}
	if c.QueryTimeout == 0 {
		c.QueryTimeout = time.Second * 3
	}
	if c.LockTimeout == 0 {
		c.LockTimeout = time.Minute
	}

	return nil
}

func (MysqlStorage) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID: "caddy.storage.mysql",
		New: func() caddy.Module {
			return new(MysqlStorage)
		},
	}
}

func NewStorage(c MysqlStorage) (certmagic.Storage, error) {
	var connStr string
	if len(c.Dsn) > 0 {
		connStr = c.Dsn
	} else {
		return nil, errors.New("Dsn not set")
	}

	database, err := sql.Open("mysql", connStr)
	if err != nil {
		return nil, err
	}
	s := &MysqlStorage{
		Database:     database,
		QueryTimeout: c.QueryTimeout,
		LockTimeout:  c.LockTimeout,
	}
	return s, s.ensureTableSetup()
}

func (c *MysqlStorage) CertMagicStorage() (certmagic.Storage, error) {
	return NewStorage(*c)
}

type DB interface {
	BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
}

// Database RDBs this library supports, currently supports Postgres only.
type Database int

const (
	Mysql Database = iota
)

func (s *MysqlStorage) ensureTableSetup() error {
	ctx, cancel := context.WithTimeout(context.Background(), s.QueryTimeout)
	defer cancel()
	tx, err := s.Database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS  `certmagic_data` ( `key` varchar(1024) NOT NULL,`value` BLOB, `modified`  TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP, UNIQUE KEY (`key`))")
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS `certmagic_locks` (`key` varchar(1024) NOT NULL,`expires` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP, UNIQUE KEY (`key`))")
	if err != nil {
		return err
	}
	return tx.Commit()
}

// Lock the key and implement certmagic.Storage.Lock.
func (s *MysqlStorage) Lock(ctx context.Context, key string) error {
	ctx, cancel := context.WithTimeout(ctx, s.QueryTimeout)
	defer cancel()

	tx, err := s.Database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	if err := s.isLocked(tx, key); err != nil {
		return err
	}

	expires := time.Now().Add(s.LockTimeout)
	if _, err := tx.ExecContext(ctx, "INSERT INTO certmagic_locks (`key`, `expires`) VALUES (?, ?) ON DUPLICATE KEY UPDATE expires = ?", key, expires, expires); err != nil {
		return fmt.Errorf("failed to lock key: %s: %w", key, err)
	}

	return tx.Commit()
}

// Unlock the key and implement certmagic.Storage.Unlock.
func (s *MysqlStorage) Unlock(ctx context.Context, key string) error {
	ctx, cancel := context.WithTimeout(ctx, s.QueryTimeout)
	defer cancel()
	_, err := s.Database.ExecContext(ctx, "DELETE FROM certmagic_locks WHERE `key` = ?", key)
	return err
}

type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

// isLocked returns nil if the key is not locked.
func (s *MysqlStorage) isLocked(queryer queryer, key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.QueryTimeout)
	defer cancel()
	row := queryer.QueryRowContext(ctx, "select exists(select 1 from certmagic_locks where `key` = ? and expires > current_timestamp)", key)
	var locked bool
	if err := row.Scan(&locked); err != nil {
		return err
	}
	if locked {
		return fmt.Errorf("key is locked: %s", key)
	}
	return nil
}

// Store puts value at key.
func (s *MysqlStorage) Store(ctx context.Context, key string, value []byte) error {
	ctx, cancel := context.WithTimeout(ctx, s.QueryTimeout)
	defer cancel()
	_, err := s.Database.ExecContext(ctx, "INSERT INTO certmagic_data (`key`, `value`) VALUES (?, ?) ON DUPLICATE KEY UPDATE  value = ?, modified = current_timestamp", key, value, value)
	return err
}

// Load retrieves the value at key.
func (s *MysqlStorage) Load(ctx context.Context, key string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, s.QueryTimeout)
	defer cancel()
	var value []byte
	err := s.Database.QueryRowContext(ctx, "SELECT value FROM certmagic_data WHERE `key` = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return nil, fs.ErrNotExist
	}
	return value, err
}

// Delete deletes key. An error should be
// returned only if the key still exists
// when the method returns.
func (s *MysqlStorage) Delete(ctx context.Context, key string) error {
	ctx, cancel := context.WithTimeout(ctx, s.QueryTimeout)
	defer cancel()
	_, err := s.Database.ExecContext(ctx, "DELETE FROM certmagic_data WHERE `key` = ?", key)
	return err
}

// Exists returns true if the key exists
// and there was no error checking.
func (s *MysqlStorage) Exists(ctx context.Context, key string) bool {
	ctx, cancel := context.WithTimeout(ctx, s.QueryTimeout)
	defer cancel()
	row := s.Database.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM certmagic_data WHERE `key` = ?)", key)
	var exists bool
	err := row.Scan(&exists)
	return err == nil && exists
}

// List returns all keys that match prefix.
// If recursive is true, non-terminal keys
// will be enumerated (i.e. "directories"
// should be walked); otherwise, only keys
// prefixed exactly by prefix will be listed.
func (s *MysqlStorage) List(ctx context.Context, prefix string, recursive bool) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, s.QueryTimeout)
	defer cancel()
	if recursive {
		return nil, fmt.Errorf("recursive not supported")
	}
	rows, err := s.Database.QueryContext(ctx, fmt.Sprintf("select `key` from certmagic_data where `key` like '%s%%'", prefix))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, nil
}

// Stat returns information about key.
func (s *MysqlStorage) Stat(ctx context.Context, key string) (certmagic.KeyInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, s.QueryTimeout)
	defer cancel()
	var modified time.Time
	var size int64
	row := s.Database.QueryRowContext(ctx, "select length(value), `modified` from certmagic_data where `key` = ?", key)
	err := row.Scan(&size, &modified)
	if err != nil {
		return certmagic.KeyInfo{}, err
	}
	return certmagic.KeyInfo{
		Key:        key,
		Modified:   modified,
		Size:       size,
		IsTerminal: true,
	}, nil
}
