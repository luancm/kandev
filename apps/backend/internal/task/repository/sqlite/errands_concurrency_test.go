package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestErrandClaimSnapshotSerializesParentAdmission(t *testing.T) {
	exerciseErrandClaimAdmissionConcurrency(t, newRepoForSessionTests(t))
}

func TestPostgresErrandClaimSnapshotSerializesParentAdmission(t *testing.T) {
	exerciseErrandClaimAdmissionConcurrency(t, newPostgresErrandRepo(t))
}

func exerciseErrandClaimAdmissionConcurrency(t *testing.T, repo *Repository) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	row := newAdmissionResult(t, repo, "serialization")
	claimGate := newErrandSQLGate("UPDATE task_session_errands SET result_state", true)
	defer claimGate.unblock()
	claimer := independentErrandRepository(t, repo, claimGate)
	claimResult := make(chan error, 1)
	go func() {
		_, err := claimer.ClaimResults(ctx, row.TaskID, row.ParentSessionID, models.ErrandDeliveryTurnContext, "serialized-token",
			[]models.ErrandClaimCandidate{{ID: row.ID, Version: row.Version}}, errandScenarioBase, claimInventory(t, claimer, row.ParentSessionID))
		claimResult <- err
	}()
	awaitSignal(t, claimGate.entered, "claim paused after its gated snapshot")
	guardQuery := "UPDATE tasks SET id = id WHERE 0"
	if dialect.IsPostgres(repo.db.DriverName()) {
		guardQuery = "SELECT pg_advisory_xact_lock"
	}
	admissionGate := newErrandSQLGate(guardQuery, false)
	admitter := independentErrandRepository(t, repo, admissionGate)
	admissionResult := make(chan error, 1)
	a := parentAdmission(row, "concurrent")
	go func() { _, err := admitter.ReserveErrandParentAdmission(ctx, a); admissionResult <- err }()
	awaitSignal(t, admissionGate.entered, "independent admission attempting the same task SQL guard")
	claimGate.unblock()
	if err := awaitError(t, claimResult, "claim commit"); err != nil {
		t.Fatal(err)
	}
	if err := awaitError(t, admissionResult, "reservation after claim"); err != nil {
		t.Fatal(err)
	}
	assertAdmissionResult(t, repo, row.ID, 0, models.ErrandResultDelivered)
	if err := admitter.DecideErrandParentAdmission(ctx, a, models.ErrandAdmissionAccepted); err != nil {
		t.Fatal(err)
	}
	claim := models.ErrandResultClaim{ErrandID: row.ID, ResultRevision: row.ResultRevision,
		ParentAdmissionGeneration: 0, Token: "serialized-token", Route: models.ErrandDeliveryTurnContext}
	if _, err := claimer.ReleaseErrandResultClaims(ctx, row.TaskID, row.ParentSessionID, []models.ErrandResultClaim{claim}); err != nil {
		t.Fatal(err)
	}
	assertAdmissionResult(t, repo, row.ID, 1, models.ErrandResultSuperseded)
}

// Each repository has a distinct native connection, not just a goroutine
// waiting for the original repository's one-connection pool.
func independentErrandRepository(t *testing.T, repo *Repository, gate *errandSQLGate) *Repository {
	t.Helper()
	var dsn string
	if dialect.IsPostgres(repo.db.DriverName()) {
		var schema string
		if err := repo.db.Get(&schema, `SELECT current_schema()`); err != nil {
			t.Fatal(err)
		}
		dsn = testutil.PostgresDSNWithSearchPath(t, testutil.PostgresDSNFromEnv(t), schema)
	} else {
		var sequence int
		var name, path string
		if err := repo.db.QueryRow(`PRAGMA database_list`).Scan(&sequence, &name, &path); err != nil {
			t.Fatal(err)
		}
		dsn = fmt.Sprintf("file:%s?_foreign_keys=on&_busy_timeout=5000&_journal_mode=WAL&_txlock=deferred", path)
	}
	raw := sql.OpenDB(&errandGateConnector{driver: repo.db.Driver(), dsn: dsn, gate: gate})
	t.Cleanup(func() { _ = raw.Close() })
	raw.SetMaxOpenConns(1)
	conn := sqlx.NewDb(raw, repo.db.DriverName())
	return &Repository{db: conn, ro: conn}
}

type errandSQLGate struct {
	prefix      string
	entered     chan struct{}
	release     chan struct{}
	once        sync.Once
	unblockOnce sync.Once
	commitFault func(driver.Tx) error
	queryFault  func(string) error
}

func newErrandSQLGate(prefix string, block bool) *errandSQLGate {
	gate := &errandSQLGate{prefix: prefix, entered: make(chan struct{}), release: make(chan struct{})}
	if !block {
		gate.unblock()
	}
	return gate
}

func (g *errandSQLGate) unblock() { g.unblockOnce.Do(func() { close(g.release) }) }

type errandGateConnector struct {
	driver driver.Driver
	dsn    string
	gate   *errandSQLGate
}

func (c *errandGateConnector) Driver() driver.Driver { return c.driver }

func (c *errandGateConnector) Connect(_ context.Context) (driver.Conn, error) {
	conn, err := c.driver.Open(c.dsn)
	if err != nil {
		return nil, err
	}
	return &errandGateConn{Conn: conn, gate: c.gate}, nil
}

type errandGateConn struct {
	driver.Conn
	gate *errandSQLGate
}

func (c *errandGateConn) BeginTx(ctx context.Context, options driver.TxOptions) (driver.Tx, error) {
	tx, err := c.Conn.(driver.ConnBeginTx).BeginTx(ctx, options)
	if err != nil || c.gate.commitFault == nil {
		return tx, err
	}
	return &errandFaultTx{Tx: tx, commit: c.gate.commitFault}, nil
}

func (c *errandGateConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if err := c.gate.wait(ctx, query); err != nil {
		return nil, err
	}
	return c.Conn.(driver.ExecerContext).ExecContext(ctx, query, args)
}

func (g *errandSQLGate) wait(ctx context.Context, query string) error {
	if strings.HasPrefix(strings.TrimSpace(query), g.prefix) {
		g.once.Do(func() { close(g.entered) })
		select {
		case <-g.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (c *errandGateConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if err := c.gate.wait(ctx, query); err != nil {
		return nil, err
	}
	if c.gate.queryFault != nil {
		if err := c.gate.queryFault(query); err != nil {
			return nil, err
		}
	}
	return c.Conn.(driver.QueryerContext).QueryContext(ctx, query, args)
}

type errandFaultTx struct {
	driver.Tx
	commit func(driver.Tx) error
}

func (t *errandFaultTx) Commit() error { return t.commit(t.Tx) }
