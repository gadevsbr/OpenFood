package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type AddonProduct struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}
type AddonConfiguration struct {
	Version      string         `json:"version"`
	Group        MenuGroup      `json:"group"`
	Description  string         `json:"description"`
	DisplayOrder int64          `json:"display_order"`
	Products     []AddonProduct `json:"products"`
}

func readAddonConfiguration(ctx context.Context, q menuQuery, store, id int64) (AddonConfiguration, error) {
	c := AddonConfiguration{Group: MenuGroup{Options: []MenuOption{}}, Products: []AddonProduct{}}
	if err := q.QueryRow(ctx, "SELECT id,name,min_selections,max_selections,required,active,COALESCE(description,''),display_order FROM addon_groups WHERE store_id=$1 AND id=$2", store, id).Scan(&c.Group.ID, &c.Group.Name, &c.Group.Min, &c.Group.Max, &c.Group.Required, &c.Group.Active, &c.Description, &c.DisplayOrder); err != nil {
		return c, err
	}
	rows, err := q.Query(ctx, "SELECT id,name,price_cents,stock_quantity,active FROM addon_options WHERE store_id=$1 AND group_id=$2 ORDER BY display_order,id", store, id)
	if err != nil {
		return c, err
	}
	for rows.Next() {
		var o MenuOption
		if err = rows.Scan(&o.ID, &o.Name, &o.Price, &o.Stock, &o.Active); err != nil {
			break
		}
		c.Group.Options = append(c.Group.Options, o)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return c, err
	}
	rows, err = q.Query(ctx, "SELECT p.id,p.name FROM product_addon_groups x JOIN products p ON p.store_id=x.store_id AND p.id=x.product_id WHERE x.store_id=$1 AND x.group_id=$2 ORDER BY p.id", store, id)
	if err != nil {
		return c, err
	}
	for rows.Next() {
		var p AddonProduct
		if err = rows.Scan(&p.ID, &p.Name); err != nil {
			break
		}
		c.Products = append(c.Products, p)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return c, err
	}
	c.Group.Shared = len(c.Products) > 1
	version := c
	version.Group.Options = append([]MenuOption(nil), c.Group.Options...)
	for i := range version.Group.Options {
		version.Group.Options[i].Stock = 0
		version.Group.Options[i].ExpectedStock = nil
	}
	raw, _ := json.Marshal(version)
	c.Version = Hash(string(raw))
	return c, nil
}

func addonFailure(w http.ResponseWriter, err error) {
	var constraint *pgconn.PgError
	if errors.As(err, &constraint) && constraint.Code == "23505" {
		failure(w, 409, "addon_name_in_use")
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		failure(w, 404, "addon_group_not_found")
		return
	}
	switch err.Error() {
	case "configuration_changed", "configuration_stock_changed", "addon_name_in_use", "addon_group_limit":
		failure(w, 409, err.Error())
	case "addon_configuration_invalid", "addon_product_invalid":
		failure(w, 400, err.Error())
	default:
		failure(w, 500, "addon_configuration_failed")
	}
}

func addonStoreLock(ctx context.Context, tx pgx.Tx, store int64) error {
	var id int64
	return tx.QueryRow(ctx, "SELECT id FROM stores WHERE id=$1 FOR UPDATE", store).Scan(&id)
}
func bumpAddonProducts(ctx context.Context, tx pgx.Tx, store int64, ids []int64) error {
	_, err := tx.Exec(ctx, "UPDATE products SET configuration_version=configuration_version+1 WHERE store_id=$1 AND id=ANY($2)", store, ids)
	return err
}
func addonAudit(ctx context.Context, tx pgx.Tx, store, user, id int64, action string) error {
	_, err := tx.Exec(ctx, "INSERT INTO audit(store_id,user_id,action,entity_id) VALUES($1,$2,$3,$4)", store, user, action, id)
	return err
}

