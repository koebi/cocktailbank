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

var cfg config

type config struct {
	Awaited  int
	Schema   string
	Current  string
	Database string
}

type cocktail struct {
	name        string
	ingredients map[string]float64
}

func newCocktail() cocktail {
	return cocktail{
		ingredients: make(map[string]float64),
	}
}

type fest struct {
	date            string
	cocktails       []string
	cocktailprices  map[string]int
	cocktailamounts map[string]int
}

//TODO: implement type shoppinglist

func newFest() fest {
	return fest{
		cocktailprices:  make(map[string]int),
		cocktailamounts: make(map[string]int),
	}
}

type DB struct {
	*sql.DB
}

type input struct {
	s *bufio.Scanner
	w *tabwriter.Writer
}

func newInput(r io.Reader, w io.Writer) *input {
	return &input{
		bufio.NewScanner(r),
		tabwriter.NewWriter(w, 0, 8, 0, '\t', 0),
	}
}

func (in *input) getString(prompt string, values ...interface{}) (string, error) {
	fmt.Fprintf(in.w, prompt, values...)
	in.w.Flush()

	if !in.s.Scan() {
		return "", in.s.Err()
	}
	return strings.TrimSpace(in.s.Text()), nil
}

func (in *input) getFloat(prompt string, values ...interface{}) (float64, error) {
	fmt.Fprintf(in.w, prompt, values...)
	in.w.Flush()

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
	in.w.Flush()

	if !in.s.Scan() {
		return 0, in.s.Err()
	}

	i, err := strconv.Atoi(strings.TrimSpace(in.s.Text()))
	if err != nil {
		return 0, err
	}
	return i, err
}

func (in *input) createCocktail(db *DB) error {
	c := newCocktail()
	var err error
	c.name, err = in.getString("Name: ")
	if err != nil {
		return err
	}

	fmt.Fprintf(in.w, "Currently available ingredients:\n")
	inventory, err := db.getInventory()
	if err != nil {
		return err
	}
	var numberedInv []string

	for item := range inventory {
		numberedInv = append(numberedInv, item)
	}

	for i, item := range numberedInv {
		fmt.Printf("%d\t%s\n", i, item)
	}

	choice, err := in.getString("Select the ingredients of %s [separate with ',', q to quit]:", c.name)
	if err != nil {
		return err
	}
	for _, i := range strings.Split(choice, ",") {
		if i == "q" {
			return fmt.Errorf("No cocktail created")
		}
		id, err := strconv.Atoi(i)
		if err != nil {
			return err
		}
		amount, err := in.getFloat("amount for %s [l]: ", numberedInv[id])
		if err != nil {
			return err
		}
		c.ingredients[numberedInv[id]] = amount
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
		c := newCocktail()

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

func (in *input) addInventory(db *DB) error {
	name, err := in.getString("name: ")
	if err != nil {
		return err
	}
	avail, err := in.getFloat("available [l]: ")
	if err != nil {
		return err
	}
	price, err := in.getInt("price [ct]: ")
	if err != nil {
		return err
	}

	_, err = db.Exec("INSERT INTO inventory (name, available, price) VALUES ($1, $2, $3)", name, avail, price)
	if err != nil {
		return err
	}
	return nil
}

func (db *DB) getInventory() (inventory map[string]float64, err error) {
	inventory = make(map[string]float64)

	rows, err := db.Query("SELECT name, available FROM inventory")
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var n string
		var v float64

		if err := rows.Scan(&n, &v); err != nil {
			return nil, err
		}
		inventory[n] = v
	}
	rows.Close()

	return inventory, err
}

func (db *DB) getInventoryPriceList() (prices map[string]float64, err error) {
	prices = make(map[string]float64)

	rows, err := db.Query("SELECT name, price FROM inventory")
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var n string
		var p float64

		if err := rows.Scan(&n, &p); err != nil {
			return nil, err
		}
		prices[n] = p
	}
	rows.Close()

	return prices, nil
}

func (in *input) updateAvailability(db *DB) error {
	inventory, err := db.getInventory()
	if err != nil {
		return err
	}
	var numberedInv []string

	for item := range inventory {
		numberedInv = append(numberedInv, item)
	}

	for i, item := range numberedInv {
		fmt.Fprintf(in.w, "%d: %s\t%.2f\n", i, item, inventory[item])
	}

	update, err := in.getInt("Which item do you want to update? ")
	if err != nil {
		return err
	}

	avail, err := in.getFloat("How much is available? [l]: ")
	if err != nil {
		return err
	}

	_, err = db.Exec("UPDATE inventory SET available = $1 WHERE name = $2;", avail, numberedInv[update])
	if err != nil {
		return err
	}
	return nil
}

