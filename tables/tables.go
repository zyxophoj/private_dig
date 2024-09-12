package tables

// These tables are in a their own file because they are large.

var Locations = map[uint8]string{
	0:  "Achilles (Troy)",
	15: "Hector (Troy)",
	17: "Helen (Troy)",
	24: "Magdaline (Padre)",
	31: "New Constantinople (New Constantinople)",
	36: "Oakham (Pentonvile)",
}

var Cargo = map[uint8]string{
	0:  "Grain",
	1:  "Generic Foods",
	2:  "Luxury Foods",
	3:  "Wood",
	4:  "Plastics",
	5:  "Iron",
	6:  "Tungsten",
	9:  "Food Dispensers",
	10: "Home Appliances",
	11: "Pre-Fabs",
	13: "Communications",
	15: "Construction",
	16: "Factory Equipment",
	17: "Space Salvage",
	18: "Robot Workers",
	24: "Furs",
	25: "Liquor",
	27: "PlayThing (tm)",
	33: "Pets",
	34: "Tobacco",
	35: "Ultimate",
	42: "Alien Artifact(s)",  //You can only have one, right?
	49: "Mission Cargo",
}
