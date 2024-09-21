package tables

// These tables are in their own file because they are large.



var Factions = []string{"Merchants", "Hunters", "Confeds", "Kilrathi", "Militia", "Pirates", "Drone", "", "Retros"}
const (
	FACTION_MERCHANTS = iota
	FACTION_HUNTERS
	FACTION_CONFEDS
	FACTION_KILRATHI
	FACTION_MILITIA
	FACTION_PIRATES
	FACTION_DRONE
	FACTION_WTF
	FACTION_RETROS

	FACTION_COUNT
)

// This is the order that they are displayed in the ship dealer, I guess
const(
	SHIP_TARSUS=0
	SHIP_ORION=1
	SHIP_CENTURION=2
	SHIP_GALAXY=3
)

var Locations = map[uint8]string{
	//Generated with rip.go
	0:  "Achilles  (Troy)",
	1:  "Anapolis  (Perry)",
	2:  "Basque  (Pyrenees)",
	3:  "Basra  (Palan)",
	4:  "Beaconsfield  (Auriga)",
	5:  "Bodensee  (Tingerhoff)",
	6:  "Burton  (Junction)",
	7:  "Charon  (Hyades)",
	8:  "Drake  (Capella)",
	9:  "Edinburgh  (New Caledonia)",
	10: "Edom  (New Constantinople)",
	11: "Elysia  (Auriga)",
	12: "Erewhon  (Shangri La)",
	13: "Glasgow  (New Caledonia)",
	14: "Gracchus  (Raxis)",
	15: "Hector  (Troy)",
	16: "Heimdel  (Midgard)",
	17: "Helen  (Troy)",
	18: "Jolson  (XXN-1927)",
	19: "Joplin  (XXN-1927)",
	20: "Kronecker  (Regallis)",
	21: "Lisacc  (Lisacc)",
	22: "Liverpool  (Newcastle)",
	23: "Macabee  (Nexus)",
	24: "Magdaline  (Padre)",
	25: "Matahari  (Aldebran)",
	26: "Meadow  (Hind's Variable N.)",
	27: "Megiddo  (Telar)",
	28: "Mjolnar  (Ragnarok)",
	29: "Munchen  (Tingerhoff)",
	30: "N1912-1  (DN-N1912)",
	31: "New Constantinople  (New Constantinople)",
	32: "New Detroit  (New Detroit)",
	33: "New Iberia  (Pyrenees)",
	34: "New Reno  (ND-57)",
	35: "Nitir  (Nitir)",
	36: "Oakham  (Pentonville)",
	37: "Olympus  (Saxtogue)",
	38: "Oresville  (Hind's Variable N.)",
	39: "Oxford  (Oxford)",
	40: "Palan  (Palan)",
	41: "Perry Naval Base  (Perry)",
	42: "Remus  (Pollux)",
	43: "Rilke  (Varnus)",
	44: "Rodin  (Varnus)",
	45: "Romulus  (Castor)",
	46: "Rygannon  (Rygannon)",
	47: "Saratov  (Prasepe)",
	48: "Siva  (Rikel)",
	49: "Smallville  (KM-252)",
	50: "Speke  (Junction)",
	51: "Surtur  (Surtur)",
	52: "Thisbury  (Manchester)",
	53: "Trinsic  (Raxis)",
	54: "Tuck's  (Sherwood)",
	55: "Valkyrie  (Valhalla)",
	56: "Victoria  (Junction)",
	57: "Vishnu  (Rikel)",
	58: "Wickerton  (Manchester)",
	59: "Derelict Base  (Delta Prime)",
}

var Systems = map[uint8]string{
	//Generated with rip.go (hence the strange order, which is ASCIIbetical-within-quadrant)
	10: "CM-N1054",
	16: "Freyja",
	19: "119CE",
	21: "Junction",
	38: "Padre",
	40: "Pender's Star",
	41: "Pentonville",
	44: "Pollux",
	45: "Prasepe",
	46: "Pyrenees",
	58: "Troy",
	64: "Varnus",

	0:  "17-AR",
	8:  "Capella",
	9:  "Castor",
	12: "Crab-12",
	13: "Death",
	15: "Famine",
	20: "J900",
	25: "KM-252",
	31: "New Caledonia",
	35: "Nexus",
	39: "Palan",
	43: "Pestilence",
	49: "Regallis",
	51: "Rygannon",
	54: "Sherwood",
	56: "Telar",
	59: "Delta",
	60: "Gamma",
	61: "Beta",
	62: "Delta Prime",
	63: "Valhalla",
	65: "War",
	66: "Xytani",
	68: "#Testbed#",

	1:  "41-GS",
	2:  "44-P-IM",
	3:  "Aldebran",
	4:  "Auriga",
	14: "DN-N1912",
	17: "Hind's Variable N.",
	27: "Manchester",
	28: "Metsor",
	30: "ND-57",
	32: "Newcastle",
	33: "New Constantinople",
	34: "New Detroit",
	37: "Oxford",
	48: "Raxis",
	52: "Saxtogue",
	53: "Shangri La",
	67: "XXN-1927",

	5:  "Blockade Point Alpha",
	6:  "Blockade Point Charlie",
	7:  "Blockade Point Tango",
	11: "CMF-A",
	18: "Hyades",
	22: "Tr'Pakh",
	23: "Sumn-Kp'ta",
	24: "Mah'Rahn",
	26: "Lisacc",
	29: "Midgard",
	36: "Nitir",
	42: "Perry",
	47: "Ragnarok",
	50: "Rikel",
	55: "Surtur",
	57: "Tingerhoff",
}

var Cargo = map[uint8]string{
	0:  "Grain",
	1:  "Generic Foods",
	2:  "Luxury Foods",
	3:  "Wood",
	4:  "Plastics",
	5:  "Iron",
	6:  "Tungsten",
	7:  "Plutonium",
	8:  "Uranium",
	9:  "Food Dispensers",
	10: "Home Appliances",
	11: "Pre-Fabs",
	12: "Robot Servants",
	13: "Communications",
	14: "Mining Equipment",
	15: "Construction",
	16: "Factory Equipment",
	17: "Space Salvage",
	18: "Robot Workers",
	19: "Computers",
	20: "Medical Equipment",
	21: "Home Entertainment",
	22: "Software",
	23: "Holographics",
	24: "Furs",
	25: "Liquor",
	26: "Gems",
	27: "PlayThing (tm)",
	28: "Games",
	29: "Books",
	30: "Movies",
	31: "Artwork",
	33: "Pets",
	34: "Tobacco",
	35: "Ultimate",
	36: "Brilliance",
	37: "Slaves",
	38: "Weaponry",
	39: "Advanced Fuels",
	42: "Alien Artifact(s)", //You can only have one, right?
	49: "Mission Cargo",
}