func (in *input) updatePrice(db *DB) error {
	inventory, err := db.getInventory()
	if err != nil {
		return err
	}
	prices, err := db.getInventoryPriceList()
	if err != nil {
		return err
	}

	var numberedInv []string

	for item := range inventory {
		numberedInv = append(numberedInv, item)
	}
	fmt.Fprintf(in.w, "   cocktail\tprice\n")
	for i, item := range numberedInv {
		fmt.Fprintf(in.w, "%d: %s\t%.2f\n", i, item, prices[item])
	}

	update, err := in.getInt("Which item do you want to update? ")
	if err != nil {
		return err
	}

	price, err := in.getFloat("What is the current price? [ct]: ")
	if err != nil {
		return err
	}

	_, err = db.Exec("UPDATE inventory SET price = $1 WHERE name = $2;", price, numberedInv[update])
	if err != nil {
		return err
	}

	return nil
}

func (db *DB) festDates() (dates []string, err error) {
	rows, err := db.Query("SELECT date FROM fests;")
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var d string

		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		dates = append(dates, d)
	}
	return dates, nil
}

func (in *input) lastFest(db *DB) error {
	fmt.Fprintf(in.w, "Select a fest. Currently available:\n")

	dates, err := db.festDates()
	if err != nil {
		return err
	}

	for i, d := range dates {
		fmt.Fprintf(in.w, "%d %s\n", i, d)
	}

	id, err := in.getInt("Which fest do you want to see?\n")
	if err != nil {
		return err
	}

	fest, err := db.getFest(dates[id])
	if err != nil {
		return err
	}

	fmt.Fprintf(in.w, "Cocktails\tplanned\tprice\n")
	for c, a := range fest.cocktailamounts {
		fmt.Fprintf(in.w, "%s\t%d\t%2f €\n", c, a, float64(fest.cocktailprices[c])/100.0)
	}

	return nil
}

func (db *DB) getFest(date string) (fest, error) {
	f := newFest()
	f.date = date

	rows, err := db.Query("SELECT cocktails, amount, price FROM fests WHERE date = $1", f.date)
	if err != nil {
		return newFest(), err
	}

	for rows.Next() {
		var c string
		var a int
		var p int

		if err := rows.Scan(&c, &a, &p); err != nil {
			return newFest(), err
		}
		f.cocktails = append(f.cocktails, c)
		f.cocktailamounts[c] = a
		f.cocktailprices[c] = p
	}
	rows.Close()

	return f, nil
}

func (in *input) listInventory(db *DB) error {
	inventory, err := db.getInventory()
	if err != nil {
		return err
	}
	prices, err := db.getInventoryPriceList()
	if err != nil {
		return err
	}

	fmt.Fprintf(in.w, "number\tingredient\tavailable\tprice\n")

	id := 0
	for item, val := range inventory {
		fmt.Fprintf(in.w, "%d\t%s\t%.2f\t%.2f €\n", id, item, val, prices[item]/100)
		id++
	}

	in.w.Flush()
	return nil
}

func (db *DB) genShoppingList() (shoppinglist map[string]float64, pricelist map[string]float64, err error) {
	cocktails, err := db.getCocktails()
	if err != nil {
		return nil, nil, err
	}
	inventory, err := db.getInventory()
	if err != nil {
		return nil, nil, err
	}
	f, err := db.getFest(cfg.Current)
	if err != nil {
		return nil, nil, err
	}

	shoppinglist = make(map[string]float64)

	for _, c := range cocktails {
		for z, m := range c.ingredients {
			shoppinglist[z] += float64(f.cocktailamounts[c.name]) * m
		}
	}

	for z, m := range inventory {
		shoppinglist[z] -= m
		if shoppinglist[z] <= 0 {
			shoppinglist[z] = 0
		}
	}

	prices, err := db.getInventoryPriceList()
	if err != nil {
		return nil, nil, err
	}
	pricelist = make(map[string]float64)

	for z, m := range shoppinglist {
		pricelist[z] = m * prices[z]
	}

	return shoppinglist, pricelist, nil
}

