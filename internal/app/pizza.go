package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Flavor prices describe one whole pizza before the common size adjustment.
// Portions affect pricing, never fractional inventory: product/size stock is whole pizzas.
type PizzaFlavor struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Price  int64  `json:"price_cents"`
	Active bool   `json:"active"`
}
type PizzaConfiguration struct {
	Enabled    bool          `json:"enabled"`
	Policy     string        `json:"pricing_policy"`
	MaxFlavors int           `json:"max_flavors"`
	Flavors    []PizzaFlavor `json:"flavors"`
}
type PizzaPart struct {
	ID    string `json:"flavor_id"`
	Parts int64  `json:"parts"`
}
type PizzaFlavorSnapshot struct {
	PizzaPart
	Name  string `json:"name"`
	Price int64  `json:"price_cents"`
}
type PizzaSnapshot struct {
	Policy  string                `json:"pricing_policy"`
	Parts   int64                 `json:"total_parts"`
	Price   int64                 `json:"price_cents"`
	Flavors []PizzaFlavorSnapshot `json:"flavors"`
}

func migratePizza(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, pizzaSchema); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, "INSERT INTO schema_migrations(version,checksum) VALUES(9,$1)", Hash(schema+cashSchema+posSchema+diningSchema+menuSchema+comboSchema+pizzaSchema))
	return err
}

func validatePizza(c PizzaConfiguration, old PizzaConfiguration, variants []MenuVariant) (PizzaConfiguration, error) {
	if (c.Policy != "highest" && c.Policy != "weighted_average") || c.MaxFlavors < 1 || c.MaxFlavors > 4 || len(c.Flavors) > 30 {
		return c, errors.New("pizza_configuration_invalid")
	}
	ids, names := map[string]bool{}, map[string]bool{}
	oldIDs := map[string]bool{}
	for _, f := range old.Flavors {
		oldIDs[f.ID] = true
	}
	active := 0
	for i := range c.Flavors {
		f := &c.Flavors[i]
		f.Name = strings.TrimSpace(f.Name)
		key := strings.ToLower(f.Name)
		if !menuName(f.Name) || names[key] || f.Price < 0 || f.Price > 1000000000 || (f.ID != "" && (!oldIDs[f.ID] || ids[f.ID])) {
			return c, errors.New("pizza_configuration_invalid")
		}
		for _, v := range variants {
			if v.Active && f.Active && f.Price+v.Adjustment < 0 {
				return c, errors.New("pizza_configuration_invalid")
			}
		}
		if f.ID == "" {
			f.ID = Token()
		}
		ids[f.ID], names[key] = true, true
		if f.Active {
			active++
		}
	}
	if c.Enabled && active == 0 {
		return c, errors.New("pizza_configuration_invalid")
	}
	return c, nil
}

func (a *App) pizzaConfiguration(w http.ResponseWriter, r *http.Request, store int64, user *SessionUser) {
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
		jsonResponse(w, 200, map[string]any{"product_id": product, "version": c.Version, "pizza": c.Pizza})
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
	var input struct {
		ProductID int64              `json:"product_id"`
		Version   int64              `json:"version"`
		Pizza     PizzaConfiguration `json:"pizza"`
	}
	if decode(w, r, &input) != nil || input.ProductID != product || input.Version < 1 {
		failure(w, 400, "pizza_configuration_invalid")
		return
	}
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		failure(w, 500, "configuration_save_failed")
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), "SELECT id FROM stores WHERE id=$1 FOR UPDATE", store); err != nil {
		failure(w, 500, "configuration_save_failed")
		return
	}
	c, err := readMenu(r.Context(), tx, store, product)
	if err != nil {
		failure(w, 404, "product_not_found")
		return
	}
	if c.Version != input.Version {
		failure(w, 409, "configuration_changed")
		return
	}
	var weight bool
	var unit string
	if err = tx.QueryRow(r.Context(), "SELECT is_weight_based,unit FROM products WHERE store_id=$1 AND id=$2", store, product).Scan(&weight, &unit); err != nil {
		failure(w, 500, "configuration_save_failed")
		return
	}
	if input.Pizza.Enabled && (weight || unit != "unidade") {
		failure(w, 400, "pizza_configuration_invalid")
		return
	}
	pizza, err := validatePizza(input.Pizza, c.Pizza, c.Variants)
	if err != nil {
		failure(w, 400, err.Error())
		return
	}
	raw, err := json.Marshal(pizza)
	if err == nil {
		_, err = tx.Exec(r.Context(), "UPDATE products SET pizza_configuration=$3,configuration_version=configuration_version+1 WHERE store_id=$1 AND id=$2", store, product, raw)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), "INSERT INTO audit(store_id,user_id,action,entity_id) VALUES($1,$2,'pizza_configuration',$3)", store, user.ID, product)
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		failure(w, 500, "configuration_save_failed")
		return
	}
	jsonResponse(w, 200, map[string]any{"product_id": product, "version": c.Version + 1, "pizza": pizza})
}

func pizzaPrice(c PizzaConfiguration, parts []PizzaPart) (*PizzaSnapshot, error) {
	if !c.Enabled {
		if len(parts) > 0 {
			return nil, errors.New("pizza_selection_invalid")
		}
		return nil, nil
	}
	if len(parts) < 1 || len(parts) > c.MaxFlavors || c.MaxFlavors < 1 || c.MaxFlavors > 4 || (c.Policy != "highest" && c.Policy != "weighted_average") {
		return nil, errors.New("pizza_selection_invalid")
	}
	flavors := map[string]PizzaFlavor{}
	for _, f := range c.Flavors {
		if f.Active {
			flavors[f.ID] = f
		}
	}
	snapshot := &PizzaSnapshot{Policy: c.Policy, Flavors: []PizzaFlavorSnapshot{}}
	seen := map[string]bool{}
	var sum, highest int64
	for _, p := range parts {
		f, ok := flavors[p.ID]
		if !ok || seen[p.ID] || p.Parts < 1 || p.Parts > 8 || f.Price < 0 || f.Price > 1000000000 {
			return nil, errors.New("pizza_selection_invalid")
		}
		seen[p.ID] = true
		snapshot.Parts += p.Parts
		if snapshot.Parts > 8 {
			return nil, errors.New("pizza_selection_invalid")
		}
		sum += f.Price * p.Parts
		if f.Price > highest {
			highest = f.Price
		}
		snapshot.Flavors = append(snapshot.Flavors, PizzaFlavorSnapshot{p, f.Name, f.Price})
	}
	if c.Policy == "highest" {
		snapshot.Price = highest
	} else {
		snapshot.Price = (sum + snapshot.Parts/2) / snapshot.Parts
	}
	sort.Slice(snapshot.Flavors, func(i, j int) bool { return snapshot.Flavors[i].ID < snapshot.Flavors[j].ID })
	return snapshot, nil
}

func pizzaName(s *PizzaSnapshot) string {
	if s == nil {
		return ""
	}
	parts := []string{}
	for _, f := range s.Flavors {
		parts = append(parts, fmt.Sprintf("%d/%d %s", f.Parts, s.Parts, f.Name))
	}
	return " · " + strings.Join(parts, " + ")
}
