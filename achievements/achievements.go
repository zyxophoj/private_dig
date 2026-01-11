package achievements

import "bytes"
import "fmt"
import "slices"
import "strconv"
import "strings"

import "privdump/tables"
import "privdump/types"
import "privdump/readers"

type Arg struct {
	types.Savedata

	// These are *the actual variables* from global_state, not copies.
	// In the case of maps, that works by itself.  Otherwise, some contortions
	// in global_state were needed to allow pointers to be exported.
	Visited map[tables.BASE_ID]bool
	Secrets *uint8

	// output variable used only (and optionally) by multi-file-achievements
	Progress string
}

// Update is almost a constructor for Arg.
// It updates persistent state (Visited and Secrets) based on file state
// Because these are pointers to global state (or simulated global state in the tests),
// it *UPDATES GLOBAL STATE AS A SIDE_EFFECT*.  This is hardly civilized constructor behaviour.
func (a *Arg) Update() {

	a.Visited[a.Location()] = true // current location

	switch a.Savedata.Game() {
	case types.GT_PRIV:

		a.Visited[0] = true // Achilles, the starting location

		// Locations that must have been visited to advance the plot.
		//
		// The best we can do without using the poorly understood flag byte is to detect if a mission has been accepted.
		// This doesn't work perfectly - for example, New Constantinople is provably visited on completion of Tayla 3,
		// but we only acknowledge the start of Tayla 4.
		infos := []struct {
			plot     string
			location tables.BASE_ID
		}{
			{"s0ma", tables.BASE_NEW_DETROIT},
			{"s1mb", tables.BASE_OAKHAM},
			{"s1mc", tables.BASE_HECTOR},
			{"s1md", tables.BASE_NEW_CONSTANTINOPLE},
			{"s2mc", tables.BASE_SIVA},
			{"s2md", tables.BASE_REMUS},
			{"s3ma", tables.BASE_OXFORD},
			{"s4ma", tables.BASE_BASRA},
			{"s4md", tables.BASE_PALAN},
			{"s5ma", tables.BASE_RYGANNON},
			{"s6ma", tables.BASE_DERELICT_BASE},
			{"s7mb", tables.BASE_PERRY_NAVAL_BASE},
		}

		str, _ := a.Plot_info()
		if len(str) == 4 {
			for _, info := range infos {
				if str >= info.plot {
					a.Visited[info.location] = true
				}
			}
		}

		// Either Steltek gun proves the player got to the derelict
		guns := a.Forms[types.OFFSET_REAL].Get("FITE", "WEAP", "GUNS")
		if guns != nil {
			for n := 0; n < len(guns.Data); n += 4 {
				if guns.Data[n] >= 8 {
					for _, info := range infos {
						a.Visited[info.location] = true
						break
					}
				}
			}
		}

		// TODO: theoretically, the player could have visited the derelict and not picked up a gun
		// Ths should in principle be detectable by the "angry drone" state, but where is that stored in the save file?
		// Also, who does that?

	case types.GT_RF:

		a.Visited[tables.BASE_JOLSON] = true // the starting location
		// Deduce visited based on RF plot state
		infos := []struct {
			flag     int
			location tables.BASE_ID
		}{
			{tables.FLAG_RF_TAYLA_1_OFFERED, tables.BASE_OAKHAM},
			{tables.FLAG_RF_TAYLA_1_DONE, tables.BASE_TUCK_S},
			{tables.FLAG_RF_TAYLA_2_DONE, tables.BASE_SARATOV},
			{tables.FLAG_RF_TAYLA_3_DONE, tables.BASE_SPEKE},
			{tables.FLAG_RF_TAYLA_4_DONE, tables.BASE_BASQUE},
			{tables.FLAG_RF_MURPHY_1_OFFERED, tables.BASE_EDOM},
			{tables.FLAG_RF_MURPHY_2_DONE, tables.BASE_LIVERPOOL},
			{tables.FLAG_RF_MURPHY_3_DONE, tables.BASE_NEW_DETROIT},
			{tables.FLAG_RF_ROMAN_LYNCH_INTRODUCED, tables.BASE_BASQUE},
			{tables.FLAG_RF_GOODIN_1_OFFERED, tables.BASE_PERRY_NAVAL_BASE},
			{tables.FLAG_RF_MASTERSON_1_OFFERED, tables.BASE_OXFORD},
			{tables.FLAG_RF_MASTERSON_1_DONE, tables.BASE_EDOM},
			{tables.FLAG_RF_MASTERSON_3_DONE, tables.BASE_SPEKE},
			{tables.FLAG_RF_MASTERSON_5_DONE, tables.BASE_PERRY_NAVAL_BASE},
			{tables.FLAG_RF_MONTE_1_OFFERED, tables.BASE_MACABEE},
			{tables.FLAG_RF_MONTE_1_DONE, tables.BASE_NEW_DETROIT},
			{tables.FLAG_RF_MONTE_2A_DONE, tables.BASE_DRAKE}, // (this is actually mission 2a)
			// TODO: Deal with 59 also being used for the derelict
			{tables.FLAG_RF_GO_TO_GAEA_DONE, 59}, // Gaea
		}
		for _, i := range infos {
			if a.Has_flags(i.flag) {
				a.Visited[i.location] = true
			}
		}

		// TODO: if imported, deduce visited based on secret compartment, unlocked jump points, and drone kill count
	}

	// update secret compartment status
	if a.Forms[types.OFFSET_REAL].Get("FITE", "CRGO", "CRGI").Data[6] != 0 {
		*a.Secrets = *a.Secrets | (1 << a.Blobs[types.OFFSET_SHIP][0])
	}
}

