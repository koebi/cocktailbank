package main

import (
	"bufio"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/BurntSushi/toml"

	_ "github.com/mattn/go-sqlite3"
)

var cfg config

var errMenuFinished = errors.New("what do you want to do next?")

var (
	configLocation = flag.String("config", "config.toml", "location of the config file")
)

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
	ingreds, err := db.getIngredients()
	if err != nil {
		return err
	}

	for i, item := range ingreds {
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
		amount, err := in.getFloat("amount for %s [l]: ", ingreds[id])
		if err != nil {
			return err
		}
		c.ingredients[ingreds[id]] = amount
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

	res, err := tx.Exec("INSERT INTO cocktails (name) values ($1);", c.name)
	if err != nil {
		return err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare("INSERT INTO cocktailingredients SET ingredient = (SELECT id FROM ingredients WHERE name = $1), amount = $2, cocktail = $3")
	if err != nil {
		return err
	}

	for zutat, amount := range c.ingredients {
		_, err := stmt.Exec(zutat, amount, id)
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

func initDB() error {
	cmd := exec.Command("sqlite3", "fest.sqlite")
	f, err := os.Open("schema.sql")
	if err != nil {
		return err
	}
	defer f.Close()

	cmd.Stdin = f
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println(string(out))
		return err
	}

	return nil
}

func (db *DB) cocktailIngredients(cocktail string) (ingredients map[string]float64, err error) {

	var id int
	err = db.QueryRow("SELECT id FROM cocktails WHERE name = $1", cocktail).Scan(&id)
	if err != nil {
		return nil, err
	}

	rows, err := db.Query("SELECT ingredients.name, cocktailingredients.amount	FROM cocktailingredients JOIN ingredients ON ingredients.id = cocktailingredients.ingredient WHERE cocktail = $1", id)
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
	name, err := in.getString("name of ingredient: ")
	if err != nil {
		return err
	}
	price, err := in.getInt("price [ct/l]: ")
	if err != nil {
		return err
	}

	_, err = db.Exec("INSERT INTO ingredients (name, price) VALUES ($1, $2)", name, price)
	if err != nil {
		return err
	}
	return nil
}

func (db *DB) getIngredients() ([]string, error) {
	var ingredients []string
	rows, err := db.Query("SELECT name FROM ingredients")
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var ing string

		if err := rows.Scan(&ing); err != nil {
			return nil, err
		}
		ingredients = append(ingredients, ing)
	}
	return ingredients, nil
}

func (db *DB) getIngredientPrices() (map[string]int, error) {
	prices := make(map[string]int)
	rows, err := db.Query("SELECT name, price FROM ingredients")
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var name string
		var price int

		if err := rows.Scan(&name, &price); err != nil {
			return nil, err
		}

		prices[name] = price
	}

	return prices, nil
}

func (db *DB) getStock() (map[string]float64, error) {
	stock := make(map[string]float64)

	rows, err := db.Query("SELECT ingredients.name, stock.available FROM ingredients JOIN stock ON ingredients.id = stock.ingredient")
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var name string
		var avail float64

		if err := rows.Scan(&name, &avail); err != nil {
			return nil, err
		}
		stock[name] = avail
	}
	rows.Close()

	return stock, err
}

func (in *input) updateAvailability(db *DB) error {
	stock, err := db.getStock()
	if err != nil {
		return err
	}
	var numberedInv []string

	for item := range stock {
		numberedInv = append(numberedInv, item)
	}

	fmt.Fprintf(in.w, "   stock\tavailable [l]")
	for i, item := range numberedInv {
		fmt.Fprintf(in.w, "%d: %s\t%.2f\n", i, item, stock[item])
	}

	update, err := in.getInt("Which item do you want to update? ")
	if err != nil {
		return err
	}

	avail, err := in.getFloat("How much is available? [l]: ")
	if err != nil {
		return err
	}

	_, err = db.Exec("UPDATE stock SET available = $1 WHERE ingredient = (SELECT id FROM ingredients WHERE name = $2)", avail, numberedInv[update])
	if err != nil {
		return err
	}
	return nil
}