func (a *App) changeAddonLink(w http.ResponseWriter, r *http.Request, store int64, user *SessionUser, product int64) {
	group := ParseInt(r.URL.Query().Get("group_id"))
	var order int64
	if r.Method == "POST" {
		var input struct {
			GroupID      int64 `json:"group_id"`
			DisplayOrder int64 `json:"display_order"`
		}
		if decode(w, r, &input) != nil {
			failure(w, 400, "product_addon_group_invalid")
			return
		}
		group, order = input.GroupID, input.DisplayOrder
	}
	if group < 1 || order < 0 || order > 1000000 {
		failure(w, 400, "product_addon_group_invalid")
		return
	}
	ctx := r.Context()
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		addonFailure(w, err)
		return
	}
	defer tx.Rollback(ctx)
	if err = addonStoreLock(ctx, tx, store); err != nil {
		addonFailure(w, err)
		return
	}
	var count int
	if err = tx.QueryRow(ctx, "SELECT (SELECT count(*) FROM product_addon_groups WHERE store_id=$1 AND product_id=$2 AND group_id<>$3) FROM products WHERE store_id=$1 AND id=$2 AND EXISTS(SELECT 1 FROM addon_groups WHERE store_id=$1 AND id=$3)", store, product, group).Scan(&count); err != nil {
		addonFailure(w, err)
		return
	}
	if r.Method == "POST" && count >= 12 {
		failure(w, 409, "addon_group_limit")
		return
	}
	if r.Method == "POST" {
		_, err = tx.Exec(ctx, "INSERT INTO product_addon_groups(store_id,product_id,group_id,display_order) VALUES($1,$2,$3,$4) ON CONFLICT(product_id,group_id) DO UPDATE SET display_order=excluded.display_order", store, product, group, order)
	} else {
		_, err = tx.Exec(ctx, "DELETE FROM product_addon_groups WHERE store_id=$1 AND product_id=$2 AND group_id=$3", store, product, group)
	}
	if err == nil {
		err = bumpAddonProducts(ctx, tx, store, []int64{product})
	}
	if err == nil {
		err = addonAudit(ctx, tx, store, user.ID, group, "addon_link_changed")
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		addonFailure(w, err)
		return
	}
	jsonResponse(w, 200, map[string]bool{"ok": true})
}

func (a *App) addonConfiguration(w http.ResponseWriter, r *http.Request, store int64, user *SessionUser) {
	if !user.CanAccessStore(store) || !user.CanManageCatalog() {
		failure(w, 403, "forbidden_catalog_management_required")
		return
	}
	id := ParseInt(r.URL.Query().Get("group_id"))
	if r.Method == "GET" {
		tx, err := a.DB.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
		if err != nil {
			addonFailure(w, err)
			return
		}
		defer tx.Rollback(r.Context())
		c, err := readAddonConfiguration(r.Context(), tx, store, id)
		if err != nil {
			addonFailure(w, err)
			return
		}
		jsonResponse(w, 200, c)
		return
	}
	if r.Method != "POST" {
		failure(w, 405, "method_not_allowed")
		return
	}
	var c AddonConfiguration
	if decode(w, r, &c) != nil || c.Group.ID != id || id < 0 {
		failure(w, 400, "addon_configuration_invalid")
		return
	}
	saved, err := a.saveAddonConfiguration(r.Context(), store, user.ID, c)
	if err != nil {
		addonFailure(w, err)
		return
	}
	jsonResponse(w, 200, saved)
}

