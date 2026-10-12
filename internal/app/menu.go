package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

type MenuVariant struct {
	ID            int64    `json:"id"`
	Name          string   `json:"name"`
	Adjustment    int64    `json:"price_adjustment_cents"`
	Stock         float64  `json:"stock_quantity"`
	ExpectedStock *float64 `json:"expected_stock_quantity,omitempty"`
	Active        bool     `json:"active"`
}

func migrateMenu(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, menuSchema); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, "INSERT INTO schema_migrations(version,checksum) VALUES(7,$1)", Hash(schema+cashSchema+posSchema+diningSchema+menuSchema))
	return err
}

type MenuOption struct {
	ID            int64    `json:"id"`
	Name          string   `json:"name"`
	Price         int64    `json:"price_cents"`
	Stock         float64  `json:"stock_quantity"`
	ExpectedStock *float64 `json:"expected_stock_quantity,omitempty"`
	Active        bool     `json:"active"`
}
type MenuGroup struct {
	ID       int64        `json:"id"`
	Name     string       `json:"name"`
	Min      int          `json:"min_selections"`
	Max      int          `json:"max_selections"`
	Required bool         `json:"required"`
	Active   bool         `json:"active"`
	Shared   bool         `json:"shared"`
	Options  []MenuOption `json:"options"`
}
type MenuConfiguration struct {
	ProductID int64         `json:"product_id"`
	Version   int64         `json:"version"`
	Variants  []MenuVariant `json:"variants"`
	Groups    []MenuGroup   `json:"groups"`
}
type menuQuery interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func readMenu(ctx context.Context, q menuQuery, store, product int64) (MenuConfiguration, error) {
	c := MenuConfiguration{ProductID: product, Variants: []MenuVariant{}, Groups: []MenuGroup{}}
	if err := q.QueryRow(ctx, "SELECT configuration_version FROM products WHERE store_id=$1 AND id=$2", store, product).Scan(&c.Version); err != nil {
		return c, err
	}
	rows, err := q.Query(ctx, "SELECT id,name,price_adjustment_cents,stock_quantity,active FROM product_variants WHERE store_id=$1 AND product_id=$2 ORDER BY display_order,id", store, product)
	if err != nil {
		return c, err
	}
	for rows.Next() {
		var v MenuVariant
		if err = rows.Scan(&v.ID, &v.Name, &v.Adjustment, &v.Stock, &v.Active); err != nil {
			break
		}
		c.Variants = append(c.Variants, v)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return c, err
	}
	rows, err = q.Query(ctx, `SELECT g.id,g.name,g.min_selections,g.max_selections,g.required,g.active,EXISTS(SELECT 1 FROM product_addon_groups x WHERE x.store_id=$1 AND x.group_id=g.id AND x.product_id<>$2) FROM product_addon_groups p JOIN addon_groups g ON g.id=p.group_id AND g.store_id=p.store_id WHERE p.store_id=$1 AND p.product_id=$2 ORDER BY p.display_order,g.id`, store, product)
	if err != nil {
		return c, err
	}
	for rows.Next() {
		g := MenuGroup{Options: []MenuOption{}}
		if err = rows.Scan(&g.ID, &g.Name, &g.Min, &g.Max, &g.Required, &g.Active, &g.Shared); err != nil {
			break
		}
		c.Groups = append(c.Groups, g)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return c, err
	}
	for i := range c.Groups {
		rows, err = q.Query(ctx, "SELECT id,name,price_cents,stock_quantity,active FROM addon_options WHERE store_id=$1 AND group_id=$2 ORDER BY display_order,id", store, c.Groups[i].ID)
		if err != nil {
			return c, err
		}
		for rows.Next() {
			var o MenuOption
			if err = rows.Scan(&o.ID, &o.Name, &o.Price, &o.Stock, &o.Active); err != nil {
				break
			}
			c.Groups[i].Options = append(c.Groups[i].Options, o)
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return c, err
		}
	}
	return c, nil
}
func (a *App) menuConfiguration(w http.ResponseWriter, r *http.Request, store int64, user *SessionUser) {
	if !user.CanAccessStore(store) {
		failure(w, 403, "forbidden_cross_store_access")
		return
	}
	product := ParseInt(r.URL.Query().Get("product_id"))
	if product < 1 {
		failure(w, 400, "product_invalid")
		return
	}
	if r.Method == "GET" {
		c, err := readMenu(r.Context(), a.DB, store, product)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				failure(w, 404, "product_not_found")
			} else {
				failure(w, 500, "configuration_read_failed")
			}
			return
		}
		jsonResponse(w, 200, c)
		return
	}
	if !user.CanManageCatalog() {
		failure(w, 403, "forbidden_catalog_management_required")
		return
	}
	if r.Method != "POST" {
		failure(w, 405, "method_not_allowed")
		return
	}
	var c MenuConfiguration
	if decode(w, r, &c) != nil || c.ProductID != product || c.Version < 1 || c.Variants == nil || c.Groups == nil || len(c.Variants) > 30 || len(c.Groups) > 12 {
		failure(w, 400, "configuration_invalid")
		return
	}
	if err := a.saveMenu(r.Context(), store, user.ID, c); err != nil {
		if strings.HasPrefix(err.Error(), "configuration_") {
			failure(w, 409, err.Error())
		} else if err.Error() == "product_not_found" {
			failure(w, 404, "product_not_found")
		} else {
			failure(w, 500, "configuration_save_failed")
		}
		return
	}
	saved, err := readMenu(r.Context(), a.DB, store, product)
	if err != nil {
		failure(w, 500, "configuration_read_failed")
		return
	}
	jsonResponse(w, 200, saved)
}
func menuName(s string) bool { return strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= 120 }
func menuStock(stock float64) bool {
	return stock >= 0 && stock <= 1000000 && stock == float64(int64(stock))
}
func (a *App) saveMenu(ctx context.Context, store, user int64, c MenuConfiguration) error {
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT id FROM stores WHERE id=$1 FOR UPDATE", store); err != nil {
		return err
	}
	var version int64
	var base int64
	if err = tx.QueryRow(ctx, "SELECT configuration_version,price_cents FROM products WHERE store_id=$1 AND id=$2 FOR UPDATE", store, c.ProductID).Scan(&version, &base); err != nil {
		return errors.New("product_not_found")
	}
	if version != c.Version {
		return errors.New("configuration_changed")
	}
	old, err := readMenu(ctx, tx, store, c.ProductID)
	if err != nil {
		return err
	}
	oldV := map[int64]MenuVariant{}
	oldG := map[int64]MenuGroup{}
	for _, v := range old.Variants {
		oldV[v.ID] = v
	}
	for _, g := range old.Groups {
		oldG[g.ID] = g
	}
	variants := []int64{}
	groups := []int64{}
	seenNames := map[string]bool{}
	seenIDs := map[int64]bool{}
	for i, v := range c.Variants {
		name := strings.TrimSpace(v.Name)
		key := strings.ToLower(name)
		if v.ID < 0 || !menuName(name) || seenNames[key] || !menuStock(v.Stock) || v.Adjustment < -base || v.Adjustment > 1000000000 {
			return errors.New("configuration_invalid")
		}
		seenNames[key] = true
		if v.ID > 0 {
			prev, ok := oldV[v.ID]
			if !ok || seenIDs[v.ID] || v.ExpectedStock == nil || *v.ExpectedStock != prev.Stock {
				return errors.New("configuration_stock_changed")
			}
			seenIDs[v.ID] = true
			_, err = tx.Exec(ctx, "UPDATE product_variants SET name=$3,price_adjustment_cents=$4,stock_quantity=$5,active=$6,display_order=$7 WHERE store_id=$1 AND id=$2", store, v.ID, name, v.Adjustment, v.Stock, v.Active, i)
		} else {
			err = tx.QueryRow(ctx, "INSERT INTO product_variants(store_id,product_id,name,price_adjustment_cents,stock_quantity,active,display_order) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id", store, c.ProductID, name, v.Adjustment, v.Stock, v.Active, i).Scan(&v.ID)
		}
		if err != nil {
			return errors.New("configuration_save_failed")
		}
		variants = append(variants, v.ID)
	}
	if _, err = tx.Exec(ctx, "UPDATE product_variants SET active=false WHERE store_id=$1 AND product_id=$2 AND NOT(id=ANY($3))", store, c.ProductID, variants); err != nil {
		return err
	}
	seenNames = map[string]bool{}
	seenIDs = map[int64]bool{}
	for i, g := range c.Groups {
		name := strings.TrimSpace(g.Name)
		key := strings.ToLower(name)
		if g.ID < 0 || !menuName(name) || seenNames[key] || g.Min < 0 || g.Max < g.Min || g.Max > 50 || (g.Required && g.Max < 1) || len(g.Options) > 50 || len(g.Options) < g.Min || (g.Required && g.Active && len(g.Options) < 1) {
			return errors.New("configuration_invalid")
		}
		seenNames[key] = true
		oldOptions := map[int64]MenuOption{}
		if g.ID > 0 {
			prev, ok := oldG[g.ID]
			if !ok || seenIDs[g.ID] {
				return errors.New("configuration_invalid")
			}
			seenIDs[g.ID] = true
			if prev.Shared {
				// Retain an unchanged shared group while editing this product's variants.
				copyGroup := g
				copyGroup.Options = append([]MenuOption(nil), g.Options...)
				for i := range copyGroup.Options {
					copyGroup.Options[i].ExpectedStock = nil
				}
				left, _ := json.Marshal(prev)
				right, _ := json.Marshal(copyGroup)
				if !bytes.Equal(left, right) {
					return errors.New("configuration_shared_group")
				}
				groups = append(groups, g.ID)
				continue
			}
			for _, o := range prev.Options {
				oldOptions[o.ID] = o
			}
			_, err = tx.Exec(ctx, "UPDATE addon_groups SET name=$3,min_selections=$4,max_selections=$5,required=$6,active=$7,display_order=$8 WHERE store_id=$1 AND id=$2", store, g.ID, name, g.Min, g.Max, g.Required, g.Active, i)
		} else {
			err = tx.QueryRow(ctx, "INSERT INTO addon_groups(store_id,name,min_selections,max_selections,required,active,display_order) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id", store, name, g.Min, g.Max, g.Required, g.Active, i).Scan(&g.ID)
		}
		if err != nil {
			return errors.New("configuration_save_failed")
		}
		options := []int64{}
		optionNames := map[string]bool{}
		optionIDs := map[int64]bool{}
		for j, o := range g.Options {
			name := strings.TrimSpace(o.Name)
			key := strings.ToLower(name)
			if o.ID < 0 || !menuName(name) || optionNames[key] || !menuStock(o.Stock) || o.Price < 0 || o.Price > 1000000000 {
				return errors.New("configuration_invalid")
			}
			optionNames[key] = true
			if o.ID > 0 {
				prev, ok := oldOptions[o.ID]
				if !ok || optionIDs[o.ID] || o.ExpectedStock == nil || *o.ExpectedStock != prev.Stock {
					return errors.New("configuration_stock_changed")
				}
				optionIDs[o.ID] = true
				_, err = tx.Exec(ctx, "UPDATE addon_options SET name=$3,price_cents=$4,stock_quantity=$5,active=$6,display_order=$7 WHERE store_id=$1 AND id=$2", store, o.ID, name, o.Price, o.Stock, o.Active, j)
			} else {
				err = tx.QueryRow(ctx, "INSERT INTO addon_options(store_id,group_id,name,price_cents,stock_quantity,active,display_order) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id", store, g.ID, name, o.Price, o.Stock, o.Active, j).Scan(&o.ID)
			}
			if err != nil {
				return errors.New("configuration_save_failed")
			}
			options = append(options, o.ID)
		}
		if _, err = tx.Exec(ctx, "UPDATE addon_options SET active=false WHERE store_id=$1 AND group_id=$2 AND NOT(id=ANY($3))", store, g.ID, options); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO product_addon_groups(store_id,product_id,group_id,display_order) VALUES($1,$2,$3,$4) ON CONFLICT(product_id,group_id) DO UPDATE SET display_order=$4", store, c.ProductID, g.ID, i); err != nil {
			return err
		}
		groups = append(groups, g.ID)
	}
	if _, err = tx.Exec(ctx, "DELETE FROM product_addon_groups WHERE store_id=$1 AND product_id=$2 AND NOT(group_id=ANY($3))", store, c.ProductID, groups); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "UPDATE products SET configuration_version=configuration_version+1 WHERE store_id=$1 AND id=$2", store, c.ProductID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO audit(store_id,user_id,action,entity_id) VALUES($1,$2,'product_configuration',$3)", store, user, c.ProductID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type menuSelection struct {
	Combo   *comboSnapshot `json:"combo,omitempty"`
	Version int64          `json:"configuration_version"`
	Base    int64          `json:"base_price_cents"`
	Variant *MenuVariant   `json:"variant,omitempty"`
	Addons  []MenuOption   `json:"addons"`
}

