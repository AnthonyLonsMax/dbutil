package dbutil

import (
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

// --- 3-level nesting: Category -> Product -> Variant ---

type testVar struct {
	ID        int     `db:"id"`
	ProductID int     `db:"product_id"`
	Name      string  `db:"name"`
	Price     float64 `db:"price"`
}

type testProd struct {
	ID         int       `db:"id"`
	Name       string    `db:"name"`
	CategoryID int       `db:"category_id"`
	Variants   []testVar `db:"-"`
}

type testCat struct {
	ID       int        `db:"id"`
	Name     string     `db:"name"`
	Products []testProd `db:"-"`
}

func TestThreeLevelNesting(t *testing.T) {
	dbs, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = dbs.Close() })

	dbs.MustExec(`
		CREATE TABLE categories (id INTEGER PRIMARY KEY, name TEXT);
		CREATE TABLE products (id INTEGER PRIMARY KEY, category_id INTEGER REFERENCES categories(id), name TEXT);
		CREATE TABLE variants (id INTEGER PRIMARY KEY, product_id INTEGER REFERENCES products(id), name TEXT, price REAL);
	`)

	dbs.MustExec(`
		INSERT INTO categories VALUES (1, 'Electronics'), (2, 'Books');
		INSERT INTO products VALUES (1, 1, 'Phone'), (2, 1, 'Laptop'), (3, 2, 'Go Book');
		INSERT INTO variants VALUES
			(1, 1, 'iPhone', 999), (2, 1, 'Android', 799),
			(3, 2, 'MacBook', 1999), (4, 2, 'ThinkPad', 1499),
			(5, 3, 'Paperback', 29), (6, 3, 'Hardcover', 49);
	`)

	// ---------- Phase 1: Query top-down (WHERE IN) ----------

	var categories []testCat
	err = dbs.Select(&categories, "SELECT id, name FROM categories ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}

	catIDs := ExtractIDs(categories, func(c testCat) int { return c.ID })

	query, args, err := sqlx.In(
		"SELECT id, category_id, name FROM products WHERE category_id IN (?) ORDER BY id",
		catIDs,
	)
	if err != nil {
		t.Fatal(err)
	}

	var products []testProd

	err = dbs.Select(&products, query, args...)
	if err != nil {
		t.Fatal(err)
	}

	prodIDs := ExtractIDs(products, func(p testProd) int { return p.ID })

	query, args, err = sqlx.In(
		"SELECT id, product_id, name, price FROM variants WHERE product_id IN (?) ORDER BY id",
		prodIDs,
	)
	if err != nil {
		t.Fatal(err)
	}

	var variants []testVar

	err = dbs.Select(&variants, query, args...)
	if err != nil {
		t.Fatal(err)
	}

	// ---------- Phase 2: Reconstruct bottom-up ----------

	MergeChildren(
		products, GroupBy(variants, func(v testVar) int { return v.ProductID }),
		func(p testProd) int { return p.ID },
		func(p *testProd, vs []testVar) { p.Variants = vs },
	)

	MergeChildren(
		categories, GroupBy(products, func(p testProd) int { return p.CategoryID }),
		func(c testCat) int { return c.ID },
		func(c *testCat, ps []testProd) { c.Products = ps },
	)

	// Assertions (order-independent)
	lookupCat := func(id int) *testCat {
		for i := range categories {
			if categories[i].ID == id {
				return &categories[i]
			}
		}

		return nil
	}
	lookupProd := func(prods []testProd, id int) *testProd {
		for i := range prods {
			if prods[i].ID == id {
				return &prods[i]
			}
		}

		return nil
	}

	elec := lookupCat(1)
	if elec == nil {
		t.Fatal("Electronics not found")
	}

	if len(elec.Products) != 2 {
		t.Fatalf("expected 2 products in Electronics, got %d", len(elec.Products))
	}

	phone := lookupProd(elec.Products, 1)
	if phone == nil {
		t.Fatal("Phone not found")
	}
	if len(phone.Variants) != 2 {
		t.Fatalf("expected 2 variants in Phone, got %d", len(phone.Variants))
	}
	if phone.Variants[0].Name != "iPhone" || phone.Variants[0].Price != 999 {
		t.Errorf("unexpected first variant: %+v", phone.Variants[0])
	}

	laptop := lookupProd(elec.Products, 2)
	if laptop == nil {
		t.Fatal("Laptop not found")
	}
	if len(laptop.Variants) != 2 {
		t.Fatalf("expected 2 variants in Laptop, got %d", len(laptop.Variants))
	}

	books := lookupCat(2)
	if books == nil {
		t.Fatal("Books not found")
	}
	if len(books.Products) != 1 {
		t.Fatalf("expected 1 product in Books, got %d", len(books.Products))
	}
	if len(books.Products[0].Variants) != 2 {
		t.Fatalf("expected 2 variants in Go Book, got %d", len(books.Products[0].Variants))
	}
}

