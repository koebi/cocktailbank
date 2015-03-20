package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/BurntSushi/toml"

	_ "github.com/mattn/go-sqlite3"
)

type config struct {
	Current  string
	Last     string
	Database string
}

type cocktail struct {
	name        string
	ingredients map[string]float64
}

type fest struct {
	date      string
	cocktails map[string]float64
}

//TODO: implement type shoppinglist

func newFest() fest {
	return fest{
		cocktails: make(map[string]float64),
	}
}

type DB struct {
	*sql.DB
}

type input struct {
	s *bufio.Scanner
	w io.Writer
}

func newInput(r io.Reader, w io.Writer) *input {
	return &input{
		bufio.NewScanner(r),
		w,
	}
}

func (in *input) getString(prompt string, values ...interface{}) (string, error) {
	fmt.Fprintf(in.w, prompt, values...)

	if !in.s.Scan() {
		return "", in.s.Err()
	}
	return strings.TrimSpace(in.s.Text()), nil
}

func (in *input) getFloat(prompt string, values ...interface{}) (float64, error) {
	fmt.Fprintf(in.w, prompt, values...)

	if !in.s.Scan() {
		return 0, in.s.Err()
	}

	f, err := strconv.ParseFloat(strings.TrimSpace(in.s.Text()), 64)
	if err != nil {
		return 0, err
	}
	return f, err
}

func (in *input) getInt(prompt string, values ...interface{}) (int, error) {
	fmt.Fprintf(in.w, prompt, values...)

	if !in.s.Scan() {
		return 0, in.s.Err()
	}

	i, err := strconv.Atoi(strings.TrimSpace(in.s.Text()))
	if err != nil {
		return 0, err
	}
	return i, err
}

//TODO: rewrite so that Ingredients only show currently available ingredients
//      so that zitronensaft doesn't get imported twice as Zitronensaft…
//      might have to implement adding an ingredient to the inventory…
func (in *input) getIngredients() (ingredients map[string]float64, err error) {
	anzahl, err := in.getInt("number of ingredients: ")
	if err != nil {
		return nil, err
	}

	ingredients = make(map[string]float64)

	for i := 0; i < anzahl; i++ {
		name, err := in.getString("name of ingredient %d: ", i+1)
		if err != nil {
			return nil, err
		}
		amount, err := in.getFloat("amount [l]: ")
		if err != nil {
			return nil, err
		}

		ingredients[name] = amount
	}
	return ingredients, nil
}

func (in *input) createCocktail(db *DB) error {
	var c cocktail
	var err error
	c.name, err = in.getString("Name: ")
	if err != nil {
		return err
	}
	c.ingredients, err = in.getIngredients()
	if err != nil {
		return err
	}

	err = db.insertCocktail(c)
	if err != nil {
		return err
	}
	return nil
}

func (db *DB) insertCocktail(c cocktail) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec("INSERT INTO cocktails (name) values ($1);", c.name)
	if err != nil {
		return err
	}

	rows, err := tx.Query("SELECT id FROM cocktails WHERE name = $1;", c.name)
	if err != nil {
		return err
	}

	var id int
	for rows.Next() {
		err = rows.Scan(&id)
		if err != nil {
			return err
		}
	}
	rows.Close()

	stmt, err := tx.Prepare("INSERT INTO ingredients (name, amount, cocktail) values($1, $2, $3)")
	if err != nil {
		return err
	}

	st, err := tx.Prepare("INSERT INTO inventory (name) values($1)")
	if err != nil {
		return err
	}

	for zutat, amount := range c.ingredients {
		_, err := stmt.Exec(zutat, amount, id)
		if err != nil {
			return err
		}

		_, err = st.Exec(zutat)
		if err != nil {
			return err
		}
	}

	err = tx.Commit()
	if err != nil {
		return err
	}
	return nil
}

