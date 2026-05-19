// Example: E-commerce API — 4 levels (Category → Product → Variant → Inventory)
//
// Demonstrates per-level filtering that is impractical with a single JOIN:
//   - Only active categories
//   - Only published products with price > 0
//   - Only variants where stock > 0
//   - Only inventory with available quantity
//
// Run: go run ./_examples/ecommerce/

package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/AnthonyLonsMax/dbutil"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

type Inventory struct {
	ID        int    `json:"id" db:"id"`
	VariantID int    `json:"variant_id" db:"variant_id"`
	Warehouse string `json:"warehouse" db:"warehouse"`
	Quantity  int    `json:"quantity" db:"quantity"`
}

type Variant struct {
	ID        int         `json:"id" db:"id"`
	ProductID int         `json:"product_id" db:"product_id"`
	Name      string      `json:"name" db:"name"`
	Stock     int         `json:"stock" db:"stock"`
	Inventory []Inventory `json:"inventory"`
}

type Product struct {
	ID         int       `json:"id" db:"id"`
	CategoryID int       `json:"category_id" db:"category_id"`
	Name       string    `json:"name" db:"name"`
	Price      float64   `json:"price" db:"price"`
	Published  bool      `json:"published" db:"published"`
	Variants   []Variant `json:"variants"`
}

type Category struct {
	ID       int       `json:"id" db:"id"`
	Name     string    `json:"name" db:"name"`
	Active   bool      `json:"active" db:"active"`
	Products []Product `json:"products"`
}

func main() {
	db := sqlx.MustOpen("sqlite3", ":memory:")
	defer db.Close()

	db.MustExec(`
		CREATE TABLE categories (id INTEGER PRIMARY KEY, name TEXT, active INTEGER);
		CREATE TABLE products (id INTEGER PRIMARY KEY, category_id INTEGER REFERENCES categories(id), name TEXT, price REAL, published INTEGER);
		CREATE TABLE variants (id INTEGER PRIMARY KEY, product_id INTEGER REFERENCES products(id), name TEXT, stock INTEGER);
		CREATE TABLE inventory (id INTEGER PRIMARY KEY, variant_id INTEGER REFERENCES variants(id), warehouse TEXT, quantity INTEGER);
	`)

	db.MustExec(`
		INSERT INTO categories VALUES (1, 'Clothing', 1), (2, 'Electronics', 1), (3, 'Discontinued', 0);
		INSERT INTO products VALUES
			(1, 1, 'T-Shirt', 19.99, 1), (2, 1, 'Jeans', 49.99, 0),
			(3, 2, 'Headphones', 99.99, 1), (4, 2, 'Phone Case', 0, 1);
		INSERT INTO variants VALUES
			(1, 1, 'Small', 10), (2, 1, 'Medium', 0), (3, 1, 'Large', 5),
			(4, 3, 'Wired', 3), (5, 3, 'Wireless', 0),
			(6, 4, 'Black', 20), (7, 4, 'White', 15);
		INSERT INTO inventory VALUES
			(1, 1, 'Warehouse A', 20), (2, 1, 'Warehouse B', 5),
			(3, 3, 'Warehouse A', 10),
			(4, 4, 'Warehouse A', 0), (5, 4, 'Warehouse B', 8),
			(6, 6, 'Warehouse A', 30), (7, 7, 'Warehouse B', 25);
	`)

	// ---- Phase 1: Query top-down with per-level filters ----

	var categories []Category
	if err := db.Select(
		&categories,
		"SELECT id, name, active FROM categories WHERE active = 1 ORDER BY id",
	); err != nil {
		log.Fatal(err)
	}

	catIDs := dbutil.ExtractIDs(categories, func(c Category) int { return c.ID })
	q, args, err := sqlx.In(
		"SELECT id, category_id, name, price, published FROM products WHERE category_id IN (?) AND published = 1 AND price > 0 ORDER BY id",
		catIDs,
	)
	if err != nil {
		log.Fatal(err)
	}
	var products []Product
	if err := db.Select(&products, q, args...); err != nil {
		log.Fatal(err)
	}

	prodIDs := dbutil.ExtractIDs(products, func(p Product) int { return p.ID })
	q, args, err = sqlx.In(
		"SELECT id, product_id, name, stock FROM variants WHERE product_id IN (?) AND stock > 0 ORDER BY id",
		prodIDs,
	)
	if err != nil {
		log.Fatal(err)
	}
	var variants []Variant
	if err := db.Select(&variants, q, args...); err != nil {
		log.Fatal(err)
	}

	varIDs := dbutil.ExtractIDs(variants, func(v Variant) int { return v.ID })
	q, args, err = sqlx.In(
		"SELECT id, variant_id, warehouse, quantity FROM inventory WHERE variant_id IN (?) AND quantity > 0 ORDER BY id",
		varIDs,
	)
	if err != nil {
		log.Fatal(err)
	}
	var inventory []Inventory
	if err := db.Select(&inventory, q, args...); err != nil {
		log.Fatal(err)
	}

	// ---- Phase 2: Reconstruct bottom-up ----

	dbutil.MergeChildren(
		variants, dbutil.GroupBy(inventory, func(i Inventory) int { return i.VariantID }),
		func(v Variant) int { return v.ID },
		func(v *Variant, inv []Inventory) { v.Inventory = inv },
	)

	dbutil.MergeChildren(
		products, dbutil.GroupBy(variants, func(v Variant) int { return v.ProductID }),
		func(p Product) int { return p.ID },
		func(p *Product, vs []Variant) { p.Variants = vs },
	)

	dbutil.MergeChildren(
		categories, dbutil.GroupBy(products, func(p Product) int { return p.CategoryID }),
		func(c Category) int { return c.ID },
		func(c *Category, ps []Product) { c.Products = ps },
	)

	out, _ := json.MarshalIndent(categories, "", "  ")
	fmt.Println(string(out))
}