// --- 4-level nesting: Continent -> Country -> City -> District ---

type testDist struct {
	ID     int    `db:"id"`
	CityID int    `db:"city_id"`
	Name   string `db:"name"`
}

type testCity4 struct {
	ID        int        `db:"id"`
	Name      string     `db:"name"`
	CountryID int        `db:"country_id"`
	Districts []testDist `db:"-"`
}

type testCountry4 struct {
	ID          int         `db:"id"`
	Name        string      `db:"name"`
	ContinentID int         `db:"continent_id"`
	Cities      []testCity4 `db:"-"`
}

type testContinent4 struct {
	ID        int            `db:"id"`
	Name      string         `db:"name"`
	Countries []testCountry4 `db:"-"`
}

func TestFourLevelNesting(t *testing.T) {
	dbs, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = dbs.Close() })

	dbs.MustExec(`
		CREATE TABLE continents (id INTEGER PRIMARY KEY, name TEXT);
		CREATE TABLE countries (id INTEGER PRIMARY KEY, continent_id INTEGER REFERENCES continents(id), name TEXT);
		CREATE TABLE cities (id INTEGER PRIMARY KEY, country_id INTEGER REFERENCES countries(id), name TEXT);
		CREATE TABLE districts (id INTEGER PRIMARY KEY, city_id INTEGER REFERENCES cities(id), name TEXT);
	`)

	dbs.MustExec(`
		INSERT INTO continents VALUES (1, 'Europe'), (2, 'Asia');
		INSERT INTO countries VALUES (1, 1, 'France'), (2, 1, 'Spain'), (3, 2, 'Japan');
		INSERT INTO cities VALUES
			(1, 1, 'Paris'), (2, 1, 'Lyon'),
			(3, 2, 'Madrid'),
			(4, 3, 'Tokyo');
		INSERT INTO districts VALUES
			(1, 1, 'Montmartre'), (2, 1, 'Le Marais'),
			(3, 2, 'Vieux Lyon'), (4, 2, 'Presqu''ile'),
			(5, 3, 'Sol'), (6, 3, 'Gran Via'),
			(7, 4, 'Shibuya'), (8, 4, 'Shinjuku');
	`)

	// ---------- Phase 1: Query top-down (WHERE IN) ----------

	var continents []testContinent4

	err = dbs.Select(&continents, "SELECT id, name FROM continents ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	contIDs := ExtractIDs(continents, func(c testContinent4) int { return c.ID })

	query, args, err := sqlx.In(
		"SELECT id, continent_id, name FROM countries WHERE continent_id IN (?) ORDER BY id",
		contIDs,
	)
	if err != nil {
		t.Fatal(err)
	}

	var countries []testCountry4

	err = dbs.Select(&countries, query, args...)
	if err != nil {
		t.Fatal(err)
	}

	ctryIDs := ExtractIDs(countries, func(c testCountry4) int { return c.ID })

	query, args, err = sqlx.In(
		"SELECT id, country_id, name FROM cities WHERE country_id IN (?) ORDER BY id",
		ctryIDs,
	)
	if err != nil {
		t.Fatal(err)
	}

	var cities []testCity4

	err = dbs.Select(&cities, query, args...)
	if err != nil {
		t.Fatal(err)
	}

	cityIDs := ExtractIDs(cities, func(c testCity4) int { return c.ID })

	query, args, err = sqlx.In(
		"SELECT id, city_id, name FROM districts WHERE city_id IN (?) ORDER BY id",
		cityIDs,
	)
	if err != nil {
		t.Fatal(err)
	}

	var districts []testDist
	err = dbs.Select(&districts, query, args...)
	if err != nil {
		t.Fatal(err)
	}

	// ---------- Phase 2: Reconstruct bottom-up ----------

	MergeChildren(
		cities, GroupBy(districts, func(d testDist) int { return d.CityID }),
		func(c testCity4) int { return c.ID },
		func(c *testCity4, ds []testDist) { c.Districts = ds },
	)

	MergeChildren(
		countries, GroupBy(cities, func(c testCity4) int { return c.CountryID }),
		func(c testCountry4) int { return c.ID },
		func(c *testCountry4, cs []testCity4) { c.Cities = cs },
	)

	MergeChildren(
		continents, GroupBy(countries, func(c testCountry4) int { return c.ContinentID }),
		func(c testContinent4) int { return c.ID },
		func(c *testContinent4, cs []testCountry4) { c.Countries = cs },
	)

	// Assertions (order-independent)
	lookupCont := func(id int) *testContinent4 {
		for i := range continents {
			if continents[i].ID == id {
				return &continents[i]
			}
		}

		return nil
	}
	lookupCity := func(cc []testCity4, id int) *testCity4 {
		for i := range cc {
			if cc[i].ID == id {
				return &cc[i]
			}
		}

		return nil
	}

	europe := lookupCont(1)
	if europe == nil {
		t.Fatal("Europe not found")
	}
	if len(europe.Countries) != 2 {
		t.Fatalf("expected 2 countries in Europe, got %d", len(europe.Countries))
	}

	france := &europe.Countries[0]
	if france.ID == 2 {
		france = &europe.Countries[1]
	}
	if france.Name != "France" || france.ID != 1 {
		t.Fatalf("expected France (ID=1), got %+v", france)
	}
	if len(france.Cities) != 2 {
		t.Fatalf("expected 2 cities in France, got %d", len(france.Cities))
	}

	paris := lookupCity(france.Cities, 1)
	if paris == nil {
		t.Fatal("Paris not found in France")
	}
	if len(paris.Districts) != 2 {
		t.Fatalf("expected 2 districts in Paris, got %d", len(paris.Districts))
	}
	if paris.Districts[0].Name != "Montmartre" && paris.Districts[1].Name != "Montmartre" {
		t.Errorf("expected Montmartre in districts, got %+v", paris.Districts)
	}

	asia := lookupCont(2)
	if asia == nil {
		t.Fatal("Asia not found")
	}
	if len(asia.Countries) != 1 {
		t.Fatalf("expected 1 country in Asia, got %d", len(asia.Countries))
	}
	if asia.Countries[0].Name != "Japan" {
		t.Errorf("expected Japan, got %s", asia.Countries[0].Name)
	}
	if len(asia.Countries[0].Cities) != 1 {
		t.Fatalf("expected 1 city in Japan, got %d", len(asia.Countries[0].Cities))
	}
	if len(asia.Countries[0].Cities[0].Districts) != 2 {
		t.Fatalf("expected 2 districts in Tokyo, got %d", len(asia.Countries[0].Cities[0].Districts))
	}
}

