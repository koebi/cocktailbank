package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"github.com/BurntSushi/toml"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

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

func (in *input) getIngredients() (ingredients map[string]float64) {
	anzahl, err := in.getInt("number of ingredients: ")
	if err != nil {
		log.Fatal(err)
	}

	ingredients = make(map[string]float64)

	for i := 0; i < anzahl; i++ {
		name, err := in.getString("name of ingredient %d", i)
		if err != nil {
			log.Fatal(err)
		}
		amount, err := in.getFloat("amount: ")
		if err != nil {
			log.Fatal(err)
		}

		ingredients[name] = amount
	}
	return ingredients
}

func (in *input) createCocktail(db *DB) {
	var c cocktail
	var err error
	c.name, err = in.getString("Name: ")
	if err != nil {
		log.Fatal(err)
	}
	c.ingredients = in.getIngredients()

	_, err = db.Exec("INSERT INTO cocktails (name) values ($1);", c.name)
	if err != nil {
		fmt.Println("Inserting into cocktails failed, getting:")
		log.Fatal(err)
	}

	rows, err := db.Query("SELECT id FROM cocktails WHERE name = $1;", c.name)
	if err != nil {
		log.Fatal("Selecting id failed, getting", err)
	}

	var id int
	for rows.Next() {
		err = rows.Scan(&id)
		if err != nil {
			log.Fatal("Scanning rows failed, getting", err)
		}
	}
	rows.Close()

	stmt, err := db.Prepare("INSERT INTO ingredients (name, amount, cocktail) values($1, $2, $3)")
	if err != nil {
		log.Fatal("Preparing insertion into ingredients failed:", err)
	}

	st, err := db.Prepare("INSERT INTO inventory (name) values($1)")
	if err != nil {
		log.Fatal("Preparing insertion into inventory failed:", err)
	}

	for zutat, amount := range c.ingredients {
		_, err := stmt.Exec(zutat, amount, id)
		if err != nil {
			log.Fatal("Inserting into Cocktailingredients failed:", err)
		}

		_, err = st.Exec(zutat)
		if err != nil {
			log.Fatal("Inserting into Inventar failed:", err)
		}
	}
}

func (db *DB) initDB() {
	tmp, err := sql.Open("sqlite3", "fest.db")
	db = &DB{tmp}
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec("CREATE TABLE cocktails (id INTEGER NOT NULL PRIMARY KEY ASC AUTOINCREMENT, name TEXT);")
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec("CREATE TABLE ingredients (name TEXT, amount FLOAT, cocktail INT);")
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec("CREATE TABLE inventory (name TEXT UNIQUE ON CONFLICT IGNORE, available FLOAT DEFAULT 0.0, price INTEGER DEFAULT 0);")
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec("CREATE TABLE fests (date TEXT, cocktails INTEGER, price INTEGER DEFAULT 0, amount INTEGER DEFAULT 0);")
	if err != nil {
		log.Fatal(err)
	}
}

func (db *DB) cocktailIngredients(cocktail string) (ingredients map[string]float64) {

	rows, err := db.Query("SELECT id FROM cocktails WHERE name = $1", cocktail)
	if err != nil {
		log.Fatal("selecting cocktail id failed:", err)
	}

	var id int
	for rows.Next() {
		if err := rows.Scan(&id); err != nil {
			log.Fatal("scanning rows failed:", err)
		}
	}
	rows.Close()

	rows, err = db.Query("SELECT name, amount FROM ingredients WHERE cocktail = $1", id)
	if err != nil {
		log.Fatal("selecting from ingredients failed:", err)
	}

	ingredients = make(map[string]float64)
	for rows.Next() {
		var name string
		var amount float64
		if err := rows.Scan(&name, &amount); err != nil {
			log.Fatal("Scanning rows failed:", err)
		}
		ingredients[name] = amount
	}
	rows.Close()

	return ingredients
}

