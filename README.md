# certmagic-sqlstorage

[![GoDoc](https://godoc.org/github.com/yroc92/certmagic-sqlstorage?status.svg)](https://godoc.org/github.com/yroc92/certmagic-sqlstorage)
[![GoDoc](https://godoc.org/github.com/yroc92/postgres-storage?status.svg)](https://godoc.org/github.com/yroc92/postgres-storage)

SQL storage for CertMagic/Caddy TLS data.

Currently supports MySQL but it'd be pretty easy to support other RDBs like
SQLite and MySQL. Please make a pull-request if you add support for them and I'll
gladly merge.

Now with support for Caddyfile and environment configuration.

# Example
- Valid values for sslmode are: disable, require, verify-ca, verify-full

## With vanilla JSON config file and single connection string:
```json
{
	  "storage": {
		"module": "mysql",
		"dsn": "caddy_user:caddy_password@tcp(127.0.0.1:3306)/caddy?charset=utf8mb4"
	  }
	  "app": {
	    	...
	  }
}
```

With Caddyfile:
```Caddyfile
# Global Config

{
	storage mysql {
		dsn caddy_user:caddy_password@tcp(127.0.0.1:3306)/caddy?charset=utf8mb4
	}
}

```

From Environment:
```text
MYSQL_DSN

```

From build
```
xcaddy build \
    --with github.com/zhangjiayin/caddy-mysql-storage           \
    --with github.com/caddy-dns/cloudflare
```

# LICENSE

MIT
