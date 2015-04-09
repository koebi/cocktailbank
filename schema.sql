CREATE TABLE cocktails (
	-- cocktails contains all cocktails available

	-- id is a sequential identifier
	id INTEGER NOT NULL PRIMARY KEY ASC AUTOINCREMENT,
	-- name is the name of the cocktail
	name TEXT,
	--
	PRIMARY KEY(id),
);

CREATE TABLE ingredients (
	-- ingredients lists all ingredients of cocktails

	-- id is a sequential identifier
	id INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
	-- name is the name of the ingredient
	name TEXT,
	-- price gives the current buying price in cents for one liter
	price INTEGER DEFAULT 0,
	--
	PRIMARY KEY(id),

);

CREATE TABLE cocktailingredients {
	-- cocktailingredients maps ingredients to cocktails

	-- ingredient references the ingredient in TABLE ingredients
	ingredient INTEGER,
	-- cocktail references the cocktail in TABLE cocktails
	cocktail INTEGER,
	-- amount is how many liters of this is in the given cocktails
	amount FLOAT,
	--
	PRIMARY KEY(ingredient, cocktail),
	FOREIGN KEY(ingredient) REFERENCES ingredients(id),
	FOREIGN KEY(cocktail) REFERENCES cocktails(id),

};

CREATE TABLE stock (
	-- inventory contains all inventory items
	-- the code makes sure that all inventory only exists once

	-- name is the name of the inventory item
	ingredient INTEGER,
	-- date is the date of the stock-taking
	date DATETIME,
	-- available gives the liters of available fluid
	available FLOAT DEFAULT 0.0,
	--
	PRIMARY KEY(ingredient, date),
	FOREIGN KEY ingredient REFERENCES ingredients(id),
);

CREATE TABLE fests (
	-- fests contains all cocktails of all fests

	-- id is a sequential identifier
	id INTEGER
	-- date is the identifier to set a cocktail to a fest
	date TEXT,
	-- awaited is the number of people that are awaited for this fest
	awaited INTEGER,
	--
	PRIMARY KEY(id),
);

CREATE TABLE festcocktails (
	-- festcocktails maps cocktails to fests

	-- fest references the fest in TABLE fests
	fest INTEGER,
	-- cocktails references the cocktails in TABLE cocktails
	cocktails INTEGER,
	-- price is the selling price of one cocktail in cents
	price INTEGER DEFAULT 0,
	-- amount is how many cocktails are planned in this given fest
	amount INTEGER DEFAULT 0,
	--
	PRIMARY KEY(fest, cocktail),
	FOREIGN KEY(fest) REFERENCES fests(id),
	FOREIGN KEY(cocktails) REFERENCES cocktails(id)
);