func (in *input) setFest(db *DB) error {
	fmt.Fprintf(in.w, "Current selection:\n")
	fest, err := db.getFest(cfg.Current)
	if err != nil {
		return err
	}

	fmt.Fprintf(in.w, "%s\t%s\t%s\n", "cocktail", "planned", "price")
	for i, c := range fest.cocktails {
		fmt.Fprintf(in.w, "%d %s\t%d\t%.2f €\n", i, c, fest.cocktailamounts[c], float64(fest.cocktailprices[c])/100.0)
	}

	fmt.Fprintf(in.w, "Choose cocktails. Available: \n")

	cocktails, err := db.getCocktails()
	if err != nil {
		return err
	}

	//TODO: use fest.cocktails instead of fest.cocktailamounts[]
	//      I am currently not quite sure how to do ,ok with arrays…
	for i, c := range cocktails {
		if _, ok := fest.cocktailamounts[c.name]; !ok {
			fmt.Fprintf(in.w, "%d\t%s\n", i, c.name)
		}
	}

	choice, err := in.getString("Separate choice with ',': ")
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
		price, err := in.getFloat("What's the price for a %s [ct]? ", cocktails[id].name)
		if err != nil {
			return err
		}

		err = db.festCocktail(cocktails[id].name, price, amount)
		if err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) festCocktail(name string, price float64, amount int) error {
	fest, err := db.getFest(cfg.Current)
	if err != nil {
		return err
	}

	for _, c := range fest.cocktails {
		if c == name {
			_, err := db.Exec("UPDATE fests SET price = $1, amount = $2 WHERE cocktails = $3", price, amount, name)
			if err != nil {
				return err
			}
			return nil
		}
	}

	_, err = db.Exec("INSERT INTO fests (date, cocktails, price, amount) values ($1, $2, $3, $4)", cfg.Current, name, price, amount)
	if err != nil {
		return err
	}
	return nil
}

func (in *input) getInventoryValue(db *DB) error {
	inventory, err := db.getInventory()
	if err != nil {
		return err
	}
	prices, err := db.getInventoryPriceList()
	if err != nil {
		return err
	}

	var val float64

	for i, a := range inventory {
		val += a * prices[i]
	}

	fmt.Fprintf(in.w, "Current inventory value: %.2f €\n", val/100)
	return nil
}

func (in *input) deleteInventory(db *DB) error {
	inventory, err := db.getInventory()
	if err != nil {
		return err
	}
	var numberedInv []string

	for item := range inventory {
		numberedInv = append(numberedInv, item)
	}

	for i, item := range numberedInv {
		fmt.Fprintf(in.w, "%d: %s\t%.2f\n", i, item, inventory[item])
	}

	delete, err := in.getInt("Which item do you want to update? ")
	if err != nil {
		return err
	}

	_, err = db.Exec("DELETE FROM inventory WHERE name = $1 LIMIT 1", numberedInv[delete])
	if err != nil {
		return err
	}

	return nil
}

func (in *input) inventoryMenu(db *DB) error {
	items := []string{"list inventory [l]", "query inventory value [v]", "add item [i]", "change availability[a]", "change price [p]", "delete item [d]", "main Menu [press enter]"}
	for _, i := range items {
		fmt.Fprintf(in.w, "%s\n", i)
	}

	c, err := in.getString("Choice: ")
	if err != nil {
		return err
	}

	switch {
	case c == "l":
		if err = in.listInventory(db); err != nil {
			return err
		}
	case c == "v":
		if err = in.getInventoryValue(db); err != nil {
			return err
		}
	case c == "i":
		if err = in.addInventory(db); err != nil {
			return err
		}
	case c == "a":
		if err = in.updateAvailability(db); err != nil {
			return err
		}
	case c == "p":
		if err = in.updatePrice(db); err != nil {
			return err
		}
	case c == "d":
		if err = in.deleteInventory(db); err != nil {
			return err
		}
	case c == "":
		return nil
	default:
		return fmt.Errorf("%s is not a valid choice", c)
	}

	return nil
}

func (in *input) showCocktails(db *DB) error {
	fmt.Fprintf(in.w, "Currently available cocktails:\n")

	cocktails, err := db.getCocktails()
	if err != nil {
		return err
	}

	for i, c := range cocktails {
		fmt.Fprintf(in.w, "%d\t%s\n", i, c.name)
	}

	i, err := in.getInt("Investigate further? Choose a cocktail: ")
	if err != nil {
		return err
	}

	fmt.Fprintf(in.w, "Ingredients for %s:\n", cocktails[i].name)
	for k, v := range cocktails[i].ingredients {
		fmt.Fprintf(in.w, "%s\t%.2f l\n", k, v)
	}

	return nil
}