func (a *App) saveAddonConfiguration(ctx context.Context, store, user int64, c AddonConfiguration) (AddonConfiguration, error) {
	fail := func(code string) (AddonConfiguration, error) { return c, errors.New(code) }
	g := c.Group
	g.Name = strings.TrimSpace(g.Name)
	if !menuName(g.Name) || g.Min < 0 || g.Max < g.Min || g.Max > 50 || (g.Required && g.Max < 1) || len(g.Options) > 50 || len(c.Products) > 1000 || utf8.RuneCountInString(c.Description) > 500 || c.DisplayOrder < 0 || c.DisplayOrder > 1000000 {
		return fail("addon_configuration_invalid")
	}
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return c, err
	}
	defer tx.Rollback(ctx)
	if err = addonStoreLock(ctx, tx, store); err != nil {
		return c, err
	}
	old := AddonConfiguration{Group: MenuGroup{Options: []MenuOption{}}, Products: []AddonProduct{}}
	if g.ID > 0 {
		old, err = readAddonConfiguration(ctx, tx, store, g.ID)
		if err != nil {
			return c, err
		}
		if c.Version == "" || c.Version != old.Version {
			return fail("configuration_changed")
		}
	} else if c.Version != "" {
		return fail("addon_configuration_invalid")
	}
	var duplicate bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM addon_groups WHERE store_id=$1 AND lower(name)=lower($2) AND id<>$3)", store, g.Name, g.ID).Scan(&duplicate); err != nil {
		return c, err
	}
	if duplicate {
		return fail("addon_name_in_use")
	}
	oldOptions := map[int64]MenuOption{}
	for _, o := range old.Group.Options {
		oldOptions[o.ID] = o
	}
	ids, names := map[int64]bool{}, map[string]bool{}
	active := 0
	for i := range g.Options {
		o := &g.Options[i]
		o.Name = strings.TrimSpace(o.Name)
		key := strings.ToLower(o.Name)
		if !menuName(o.Name) || names[key] || o.ID < 0 || ids[o.ID] && o.ID > 0 || !menuStock(o.Stock) || o.Price < 0 || o.Price > 1000000000 {
			return fail("addon_configuration_invalid")
		}
		if o.ID > 0 {
			p, ok := oldOptions[o.ID]
			if !ok {
				return fail("addon_configuration_invalid")
			}
			if o.ExpectedStock == nil || *o.ExpectedStock != p.Stock {
				return fail("configuration_stock_changed")
			}
		}
		names[key], ids[o.ID] = true, true
		if o.Active {
			active++
		}
	}
	minimum := g.Min
	if g.Required && minimum < 1 {
		minimum = 1
	}
	if g.Active && active < minimum {
		return fail("addon_configuration_invalid")
	}
	selected, affected := map[int64]bool{}, map[int64]bool{}
	for _, p := range old.Products {
		affected[p.ID] = true
	}
	for _, p := range c.Products {
		if p.ID < 1 || selected[p.ID] {
			return fail("addon_product_invalid")
		}
		selected[p.ID], affected[p.ID] = true, true
		var count int
		if err = tx.QueryRow(ctx, "SELECT (SELECT count(*) FROM product_addon_groups WHERE store_id=$1 AND product_id=$2 AND group_id<>$3) FROM products WHERE store_id=$1 AND id=$2", store, p.ID, g.ID).Scan(&count); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fail("addon_product_invalid")
			}
			return c, err
		}
		if count >= 12 {
			return fail("addon_group_limit")
		}
	}
	if g.ID == 0 {
		err = tx.QueryRow(ctx, "INSERT INTO addon_groups(store_id,name,description,min_selections,max_selections,required,active,display_order) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id", store, g.Name, c.Description, g.Min, g.Max, g.Required, g.Active, c.DisplayOrder).Scan(&g.ID)
	} else {
		_, err = tx.Exec(ctx, "UPDATE addon_groups SET name=$3,description=$4,min_selections=$5,max_selections=$6,required=$7,active=$8,display_order=$9 WHERE store_id=$1 AND id=$2", store, g.ID, g.Name, c.Description, g.Min, g.Max, g.Required, g.Active, c.DisplayOrder)
	}
	if err != nil {
		return c, err
	}
	optionIDs := []int64{}
	for i, o := range g.Options {
		if o.ID == 0 {
			err = tx.QueryRow(ctx, "INSERT INTO addon_options(store_id,group_id,name,price_cents,stock_quantity,active,display_order) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id", store, g.ID, o.Name, o.Price, o.Stock, o.Active, i).Scan(&o.ID)
		} else {
			_, err = tx.Exec(ctx, "UPDATE addon_options SET name=$3,price_cents=$4,stock_quantity=$5,active=$6,display_order=$7 WHERE store_id=$1 AND id=$2 AND group_id=$8", store, o.ID, o.Name, o.Price, o.Stock, o.Active, i, g.ID)
		}
		if err != nil {
			return c, err
		}
		optionIDs = append(optionIDs, o.ID)
	}
	if _, err = tx.Exec(ctx, "UPDATE addon_options SET active=false WHERE store_id=$1 AND group_id=$2 AND NOT(id=ANY($3))", store, g.ID, optionIDs); err != nil {
		return c, err
	}
	productIDs := []int64{}
	for id := range selected {
		productIDs = append(productIDs, id)
	}
	sort.Slice(productIDs, func(i, j int) bool { return productIDs[i] < productIDs[j] })
	if _, err = tx.Exec(ctx, "DELETE FROM product_addon_groups WHERE store_id=$1 AND group_id=$2 AND NOT(product_id=ANY($3))", store, g.ID, productIDs); err != nil {
		return c, err
	}
	for _, id := range productIDs {
		if _, err = tx.Exec(ctx, "INSERT INTO product_addon_groups(store_id,product_id,group_id,display_order) SELECT $1,$2,$3,COALESCE(max(display_order),-1)+1 FROM product_addon_groups WHERE store_id=$1 AND product_id=$2 ON CONFLICT(product_id,group_id) DO NOTHING", store, id, g.ID); err != nil {
			return c, err
		}
	}
	affectedIDs := []int64{}
	for id := range affected {
		affectedIDs = append(affectedIDs, id)
	}
	if err = bumpAddonProducts(ctx, tx, store, affectedIDs); err != nil {
		return c, err
	}
	if err = addonAudit(ctx, tx, store, user, g.ID, "shared_addon_configuration"); err != nil {
		return c, err
	}
	saved, err := readAddonConfiguration(ctx, tx, store, g.ID)
	if err != nil {
		return c, err
	}
	if err = tx.Commit(ctx); err != nil {
		return c, err
	}
	return saved, nil
}