// --- 6-level nesting: Continent -> Country -> Region -> City -> District -> Building ---

type testBuilding6 struct {
	ID         int    `db:"id"`
	DistrictID int    `db:"district_id"`
	Name       string `db:"name"`
}

type testDistrict6 struct {
	ID        int             `db:"id"`
	CityID    int             `db:"city_id"`
	Name      string          `db:"name"`
	Buildings []testBuilding6 `db:"-"`
}

type testCity6 struct {
	ID        int             `db:"id"`
	RegionID  int             `db:"region_id"`
	Name      string          `db:"name"`
	Districts []testDistrict6 `db:"-"`
}

type testRegion6 struct {
	ID        int         `db:"id"`
	CountryID int         `db:"country_id"`
	Name      string      `db:"name"`
	Cities    []testCity6 `db:"-"`
}

type testCountry6 struct {
	ID          int           `db:"id"`
	ContinentID int           `db:"continent_id"`
	Name        string        `db:"name"`
	Regions     []testRegion6 `db:"-"`
}

type testContinent6 struct {
	ID        int            `db:"id"`
	Name      string         `db:"name"`
	Countries []testCountry6 `db:"-"`
}

func TestSixLevelNesting(t *testing.T) {
	dbs, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = dbs.Close() })

	dbs.MustExec(`
		CREATE TABLE continents (id INTEGER PRIMARY KEY, name TEXT);
		CREATE TABLE countries (id INTEGER PRIMARY KEY, continent_id INTEGER REFERENCES continents(id), name TEXT);
		CREATE TABLE regions (id INTEGER PRIMARY KEY, country_id INTEGER REFERENCES countries(id), name TEXT);
		CREATE TABLE cities (id INTEGER PRIMARY KEY, region_id INTEGER REFERENCES regions(id), name TEXT);
		CREATE TABLE districts (id INTEGER PRIMARY KEY, city_id INTEGER REFERENCES cities(id), name TEXT);
		CREATE TABLE buildings (id INTEGER PRIMARY KEY, district_id INTEGER REFERENCES districts(id), name TEXT);
	`)

	dbs.MustExec(`
		INSERT INTO continents VALUES (1, 'Europe'), (2, 'Asia');
		INSERT INTO countries VALUES (1, 1, 'France'), (2, 1, 'Germany'), (3, 2, 'Japan');
		INSERT INTO regions VALUES (1, 1, 'Ile-de-France'), (2, 2, 'Bavaria'), (3, 3, 'Kanto');
		INSERT INTO cities VALUES (1, 1, 'Paris'), (2, 2, 'Munich'), (3, 3, 'Tokyo');
		INSERT INTO districts VALUES
			(1, 1, 'Montmartre'), (2, 1, 'Le Marais'),
			(3, 2, 'Altstadt'),
			(4, 3, 'Shibuya'), (5, 3, 'Shinjuku');
		INSERT INTO buildings VALUES
			(1, 1, 'Sacre Coeur'),
			(2, 2, 'Place des Vosges'),
			(3, 3, 'Marienplatz'),
			(4, 4, 'Shibuya Crossing'),
			(5, 5, 'Tokyo Metro');
	`)

	// ---------- Phase 1: Query top-down (WHERE IN) ----------

	var continents []testContinent6
	err = dbs.Select(&continents, "SELECT id, name FROM continents ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	contIDs := ExtractIDs(continents, func(c testContinent6) int { return c.ID })

	query, args, err := sqlx.In(
		"SELECT id, continent_id, name FROM countries WHERE continent_id IN (?) ORDER BY id",
		contIDs,
	)
	if err != nil {
		t.Fatal(err)
	}

	var countries []testCountry6
	err = dbs.Select(&countries, query, args...)
	if err != nil {
		t.Fatal(err)
	}
	ctryIDs := ExtractIDs(countries, func(c testCountry6) int { return c.ID })

	query, args, err = sqlx.In(
		"SELECT id, country_id, name FROM regions WHERE country_id IN (?) ORDER BY id",
		ctryIDs,
	)
	if err != nil {
		t.Fatal(err)
	}

	var regions []testRegion6
	err = dbs.Select(&regions, query, args...)
	if err != nil {
		t.Fatal(err)
	}
	regIDs := ExtractIDs(regions, func(r testRegion6) int { return r.ID })

	query, args, err = sqlx.In(
		"SELECT id, region_id, name FROM cities WHERE region_id IN (?) ORDER BY id",
		regIDs,
	)
	if err != nil {
		t.Fatal(err)
	}

	var cities []testCity6
	err = dbs.Select(&cities, query, args...)
	if err != nil {
		t.Fatal(err)
	}
	cityIDs := ExtractIDs(cities, func(c testCity6) int { return c.ID })

	query, args, err = sqlx.In(
		"SELECT id, city_id, name FROM districts WHERE city_id IN (?) ORDER BY id",
		cityIDs,
	)
	if err != nil {
		t.Fatal(err)
	}

	var districts []testDistrict6

	err = dbs.Select(&districts, query, args...)
	if err != nil {
		t.Fatal(err)
	}

	distIDs := ExtractIDs(districts, func(d testDistrict6) int { return d.ID })

	query, args, err = sqlx.In(
		"SELECT id, district_id, name FROM buildings WHERE district_id IN (?) ORDER BY id",
		distIDs,
	)
	if err != nil {
		t.Fatal(err)
	}

	var buildings []testBuilding6
	err = dbs.Select(&buildings, query, args...)
	if err != nil {
		t.Fatal(err)
	}

	// ---------- Phase 2: Reconstruct bottom-up ----------

	MergeChildren(
		districts, GroupBy(buildings, func(b testBuilding6) int { return b.DistrictID }),
		func(d testDistrict6) int { return d.ID },
		func(d *testDistrict6, bs []testBuilding6) { d.Buildings = bs },
	)

	MergeChildren(
		cities, GroupBy(districts, func(d testDistrict6) int { return d.CityID }),
		func(c testCity6) int { return c.ID },
		func(c *testCity6, ds []testDistrict6) { c.Districts = ds },
	)

	MergeChildren(
		regions, GroupBy(cities, func(c testCity6) int { return c.RegionID }),
		func(r testRegion6) int { return r.ID },
		func(r *testRegion6, cs []testCity6) { r.Cities = cs },
	)

	MergeChildren(
		countries, GroupBy(regions, func(r testRegion6) int { return r.CountryID }),
		func(c testCountry6) int { return c.ID },
		func(c *testCountry6, rs []testRegion6) { c.Regions = rs },
	)

	MergeChildren(
		continents, GroupBy(countries, func(c testCountry6) int { return c.ContinentID }),
		func(c testContinent6) int { return c.ID },
		func(c *testContinent6, cs []testCountry6) { c.Countries = cs },
	)

	// Assertions (order-independent)
	lookupCont := func(id int) *testContinent6 {
		for i := range continents {
			if continents[i].ID == id {
				return &continents[i]
			}
		}

		return nil
	}
	lookupDist := func(dd []testDistrict6, id int) *testDistrict6 {
		for i := range dd {
			if dd[i].ID == id {
				return &dd[i]
			}
		}

		return nil
	}

	europe := lookupCont(1)
	if europe == nil {
		t.Fatal("Europe not found")
	}
	if len(europe.Countries) != 2 {
		t.Fatalf("expected 2 countries in Europe, got %d", len(europe.Countries))
	}

	france := &europe.Countries[0]
	if france.ID != 1 {
		france = &europe.Countries[1]
	}

	if france.Name != "France" {
		t.Fatalf("expected France, got %s", france.Name)
	}

	if len(france.Regions) != 1 {
		t.Fatalf("expected 1 region in France, got %d", len(france.Regions))
	}

	paris := france.Regions[0].Cities[0]
	if len(paris.Districts) != 2 {
		t.Fatalf("expected 2 districts in Paris, got %d", len(paris.Districts))
	}

	montmartre := lookupDist(paris.Districts, 1)
	if montmartre == nil {
		t.Fatal("Montmartre not found in Paris")
	}
	if len(montmartre.Buildings) != 1 {
		t.Fatalf("expected 1 building in Montmartre, got %d", len(montmartre.Buildings))
	}
	if montmartre.Buildings[0].Name != "Sacre Coeur" {
		t.Errorf("expected Sacre Coeur, got %s", montmartre.Buildings[0].Name)
	}

	asia := lookupCont(2)
	if asia == nil {
		t.Fatal("Asia not found")
	}
	if len(asia.Countries) != 1 {
		t.Fatalf("expected 1 country in Asia, got %d", len(asia.Countries))
	}
	japan := asia.Countries[0]
	if japan.Name != "Japan" {
		t.Errorf("expected Japan, got %s", japan.Name)
	}
	if len(japan.Regions) != 1 {
		t.Fatalf("expected 1 region in Japan, got %d", len(japan.Regions))
	}

	tokyo := japan.Regions[0].Cities[0]
	if tokyo.Name != "Tokyo" {
		t.Errorf("expected Tokyo, got %s", tokyo.Name)
	}
	if len(tokyo.Districts) != 2 {
		t.Fatalf("expected 2 districts in Tokyo, got %d", len(tokyo.Districts))
	}

	shibuyaDist := lookupDist(tokyo.Districts, 4)
	if shibuyaDist == nil {
		t.Fatal("Shibuya district not found in Tokyo")
	}
	if len(shibuyaDist.Buildings) != 1 {
		t.Fatalf("expected 1 building in Shibuya, got %d", len(shibuyaDist.Buildings))
	}
	if shibuyaDist.Buildings[0].Name != "Shibuya Crossing" {
		t.Errorf("expected Shibuya Crossing, got %s", shibuyaDist.Buildings[0].Name)
	}
}
