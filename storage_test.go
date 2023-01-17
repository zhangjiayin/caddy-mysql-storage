package storagemysql

import (
	"context"
	"os"
	"testing"

	"github.com/caddyserver/certmagic"
	_ "github.com/go-sql-driver/mysql"
)

func setup(t *testing.T) certmagic.Storage {
	return setupWithOptions(t)
}

func setupWithOptions(t *testing.T) certmagic.Storage {
	os.Setenv("MYSQL_DSN", "caddy_user:caddy_password@tcp(127.0.0.1:3306)/caddy?charset=utf8mb4&parseTime=true")
	connStr := os.Getenv("MYSQL_DSN")
	if connStr == "" {
		t.Skipf("must set MYSQL_DSN")
	}
	// _, err := sql.Open("mysql", connStr)
	// if err != nil {
	// 	t.Fatal(err)
	// }

	c := MysqlStorage{}

	c.Dsn = connStr
	c.QueryTimeout = 10
	c.LockTimeout = 60
	storage, err := NewStorage(c)
	if err != nil {
		t.Fatal(err)
	}
	return storage

}

func TestCaddyMySQLAdapter(t *testing.T) {
	storage := setup(t)

	var ctx = context.Background()

	var testDataSet = []string{"test", "test1", "test2"}

	for _, s := range testDataSet {
		err := storage.Store(ctx, s, []byte(s))
		if err != nil {
			t.Fatalf("TestCaddyMySQLAdapter Store %v", err)
		}
	}

	list_res, list_err := storage.List(ctx, "test", false)
	if list_err != nil {
		t.Fatalf("TestCaddyMySQLAdapter List Error %v", list_err)
	}
	t.Logf("TestCaddyMySQLAdapter list res %s err %v", list_res, list_err)
	for _, s := range testDataSet {
		exists := storage.Exists(ctx, s)
		if !exists {
			t.Fatalf("TestCaddyMySQLAdapter Exists %s not found ", s)
		}

		info, stat_err := storage.Stat(ctx, s)
		if stat_err != nil {
			t.Fatalf("TestCaddyMySQLAdapter Stat %v", stat_err)
		}
		t.Logf("TestCaddyMySQLAdapter res %v", info)

		delete_err := storage.Delete(ctx, s)
		if delete_err != nil {
			t.Fatalf("TestCaddyMySQLAdapter Delete %v", delete_err)
		}

		exists = storage.Exists(ctx, s)
		if exists {
			t.Fatalf("TestCaddyMySQLAdapter Delete %s not works ", s)
		}

		lock_err := storage.Lock(ctx, s)
		if lock_err != nil {
			t.Fatalf("TestCaddyMySQLAdapter Lock %v", lock_err)
		}

		lock_err = storage.Lock(ctx, s)
		if lock_err == nil {
			t.Fatalf("TestCaddyMySQLAdapter Lock not works %v", lock_err)
		}

		lock_err = storage.Unlock(ctx, s)
		if lock_err != nil {
			t.Fatalf("TestCaddyMySQLAdapter Unlock %v", lock_err)
		}
	}

	// res, err := storage.Load(ctx, "ddd")
	// if err != nil {
	// 	t.Fatalf("TestCaddyMySQLAdapter Error %v", err)
	// }

	// t.Logf("TestCaddyMySQLAdapter res %s", string(res))
	// cancel()
}