func (a *Arg) Ship() uint8 {
	return a.Blobs[types.OFFSET_SHIP][0]
}

func (a *Arg) Location() tables.BASE_ID {
	return tables.BASE_ID(a.Blobs[types.OFFSET_SHIP][2])
}

func (a *Arg) Plot_info() (string, byte) {
	plot := a.Blobs[types.OFFSET_PLOT]
	str, _, err := readers.Read_string(bytes.NewReader(plot))
	if err != nil {
		// *very* corrupt savefile
		panic(err)
	}
	flag := plot[9]

	return str, flag
}

func (a *Arg) Missions() int {
	return int16_from_bytes(a.Blobs[types.OFFSET_MISSIONS])
}

func (a *Arg) Has_flags(flags ...int) bool {
	data := a.Blobs[types.OFFSET_WTF][11:]
	for _, flag := range flags {
		if data[flag] == 0 {
			return false
		}
	}
	return true
}

func (a *Arg) kills(faction int) int {
	return int16_from_bytes(a.Forms[types.OFFSET_PLAY].Get("KILL").Data[2*faction:])
}

type Achievement struct {
	Id    string
	Name  string
	Expl  string
	Multi bool
	Test  func(a *Arg) bool
}

// achievement helper functions

func int_le_from_bytes(data []byte) int {
	i, _ := readers.Read_int_le(bytes.NewReader(data))
	return i
}

func int16_from_bytes(data []byte) int {
	i, _ := readers.Read_int16(bytes.NewReader(data))
	return i
}

func is_completed_status(flag uint8) bool {
	return flag&1 != 0
}

func find_all_places(bt tables.BASE_TYPE) []tables.BASE_ID {
	out := []tables.BASE_ID{}
	for id, info := range tables.Locations(types.GT_PRIV) {
		if info.Type == bt {
			out = append(out, id)
		}
	}
	return out
}

// mcs = Make Cheev Struct

// mcs_kill makes a "kill a bunch of people" achievement
func mcs_kill(id string, name string, number int, faction int) Achievement {
	return Achievement{
		id,
		name,
		fmt.Sprintf("Kill %v %v", number, tables.Factions[faction]),
		false,
		func(a *Arg) bool {
			return a.kills(faction) >= number
		},
	}
}

// mcs_complete_series makes a "Finish a mission series" achievement
func mcs_complete_series(id string, name string, expl string, number uint8) Achievement {
	return Achievement{
		id,
		name,
		expl,
		false,
		func(a *Arg) bool {
			if a.Savedata.Game() == types.GT_RF {
				// RF plot info tells us nothing about what (if anything) was done in Privateer

				// TODO: however, deductions can and should be made from secret compartment, unlocked jump points, and drone kill
				return false
			}
			str, flag := a.Plot_info()

			// Possibility 1: already on later missions
			if len(str) == 4 && str[0] == 's' && str[1] > '0'+number {
				return true
			}
			// Possibility 2: last mission in "complete" status
			if str == fmt.Sprintf("s%vmd", number) && is_completed_status(flag) {
				return true
			}

			return false
		},
	}
}

func full_location(gt types.Game, id tables.BASE_ID) string {
	loc := tables.Locations(gt)[id]
	return loc.Name + " (" + tables.Systems(gt)[loc.System].Name + ")"
}

// mcs_go_places makes a "visit places" achievement
func mcs_go_places(id string, name string, expl string, locations []tables.BASE_ID) Achievement {
	return Achievement{
		id,
		name,
		expl,
		true,
		func(a *Arg) bool {
			count := 0
			missed := ""
			for _, l := range locations {
				if a.Visited[l] {
					count += 1
				} else {
					missed = full_location(a.Savedata.Game(), l)
				}
			}

			a.Progress = fmt.Sprintf("%v/%v (visit %v)", count, len(locations), missed)

			return count == len(locations)
		},
	}
}

func is_all_zero(bs []byte) bool {
	for _, b := range bs {
		if b != 0 {
			return false
		}
	}
	return true
}

