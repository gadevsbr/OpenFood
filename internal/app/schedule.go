package app

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"sort"
)

//go:embed schedule.sql
var scheduleSchema string

type SaleWindow struct {
	Day   int `json:"day"`
	Start int `json:"start"`
	End   int `json:"end"`
}
type SaleSchedule struct {
	Enabled bool         `json:"enabled"`
	Windows []SaleWindow `json:"windows"`
}
type ScheduleConfiguration struct {
	CategoryID int64        `json:"category_id"`
	Version    int64        `json:"version"`
	Timezone   string       `json:"timezone"`
	Name       string       `json:"name"`
	Schedule   SaleSchedule `json:"schedule"`
}

func migrateSchedule(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, scheduleSchema); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, "INSERT INTO schema_migrations(version,checksum) VALUES(10,$1)", Hash(schema+cashSchema+posSchema+diningSchema+menuSchema+comboSchema+pizzaSchema+scheduleSchema))
	return err
}

func validateSchedule(rule *SaleSchedule) bool {
	if len(rule.Windows) > 42 {
		return false
	}
	if rule.Windows == nil {
		rule.Windows = []SaleWindow{}
	}
	// Expand overnight windows onto a seven-day timeline and reject all overlap, including Sunday wrap.
	occupied := make([]bool, 7*1440)
	for _, w := range rule.Windows {
		if w.Day < 0 || w.Day > 6 || w.Start < 0 || w.Start >= 1440 || w.End < 1 || w.End > 1440 || w.Start == w.End {
			return false
		}
		end := w.End
		if end < w.Start {
			end += 1440
		}
		for n := w.Start; n < end; n++ {
			idx := (w.Day*1440 + n) % len(occupied)
			if occupied[idx] {
				return false
			}
			occupied[idx] = true
		}
	}
	sort.Slice(rule.Windows, func(i, j int) bool {
		a, b := rule.Windows[i], rule.Windows[j]
		if a.Day != b.Day {
			return a.Day < b.Day
		}
		return a.Start < b.Start
	})
	return true
}

func readSchedule(ctx context.Context, q menuQuery, store, category int64) (ScheduleConfiguration, error) {
	c := ScheduleConfiguration{CategoryID: category}
	var raw []byte
	var err error
	if category == 0 {
		err = q.QueryRow(ctx, "SELECT name,timezone,schedule_version,sale_schedule FROM stores WHERE id=$1", store).Scan(&c.Name, &c.Timezone, &c.Version, &raw)
	} else {
		err = q.QueryRow(ctx, "SELECT c.name,s.timezone,c.schedule_version,c.sale_schedule FROM categories c JOIN stores s ON s.id=c.store_id WHERE c.store_id=$1 AND c.id=$2", store, category).Scan(&c.Name, &c.Timezone, &c.Version, &raw)
	}
	if err == nil {
		err = json.Unmarshal(raw, &c.Schedule)
	}
	return c, err
}

func scheduleFailure(w http.ResponseWriter, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		failure(w, 404, "schedule_not_found")
		return
	}
	switch err.Error() {
	case "configuration_changed":
		failure(w, 409, err.Error())
	case "schedule_invalid":
		failure(w, 400, err.Error())
	default:
		failure(w, 500, "schedule_failed")
	}
}

func (a *App) scheduleConfiguration(w http.ResponseWriter, r *http.Request, store int64, user *SessionUser) {
	if !user.CanAccessStore(store) || !user.CanManageCatalog() {
		failure(w, 403, "forbidden_catalog_management_required")
		return
	}
	category := ParseInt(r.URL.Query().Get("category_id"))
	if category < 0 {
		failure(w, 400, "schedule_invalid")
		return
	}
	if r.Method == "GET" {
		c, e := readSchedule(r.Context(), a.DB, store, category)
		if e != nil {
			scheduleFailure(w, e)
			return
		}
		jsonResponse(w, 200, c)
		return
	}
	if r.Method != "POST" {
		failure(w, 405, "method_not_allowed")
		return
	}
	var c ScheduleConfiguration
	if decode(w, r, &c) != nil || c.CategoryID != category || !validateSchedule(&c.Schedule) {
		failure(w, 400, "schedule_invalid")
		return
	}
	ctx := r.Context()
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		scheduleFailure(w, e)
		return
	}
	defer tx.Rollback(ctx)
	if e = addonStoreLock(ctx, tx, store); e != nil {
		scheduleFailure(w, e)
		return
	}
	old, e := readSchedule(ctx, tx, store, category)
	if e != nil {
		scheduleFailure(w, e)
		return
	}
	if old.Version != c.Version || category != 0 && old.Timezone != c.Timezone {
		failure(w, 409, "configuration_changed")
		return
	}
	var valid bool
	if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=$1 AND (name='UTC' OR name LIKE 'America/%'))", c.Timezone).Scan(&valid); e != nil {
		scheduleFailure(w, e)
		return
	}
	if !valid {
		failure(w, 400, "schedule_invalid")
		return
	}
	raw, _ := json.Marshal(c.Schedule)
	if category == 0 {
		_, e = tx.Exec(ctx, "UPDATE stores SET timezone=$2,sale_schedule=$3,schedule_version=schedule_version+1 WHERE id=$1", store, c.Timezone, raw)
	} else {
		_, e = tx.Exec(ctx, "UPDATE categories SET sale_schedule=$3,schedule_version=schedule_version+1 WHERE store_id=$1 AND id=$2", store, category, raw)
	}
	if e == nil {
		_, e = tx.Exec(ctx, "UPDATE products SET configuration_version=configuration_version+1 WHERE store_id=$1", store)
	}
	if e == nil {
		e = addonAudit(ctx, tx, store, user.ID, category, "sale_schedule_changed")
	}
	if e == nil {
		c, e = readSchedule(ctx, tx, store, category)
	}
	if e == nil {
		e = tx.Commit(ctx)
	}
	if e != nil {
		scheduleFailure(w, e)
		return
	}
	jsonResponse(w, 200, c)
}