func (in *input) updatePrice(db *DB) error {
	prices, err := db.getIngredientPrices()
	if err != nil {
		return err
	}

	var numberedInv []string

	for item := range prices {
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

	_, err = db.Exec("UPDATE ingredients SET price = $1 WHERE name = $2;", price, numberedInv[update])
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

	rows, err := db.Query("SELECT cocktails, amount, price FROM festcocktails WHERE fest = (SELECT id FROM fests WHERE date = $1)", f.date)
	if err != nil {
		return newFest(), err
	}

	for rows.Next() {
		var id int
		var c string
		var a int
		var p int

		if err := rows.Scan(&id, &a, &p); err != nil {
			return newFest(), err
		}
		err = db.QueryRow("SELECT name FROM cocktails WHERE id = $1", id).Scan(&c)
		if err != nil {
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
	stock, err := db.getStock()
	if err != nil {
		return err
	}
	prices, err := db.getIngredientPrices()
	if err != nil {
		return err
	}

	fmt.Fprintf(in.w, "number\tingredient\tavailable\tprice\n")

	id := 0
	for item, val := range stock {
		fmt.Fprintf(in.w, "%d\t%s\t%.2f\t%.2f €\n", id, item, val, prices[item]/100)
		id++
	}

	return nil
}

func (db *DB) genShoppingList() (shoppinglist map[string]float64, pricelist map[string]float64, err error) {
	cocktails, err := db.getCocktails()
	if err != nil {
		return nil, nil, err
	}
	stock, err := db.getStock()
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

	for ing, avail := range stock {
		shoppinglist[ing] -= avail
		if shoppinglist[ing] <= 0 {
			shoppinglist[ing] = 0
		}
	}

	prices, err := db.getIngredientPrices()
	if err != nil {
		return nil, nil, err
	}
	pricelist = make(map[string]float64)

	for ing, need := range shoppinglist {
		pricelist[ing] = need * float64(prices[ing])
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

	add, err := in.getString("Do you want to [a]dd, [c]hange or [d]eselect a cocktails? ")
	if err != nil {
		return err
	}

	if add == "d" {
		sel, err := in.getInt("Which cocktail do you want to deselect? ")
		if err != nil {
			return err
		}
		err = db.festCocktail(fest.cocktails[sel], 0, 0, true)
		if err != nil {
			return err
		}
	} else if add == "c" {
		sel, err := in.getInt("Which cocktail do you want to change? ")
		if err != nil {
			return err
		}

		amount, err := in.getFloat("How many %s are you planning for? ", fest.cocktails[sel])
		if err != nil {
			return err
		}
		price, err := in.getInt("What's the price for a %s [ct]? ", fest.cocktails[sel])
		if err != nil {
			return err
		}

		err = db.festCocktail(fest.cocktails[sel], amount, price, false)
		if err != nil {
			return err
		}
	} else if add == "a" {
		fmt.Fprintf(in.w, "Choose cocktails. Available: \n")

		cocktails, err := db.getCocktails()
		if err != nil {
			return err
		}

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

			err = db.festCocktail(cocktails[id].name, price, amount, false)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (db *DB) festCocktail(name string, price float64, amount int, del bool) error {
	fest, err := db.getFest(cfg.Current)
	if err != nil {
		return err
	}

	var id int
	err = db.QueryRow("SELECT id FROM cocktails WHERE name = $1", name).Scan(&id)
	if err != nil {
		return err
	}

	if del == true {
		for _, c := range fest.cocktails {
			if c == name {
				_, err := db.Exec("DELETE FROM festcocktails WHERE cocktails = $1 AND fest = (SELECT id FROM fests WHERE date = $2)", id, cfg.Current)
				if err != nil {
					return err
				}
				return nil
			}
		}
	} else {
		for _, c := range fest.cocktails {
			if c == name {
				_, err := db.Exec("UPDATE festcocktails SET price = $1, amount = $2 WHERE cocktails = $3 AND fest = (SELECT id FROM fests WHERE date = $2)", price, amount, id, cfg.Current)
				if err != nil {
					return err
				}
				return nil
			}
		}
	}

	_, err = db.Exec("INSERT INTO festcocktails SET cocktail = $1, price = $2, amount = $3, fest = (SELECT id FROM fests WHERE date = $4)", id, price, amount, cfg.Current)
	if err != nil {
		return err
	}
	return nil
}

func (in *input) getInventoryValue(db *DB) error {
	stock, err := db.getStock()
	if err != nil {
		return err
	}
	prices, err := db.getIngredientPrices()
	if err != nil {
		return err
	}

	var val float64

	for i, a := range stock {
		val += a * float64(prices[i])
	}

	fmt.Fprintf(in.w, "Current inventory value: %.2f €\n", val/100)
	return nil
}

func (in *input) inventoryMenu(db *DB) error {
	items := []string{"list inventory [l]", "query inventory value [v]", "add item [i]", "change availability[a]", "change price [p]", "main Menu [press enter]"}
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

func (db *DB) updateCocktailName(oldName, newName string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var id int
	err = tx.QueryRow("SELECT id FROM cocktails WHERE name = $1", oldName).Scan(&id)
	if err != nil {
		return err
	}

	_, err = tx.Exec("UPDATE cocktails SET name = $1 WHERE id = $2", newName, id)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

func (db *DB) updateIngredients(cocktail, ing string, amount float64) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var id int
	err = tx.QueryRow("SELECT id FROM cocktails WHERE name = $1", cocktail).Scan(&id)
	if err != nil {
		return err
	}

	_, err = tx.Exec("UPDATE cocktailingredients SET amount = $1 WHERE cocktail = $2 AND ingredient = (SELECT id FROM ingredients WHERE name = $3)", amount, id, ing)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

func (in *input) alterCocktail(db *DB) error {
	fmt.Fprintf(in.w, "Currently available cocktails:\n")

	cocktails, err := db.getCocktails()
	if err != nil {
		return err
	}

	for i, c := range cocktails {
		fmt.Fprintf(in.w, "%d\t%s\n", i, c.name)
	}

	alter, err := in.getString("Alter [n]ame or [i]ngredients? ")
	if err != nil {
		return err
	}

	switch {
	case alter == "n":
		err = in.alterCocktailName(db)
		return err
	case alter == "i":
		alter, err := in.getInt("Which one would you like to alter? ")
		if err != nil {
			return err
		}

		err = in.alterIngredients(cocktails[alter].name, db)
		return err
	default:
		return fmt.Errorf("%s is not a valid Choice", alter)
	}

	return nil
}

func (in *input) alterCocktailName(db *DB) error {
	fmt.Fprintf(in.w, "Currently available cocktails:\n")

	cocktails, err := db.getCocktails()
	if err != nil {
		return err
	}

	for i, c := range cocktails {
		fmt.Fprintf(in.w, "%d\t%s\n", i, c.name)
	}

	alter, err := in.getInt("Which one would you like to alter? ")
	if err != nil {
		return err
	}

	newName, err := in.getString("What is %s actually called? ", cocktails[alter].name)
	if err != nil {
		return err
	}

	err = db.updateCocktailName(cocktails[alter].name, newName)
	if err != nil {
		return err
	}

	return nil
}

func (in *input) alterIngredients(name string, db *DB) error {
	cocktails, err := db.getCocktails()
	if err != nil {
		return err
	}

	var alterct int
	for i, c := range cocktails {
		if c.name == name {
			alterct = i
		}
	}

	fmt.Fprintf(in.w, "Current Ingredients for %s:\n", cocktails[alterct].name)

	var numberedIngreds []string
	for ing := range cocktails[alterct].ingredients {
		numberedIngreds = append(numberedIngreds, ing)
	}

	for i, ing := range numberedIngreds {
		fmt.Fprintf(in.w, "%d %s\t%.2f l\n", i, ing, cocktails[alterct].ingredients[ing])
	}

	alter, err := in.getInt("Which ingredient do you want to alter? ")
	if err != nil {
		return err
	}

	amount, err := in.getFloat("How much %s is actually needed [l]? ", numberedIngreds[alter])
	if err != nil {
		return err
	}

	err = db.updateIngredients(cocktails[alterct].name, numberedIngreds[alter], amount)
	if err != nil {
		return err
	}

	return nil
}

func (in *input) cocktailMenu(db *DB) error {
	items := []string{"create cocktail [c]", "list cocktails [l]", "alter cocktail [a]", "main Menu [press enter]"}
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
		err = in.showCocktails(db)
		return err
	case c == "a":
		err = in.alterCocktail(db)
		return err
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
	items := []string{"show current fest [c]", "alter current selection [a]", "generate shopping list [g]", "show last fests [l]", "main Menu [press enter]"}
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
	case c == "a":
		err := in.setFest(db)
		if err != nil {
			return err
		}
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
	items := []string{"add/delete/alter/… cocktails [c]", "modify/print inventory [i]", "modify/print fest [f]", "quit [q]"}
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
	case c == "i":
		err := in.inventoryMenu(db)
		if err != nil {
			return err
		}
	case c == "f":
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
		if err != nil {
			return nil, err
		} else {
			err = initDB()
			if err != nil {
				return nil, err
			}
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
	flag.Parse()

	if _, err := toml.DecodeFile(*configLocation, &cfg); err != nil {
		fmt.Println("Config-File at location", *configLocation, "not found, exiting")
		fmt.Println(err)
		return
	}

	//creating Input for user interaction
	in := newInput(os.Stdin, os.Stdout)

	//creating/opening database
	db, err := createOrOpenDB(cfg.Database)
	if err != nil {
		fmt.Println(err)
		return
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