func configuredItem(ctx context.Context, tx pgx.Tx, store int64, item Item, name string, price int64) (string, int64, menuSelection, error) {
	selection := menuSelection{Base: price, Addons: []MenuOption{}}
	if utf8.RuneCountInString(item.Notes) > 500 || len(item.Addons) > 50 || item.VariantID < 0 {
		return name, price, selection, errors.New("configuration_invalid")
	}
	c, err := readMenu(ctx, tx, store, item.ProductID)
	if err != nil {
		return name, price, selection, err
	}
	if item.ConfigurationVersion != 0 && item.ConfigurationVersion != c.Version {
		return name, price, selection, errors.New("configuration_changed")
	}
	selection.Version = c.Version
	activeVariants := 0
	found := false
	for _, v := range c.Variants {
		if !v.Active {
			continue
		}
		activeVariants++
		if v.ID == item.VariantID {
			found = true
			if v.Stock < float64(item.Quantity) {
				return name, price, selection, errors.New("stock_insufficient")
			}
			price += v.Adjustment
			if price < 0 {
				return name, price, selection, errors.New("configuration_invalid")
			}
			name += " · " + v.Name
			selection.Variant = &v
			if _, err = tx.Exec(ctx, "UPDATE product_variants SET stock_quantity=stock_quantity-$3 WHERE store_id=$1 AND id=$2", store, v.ID, item.Quantity); err != nil {
				return name, price, selection, err
			}
		}
	}
	if (activeVariants > 0 && !found) || (item.VariantID != 0 && !found) {
		return name, price, selection, errors.New("variant_required")
	}
	chosen := map[int64]bool{}
	for _, id := range item.Addons {
		if id < 1 || chosen[id] {
			return name, price, selection, errors.New("configuration_invalid")
		}
		chosen[id] = true
	}
	matched := 0
	for _, g := range c.Groups {
		if !g.Active {
			continue
		}
		count := 0
		for _, o := range g.Options {
			if !chosen[o.ID] {
				continue
			}
			if !o.Active {
				return name, price, selection, errors.New("addon_unavailable")
			}
			count++
			matched++
			if o.Stock < float64(item.Quantity) {
				return name, price, selection, errors.New("stock_insufficient")
			}
			price += o.Price
			name += " + " + o.Name
			selection.Addons = append(selection.Addons, o)
			if _, err = tx.Exec(ctx, "UPDATE addon_options SET stock_quantity=stock_quantity-$3 WHERE store_id=$1 AND id=$2", store, o.ID, item.Quantity); err != nil {
				return name, price, selection, err
			}
		}
		minimum := g.Min
		if g.Required && minimum < 1 {
			minimum = 1
		}
		if count < minimum || count > g.Max {
			return name, price, selection, errors.New("addon_limits_invalid")
		}
	}
	if matched != len(chosen) || price < 0 || price > 1000000000000 {
		return name, price, selection, errors.New("configuration_invalid")
	}
	return name, price, selection, nil
}
func itemIdentity(item Item) string {
	item.Quantity = 0
	item.ConfigurationVersion = 0
	item.Notes = strings.TrimSpace(item.Notes)
	item.Addons = append([]int64(nil), item.Addons...)
	sort.Slice(item.Addons, func(i, j int) bool { return item.Addons[i] < item.Addons[j] })
	b, _ := json.Marshal(item)
	return string(b)
}