func (db *DB) initDB() error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec("CREATE TABLE cocktails (id INTEGER NOT NULL PRIMARY KEY ASC AUTOINCREMENT, name TEXT);")
	if err != nil {
		tx.Rollback()
		return err
	}

	_, err = tx.Exec("CREATE TABLE ingredients (name TEXT, amount FLOAT, cocktail INT);")
	if err != nil {
		tx.Rollback()
		return err
	}

	_, err = tx.Exec("CREATE TABLE inventory (name TEXT UNIQUE ON CONFLICT IGNORE, available FLOAT DEFAULT 0.0, price INTEGER DEFAULT 0);")
	if err != nil {
		tx.Rollback()
		return err
	}

	_, err = tx.Exec("CREATE TABLE fests (date TEXT, cocktails INTEGER, price INTEGER DEFAULT 0, amount INTEGER DEFAULT 0);")
	if err != nil {
		tx.Rollback()
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

func (db *DB) cocktailIngredients(cocktail string) (ingredients map[string]float64, err error) {

	rows, err := db.Query("SELECT id FROM cocktails WHERE name = $1", cocktail)
	if err != nil {
		return nil, err
	}

	var id int
	for rows.Next() {
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
	}
	rows.Close()

	rows, err = db.Query("SELECT name, amount FROM ingredients WHERE cocktail = $1", id)
	if err != nil {
		return nil, err
	}

	ingredients = make(map[string]float64)
	for rows.Next() {
		var name string
		var amount float64
		if err := rows.Scan(&name, &amount); err != nil {
			return nil, err
		}
		ingredients[name] = amount
	}
	rows.Close()

	return ingredients, nil
}

func (db *DB) getCocktails() ([]cocktail, error) {
	rows, err := db.Query("SELECT name FROM cocktails")
	if err != nil {
		return nil, err
	}

	var cocktails []cocktail

	for rows.Next() {
		var c cocktail

		if err := rows.Scan(&c.name); err != nil {
			return nil, err
		}

		c.ingredients, err = db.cocktailIngredients(c.name)
		if err != nil {
			return nil, err
		}
		cocktails = append(cocktails, c)
	}

	rows.Close()
	return cocktails, nil
}

func (in *input) addInventory(db *DB) {
	name, err := in.getString("name: ")
	if err != nil {
		log.Fatal(err)
	}
	avail, err := in.getFloat("available [l]: ")
	if err != nil {
		log.Fatal(err)
	}
	price, err := in.getInt("price [ct]: ")
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec("INSERT INTO inventory (name, available, price) VALUES ($1, $2, $3)", name, avail, price)
	if err != nil {
		log.Fatal("Adding Inventory entry failed:", err)
	}
}

func (db *DB) getInventory() (inventory map[string]float64) {
	inventory = make(map[string]float64)

	rows, err := db.Query("SELECT name, available FROM inventory")
	if err != nil {
		log.Fatal("selecting inventory failed", err)
	}

	for rows.Next() {
		var n string
		var v float64

		if err := rows.Scan(&n, &v); err != nil {
			log.Fatal("scanning rows failed:", err)
		}
		inventory[n] = v
	}
	rows.Close()

	return inventory
}

func (db *DB) getInventoryPriceList() (prices map[string]float64) {
	prices = make(map[string]float64)

	rows, err := db.Query("SELECT name, price FROM inventory")
	if err != nil {
		log.Fatal(err)
	}

	for rows.Next() {
		var n string
		var p float64

		if err := rows.Scan(&n, &p); err != nil {
			log.Fatal(err)
		}
		prices[n] = p
	}
	rows.Close()

	return prices
}

func (in *input) updateAvailability(db *DB) {
	inventory := db.getInventory()
	var numberedInv []string

	for item := range inventory {
		numberedInv = append(numberedInv, item)
	}

	for i, item := range numberedInv {
		fmt.Printf("%d: %s\t%.2f\n", i, item, inventory[item])
	}

	update, err := in.getInt("Which item do you want to update? ")
	if err != nil {
		log.Fatal(err)
	}

	avail, err := in.getFloat("How much is available? [l]: ")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("%T, %T", numberedInv[update], avail)
	fmt.Printf("UPDATE inventory SET available = %f WHERE name = %s", avail, numberedInv[update])
	res, err := db.Exec("UPDATE inventory SET available = $1 WHERE name = $2;", avail, numberedInv[update])
	id, lasterr := res.LastInsertId()
	aff, rowserr := res.RowsAffected()
	fmt.Println(id, lasterr, aff, rowserr, err)
	if err != nil {
		log.Fatal(err)
	}
}

func (in *input) updatePrice(db *DB) {
	name, err := in.getString("Name: ")
	if err != nil {
		log.Fatal(err)
	}
	price, err := in.getInt("Preis [ct]: ")
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec("UPDATE inventory SET price = $2 WHERE name = $1", name, price)
	if err != nil {
		log.Fatal(err)
	}
}

func (db *DB) getFest(cfg config) fest {
	f := newFest()
	f.date = cfg.Current

	rows, err := db.Query("SELECT cocktails, amount FROM fests WHERE date = $1", f.date)
	if err != nil {
		log.Fatal("selecting cocktails failed:", err)
	}

	for rows.Next() {
		var c string
		var m float64

		if err := rows.Scan(&c, &m); err != nil {
			log.Fatal("scanning rows failed:", err)
		}
		f.cocktails[c] = m
	}
	rows.Close()

	return f
}

func (db *DB) genShoppingList(cfg config) (shoppinglist map[string]float64, pricelist map[string]float64, err error) {
	cocktails, err := db.getCocktails()
	if err != nil {
		return nil, nil, err
	}
	inventory := db.getInventory()
	f := db.getFest(cfg)

	shoppinglist = make(map[string]float64)

	for _, c := range cocktails {
		for z, m := range c.ingredients {
			shoppinglist[z] += f.cocktails[c.name] * m
		}
	}

	for z, m := range inventory {
		shoppinglist[z] -= m
		if shoppinglist[z] <= 0 {
			shoppinglist[z] = 0
		}
	}

	prices := db.getInventoryPriceList()
	pricelist = make(map[string]float64)

	for z, m := range shoppinglist {
		pricelist[z] = m * prices[z]
	}

	return shoppinglist, pricelist, nil
}

func (in *input) updateInventory(db *DB) {
	items := []string{"add item [i]", "change availability [a]", "change price [p]"}
	for _, i := range items {
		fmt.Println(i)
	}

	c, err := in.getString("Choice: ")
	if err != nil {
		log.Fatal(err)
	}

	switch {
	case c == "i":
		in.addInventory(db)
	case c == "a":
		in.updateAvailability(db)
	case c == "p":
		in.updatePrice(db)
	}
}

func (in *input) setFest(db *DB, cfg config) error {
	fmt.Println("Choose cocktails. Available: ")

	cocktails, err := db.getCocktails()
	if err != nil {
		return err
	}

	for i, c := range cocktails {
		fmt.Println(i, c.name)
	}
	choice, err := in.getString("Separate choice with ',': ")
	if err != nil {
		return err
	}

	//TODO: change to Update, if Cocktail already selected.
	//      delete if neccessary
	stmt, err := db.Prepare("INSERT INTO fests (date, cocktails, price, amount) values ($1, $2, $3, $4)")
	if err != nil {
		return err
	}

	for _, c := range strings.Split(choice, ",") {
		id, err := strconv.Atoi(c)
		if err != nil {
			return err
		}

		amount, err := in.getInt("How many %s are you planning for? ", cocktails[id].name)
		if err != nil {
			return err
		}
		price, err := in.getFloat("What's the price for a %s? ", cocktails[id].name)
		if err != nil {
			return err
		}

		stmt.Exec(cfg.Current, cocktails[id].name, price, amount)
	}
	return nil
}

func (in *input) mainMenu(db *DB, cfg config) error {
	items := []string{"create cocktail [c]", "update inventory [u]", "set fest cocktails [s]", "generate shopping list [g]", "quit [q]"}
	for _, i := range items {
		fmt.Println(i)
	}

	c, err := in.getString("Choice: ")
	if err != nil {
		log.Fatal(err)
	}

	switch {
	case c == "c":
		in.createCocktail(db)
	case c == "u":
		in.updateInventory(db)
	case c == "s":
		in.setFest(db, cfg)
	case c == "g":
		shoppinglist, pricelist, err := db.genShoppingList(cfg)
		if err != nil {
			return err
		}
		err = printLists(shoppinglist, pricelist)
		return err
	case c == "q":
		return nil
	default:
		return fmt.Errorf("No valid choice, try again…")
	}

	return fmt.Errorf("control reached end of function mainMenu")
}

func printLists(l1 map[string]float64, l2 map[string]float64) error {
	w := new(tabwriter.Writer)

	w.Init(os.Stdout, 0, 8, 0, '\t', 0)
	fmt.Fprintf(w, "ingredient\tamount\tprice\n")

	for name, menge := range l1 {
		fmt.Fprintf(w, "%s\t%.2f\t%.2f\n", name, menge, l2[name])
	}
	w.Flush()
	return nil
}

func createOrOpenDB(database string) (*DB, error) {
	_, err := os.Stat(database)
	if os.IsNotExist(err) {
		fmt.Printf("Database not found, creating new…\n")

		tmp, err := sql.Open("sqlite3", database)
		if err != nil {
			return nil, err
		}
		db := &DB{tmp}
		if err = db.initDB(); err != nil {
			return nil, err
		}
		return db, nil
	} else if err != nil {
		return nil, err
	} else {
		tmp, err := sql.Open("sqlite3", database)
		if err != nil {
			return nil, err
		}
		return &DB{tmp}, err
	}
}

func main() {

	//parsing config
	configLocation := "config.toml"

	var cfg config
	if _, err := toml.DecodeFile(configLocation, &cfg); err != nil {
		fmt.Println("Config-File at location", configLocation, "not found, exiting")
		fmt.Println(err)
		return
	}

	//creating Input for user interaction
	in := newInput(os.Stdin, os.Stdout)

	//creating/opening database
	db, err := createOrOpenDB(cfg.Database)
	if err != nil {
		log.Fatal(err)
	}

	//main loop running the menu until quit.
	for {
		err := in.mainMenu(db, cfg)
		if err == nil {
			return
		}
		fmt.Println(err)
	}

	//	createCocktail(db)
	//	addInventory(db)
	//	liste, pricelist := genShoppingList(db)
	//  fmt.Println(liste, pricelist)
}
