package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
)

// Offline Compose maintenance only. Call after stopping the HTTP app.
func PrivacyMaintenance(ctx context.Context, dsn string, args []string, in io.Reader, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("maintenance_command_required")
	}
	if args[0] == "--privacy-reapply" {
		if len(args) != 2 {
			return errors.New("staging_database_required")
		}
		// Never allow the destructive staging command to target the live database.
		if len(args[1]) < 18 || args[1][:18] != "openfood_validate_" {
			return errors.New("invalid_staging_database")
		}
		u, e := url.Parse(dsn)
		if e != nil {
			return e
		}
		u.Path = "/" + args[1]
		dsn = u.String()
	}
	a, e := Open(ctx, dsn, "", nil)
	if e != nil {
		return e
	}
	defer a.DB.Close()
	switch args[0] {
	case "--privacy-export":
		ledger, e := PrivacyRemovals(ctx, a.DB)
		if e != nil {
			return e
		}
		return json.NewEncoder(out).Encode(ledger)
	case "--privacy-reapply":
		var ledger []PrivacyRemoval
		if e = json.NewDecoder(in).Decode(&ledger); e != nil {
			return e
		}
		return ReapplyPrivacyRemovals(ctx, a.DB, ledger)
	}
	return errors.New("unknown_maintenance_command")
}
