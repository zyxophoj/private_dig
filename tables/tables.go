package tables

// These tables are in their own file because they are large.

import "privdump/types"

var Factions = []string{"Merchants", "Hunters", "Confeds", "Kilrathi", "Militia", "Pirates", "Drone", "Steltek", "Retros"}

const (
	FACTION_MERCHANTS = iota
	FACTION_HUNTERS
	FACTION_CONFEDS
	FACTION_KILRATHI
	FACTION_MILITIA
	FACTION_PIRATES
	FACTION_DRONE
	FACTION_STELTEK
	FACTION_RETROS

	FACTION_COUNT
)

// This is the order that they are displayed in the ship dealer, I guess
const (
	SHIP_TARSUS    = 0
	SHIP_ORION     = 1
	SHIP_CENTURION = 2
	SHIP_GALAXY    = 3
)

// Equipment
const (
	//Why do we start counting at 90?  I have no clue.
	SHIELD_BASE_0 = 89
)

var Turrets = map[int]string{
	1: "Rear",
	2: "Top",
	3: "Bottom",
}

func Guns(t types.Game) map[int]string {
	return map[types.Game]map[int]string{
		types.GT_PRIV: map[int]string{
			5: "Laser",
			3: "Mass Driver",
			1: "Meson Blaster",
			0: "Neutron gun",
			4: "Particle Cannon",
			7: "Tachyon Cannon",
			2: "Ionic Pulse Cannon",
			6: "Plasma Gun",

			8: "Steltek Gun",
			9: "Boosted Steltek Gun",
		},
		types.GT_RF: map[int]string{
			5: "Laser",
			3: "Mass Driver",
			1: "Meson Blaster",
			0: "Neutron gun",
			4: "Particle Cannon",
			7: "Tachyon Cannon",
			2: "Ionic Pulse Cannon",
			6: "Plasma Gun",

			8: "Fusion Cannon",
		},
	}[t]
}

// mounts: turret, gun, launcher
const (
	TM_REAR   = 1
	TM_TOP    = 2
	TM_BOTTOM = 3

	GM_LEFT_OUT   = 1
	GM_LEFT       = 2
	GM_RIGHT      = 3
	GM_RIGHT_OUT  = 4
	GM_TURRET_1_2 = 5
	GM_TURRET_1_1 = 7
	GM_TURRET_2_2 = 8
	GM_TURRET_2_1 = 10

	// The Centurion just has to be special and have its own unique constants for "left" and "right"
	// This may be a relic from when the Centurion had 4 launchers.
	LM_CENTRE    = 0
	LM_LEFT      = 1
	LM_LEFT_CEN  = 2
	LM_RIGHT_CEN = 3
	LM_RIGHT     = 4
	LM_TURRET_1  = 6
	LM_TURRET_2  = 9
)

// rear/top is "turret 1" depending on ship
var Gun_mounts = map[int]string{
	GM_LEFT_OUT:   "Left outer",
	GM_LEFT:       "Left",
	GM_RIGHT:      "Right",
	GM_RIGHT_OUT:  "Right outer",
	GM_TURRET_1_2: "Rear/Top 2",
	GM_TURRET_1_1: "Rear/Top 1",
	GM_TURRET_2_2: "Bottom 2",
	GM_TURRET_2_1: "Bottom 1",
}

var Launchers = map[int]string{
	50: "Missile Launcher",
	51: "Torpedo Launcher",
	52: "Tractor Beam",
}

var Launcher_mounts = map[int]string{
	LM_CENTRE:    "Centre",
	LM_LEFT:      "Left",
	LM_LEFT_CEN:  "Left(c)",
	LM_RIGHT_CEN: "Right(c)",
	LM_RIGHT:     "Right",

	LM_TURRET_1: "Turret 1",
	LM_TURRET_2: "Turret 2",
}

var Missiles = map[int]string{
	1: "Torpedo",
	4: "Dumbfire",
	2: "Heat Seeker",
	5: "Image Rec",
	3: "Friend or Foe",
}

var locations_rf = func() map[BASE_ID]Baseinfo {
	m := map[BASE_ID]Baseinfo{}
	for k, v := range Bases {
		m[k] = v
	}
	m[59] = Baseinfo{Name: "Gaea", Type: BT_SPECIAL, System: SYS_DELTA_PRIME} //UGH!!!!!
	return m
}()

func Locations(gt types.Game) map[BASE_ID]Baseinfo {
	if gt == types.GT_RF {
		return locations_rf
	}

	return Bases
}

var systems_rf = func() map[SYS_ID]Sysinfo {
	m := map[SYS_ID]Sysinfo{}
	for k, v := range systems {
		m[k] = v
	}
	m[68] = Sysinfo{Name: "Eden", Quadrant: QUAD_FARISS, Bases: []BASE_ID{59}} //UGH!
	return m
}()

func Systems(gt types.Game) map[SYS_ID]Sysinfo {
	if gt == types.GT_RF {
		return systems_rf
	}

	return systems
}

var Cargo = map[int]string{
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

var Ship_names = map[int]string{
	SHIP_TARSUS:    "Tarsus",
	SHIP_ORION:     "Orion",
	SHIP_CENTURION: "Centurion",
	SHIP_GALAXY:    "Galaxy",
}

// Yes, really.  There is clearly some structure in here, but I can't make any sense out of it.
// As a practical matter, you can edit level 7 engines into a Centurion and have an absurdly overpowered ship,
// but the things that you'd expect to appear next in the list (like "122531415162" are worse than level 2 engines.
var Shield_names = map[string]string{
	"1261":         "0",
	"124151":       "1",
	"12314151":     "2",
	"1231415162":   "3",
	"122131415161": "4a",
	"122131415162": "4b",
	"122231415162": "5",
	"122331415162": "6",
	"122431415162": "7",
}
