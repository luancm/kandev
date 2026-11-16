package config

import (
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestDatabaseDSNRoundTripsConnectionValues(t *testing.T) {
	for _, password := range []string{"", "a b", "quote'and\\slash", "dbname=other"} {
		t.Run(password, func(t *testing.T) {
			cfg := DatabaseConfig{Host: "127.0.0.1", Port: 55439, User: "user name", Password: password, DBName: "requested db", SSLMode: "disable"}
			parsed, err := pgconn.ParseConfig(cfg.DSN())
			require.NoError(t, err)
			require.Equal(t, cfg.DBName, parsed.Database)
			require.Equal(t, cfg.User, parsed.User)
			require.Equal(t, cfg.Password, parsed.Password)
			require.Equal(t, cfg.Host, parsed.Host)
			require.Equal(t, uint16(cfg.Port), parsed.Port)
		})
	}
}