func (db *DB) getCocktails() []cocktail {
	rows, err := db.Query("SELECT name FROM cocktails")
	if err != nil {
		log.Fatal("Selecting cocktail name failed:", err)
	}

	var cocktails []cocktail

	for rows.Next() {
		var c cocktail

		if err := rows.Scan(&c.name); err != nil {
			log.Fatal("Scanning Rows failed:", err)
		}

		c.ingredients = db.cocktailIngredients(c.name)
		cocktails = append(cocktails, c)
	}

	rows.Close()
	return cocktails
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
	name, err := in.getString("Name: ")
	if err != nil {
		log.Fatal(err)
	}
	avail, err := in.getFloat("available [l]: ")
	if err != nil {
		log.Fatal(err)
	}
	_, err = db.Exec("UPDATE inventory SET available = $2 WHERE name = $1", name, avail)
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

func (db *DB) genShoppingList(cfg config) (shoppinglist map[string]float64, pricelist map[string]float64) {
	cocktails := db.getCocktails()
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

	return shoppinglist, pricelist
}

func (in *input) updateInventory(db *DB) {
	items := []string{"add item [a]", "change availability [a]", "change price [p]"}
	for i := range items {
		fmt.Println(i)
	}

	c, err := in.getString("Choice: ")
	if err != nil {
		log.Fatal(err)
	}

	switch {
	case c == "a":
		in.addInventory(db)
	case c == "a":
		in.updateAvailability(db)
	case c == "p":
		in.updatePrice(db)
	}
}

func (in *input) setFest(db *DB, cfg config) {
	fmt.Println("Choose cocktails. Available: ")

	cocktails := db.getCocktails()
	for i, c := range cocktails {
		fmt.Println(i, c.name)
	}
	choice, err := in.getString("Separate choice with ','.")
	if err != nil {
		log.Fatal(err)
	}

	stmt, err := db.Prepare("INSERT INTO fests (date, cocktails, price, amount) values ($1, $2, $3, $4)")
	if err != nil {
		log.Fatal(err)
	}

	for _, c := range strings.Split(choice, ",") {
		id, err := strconv.Atoi(c)
		if err != nil {
			log.Fatal(err)
		}

		amount, err := in.getInt("How many %s are you planning for?", cocktails[id].name)
		if err != nil {
			log.Fatal(err)
		}
		price, err := in.getFloat("What's the price for a %s?", cocktails[id].name)
		if err != nil {
			log.Fatal(err)
		}

		stmt.Exec(cfg.Current, cocktails[id].name, price, amount)
	}
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
		shoppinglist, pricelist := db.genShoppingList(cfg)
		printLists(shoppinglist, pricelist)
	case c == "q":
		return nil
	default:
		return fmt.Errorf("No valid choice, try again…")
	}

	return fmt.Errorf("control reached end of function mainMenu")
}

func printLists(l1 map[string]float64, l2 map[string]float64) {
	w := new(tabwriter.Writer)

	w.Init(os.Stdout, 0, 8, 0, '\t', 0)
	fmt.Fprintf(w, "ingredient\tamount\tprice\n")

	for name, menge := range l1 {
		fmt.Fprintf(w, name, "\t", menge, "\t", l2[name], "\n")
	}
	w.Flush()
}

func (db *DB) createOrOpenDB(database string) {
	_, err := os.Stat(database)
	if os.IsNotExist(err) {
		db.initDB()
	} else if err != nil {
		log.Fatal("Stat returned:", err)
	} else {
		tmp, err := sql.Open("sqlite3", database)
		db = &DB{tmp}
		if err != nil {
			log.Fatal("Opening Database Failed:", err)
		}
	}
}

func main() {

	//parsing config
	configLocation := "/home/koebi/go/src/github.com/koebi/cocktailbank/config.toml"

	var cfg config
	if _, err := toml.Decode(configLocation, &cfg); err != nil {
		fmt.Println("Config-File at location", configLocation, "not found, exiting")
		fmt.Println(err)
		return
	}

	//creating Input for user interaction
	in := newInput(os.Stdin, os.Stdout)

	//creating/opening database
	var db *DB
	db.createOrOpenDB(cfg.Database)

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
