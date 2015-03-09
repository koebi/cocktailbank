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

	_ "github.com/mattn/go-sqlite3"
)

type config struct {
	Current  string
	Last     string
	Database string
}

type cocktail struct {
	name    string
	zutaten map[string]float64
}

type fest struct {
	datum     string
	cocktails map[string]float64
}

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

func (in *input) getZutaten() (zutaten map[string]float64) {
	anzahl, err := in.getInt("Anzahl der Zutaten: ")
	if err != nil {
		log.Fatal(err)
	}

	zutaten = make(map[string]float64)

	for i := 0; i < anzahl; i++ {
		name, err := in.getString("Name der Zutat " + strconv.Itoa(i) + ": ")
		if err != nil {
			log.Fatal(err)
		}
		menge, err := in.getFloat("Menge: ")
		if err != nil {
			log.Fatal(err)
		}

		zutaten[name] = menge
	}
	return zutaten
}

func (in *input) createCocktail(db *DB) {
	var c cocktail
	var err error
	c.name, err = in.getString("Name: ")
	if err != nil {
		log.Fatal(err)
	}
	c.zutaten = in.getZutaten()

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

	stmt, err := db.Prepare("INSERT INTO zutaten (name, menge, cocktail) values($1, $2, $3)")
	if err != nil {
		log.Fatal("Preparing insertion into zutaten failed:", err)
	}

	st, err := db.Prepare("INSERT INTO inventar (name) values($1)")
	if err != nil {
		log.Fatal("Preparing insertion into inventar failed:", err)
	}

	for zutat, menge := range c.zutaten {
		_, err := stmt.Exec(zutat, menge, id)
		if err != nil {
			log.Fatal("Inserting into Cocktailzutaten failed:", err)
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

	_, err = db.Exec("CREATE TABLE zutaten (name TEXT, menge FLOAT, cocktail INT);")
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec("CREATE TABLE inventar (name TEXT UNIQUE ON CONFLICT IGNORE, vorhanden FLOAT DEFAULT 0.0, preis INTEGER DEFAULT 0);")
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec("CREATE TABLE feste (datum TEXT, cocktails INTEGER, preis INTEGER DEFAULT 0, menge INTEGER DEFAULT 0);")
	if err != nil {
		log.Fatal(err)
	}
}

func (db *DB) cocktailZutaten(cocktail string) (zutaten map[string]float64) {

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

	rows, err = db.Query("SELECT name, menge FROM zutaten WHERE cocktail = $1", id)
	if err != nil {
		log.Fatal("selecting from zutaten failed:", err)
	}

	zutaten = make(map[string]float64)
	for rows.Next() {
		var name string
		var menge float64
		if err := rows.Scan(&name, &menge); err != nil {
			log.Fatal("Scanning rows failed:", err)
		}
		zutaten[name] = menge
	}
	rows.Close()

	return zutaten
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

		c.zutaten = db.cocktailZutaten(c.name)
		cocktails = append(cocktails, c)
	}

	rows.Close()
	return cocktails
}

func (in *input) addInventory(db *DB) {
	name, err := in.getString("Name: ")
	if err != nil {
		log.Fatal(err)
	}
	avail, err := in.getFloat("Vorhanden [l]: ")
	if err != nil {
		log.Fatal(err)
	}
	price, err := in.getInt("Preis [ct]: ")
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec("INSERT INTO inventar (name, vorhanden, preis) VALUES ($1, $2, $3)", name, avail, price)
	if err != nil {
		log.Fatal("Adding Inventory entry failed:", err)
	}
}

func (db *DB) getInventory() (inventory map[string]float64) {
	inventory = make(map[string]float64)

	rows, err := db.Query("SELECT name, vorhanden FROM inventar")
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

	rows, err := db.Query("SELECT name, preis FROM inventar")
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
	avail, err := in.getFloat("Vorhanden [l]: ")
	if err != nil {
		log.Fatal(err)
	}
	_, err = db.Exec("UPDATE inventar SET vorhanden = $2 WHERE name = $1", name, avail)
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

	_, err = db.Exec("UPDATE inventar SET price = $2 WHERE name = $1", name, price)
	if err != nil {
		log.Fatal(err)
	}
}

func (db *DB) getFest(cfg config) fest {
	f := newFest()
	f.datum = cfg.Current

	rows, err := db.Query("SELECT cocktails, menge FROM feste WHERE datum = $1", f.datum)
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

func (db *DB) genShoppingList(cfg config) (einkaufsliste map[string]float64, preisliste map[string]float64) {
	cocktails := db.getCocktails()
	inventar := db.getInventory()
	f := db.getFest(cfg)

	einkaufsliste = make(map[string]float64)

	for _, c := range cocktails {
		for z, m := range c.zutaten {
			einkaufsliste[z] += f.cocktails[c.name] * m
		}
	}

	for z, m := range inventar {
		einkaufsliste[z] -= m
		if einkaufsliste[z] <= 0 {
			einkaufsliste[z] = 0
		}
	}

	prices := db.getInventoryPriceList()
	preisliste = make(map[string]float64)

	for z, m := range einkaufsliste {
		preisliste[z] = m * prices[z]
	}

	return einkaufsliste, preisliste
}

func (in *input) updateInventory(db *DB) {
	items := []string{"Item hinzufügen [i]", "Menge ändern [m]", "Preis ändern [p]"}
	for i := range items {
		fmt.Println(i)
	}

	c, err := in.getString("Auswahl: ")
	if err != nil {
		log.Fatal(err)
	}

	switch {
	case c == "i":
		in.addInventory(db)
	case c == "m":
		in.updateAvailability(db)
	case c == "p":
		in.updatePrice(db)
	}
}

func (in *input) setFest(db *DB, cfg config) {
	fmt.Println("Wähle Cocktails aus. Zur Verfügung stehen: ")

	cocktails := db.getCocktails()
	for i, c := range cocktails {
		fmt.Println(i, c.name)
	}
	choice, err := in.getString("Bitte Auswahl mit ',' trennen.")
	if err != nil {
		log.Fatal(err)
	}

	stmt, err := db.Prepare("INSERT INTO feste (datum, cocktails, preis, menge) values ($1, $2, $3, $4)")
	if err != nil {
		log.Fatal(err)
	}

	for _, c := range strings.Split(choice, ",") {
		id, err := strconv.Atoi(c)
		if err != nil {
			log.Fatal(err)
		}

		menge, err := in.getInt("Wie viele %s sollen angeboten werden?", cocktails[id].name)
		if err != nil {
			log.Fatal(err)
		}
		preis, err := in.getFloat("Wie viel soll ein " + cocktails[id].name + " kosten [ct]? ")
		if err != nil {
			log.Fatal(err)
		}

		stmt.Exec(cfg.Current, cocktails[id].name, preis, menge)
	}
}

func (in *input) mainMenu(db *DB, cfg config) error {
	items := []string{"Cocktail erstellen [c]", "Inventar updaten [i]", "Festcocktails festlegen [f]", "Einkaufsliste erstellen [e]", "Beenden [q]"}
	for _, i := range items {
		fmt.Println(i)
	}

	c, err := in.getString("Aktion: ")
	if err != nil {
		log.Fatal(err)
	}

	switch {
	case c == "c":
		in.createCocktail(db)
	case c == "i":
		in.updateInventory(db)
	case c == "f":
		in.setFest(db, cfg)
	case c == "e":
		liste, preisliste := db.genShoppingList(cfg)
		fmt.Println(liste)
		fmt.Println(preisliste)
	case c == "q":
		return nil
	}

	return fmt.Errorf("Keinr valide Auswahl. Erneut versuchen")
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
	//	liste, preisliste := genShoppingList(db)
	//  fmt.Println(liste, preisliste)
}