// Here is the list of achievements.
// Achievement id-s must remain unchanged FOREVER, even if they contain the worst possible typos,
// as they are stored in state files, and we don't want to have a situation where upgrading
// priv_ach will randomise what achievements people have.
//
// Because IDs are the one thing that can't be fixed after the fact, here are some guidelines:
//
// Start with "AID" and use caps and underscores
//
// Don't be too specific.  For example, use "AID_KILL_LOTS_OF_PIRATES"; not "AID_KILL_100_PIRATES",
// ...because we might change our minds over how many pirate kills is a reasonable number for a cheev
// (especially if "we" is a speedrunner who can't remember how to play the game normally)
//
// Check for typos before pushing!
var Cheev_list = []struct {
	Category string
	Cheeves  []Achievement
}{
	{"Tarsus Grind", []Achievement{ //Because not everybody gets their Centurion at the 3-minute mark :D

		{"AID_AFTERBURNER", "I am speed", "Equip an afterburner", false, func(a *Arg) bool {
			return a.Forms[types.OFFSET_REAL].Get("FITE", "AFTB") != nil
		}},

		{"AID_OPTIMISM", "Optimism", "Have Merchant's guild membership but no jump drive", false, func(a *Arg) bool {
			return a.Blobs[types.OFFSET_SHIP][6] != 0 && a.Forms[types.OFFSET_REAL].Get("FITE", "JDRV", "INFO") == nil
		}},

		{"AID_NOOBSHIELDS", "Shields to maximum!", "Equip level 2 shields", false, func(a *Arg) bool {
			shields := a.Forms[types.OFFSET_REAL].Get("FITE", "SHLD", "INFO")
			if shields == nil {
				return false
			}
			return shields.Data[8] == tables.SHIELD_BASE_0+2
		}},

		{"AID_KILL1", "It gets easier", "Kill another person, forever destroying everything they are or could be", false, func(a *Arg) bool {
			return !is_all_zero(a.Forms[types.OFFSET_PLAY].Get("KILL").Data)
		}},

		{"AID_2LAUNCHERS", "\"I am become death, destroyer of Talons\"", "Have 2 missile launchers", false, func(a *Arg) bool {
			launchers := a.Forms[types.OFFSET_REAL].Get("FITE", "WEAP", "LNCH")
			count := 0
			if launchers != nil {
				for i := 0; i < len(launchers.Data); i += 4 {
					if launchers.Data[i] == 50 {
						count += 1
					}
				}
			}
			return count == 2
		}},

		{"AID_TACHYON", "Now witness the firepower", "Equip a Tachyon Cannon", false, func(a *Arg) bool {
			guns := a.Forms[types.OFFSET_REAL].Get("FITE", "WEAP", "GUNS")
			if guns == nil {
				return false
			}
			for n := 0; n < len(guns.Data); n += 4 {
				if guns.Data[n] == 7 {
					return true
				}
			}

			return false
		}},

		{"AID_REPAIRBOT", "They fix Everything", "Have a repair-bot", false, func(a *Arg) bool {
			return a.Forms[types.OFFSET_REAL].Get("FITE", "REPR") != nil
		}},

		{"AID_COLOUR_SCANNER", "\"Red\" rhymes with \"Dead\"", "Equip a colour scanner", false, func(a *Arg) bool {
			scanner := a.Forms[types.OFFSET_REAL].Get("FITE", "TRGT", "INFO")
			return (scanner != nil) && (scanner.Data[len("TARGETNG")]-60 > 2)
		}},

		{"AID_SCANNER_DAMAGE", "Crackle crackle", "Forget to repair your scanner", false, func(a *Arg) bool {
			armour := a.Forms[types.OFFSET_REAL].Get("FITE", "SHLD", "ARMR")
			scanner := a.Forms[types.OFFSET_REAL].Get("FITE", "TRGT", "DAMG")
			//fmt.Println("Repairbot", forms[types.OFFSET_REAL].Get("FITE", "REPR") != nil)

			// Scanner damage, no armour damage, no repair bot.  This is a very easy mistake to make due to scanner repair being
			// only available in the "Software" store.
			return a.Forms[types.OFFSET_REAL].Get("FITE", "REPR") == nil &&
				(scanner != nil) && !is_all_zero(scanner.Data) &&
				slices.Equal(armour.Data[:8], armour.Data[8:])
		}},

		{"AID_INTERSTELLAR", "Interstellar Rubicon", "Leave the Troy system", false, func(a *Arg) bool {
			return slices.Index(tables.Systems(a.Savedata.Game())[tables.SYS_TROY].Bases, a.Location()) == -1
		}},
	}},

	{"Plot", []Achievement{

		{"AID_SANDOVAL", "Cargo parasite", "Start the plot", false, func(a *Arg) bool {
			cargo := a.Forms[types.OFFSET_REAL].Get("FITE", "CRGO", "DATA")
			for n := 0; n < len(cargo.Data); n += 4 {
				if cargo.Data[n] == 42 { // Alien Artifact(s)
					return true
				}
			}

			return false
		}},

		mcs_complete_series("AID_TAYLA", "I'm not a pirate, I just work for them", "Complete Tayla's missions", 1),

		{"AID_LYNCH", "Can't you see that I am a privateer?", "Complete Roman Lynch's Missions", false, func(a *Arg) bool {
			if  a.Savedata.Game() == types.GT_RF {
				// RF plot info tells us nothing about what (if anything) was done in Privateer
				return false
			}

			// Note: The final "Get ambushed by Miggs" mission can't have completed status.
			// We're lying in the description to avoid spoiling a 30-year-old game.
			str, flag := a.Plot_info()

			// Possibility 1: already on later missions
			if len(str) == 4 && str[0] == 's' && str[1] > '2' {
				return true
			}

			// Possibility 2: last mission in "failed" status
			// Observed values: 128 for accepted
			// 161 if we launch and immediately land   128 + 32+1
			// 161 if we went directly to oxford
			// 191 if we went to newcastle             128 + 32+16+8+4+2+1
			if str == "s2md" && (flag == 161 || flag == 162 || flag == 191 || flag == 226) {
				return true
			}

			return false
		}},

		mcs_complete_series("AID_OXFORD", "Unlocking the greatest mysteries", "Complete Masterson's missions", 3),
		mcs_complete_series("AID_PALAN", "I travel the galaxy", "Complete the Palan missions", 4),
		mcs_complete_series("AID_RYGANNON", "...and far beyond", "Complete Taryn Cross's missions", 5),

		{"AID_STELTEK_GUN", "Strategically Transfer Equipment to Alternative Location", "Acquire the Steltek gun", false, func(a *Arg) bool {
			if a.Savedata.Game() == types.GT_RF {
				// Gun type 8 is re-used in RF for the fusion cannon
				return false
			}

			guns := a.Forms[types.OFFSET_REAL].Get("FITE", "WEAP", "GUNS")
			if guns != nil { //Newly-bought ships have no GUNS record
				for n := 0; n < len(guns.Data); n += 4 {
					if guns.Data[n] >= 8 { //8==steltek gun, 9==super steltek gun.
						return true
					}
				}
			}
			return false
		}},

		{"AID_WON", "That'll be 30000 credits", "Win the game (and get paid for it)", false, func(a *Arg) bool {
			str, flag := a.Plot_info()

			// This flag is 191 regardless of whether we've returned to the Admiral and heard his "well done" speech.
			return str == "s7mb" && is_completed_status(flag)
		}},
	}},

	{"Ships", []Achievement{ // The idea here is one achievement per ship which exemplifies what that ship is for.

		{"AID_CENTURION", "Pew Pew Pew", "Mount 4 front guns and 20 warheads (on a Centurion)", false, func(a *Arg) bool {
			count := 0
			guns := a.Forms[types.OFFSET_REAL].Get("FITE", "WEAP", "GUNS")
			if guns != nil {
				for n := 1; n < len(guns.Data); n += 4 {
					if guns.Data[n] >= 1 && guns.Data[n] <= 4 {
						count += 1
					}
				}
			}
			if count < 4 {
				return false
			}

			warheads := a.Forms[types.OFFSET_REAL].Get("FITE", "WEAP", "MISL")
			count = 0
			for n := 1; n < len(warheads.Data); n += 3 {
				count += int(warheads.Data[n])
			}

			return count == 20
		}},

		{"AID_GALAXY", "Star Truck", "Carry more than 200T of cargo in a Galaxy", false, func(a *Arg) bool {
			// This check is necessary, because of cargo missions and also because it's possible to exchange ships when you shouldn't be able to thanks to
			// (I guess) 8-bit wrap around in stored cargo.
			if a.Ship() != tables.SHIP_GALAXY {
				return false
			}

			total := 0
			cargo := a.Forms[types.OFFSET_REAL].Get("FITE", "CRGO", "DATA")
			for n := 0; n < len(cargo.Data); n += 4 {
				total += int16_from_bytes(cargo.Data[n+1 : n+3])
			}

			return total > 200
		}},

		{"AID_ORION", "Expensive Paperweight", "Have Level 5 engines and level 5 shields (on an Orion)", false, func(a *Arg) bool {
			if a.Ship() != tables.SHIP_ORION {
				// It's possible in RF for a Galaxy to have level 5 shields and engines.
				// TODO: we need a test case for this one
				return false
			}
			if !slices.Equal(a.Forms[types.OFFSET_REAL].Get("FITE", "ENER", "INFO").Data, []byte{'E', 'N', 'E', 'R', 'G', 'Y', 0, 0, 1, 2, 2, 2, 3, 1, 4, 1, 5, 1, 6, 2}) {
				return false
			}

			shields := a.Forms[types.OFFSET_REAL].Get("FITE", "SHLD", "INFO")
			if shields == nil {
				return false
			}
			return shields.Data[8] == tables.SHIELD_BASE_0+5
		}},

		{"AID_TARSUS", "Tarsus gonna Tarsus", "Take damage to all four armour facings on a Tarsus", false, func(a *Arg) bool {
			if a.Ship() != tables.SHIP_TARSUS {
				return false
			}

			armour := a.Forms[types.OFFSET_REAL].Get("FITE", "SHLD", "ARMR")
			if armour == nil {
				return false
			}

			var armours [8]int
			for i := range armours {
				armours[i] = int16_from_bytes(armour.Data[2*i : 2*i+2])
			}
			for i := 0; i < 4; i += 1 {
				// Empty armour slot can show up as 0 or 1
				if armours[i] == 1 || armours[i] == armours[i+4] {
					return false
				}
			}
			return true
		}},
	}},

	{"Random", []Achievement{
		{"AID_DUPER", "I know what you did", "Equip multiple tractor beams in front mounts", false, func(a *Arg) bool {
			// It is difficult to imagine a reason (other than the cargo duping exploit) to have multiple front tractor beams
			// OTOH, a Galaxy with a tractor in each turret isn't particularly suspicious and shouldn't get this cheev.
			launchers := a.Forms[types.OFFSET_REAL].Get("FITE", "WEAP", "LNCH")
			count := 0
			if launchers != nil {
				for i := 0; i < len(launchers.Data); i += 4 {
					if launchers.Data[i] == 52 && launchers.Data[i+1] < 6 {
						count += 1
					}
				}
			}
			return count > 1
		}},

		{"AID_PORNO", "I trade it for the articles", "Carry at least one ton of PlayThing(tm)", false, func(a *Arg) bool {
			cargo := a.Forms[types.OFFSET_REAL].Get("FITE", "CRGO", "DATA")
			for n := 0; n < len(cargo.Data); n += 4 {
				if cargo.Data[n] == 27 {
					return true
				}
			}

			return false
		}},

		{"AID_BAD_FRIENDLY", "Questionable morality", "Become friendly with Pirates and Kilrathi", false, func(a *Arg) bool {
			rep := a.Forms[types.OFFSET_PLAY].Get("SCOR")
			for _, f := range []int{tables.FACTION_PIRATES, tables.FACTION_KILRATHI} {
				if int16_from_bytes(rep.Data[2*f:2*f+2]) <= 25 {
					return false
				}

			}
			return true
		}},

		{"AID_SUPERFRIENDLY", "Insane morality", "Become friendly with everyone except retros", false, func(a *Arg) bool {
			rep := a.Forms[types.OFFSET_PLAY].Get("SCOR")
			for _, f := range []int{tables.FACTION_MERCHANTS, tables.FACTION_HUNTERS, tables.FACTION_CONFEDS, tables.FACTION_KILRATHI, tables.FACTION_MILITIA, tables.FACTION_PIRATES} {
				if int16_from_bytes(rep.Data[2*f:2*f+2]) <= 25 {
					return false
				}
			}

			return true
		}},

		{"AID_RICH", "Dr. Evil Pinky Finger", "Possess One Million Credits", false, func(a *Arg) bool {
			return int_le_from_bytes(a.Forms[types.OFFSET_REAL].Get("FITE", "CRGO", "CRGI").Data[0:4]) >= 1000000
		}},

		{"AID_CARGO_IS_NIGGER", "Just glue it to the outside", "Carry more cargo than will fit in your ship", false, func(a *Arg) bool {
			// Assuming the player isn't just cheating, this is possible because cargo-delivery missions don't bother to check cargo capacity when you accept them.
			// This could be a bug, but maybe it's a convenience feature?

			info := a.Forms[types.OFFSET_REAL].Get("FITE", "CRGO", "CRGI")
			capacity := int(info.Data[4])
			if info.Data[6] != 0 {
				capacity += 20 //secret compartment
			}

			stored := 0
			cargo := a.Forms[types.OFFSET_REAL].Get("FITE", "CRGO", "DATA")
			for n := 0; n < len(cargo.Data); n += 4 {
				stored += int16_from_bytes(cargo.Data[n+1 : n+3])
			}

			//fmt.Println("Stored:", stored, "Capacity:", capacity)
			return stored > capacity
		}},

		{"AID_KILL_DRONE", "No kill stealing", "Personally kill the Steltek Drone", false, func(a *Arg) bool {
			// Although only the improved Steltek gun can knock down the drone's shields,
			// anything can damage the soft and squishy egg inside.
			return a.kills(tables.FACTION_DRONE) > 0
		}},

		{"AID_WIN_KILL_NO_KILRATHI", "Cat Lover", "Win the game without killing any Kilrathi", false, func(a *Arg) bool {
			if a.kills(tables.FACTION_KILRATHI) > 0 {
				return false
			}

			str, flag := a.Plot_info()
			return str == "s7mb" && is_completed_status(flag)
		}},

		{"AID_WIN_KILL_NO_GOOD", "Good Guy", "Win the game without killing any Militia, Merchants or Confeds", false, func(a *Arg) bool {
			for _, faction := range []int{tables.FACTION_MILITIA, tables.FACTION_MERCHANTS, tables.FACTION_CONFEDS} {
				if a.kills(faction) > 0 {
					return false
				}
			}

			str, flag := a.Plot_info()
			return str == "s7mb" && is_completed_status(flag)
		}},

		{"AID_3_DELIVERIES", "Tagon would be proud", "Accept three delivery missions to the same location", false, func(a *Arg) bool {
			// Captain Tagon - from the Schlock Mercenary webcomic - loved to get paid twice(or more) for essentally the same task.
			if a.Missions() != 3 {
				return false
			}

			destinations := map[uint8]bool{}
			for i := 1; i < 6; i += 2 {
				cargo := a.Forms[types.OFFSET_MISSION_BASE+i].Get("CARG")
				if cargo == nil {
					// not a cargo mission
					return false
				}

				destinations[cargo.Data[0]] = true
			}

			return len(destinations) == 1
		}},

		{"AID_FAIL_ESCORT", "Wing Commander nostalgia", "Fail a Drayman escort mission", false, func(a *Arg) bool {
			// There are 3 such missions - Oxford 1, 3 and 4.
			// RF adds one more.  Even though RF isn't (except when it is) an "unwinnable state" kind of game, teh failure
			// falag can be set before it is cleared by talking ot MAsterson again
			str, flag := a.Plot_info()

			switch str {
			case "s3ma", "s3mc", "s3md", "s11md":
				return (flag == 226 || flag == 162)
			}
			return false
		}},

		{"AID_BITCORES_MAN", "The Bitcores maneuver", "Put the Steltek gun on a central mount", false, func(a *Arg) bool {
			// To pull this one off, you have to remove a central gun at Rygannon then get to the derelict on just 3 guns.
			if a.Savedata.Game() == types.GT_RF {
				// Gun type 8 is re-used in RF for the fusion cannon
				return false
			}

			if a.Blobs[types.OFFSET_SHIP][0] != tables.SHIP_CENTURION {
				return false
			}
			guns := a.Forms[types.OFFSET_REAL].Get("FITE", "WEAP", "GUNS")
			if guns == nil {
				return false
			}
			for n := 0; n < len(guns.Data); n += 4 {
				if guns.Data[n] >= 8 && (guns.Data[n+1] == 2 || guns.Data[n+1] == 3) {
					return true
				}
			}

			return false
		}},

		{"AID_DO_MISSIONS", "Space-Hobo", "Do 100 non-plot missions", false, func(a *Arg) bool {
			// There is only 1 mission counter, which does not seem to include plot missions
			return int16_from_bytes(a.Blobs[types.OFFSET_SHIP][3:5]) >= 100
		}},

		mcs_go_places("AID_PIRATE_BASES", "Press C to spill secrets", "Visit all pirate bases", find_all_places(tables.BT_PIRATE)),
		// The alleged joke here is that the object of "pick up" could be women or STDs.
		mcs_go_places("AID_PLEASURE_BASES", "Pick up more than cargo", "Visit all pleasure planets", find_all_places(tables.BT_PLEASURE)),

		{"AID_TRANSFER_SECRET", "How does that work?", "Transfer your secret compartment to a new ship", true, func(a *Arg) bool {
			// This is an unfortunate case where json unmarshaling and default values fail us.
			// nil maps and arrays behave like unmodifiable empty containers, but nil pointers behave like landmines.
			// ...and we can get uninitialised data when called in progress mode.
			if a.Secrets == nil {
				return false
			}
			return ((*a.Secrets - 1) & *a.Secrets) != 0
		}},

		{"AID_NO_ARMOR_LAUNCH", "I feel the same breeze you do", "Survive a trip with no armour", false, func(a *Arg) bool {
			// Suggested by Grok - yes, really.
			// It is a little bit surprising that this achievement is even possible to implement.
			// No armour is represented as 0 when you first buy a new ship or sell existing armour, but after
			// launch-landing, it changes to 1.
			armour := a.Forms[types.OFFSET_REAL].Get("FITE", "SHLD", "ARMR")
			return int16_from_bytes(armour.Data) == 1
		}},
		// TODO: would be fun but needs multi-file checkingjj
		// "The Militia would be proud", "Kill the Black Rhombus without killing any of its escorts"
	}},

	{"Mostly Peaceful", []Achievement{
		mcs_kill("AID_KILL_RETROS", "Defender of toasters", 20, tables.FACTION_RETROS),
		mcs_kill("AID_KILL_PIRATES", "We are not the same", 20, tables.FACTION_PIRATES),
		mcs_kill("AID_KILL_HUNTERS", "Avril Lavigne mode", 30, tables.FACTION_HUNTERS),
		mcs_kill("AID_KILL_KILRATHI", "Also Try Wing Commander", 10, tables.FACTION_KILRATHI),
		mcs_kill("AID_KILL_MILITIA", "Criminal", 6, tables.FACTION_MILITIA),
		mcs_kill("AID_KILL_CONFEDS", "Traitor", 6, tables.FACTION_CONFEDS),
	}},

	{"Mass-murder?  I hardly...", []Achievement{
		mcs_kill("AID_KILL_MANY_RETROS", "Guardian angel of toasters", 100, tables.FACTION_RETROS),
		mcs_kill("AID_KILL_MANY_PIRATES", "Your Letter of Marque is in the post", 100, tables.FACTION_PIRATES),
		mcs_kill("AID_KILL_MANY_HUNTERS", "Joan Jett mode", 100, tables.FACTION_HUNTERS),
		mcs_kill("AID_KILL_MANY_KILRATHI", "Also Try Wing Commander 3", 50, tables.FACTION_KILRATHI),
		mcs_kill("AID_KILL_MANY_MILITIA", "Menesch's apprentice", 30, tables.FACTION_MILITIA),
		mcs_kill("AID_KILL_MANY_CONFEDS", "Arch-traitor", 30, tables.FACTION_CONFEDS),
	}},

	{"Feats of Insanity", []Achievement{
		{"AID_TARSUS_DERELICT", "Get that trophy screenshot", "Get to the derelict in a Tarsus", false, func(a *Arg) bool {
			if a.Savedata.Game() == types.GT_RF {
				// Not possible in RF (and we don't want false positives caused by a fusion cannon)
				return false
			}

			// I've done this.  It was painful.
			// The Centurions at Palan can be handled by kiting them into the asteroid field.
			// Cross 3 method: clear nav 1 (asteroids will help you here), then hit nav 4, wipe out the Gothri there, again taking full advantage of the asteroids.
			// Run from the Kamekh, auto to nav 3, then to nav 2, kill 2 out of 3 Dralthi then burn back to nav 1.
			if a.Ship() != tables.SHIP_TARSUS {
				return false
			}

			/// actually at the derelict
			if a.Location() == tables.BASE_DERELICT_BASE {
				return true
			}

			// Anyone with the steltek gun must have been there
			guns := a.Forms[types.OFFSET_REAL].Get("FITE", "WEAP", "GUNS")
			if guns != nil {
				for n := 0; n < len(guns.Data); n += 4 {
					if guns.Data[n] >= 8 { //8==steltek gun, 9==super steltek gun.
						return true
					}
				}
			}
			return false
		}},

		{"AID_VERY_RICH", "Almost ready to start Righteous Fire", "Possess twenty million credits", false, func(a *Arg) bool {
			return int_le_from_bytes(a.Forms[types.OFFSET_REAL].Get("FITE", "CRGO", "CRGI").Data) >= 20000000
		}},

		{"AID_FIX_HUNTER_REP", "Grinder", "Recover hunter reputation to non-hostile before winning", false, func(a *Arg) bool {
			// Hunters are notoriously hard to please.  The problem is that you have to kill a lot of them to win the game,
			// losing 15 rep per Demon and 20 rep per Centurion - but nothing (except the drone) will improve hunter rep by more than 1.
			// (That's right, killing a pirate talon impresses them exactly as much as killing a Kamekh)
			if a.Savedata.Game() == types.GT_RF {
				// This one doesn't make any sense in RF
				return false
			}

			str, flag := a.Plot_info()

			if len(str) != 4 {
				// Either too early or too late
				return false
			}

			// Not onto Murphy
			if str[0] == 's' && str[1] < '4' {
				return false
			}

			// Not onto Murphy 3 (we need to get past the 3 "Kill hunters" missions to have the bad rep)
			if str[0] == 's' && str[1] == '4' && str[3] < 'c' {
				return false
			}

			// Murphy 3 not yet complete
			// Some notes:
			// The "correct" way to do this is to kill all the hunters, resulting in flag=191
			// The alternative is to just afterburn past them and land at Palan, resulting in flag=162.
			// The game is still winnable; just pick up Dr. Monkhouse and continue.
			// But if you fail the mission then go back and talk to Murphy without doing Monkhouse first she sets the plot to "failed"
			if str[0] == 's' && str[1] == '4' && str[3] == 'c' {
				if !(flag == 162 || flag == 191) {
					return false
				}
			}
			// If we get this far, the player has done (possibly unsuccessfully) the first 3 Palan missions and is not in an unwinnable state

			if flag == 226 {
				// failed!
				return false
			}

			if str == "s7mb" && is_completed_status(flag) {
				// won the game!
				return false
			}

			return int16_from_bytes(a.Forms[types.OFFSET_PLAY].Get("SCOR").Data[2*tables.FACTION_HUNTERS:]) >= -25
		}},

		{"AID_CARGO_IS_TWICE_BIGGER", "How much glue do you have?", "Carry more than twice as much cargo as will fit in your ship", false, func(a *Arg) bool {
			// Probably the easiest way to do this is to get a centurion without a secret compartment, buy 50T of whatever,
			// then accept 4 cargo missions.  That's how I did it, anyway.  Some savescumming required.
			info := a.Forms[types.OFFSET_REAL].Get("FITE", "CRGO", "CRGI")
			capacity := int(info.Data[4])
			if info.Data[6] != 0 {
				capacity += 20 //secret compartment
			}

			stored := 0
			cargo := a.Forms[types.OFFSET_REAL].Get("FITE", "CRGO", "DATA")
			for n := 0; n < len(cargo.Data); n += 4 {
				stored += int16_from_bytes(cargo.Data[n+1:])
			}

			//fmt.Println("Stored:", stored, "Capacity:", capacity)
			return stored > capacity*2
		}},

		{"AID_INSANE_FRIENDLY", "No-one, you see, is smarter than he", "Become friendly with every real faction", false, func(a *Arg) bool {
			// The problem here is that retros start out hostile, and it is not possible to improve retro rep by any means... other than getting it below -32768,
			// causing 16-bit wraparound, flipping them to maximally friendly!  This will require about 6000 retro kills.
			// Cheev name is a reference to "Flipper", which is sort of a hint as to the only way to do this.
			rep := a.Forms[types.OFFSET_PLAY].Get("SCOR")
			for _, f := range []int{tables.FACTION_MERCHANTS, tables.FACTION_HUNTERS, tables.FACTION_CONFEDS, tables.FACTION_KILRATHI,
				tables.FACTION_MILITIA, tables.FACTION_PIRATES, tables.FACTION_RETROS} {
				if int16_from_bytes(rep.Data[2*f:]) <= 25 {
					return false
				}
			}

			return true
		}},

		{"AID_TROY_TRADE", "Optimism Rewarded", "Accept a cargo mission between two bases in the Troy system", false, func(a *Arg) bool {
			troy_bases := tables.Systems(a.Savedata.Game())[tables.SYS_TROY].Bases

			if slices.Index(troy_bases, a.Location()) == -1 {
				// Not in troy
				return false
			}

			for m := 0; m < a.Missions(); m += 1 {
				cargo := a.Forms[types.OFFSET_MISSION_BASE+2*m+1].Get("CARG")
				if cargo == nil {
					// not a cargo mission
					continue
				}

				if slices.Index(troy_bases, tables.BASE_ID(cargo.Data[0])) != -1 {
					return true
				}
			}

			// No missions to Troy
			return false
		}},
	}},
}

