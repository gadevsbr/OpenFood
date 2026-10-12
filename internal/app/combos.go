package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"math"
	"strings"
	"unicode/utf8"
)

func migrateCombos(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, comboSchema); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, "INSERT INTO schema_migrations(version,checksum) VALUES(8,$1)", Hash(schema+cashSchema+posSchema+diningSchema+menuSchema+comboSchema))
	return err
}

type comboComponentSnapshot struct {
	ProductID int64         `json:"product_id"`
	Quantity  int64         `json:"quantity"`
	Name      string        `json:"name"`
	Price     int64         `json:"price_cents"`
	Selection menuSelection `json:"selection"`
	Notes     string        `json:"notes,omitempty"`
}
type comboSnapshot struct {
	ID         int64                    `json:"id"`
	Name       string                   `json:"name"`
	Policy     string                   `json:"pricing_policy"`
	Components []comboComponentSnapshot `json:"components"`
}
type comboDefinition struct {
	Product  int64
	Quantity float64
	Optional bool
	Group    string
	Min, Max int
}

func configuredCombo(ctx context.Context, tx pgx.Tx, store int64, item Item) (string, int64, menuSelection, error) {
	fail := func(code string) (string, int64, menuSelection, error) {
		return "", 0, menuSelection{}, errors.New(code)
	}
	if item.ProductID != 0 || item.VariantID != 0 || len(item.Addons) > 0 || item.Quantity < 1 || item.Quantity > 100000 || len(item.Components) > 50 || utf8.RuneCountInString(item.Notes) > 500 {
		return fail("combo_invalid")
	}
	c := comboSnapshot{ID: item.ComboID, Components: []comboComponentSnapshot{}}
	var fixed int64
	var active bool
	if err := tx.QueryRow(ctx, "SELECT name,price_cents,pricing_policy,active FROM combos WHERE store_id=$1 AND id=$2 FOR UPDATE", store, item.ComboID).Scan(&c.Name, &fixed, &c.Policy, &active); err != nil || !active {
		return fail("combo_unavailable")
	}
	rows, err := tx.Query(ctx, "SELECT product_id,quantity,is_optional,COALESCE(group_name,''),min_selections,max_selections FROM combo_items WHERE store_id=$1 AND combo_id=$2 ORDER BY display_order,product_id", store, item.ComboID)
	if err != nil {
		return "", 0, menuSelection{}, err
	}
	defs := []comboDefinition{}
	for rows.Next() {
		var d comboDefinition
		if err = rows.Scan(&d.Product, &d.Quantity, &d.Optional, &d.Group, &d.Min, &d.Max); err != nil {
			rows.Close()
			return "", 0, menuSelection{}, err
		}
		defs = append(defs, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return "", 0, menuSelection{}, err
	}
	if len(defs) == 0 || len(defs) > 50 {
		return fail("combo_invalid")
	}
	selected := map[int64]Item{}
	for _, i := range item.Components {
		if i.ProductID < 1 || i.ComboID != 0 || len(i.Components) > 0 || i.Quantity != 1 || selected[i.ProductID].ProductID != 0 {
			return fail("combo_invalid")
		}
		selected[i.ProductID] = i
	}
	groups := map[string][3]int{}
	for _, d := range defs {
		if d.Optional {
			g := groups[d.Group]
			if g[0] != 0 && (g[1] != d.Min || g[2] != d.Max) {
				return fail("combo_invalid")
			}
			g[0]++
			g[1] = d.Min
			g[2] = d.Max
			groups[d.Group] = g
		}
	}
	if item.ComboVersion != "" && item.ComboVersion != comboDefinitionVersion(c.Name, c.Policy, fixed, active, defs) {
		return fail("configuration_changed")
	}
	counts := map[string]int{}
	names := []string{}
	var sum, highest, extras int64
	for _, d := range defs {
		choice, chosen := selected[d.Product]
		if d.Optional && !chosen {
			continue
		}
		if !chosen {
			choice = Item{ProductID: d.Product}
		}
		delete(selected, d.Product)
		if d.Optional {
			counts[d.Group]++
		}
		if d.Quantity != math.Trunc(d.Quantity) || d.Quantity < 1 || d.Quantity > 100000 || d.Quantity*float64(item.Quantity) > 100000 {
			return fail("combo_quantity_invalid")
		}
		q := int64(d.Quantity)
		choice.Quantity = q * item.Quantity
		var name, unit string
		var price int64
		var stock, minimum, maximum float64
		var available bool
		err = tx.QueryRow(ctx, `SELECT name,price_cents,stock_quantity,unit,COALESCE(min_order_quantity,1),COALESCE(max_order_quantity,0),active AND NOT is_weight_based AND (available_from IS NULL OR available_until IS NULL OR CASE WHEN available_from<=available_until THEN (now() AT TIME ZONE 'America/Sao_Paulo')::time BETWEEN available_from AND available_until ELSE (now() AT TIME ZONE 'America/Sao_Paulo')::time>=available_from OR (now() AT TIME ZONE 'America/Sao_Paulo')::time<=available_until END) FROM products WHERE store_id=$1 AND id=$2 FOR UPDATE`, store, d.Product).Scan(&name, &price, &stock, &unit, &minimum, &maximum, &available)
		if err != nil || !available || (unit != "unidade" && unit != "porcao") {
			return fail("product_unavailable")
		}
		if float64(choice.Quantity) < minimum || (maximum > 0 && float64(choice.Quantity) > maximum) {
			return fail("quantity_invalid")
		}
		if stock < float64(choice.Quantity) {
			return fail("stock_insufficient")
		}
		base := price
		var selection menuSelection
		name, price, selection, err = configuredItem(ctx, tx, store, choice, name, price)
		if err != nil {
			return "", 0, menuSelection{}, err
		}
		if _, err = tx.Exec(ctx, "UPDATE products SET stock_quantity=stock_quantity-$3 WHERE store_id=$1 AND id=$2", store, d.Product, choice.Quantity); err != nil {
			return "", 0, menuSelection{}, err
		}
		c.Components = append(c.Components, comboComponentSnapshot{d.Product, q, name, price, selection, strings.TrimSpace(choice.Notes)})
		names = append(names, fmt.Sprintf("%dx %s", q, name))
		sum += price * q
		extras += (price - base) * q
		if price*q > highest {
			highest = price * q
		}
	}
	if len(selected) > 0 {
		return fail("combo_invalid")
	}
	for name, g := range groups {
		if counts[name] < g[1] || counts[name] > g[2] {
			return fail("combo_limits_invalid")
		}
	}
	if len(c.Components) == 0 {
		return fail("combo_invalid")
	}
	price := sum
	switch c.Policy {
	case "fixed":
		price = fixed + extras
	case "highest":
		price = highest
	case "sum":
	default:
		return fail("combo_invalid")
	}
	if price < 0 || price > 1000000000 {
		return fail("total_limit")
	}
	return c.Name + " · " + strings.Join(names, " + "), price, menuSelection{Base: price, Addons: []MenuOption{}, Combo: &c}, nil
}
func restoreComboStock(ctx context.Context, tx pgx.Tx, store, order int64) error {
	rows, err := tx.Query(ctx, "SELECT quantity,selection FROM order_items WHERE store_id=$1 AND order_id=$2 AND product_id IS NULL", store, order)
	if err != nil {
		return err
	}
	type line struct {
		Quantity  int64
		Selection menuSelection
	}
	lines := []line{}
	for rows.Next() {
		var l line
		var raw []byte
		if err = rows.Scan(&l.Quantity, &raw); err != nil {
			rows.Close()
			return err
		}
		if err = json.Unmarshal(raw, &l.Selection); err != nil {
			rows.Close()
			return err
		}
		lines = append(lines, l)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, l := range lines {
		if l.Selection.Combo == nil {
			return errors.New("combo_snapshot_invalid")
		}
		for _, c := range l.Selection.Combo.Components {
			q := c.Quantity * l.Quantity
			if _, err = tx.Exec(ctx, "UPDATE products SET stock_quantity=stock_quantity+$3 WHERE store_id=$1 AND id=$2", store, c.ProductID, q); err != nil {
				return err
			}
			if c.Selection.Variant != nil {
				if _, err = tx.Exec(ctx, "UPDATE product_variants SET stock_quantity=stock_quantity+$3 WHERE store_id=$1 AND id=$2", store, c.Selection.Variant.ID, q); err != nil {
					return err
				}
			}
			for _, o := range c.Selection.Addons {
				if _, err = tx.Exec(ctx, "UPDATE addon_options SET stock_quantity=stock_quantity+$3 WHERE store_id=$1 AND id=$2", store, o.ID, q); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func comboDefinitionVersion(name, policy string, price int64, active bool, defs []comboDefinition) string {
	raw, _ := json.Marshal(struct {
		Name, Policy string
		Price        int64
		Active       bool
		Definitions  []comboDefinition
	}{name, policy, price, active, defs})
	return Hash(string(raw))
}