func (a *App) productPhoto(w http.ResponseWriter, r *http.Request, store int64, user *SessionUser) {
	if !user.CanAccessStore(store) {
		failure(w, 403, "forbidden_cross_store_access")
		return
	}
	id := ParseInt(r.URL.Query().Get("product_id"))
	if id < 1 {
		failure(w, 400, "product_invalid")
		return
	}
	if r.Method == "GET" {
		var data []byte
		if err := a.DB.QueryRow(r.Context(), "SELECT data FROM product_photos WHERE store_id=$1 AND product_id=$2", store, id).Scan(&data); err != nil {
			failure(w, 404, "photo_not_found")
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "private, no-store")
		w.Write(data)
		return
	}
	if !user.CanManageCatalog() {
		failure(w, 403, "forbidden_catalog_management_required")
		return
	}
	if r.Method != "POST" && r.Method != "DELETE" {
		failure(w, 405, "method_not_allowed")
		return
	}
	var exists bool
	if err := a.DB.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM products WHERE store_id=$1 AND id=$2)", store, id).Scan(&exists); err != nil || !exists {
		failure(w, 404, "product_not_found")
		return
	}
	var data []byte
	if r.Method == "POST" {
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<20))
		if err != nil {
			failure(w, 413, "photo_too_large")
			return
		}
		cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
		if err != nil || (format != "jpeg" && format != "png") || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 4096 || cfg.Height > 4096 || int64(cfg.Width)*int64(cfg.Height) > 12000000 {
			failure(w, 400, "photo_invalid")
			return
		}
		original, _, err := image.Decode(bytes.NewReader(raw))
		if err != nil {
			failure(w, 400, "photo_invalid")
			return
		}
		// Re-encode pixels on white: discard metadata and prevent active image content.
		canvas := image.NewRGBA(original.Bounds())
		draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
		draw.Draw(canvas, canvas.Bounds(), original, original.Bounds().Min, draw.Over)
		var buf bytes.Buffer
		if err = jpeg.Encode(&buf, canvas, &jpeg.Options{Quality: 82}); err != nil || buf.Len() > 2<<20 {
			failure(w, 413, "photo_too_large")
			return
		}
		data = buf.Bytes()
	}
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		failure(w, 500, "photo_save_failed")
		return
	}
	defer tx.Rollback(r.Context())
	if r.Method == "DELETE" {
		_, err = tx.Exec(r.Context(), "DELETE FROM product_photos WHERE store_id=$1 AND product_id=$2", store, id)
	} else {
		_, err = tx.Exec(r.Context(), "INSERT INTO product_photos(store_id,product_id,data) VALUES($1,$2,$3) ON CONFLICT(store_id,product_id) DO UPDATE SET data=$3,updated_at=now()", store, id, data)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), "INSERT INTO audit(store_id,user_id,action,entity_id) VALUES($1,$2,'product_photo',$3)", store, user.ID, id)
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		failure(w, 500, "photo_save_failed")
		return
	}
	jsonResponse(w, 200, map[string]bool{"saved": true})
}

func selectionJSON(s menuSelection) []byte { b, _ := json.Marshal(s); return b }
func restoreMenuStock(ctx context.Context, tx pgx.Tx, store, order int64) error {
	for _, sql := range []string{
		`UPDATE product_variants v SET stock_quantity=v.stock_quantity+i.quantity FROM (SELECT variant_id,sum(quantity) quantity FROM order_items WHERE store_id=$1 AND order_id=$2 AND variant_id IS NOT NULL GROUP BY variant_id) i WHERE v.store_id=$1 AND v.id=i.variant_id`,
		`UPDATE addon_options o SET stock_quantity=o.stock_quantity+i.quantity FROM (SELECT a.addon_option_id,sum(a.quantity) quantity FROM order_item_addons a JOIN order_items i ON i.id=a.order_item_id AND i.store_id=a.store_id WHERE i.store_id=$1 AND i.order_id=$2 GROUP BY a.addon_option_id) i WHERE o.store_id=$1 AND o.id=i.addon_option_id`,
	} {
		if _, err := tx.Exec(ctx, sql, store, order); err != nil {
			return fmt.Errorf("stock_release_failed: %w", err)
		}
	}
	return restoreComboStock(ctx, tx, store, order)
}