var Cheev_list_rf = map[string][]Achievement{
	"Plot": []Achievement{
		// RF's use of flags makes these slightly easier than the Privateer plot achievements
		// flags don't always get set on completion of the last mission; instead, they get set when a new mission sequence would cause the game to forget the status of the old one.
		// So we generally have to check the flag and check specifically for (last mission && completed state)
		{"AID_RF_TAYLA", "For medicinal use only", "Complete Tayla's missions  (RF)", false, func(a *Arg) bool {
			mission, flag := a.Plot_info()
			return a.Has_flags(tables.FLAG_RF_TAYLA_4_DONE) || (mission == "s8md" && is_completed_status(flag))
		}},
		{"AID_RF_MURPHY", "Corporate lackey", "Complete Lynn Murphy's missions (RF)", false, func(a *Arg) bool {
			mission, flag := a.Plot_info()
			return a.Has_flags(tables.FLAG_RF_MURPHY_4_DONE) || (mission == "s9md" && is_completed_status(flag))
		}},
		{"AID_RF_GOODIN", "Kamekhs and Kamikazes", "Complete Sandra Goodin's missions (RF)", false, func(a *Arg) bool {
			mission, flag := a.Plot_info()
			return a.Has_flags(tables.FLAG_RF_GOODIN_4_DONE) || (mission == "s10md" && is_completed_status(flag))
		}},
		{"AID_RF_MASTERSON", "Not Brogues", "Complete Masterson's missions (RF)", false, func(a *Arg) bool {
			return a.Has_flags(tables.FLAG_RF_MASTERSON_5_DONE)
		}},
		{"AID_RF_MONTE", "The full Monte", "Complete the sociologist's missions (RF)", false, func(a *Arg) bool {
			// There's no flag for this!
			status, flag := a.Plot_info()
			if status == "" {
				return false // plot not started
			}

			m := strings.Index(status, "m")
			series, err := strconv.Atoi(status[1:m])
			if err != nil {
				// This simply should not happen
				return false
			}
			mission := status[m+1 : len(status)]

			return series > 12 || (series == 12 && mission == "d" && flag == 191)
		}},
		{"AID_RF_GOODIN_5", "Kahl and Collusion", "Complete Sandra Goodin's final mission (RF)", false, func(a *Arg) bool {
			return a.Has_flags(tables.FLAG_RF_GOODIN_5_DONE)
		}},
		{"AID_RF_TERRELL", "Patrol Mission of the Apocalypse", "Complete Admiral Terrell's mission (RF)", false, func(a *Arg) bool {
			return a.Has_flags(tables.FLAG_RF_TERRELL_DONE)
		}},
		{"AID_RF_WIN", "God Emperor of toasters", "Kill Mordecai Jones (RF)", false, func(a *Arg) bool {
			mission, flag := a.Plot_info()
			return a.Has_flags(tables.FLAG_RF_KILL_JONES_DONE) || (mission=="s14mb" && is_completed_status(flag))
		}},
	},
	"Random": []Achievement{
		{"AID_RF_IMPORT_WINNER", "The adventure continues", "Import a privateer savefile that killed the drone (RF)", false, func(a *Arg) bool {
			return a.kills(tables.FACTION_DRONE) > 0
		}},
		{"AID_RF_ALL_STARTERS", "Overqualified", "Do all 3 Murphy/Tayla/Goodin mission sets (RF)", false, func(a *Arg) bool {
			mission, flag := a.Plot_info()
			return (a.Has_flags(tables.FLAG_RF_TAYLA_4_DONE) || (mission == "s8md" && is_completed_status(flag))) &&
				(a.Has_flags(tables.FLAG_RF_MURPHY_4_DONE) || (mission == "s9md" && is_completed_status(flag))) &&
				(a.Has_flags(tables.FLAG_RF_GOODIN_4_DONE) || (mission == "s10md" && is_completed_status(flag)))

		}},
		{"AID_RF_PAID_3_TIMES", "Tagon would be proud, again", "Collect all 3 rewards for killing Menesch (RF)", false, func(a *Arg) bool {
			// There is a problem here.
			// "Free reset unavailable"  could be because the reward has already been taken, or it could be because
			// it was never offered (if the player kills Menesch before even talking to Lynch).  We sort of justify this by
			// saying that in this case, the third reward is: nothing.
			return a.Has_flags(tables.FLAG_RF_MURPHY_BOUNTY_PAID, tables.FLAG_RF_GOODIN_5_OFFERED, tables.FLAG_RF_ROMAN_LYNCH_FREE_RESET_UNAVAILABLE)
		}},
		{"AID_RF_SECRET_NAV", "Not so secret", "Accept a non-plot mission involving Nav 4 in Valhalla (RF)", false, func(a *Arg) bool {
			// That's the jump to Gaea, which is supposed to be secret, but nobody told mission generation that.

			missions := a.Missions()
			for m := 0; m < missions; m += 1 {
				objectives := a.Forms[types.OFFSET_MISSION_BASE+2*m+1].Get("SCRP", "PROG")
				if objectives == nil {
					continue
				}

				// MSSN-SCRP-PROG data appears to be a sequence of 4-byte chunks.
				// I don't have much understanding of what this means, but I think we can recognize "go to navpoint" chunks.
				for i := 0; i < len(objectives.Data); i += 4 {
					if tables.SYS_ID(objectives.Data[i+1]) == tables.SYS_VALHALLA && objectives.Data[i+3] == 4 {
						return true
					}
				}
			}
			return false
		}},
	},
	"Feats of Insanity": []Achievement{
		{"AID_RF_PIMPED_TARSUS", "Lipstick on a Pig", "Equip all 6 new technologies on a Tarsus (RF)", false, func(a *Arg) bool {
			if a.Ship() != tables.SHIP_TARSUS {
				return false
			}

			// Easy ones first: these techs are simply present or not.
			for _, what := range []string{"COOL", "SHBO", "SPEE", "THRU"} {
				if a.Forms[types.OFFSET_REAL].Get("FITE", what) == nil {
					return false
				}
			}

			// Isometal armour
			armour := a.Forms[types.OFFSET_REAL].Get("FITE", "SHLD", "ARMR")
			if int16_from_bytes(armour.Data) != 3000 {
				return false
			}

			// Fusion cannons - one is enough
			guns := a.Forms[types.OFFSET_REAL].Get("FITE", "WEAP", "GUNS")
			if guns != nil { //Newly-bought ships have no GUNS record
				for n := 0; n < len(guns.Data); n += 4 {
					if guns.Data[n] == 8 {
						return true
					}
				}
			}
			return false
		}},
		{"AID_RF_GOODGUY_WIN", "Saint Grayson of Gemini", "Win without having killed any Confeds, Militia, Hunters or Merchants. (RF)", false, func(a *Arg) bool {
			// Have fun with this one.
			// Kills from an imported Privateer save do count (mostly because there's no way to exclude them) so you might as well start a new game.
			// Backdooring Murphy 1 and Monte 4 is definitely recommended, as is a speed enhancer
			// This is only possible because the kill count considers Menesch to be a pirate (LOL), and Jones a retro (reasonable).
			for _, faction := range []int{tables.FACTION_MILITIA, tables.FACTION_MERCHANTS, tables.FACTION_CONFEDS, tables.FACTION_HUNTERS} {
				if a.kills(faction) > 0 {
					return false
				}
			}

			mission, flag := a.Plot_info()
			return a.Has_flags(tables.FLAG_RF_KILL_JONES_DONE) || (mission=="s14mb" && is_completed_status(flag))
		}},
	},
}