func (in *input) cocktailMenu(db *DB) error {
	items := []string{"create cocktail [c]", "list cocktails [l]", "alter cocktail [a]", "delete cocktail [d]", "main Menu [press enter]"}
	for _, i := range items {
		fmt.Fprintf(in.w, "%s\n", i)
	}

	c, err := in.getString("Choice: ")
	if err != nil {
		return err
	}

	switch {
	case c == "c":
		in.createCocktail(db)
	case c == "l":
		err := in.showCocktails(db)
		return err
	case c == "a":
		//TODO: write function alter cocktail
		return fmt.Errorf("function not implemented yet")
	case c == "d":
		//TODO: write function delete cocktail
		return fmt.Errorf("function not implemented yet")
	default:
	}
	return nil
}

func (in *input) currentFest(db *DB) error {
	fmt.Fprintf(in.w, "Current selection:\n")
	fest, err := db.getFest(cfg.Current)
	if err != nil {
		return err
	}

	fmt.Fprintf(in.w, "%s\t%s\t%s\n", "cocktail", "planned", "price")
	for i, c := range fest.cocktails {
		fmt.Fprintf(in.w, "%d %s\t%d\t%.2f €\n", i, c, fest.cocktailamounts[c], float64(fest.cocktailprices[c])/100.0)
	}

	var allcs int
	for _, a := range fest.cocktailamounts {
		allcs += a
	}
	ratio := float64(allcs) / float64(cfg.Awaited)

	fmt.Fprintf(in.w, "You are currently planning for %d cocktails and %d guests. That makes for %.2f cocktails/guest\n", allcs, cfg.Awaited, ratio)
	if ratio <= 1.6 {
		fmt.Fprintf(in.w, "This ratio should be around 2, it is a bit low.\n")
	} else if ratio >= 2.4 {
		fmt.Fprintf(in.w, "This ratio should be around 2, it is a bit high.\n")
	} else {
		fmt.Fprintf(in.w, "This ratio should be around 2, so, it is looking good. Remember not to calculate for too many people ;)\n")
	}

	return nil
}

func (in *input) festMenu(db *DB) error {
	items := []string{"show current fest [c]", "print current fest to file [p]", "select cocktails [s]", "alter current selection [a]", "generate shopping list [g]", "show last fests [l]", "main Menu [press enter]"}
	for _, i := range items {
		fmt.Fprintf(in.w, "%s\n", i)
	}

	c, err := in.getString("Choice: ")
	if err != nil {
		return err
	}

	switch {
	case c == "c":
		err := in.currentFest(db)
		if err != nil {
			return err
		}
	case c == "p":
		//TODO: add function print Fest
		return fmt.Errorf("function not implemented yet")
	case c == "s":
		in.setFest(db)
	case c == "a":
		//TODO: add function to alter current selection
		return fmt.Errorf("function not implemented yet")
	case c == "g":
		shoppinglist, pricelist, err := db.genShoppingList()
		if err != nil {
			in.w.Flush()
			return err
		}
		err = in.printLists(shoppinglist, pricelist)
		in.w.Flush()
		return err
	case c == "l":
		err := in.lastFest(db)
		if err != nil {
			return err
		}
	default:
	}
	return nil
}

func (in *input) mainMenu(db *DB) error {
	items := []string{"add/delete/alter/… cocktails [c]", "modify/print inventory [u]", "modify/print fest[s]", "quit [q]"}
	for _, i := range items {
		fmt.Fprintf(in.w, "%s\n", i)
	}

	c, err := in.getString("Choice: ")
	if err != nil {
		return err
	}

	switch {
	case c == "c":
		err := in.cocktailMenu(db)
		if err != nil {
			return err
		}
	case c == "u":
		err := in.inventoryMenu(db)
		if err != nil {
			return err
		}
	case c == "s":
		err := in.festMenu(db)
		if err != nil {
			return err
		}
	case c == "q":
		in.w.Flush()
		return nil
	default:
		in.w.Flush()
		return fmt.Errorf("No valid choice, try again…")
	}
	in.w.Flush()
	return fmt.Errorf("What do you want to do next?")
}

func (in *input) printLists(l1 map[string]float64, l2 map[string]float64) error {
	fmt.Fprintf(in.w, "ingredient\tamount\tprice\n")

	for name, menge := range l1 {
		fmt.Fprintf(in.w, "%s\t%.2f\t%.2f €\n", name, menge, l2[name]/100)
	}
	in.w.Flush()
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

		_, err = os.Stat(cfg.Schema)
		if os.IsNotExist(err) {
			if err = db.initDB(); err != nil {
				return nil, err
			}
		} else if err != nil {
			return nil, err
		} else {
			fmt.Println("There is a schema-file included, use it!")
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
		err := in.mainMenu(db)
		if err == nil {
			return
		}
		fmt.Fprintf(in.w, "\n%s\n\n", err)
	}
}
