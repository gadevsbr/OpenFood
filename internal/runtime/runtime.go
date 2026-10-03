package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"openfood/internal/app"
)

type Config struct {
	Password   string `json:"password"`
	DBPort     int    `json:"db_port"`
	SetupToken string `json:"setup_token"`
}
type Runtime struct {
	Data, Bin, DSN string
	Config         Config
	mu             sync.Mutex
	Codes          []string
}

func Port() (int, error) {
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		return 0, e
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
func Load(data, bin string) (*Runtime, error) {
	if e := os.MkdirAll(data, 0700); e != nil {
		return nil, e
	}
	if e := Protect(data); e != nil {
		return nil, e
	}
	r := &Runtime{Data: data, Bin: ToolPath(bin)}
	path := filepath.Join(data, "config.json")
	raw, e := os.ReadFile(path)
	if errors.Is(e, os.ErrNotExist) {
		port, e := Port()
		if e != nil {
			return nil, e
		}
		r.Config = Config{app.Token(), port, app.Token()}
		raw, _ = json.MarshalIndent(r.Config, "", "  ")
		if e = os.WriteFile(path, raw, 0600); e != nil {
			return nil, e
		}
	} else if e != nil {
		return nil, e
	} else if e = json.Unmarshal(raw, &r.Config); e != nil {
		return nil, e
	}
	if r.Config.Password == "" || r.Config.DBPort < 1024 || r.Config.DBPort > 65535 {
		return nil, errors.New("local_configuration_invalid")
	}
	u := url.URL{Scheme: "postgres", User: url.UserPassword("openfood", r.Config.Password), Host: net.JoinHostPort("127.0.0.1", strconv.Itoa(r.Config.DBPort)), Path: "/openfood"}
	q := u.Query()
	q.Set("sslmode", "disable")
	u.RawQuery = q.Encode()
	r.DSN = u.String()
	return r, nil
}
func (r *Runtime) Code(code string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Codes = append(r.Codes, code)
	if len(r.Codes) > 100 {
		r.Codes = r.Codes[len(r.Codes)-100:]
	}
	raw, _ := json.Marshal(map[string]any{"at": time.Now().UTC(), "code": code})
	f, e := os.OpenFile(filepath.Join(r.Data, "operations.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e == nil {
		f.Write(append(raw, '\n'))
		f.Close()
	}
}
func (r *Runtime) run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, filepath.Join(r.Bin, name), args...)
	Hide(cmd)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+r.Config.Password, "PGCONNECT_TIMEOUT=10")
	if os.Getenv("OPENFOOD_TEST_DEBUG") == "1" {
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
	}
	if e := cmd.Run(); e != nil {
		r.Code(name + "_failed")
		return errors.New(name + "_failed")
	}
	return nil
}
func (r *Runtime) Start(ctx context.Context) error {
	data := filepath.Join(r.Data, "database")
	if _, e := os.Stat(filepath.Join(data, "PG_VERSION")); errors.Is(e, os.ErrNotExist) {
		pass := filepath.Join(r.Data, "init-password.tmp")
		if e = os.WriteFile(pass, []byte(r.Config.Password), 0600); e != nil {
			return e
		}
		defer os.Remove(pass)
		if e = r.run(ctx, "initdb", "-D", data, "-U", "openfood", "--encoding=UTF8", "--locale=C", "--auth=scram-sha-256", "--pwfile="+pass); e != nil {
			return e
		}
		config := fmt.Sprintf("\nlisten_addresses = '127.0.0.1'\nport = %d\npassword_encryption = 'scram-sha-256'\nlogging_collector = off\nlog_statement = 'none'\nlog_min_error_statement = panic\n", r.Config.DBPort)
		f, e := os.OpenFile(filepath.Join(data, "postgresql.conf"), os.O_APPEND|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		_, e = f.WriteString(config)
		f.Close()
		if e != nil {
			return e
		}
	}
	version, e := os.ReadFile(filepath.Join(data, "PG_VERSION"))
	if e != nil || strings.TrimSpace(string(version)) != "17" {
		return errors.New("postgres_major_incompatible")
	}
	// Existing PID files are handled by PostgreSQL itself, never manually deleted.
	if r.run(ctx, "pg_ctl", "status", "-D", data) != nil {
		if e = r.run(ctx, "pg_ctl", "start", "-D", data, "-l", filepath.Join(r.Data, "postgres.log"), "-w", "-t", "60"); e != nil {
			return e
		}
	}
	u, _ := url.Parse(r.DSN)
	u.Path = "/postgres"
	conn, e := pgx.Connect(ctx, u.String())
	if e != nil {
		return errors.New("database_not_ready")
	}
	defer conn.Close(ctx)
	var exists bool
	if e = conn.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname='openfood')").Scan(&exists); e != nil {
		return e
	}
	if !exists {
		_, e = conn.Exec(ctx, "CREATE DATABASE openfood")
	}
	if e == nil {
		r.Code("database_ready")
	}
	return e
}
func (r *Runtime) Stop(ctx context.Context) error {
	e := r.run(ctx, "pg_ctl", "stop", "-D", filepath.Join(r.Data, "database"), "-m", "fast", "-w", "-t", "30")
	if e == nil {
		r.Code("database_stopped")
		ReleaseToolPath(r.Bin)
	}
	return e
}
func (r *Runtime) dbArgs(db string) []string {
	return []string{"-h", "127.0.0.1", "-p", strconv.Itoa(r.Config.DBPort), "-U", "openfood", "-d", db}
}
func (r *Runtime) Backup(ctx context.Context) (string, error) {
	dir := filepath.Join(r.Data, "backups")
	if e := os.MkdirAll(dir, 0700); e != nil {
		return "", e
	}
	path := filepath.Join(dir, "openfood-"+time.Now().UTC().Format("20060102T150405.000000000")+".dump")
	args := append(r.dbArgs("openfood"), "--format=custom", "--file="+path, "--no-owner", "--no-acl")
	if e := r.run(ctx, "pg_dump", args...); e != nil {
		os.Remove(path)
		return "", e
	}
	if e := r.run(ctx, "pg_restore", "--list", path); e != nil {
		return "", e
	}
	r.Code("backup_completed")
	return path, nil
}
func (r *Runtime) Restore(ctx context.Context, raw []byte) error {
	if len(raw) < 5 || string(raw[:5]) != "PGDMP" {
		return errors.New("backup_format_invalid")
	}
	path := filepath.Join(r.Data, "restore-"+app.Token()[:12]+".dump")
	if e := os.WriteFile(path, raw, 0600); e != nil {
		return e
	}
	defer os.Remove(path)
	u, _ := url.Parse(r.DSN)
	u.Path = "/postgres"
	conn, e := pgx.Connect(ctx, u.String())
	if e != nil {
		return e
	}
	defer conn.Close(ctx)
	stage := "openfood_validate_" + app.Token()[:12]
	if _, e = conn.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{stage}.Sanitize()); e != nil {
		return e
	}
	defer conn.Exec(context.Background(), "DROP DATABASE "+pgx.Identifier{stage}.Sanitize()+" WITH (FORCE)")
	args := append(r.dbArgs(stage), "--exit-on-error", "--no-owner", "--no-acl", path)
	if e = r.run(ctx, "pg_restore", args...); e != nil {
		return e
	}
	testURL, _ := url.Parse(r.DSN)
	testURL.Path = "/" + stage
	check, e := pgx.Connect(ctx, testURL.String())
	if e != nil {
		return e
	}
	var checksum string
	var version int
	e = check.QueryRow(ctx, "SELECT version,checksum FROM schema_migrations ORDER BY version DESC LIMIT 1").Scan(&version, &checksum)
	if e == nil {
		e = app.ValidateSchema(version, checksum)
	}
	check.Close(ctx)
	if e != nil {
		return e
	}
	if _, e = r.Backup(ctx); e != nil {
		return e
	}
	// Single transaction ensures a failed restore does not replace the existing database.
	args = append(r.dbArgs("openfood"), "--clean", "--if-exists", "--single-transaction", "--exit-on-error", "--no-owner", "--no-acl", path)
	if e = r.run(ctx, "pg_restore", args...); e != nil {
		return e
	}
	db, e := pgx.Connect(ctx, r.DSN)
	if e != nil {
		return e
	}
	defer db.Close(ctx)
	_, e = db.Exec(ctx, "DELETE FROM sessions; UPDATE jobs SET state='failed',last_error_code='restored_requires_review',lease_until=NULL WHERE state IN ('running','pending')")
	if e != nil {
		return e
	}
	r.Code("restore_completed")
	return nil
}
func (r *Runtime) Diagnostics() any {
	r.mu.Lock()
	defer r.mu.Unlock()
	return map[string]any{"version": app.Version, "database": "PostgreSQL 17", "mode": "local", "events": append([]string{}, r.Codes...), "generated_at": time.Now().UTC(), "integrations": "disabled", "note": "Somente códigos operacionais; credenciais, sessões e logs brutos excluídos."}
}
