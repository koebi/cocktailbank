CREATE TABLE cocktails (
	-- cocktails contains all cocktails available

	-- id is a sequential identifier
	id INTEGER NOT NULL PRIMARY KEY ASC AUTOINCREMENT,
	-- name is the name of the cocktail
	name TEXT
);

CREATE TABLE ingredients (
	-- ingredients lists all ingredients of cocktails

	-- name is the name of the ingredient
	name TEXT,
	-- amount is how many liters of this is in the given cocktails
	amount FLOAT,
	-- cocktail references the cocktail in TABLE cocktails
	cocktail INTEGER

);

CREATE TABLE inventory (
	-- inventory contains all inventory items
	-- the code makes sure that all inventory only exists once

	-- name is the name of the inventory item
	name TEXT UNIQUE ON CONFLICT IGNORE,
	-- available gives the liters of available fluid
	available FLOAT DEFAULT 0.0,
	-- price gives the current buying price in cents for one liter
	price INTEGER DEFAULT 0
);

CREATE TABLE fests (
	-- fests contains all cocktails of all fests

	-- date is the identifier to set a cocktail to a fest
	date TEXT,
	-- cocktails references the cocktails in TABLE cocktails
	cocktails INTEGER,
	-- price is the selling price of one cocktail in cents
	price INTEGER DEFAULT 0,
	-- amount is how many cocktails are planned in this given fest
	amount INTEGER DEFAULT 0
);
