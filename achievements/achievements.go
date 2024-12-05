package achievements

import "fmt"
import "slices"

import "privdump/tables"
import "privdump/types"
import "privdump/readers"

type Achievement struct {
	Id   string
	Name string
	Expl string
	Test func(types.Header, []byte, map[int]*types.Form) bool
}

// achievment helper functions

//mcs_kill makes a "kill a bunch of peoplke" achievement
func mcs_kill(id string, name string, number int, who int) Achievement {
	return Achievement{
		id,
		name,
		fmt.Sprintf("Kill %v %v", number, tables.Factions[who]),
		func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			cur := 2 * who
			return readers.Read_int16(forms[types.OFFSET_PLAY].Get("KILL").Data, &cur) >= number
		},
	}
}

//mcs_kill makes a "Finish a mission series" achievement
func mcs_complete_series(id string, name string, expl string, number uint8) Achievement {
	return Achievement{
		id,
		name,
		expl,
		func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			cur := h.Offsets[types.OFFSET_PLOT]
			str, _, _ := readers.Read_string(bs, &cur)
			flag := bs[h.Offsets[types.OFFSET_PLOT]+9]

			// Possibility 1: already on later missions
			if len(str) == 4 && str[0] == 's' && str[1] > '0'+number {
				return true
			}
			// Possibility 2: last mission in "complete" status
			if str == fmt.Sprintf("s%vmd", number) && (flag == 191 || flag == 255) {
				return true
			}

			return false
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

		{"AID_AFTERBURNER", "I am speed", "Equip an afterburner", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			return forms[types.OFFSET_REAL].Get("FITE", "AFTB") != nil
		}},

		{"AID_OPTIMISM", "Optimism", "Have Merchant's guild membership but no jump drive", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			if bs[h.Offsets[types.OFFSET_SHIP]+6] == 0 {
				return false
			}

			return forms[types.OFFSET_REAL].Get("FITE", "JDRV", "INFO") == nil
		}},

		{"AID_NOOBSHIELDS", "Shields to maximum!", "Equip level 2 shields", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			shields := forms[types.OFFSET_REAL].Get("FITE", "SHLD", "INFO")
			if shields == nil {
				return false
			}
			return shields.Data[8] == 89+2 //Why do we start counting at 90?  I have no clue
		}},

		{"AID_KILL1", "It gets easier", "Kill another person, forever destroying everything they are or could be", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			kills := forms[types.OFFSET_PLAY].Get("KILL")
			return !is_all_zero(kills.Data)
		}},

		{"AID_2LAUNCHERS", "\"I am become death, destroyer of Talons\"", "Have 2 missile launchers", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			launchers := forms[types.OFFSET_REAL].Get("FITE", "WEAP", "LNCH")
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

		{"AID_TACHYON", "Now witness the firepower", "Equip a Tachyon Cannon", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			guns := forms[types.OFFSET_REAL].Get("FITE", "WEAP", "GUNS")
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

		{"AID_REPAIRBOT", "They fix Everything", "Have a repair-bot", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			return forms[types.OFFSET_REAL].Get("FITE", "REPR") != nil
		}},

		{"AID_COLOUR_SCANNER", "\"Red\" rhymes with \"Dead\"", "Equip a colour scanner", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
		    scanner := forms[types.OFFSET_REAL].Get("FITE", "TRGT", "INFO")
			return (scanner != nil) && (scanner.Data[len("TARGETNG")]-60 > 2)
		}},

		{"AID_SCANNER_DAMAGE", "Crackle crackle", "Forget to repair your scanner", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			armour := forms[types.OFFSET_REAL].Get("FITE", "SHLD", "ARMR")
            scanner := forms[types.OFFSET_REAL].Get("FITE", "TRGT", "DAMG")
			//fmt.Println("Repairbot", forms[types.OFFSET_REAL].Get("FITE", "REPR") != nil)

			// Scanner damage, no armour damage, no repair bot.  This is a very easy mistake to make due to scanner repair being
			// only available in the "Software" store.
			return forms[types.OFFSET_REAL].Get("FITE", "REPR") == nil &&
				(scanner != nil) && !is_all_zero(scanner.Data) &&
				slices.Equal(armour.Data[:8], armour.Data[8:])
		}},

		{"AID_INTERSTELLAR", "Interstellar Rubicon", "Leave the Troy system", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			switch bs[h.Offsets[types.OFFSET_SHIP]+2] {
			case 0, 15, 17:
				return false
			}

			return true
		}},
	}},

	{"Plot", []Achievement{

		{"AID_SANDOVAL", "Cargo parasite", "Start the plot", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			cargo := forms[types.OFFSET_REAL].Get("FITE", "CRGO", "DATA")
			for n := 0; n < len(cargo.Data); n += 4 {
				if cargo.Data[n] == 42 {
					return true
				}
			}

			return false
		}},

		mcs_complete_series("AID_TAYLA", "I'm not a pirate, I just work for them", "Complete Tayla's missions", 1),

		{"AID_LYNCH", "Can't you see that I am a privateer?", "Complete Roman Lynch's Missions", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			// Note: The final "Get ambushed by Miggs" mission can't have completed status.
			// We're lying in the description to avoid spoiling a 30-year-old game.
			cur := h.Offsets[types.OFFSET_PLOT]
			str, _, _ := readers.Read_string(bs, &cur)
			flag := bs[h.Offsets[types.OFFSET_PLOT]+9]

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

		{"AID_STELTEK_GUN", "Strategically Transfer Equipment to Alternative Location", "Acquire the Steltek gun", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			guns := forms[types.OFFSET_REAL].Get("FITE", "WEAP", "GUNS")
			if guns != nil { //Newly-bought ships have no GUNS record
				for n := 0; n < len(guns.Data); n += 4 {
					if guns.Data[n] >= 8 { //8==steltek gun, 9==super steltek gun.
						return true
					}
				}
			}
			return false
		}},

		{"AID_WON", "That'll be 30000 credits", "Win the game (and get paid for it)", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			cur := h.Offsets[types.OFFSET_PLOT]
			str, _, _ := readers.Read_string(bs, &cur)
			flag := bs[h.Offsets[types.OFFSET_PLOT]+9]

			// This flag is 191 regardless of whether we've returned to the Admiral and heard his "well done" speech.
			return str == "s7mb" && flag == 191
		}},
	}},

	{"Ships", []Achievement{ // The idea here is one achievement per ship which exemplifies what that ship is for.

		{"AID_CENTURION", "Pew Pew Pew", "Mount 4 front guns and 20 warheads (on a Centurion)", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			count := 0
			guns := forms[types.OFFSET_REAL].Get("FITE", "WEAP", "GUNS")
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

			warheads := forms[types.OFFSET_REAL].Get("FITE", "WEAP", "MISL")
			count = 0
			for n := 1; n < len(warheads.Data); n += 3 {
				count += int(warheads.Data[n])
			}

			return count == 20
		}},

		{"AID_GALAXY", "Star Truck", "Carry more than 200T of cargo in a Galaxy", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			// This check is necessary, because of cargo misions and also because it's possible to exchange ships when you shouldn't be able to thanks to
			// (I guess) 8-bit wrap around in stored cargo.
			if bs[h.Offsets[types.OFFSET_SHIP]] != tables.SHIP_GALAXY {
				return false
			}

			total := 0
			cargo := forms[types.OFFSET_REAL].Get("FITE", "CRGO", "DATA")
			for n := 0; n < len(cargo.Data); n += 4 {
				cur := n + 1
				total += readers.Read_int16(cargo.Data, &cur)
			}

			return total > 200
		}},

		{"AID_ORION", "Expensive Paperweight", "Have Level 5 engines and level 5 shields (on an Orion)", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			if !slices.Equal(forms[types.OFFSET_REAL].Get("FITE", "ENER", "INFO").Data, []byte{'E', 'N', 'E', 'R', 'G', 'Y', 0, 0, 1, 2, 2, 2, 3, 1, 4, 1, 5, 1, 6, 2}) {
				return false
			}

			shields := forms[types.OFFSET_REAL].Get("FITE", "SHLD", "INFO")
			if shields == nil {
				return false
			}
			return shields.Data[8] == 89+5 //Why do we start counting at 90?  I have no clue
		}},

		{"AID_TARSUS", "Tarsus gonna Tarsus", "Take damage to all four armour facings on a Tarsus", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			if bs[h.Offsets[types.OFFSET_SHIP]] != tables.SHIP_TARSUS {
				return false
			}

			armour := forms[types.OFFSET_REAL].Get("FITE", "SHLD", "ARMR")
			if armour == nil {
				return false
			}

			var armours [8]int
			cur := 0
			for i := range armours {
				armours[i] = readers.Read_int16(armour.Data, &cur)
			}
			for i := 0; i < 4; i += 1 {
				if armours[i] == 1 || armours[i] == armours[i+4] {
					return false
				}
			}
			return true
		}},
	}},

	{"Random", []Achievement{
		{"AID_DUPER", "I know what you did", "Equip multiple tractor beams in front mounts", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			// It is diccucult to imagine a reason (other than the cargo duping exploit) to have multiple front tractor beams
			// OTOH, a Galaxy with a tracter in each turret isn't particularly suspicious ans shouldn't get this cheev.
			launchers := forms[types.OFFSET_REAL].Get("FITE", "WEAP", "LNCH")
			count := 0
			if launchers != nil {
				for i := 0; i < len(launchers.Data); i += 4 {
					if launchers.Data[i] == 52 && launchers.Data[i+1]<6{
						count += 1
					}
				}
			}
			return count > 1
		}},

		{"AID_PORNO", "I trade it for the articles", "Carry at least one ton of PlayThing(tm)", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			cargo := forms[types.OFFSET_REAL].Get("FITE", "CRGO", "DATA")
			for n := 0; n < len(cargo.Data); n += 4 {
				if cargo.Data[n] == 27 {
					return true
				}
			}

			return false
		}},

		{"AID_BAD_FRIENDLY", "Questionable morality", "Become friendly with Pirates and Kilrathi", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			rep := forms[types.OFFSET_PLAY].Get("SCOR")
			for _, f := range []int{tables.FACTION_PIRATES, tables.FACTION_KILRATHI} {
				cur := 2 * f
				if readers.Read_int16(rep.Data, &cur) <= 25 {
					return false
				}

			}
			return true
		}},

		{"AID_SUPERFRIENDLY", "Insane morality", "Become friendly with everyone except retros", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			rep := forms[types.OFFSET_PLAY].Get("SCOR")
			for _, f := range []int{tables.FACTION_MERCHANTS, tables.FACTION_HUNTERS, tables.FACTION_CONFEDS, tables.FACTION_KILRATHI, tables.FACTION_MILITIA, tables.FACTION_PIRATES} {
				cur := 2 * f
				if readers.Read_int16(rep.Data, &cur) <= 25 {
					return false
				}
			}

			return true
		}},

		{"AID_RICH", "Dr. Evil Pinky Finger", "Possess One Million Credits", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {

			cur := 0
			return readers.Read_int_le(forms[types.OFFSET_REAL].Get("FITE", "CRGO", "CRGI").Data, &cur) >= 1000000
		}},

		{"AID_CARGO_IS_NIGGER", "Just glue it to the outside", "Carry more cargo than will fit in your ship", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			// Assuming the player isn't just cheating, this is possible because cargo-delivery missions don't bother to check cargo capacity when you accept them.
			// This could be a bug, but maybe it's a convenience feature?

			info := forms[types.OFFSET_REAL].Get("FITE", "CRGO", "CRGI")
			capacity := int(info.Data[4])
			if info.Data[6] != 0 {
				capacity += 20 //secfet compartment
			}

			stored := 0
			cargo := forms[types.OFFSET_REAL].Get("FITE", "CRGO", "DATA")
			for n := 0; n < len(cargo.Data); n += 4 {
				cur := n + 1
				stored += readers.Read_int16(cargo.Data, &cur)
			}

			//fmt.Println("Stored:", stored, "Capacity:", capacity)
			return stored > capacity
		}},

		{"AID_KILL_DRONE", "No kill stealing", "Personally kill the Steltek Drone", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			// Although only the improved Steltek gun can knock down the drone's shields,
			// anything can damage the soft and squishy egg inside.
			cur := 2 * tables.FACTION_DRONE
			return readers.Read_int16(forms[types.OFFSET_PLAY].Get("KILL").Data, &cur) > 0
		}},

		{"AID_WIN_KILL_NO_KILRATHI", "Cat Lover", "Win the game without killing any Kilrathi", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			cur := 2 * tables.FACTION_KILRATHI
			if readers.Read_int16(forms[types.OFFSET_PLAY].Get("KILL").Data, &cur) > 0 {
				return false
			}

			cur = h.Offsets[types.OFFSET_PLOT]
			str, _, _ := readers.Read_string(bs, &cur)
			flag := bs[h.Offsets[types.OFFSET_PLOT]+9]

			return str == "s7mb" && flag == 191
		}},

		{"AID_WIN_KILL_NO_GOOD", "Good Guy", "Win the game without killing any Militia, Merchants or Confeds", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			for _, faction := range []int{tables.FACTION_MILITIA, tables.FACTION_MERCHANTS, tables.FACTION_CONFEDS} {
				cur := 2 * faction
				if readers.Read_int16(forms[types.OFFSET_PLAY].Get("KILL").Data, &cur) > 0 {
					return false
				}
			}

			cur := h.Offsets[types.OFFSET_PLOT]
			str, _, _ := readers.Read_string(bs, &cur)
			flag := bs[h.Offsets[types.OFFSET_PLOT]+9]

			return str == "s7mb" && flag == 191
		}},

		{"AID_3_DELIVERIES", "Tagon would be proud", "Accept three delivery missions to the same location", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			if len(h.Mission_offsets) != 6 {
				return false
			}

			destinations := map[uint8]bool{}
			for i := 1; i < 6; i += 2 {
				cur := h.Mission_offsets[i]
				form, err := readers.Read_form(bs, &cur)
				if err != nil {
					//fmt.Println("BAd form!")
					return false
				}

				cargo := form.Get("CARG")
				if cargo == nil {
					// not a cargo mission
					//fmt.Println("Bad cargo")
					return false
				}

				destinations[cargo.Data[0]] = true
			}

			//fmt.Println(destinations)
			return len(destinations) == 1
		}},

		{"AID_FAIL_ESCORT", "Wing Commander nostalgia", "Fail a Drayman escort mission", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			// There are 3 such missions - Oxford 1, 3 and 4.
			cur := h.Offsets[types.OFFSET_PLOT]
			str, _, _ := readers.Read_string(bs, &cur)
			flag := bs[h.Offsets[types.OFFSET_PLOT]+9]

			switch str {
			case "s3ma", "s3mc", "s3md":
				return (flag == 226 || flag == 162)
			}
			return false
		}},

		{"AID_BITCORES_MAN", "The Bitcores maneuver", "Put the Steltek gun on a central mount", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			// To pull this one off, you have to remove a central gun at Rygannon then get to the derelict on just 3 guns.
			if bs[h.Offsets[types.OFFSET_SHIP]] != 2 {
				return false
			}
			guns := forms[types.OFFSET_REAL].Get("FITE", "WEAP", "GUNS")
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

		{"AID_DO_MISSIONS", "Space-Hobo", "Do 100 non-plot missions", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			cur := h.Offsets[types.OFFSET_SHIP] + 3
			return readers.Read_int16(bs, &cur) >= 100
		}},

		// TODO: these would be fun but needs multi-file checking
		// "The Militia would be proud", "Kill the Black Rhombus without killing any of its escorts"
		// "How does that work?", "Transfer your secret compartment to a new ship"
		// "Press C to spill secrets", "Visit all secret bases"
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
		{"AID_TARSUS_DERELICT", "Get that trophy screenshot", "Get to the derelict in a Tarsus", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			// I've done this.  It was painful.
			// The Centurions at Palan can be handled by kiting them into the asteroid field.
			// Cross 3 method: clear nav 1 (asteroids will help you here), then hit nav 4, wipe out the Gothri there, again taking full advantage of the asteroids. 
			// Run from the Kamekh, auto to nav 3, then to nav 2, kill 2 out of 3 Dralthi then burn back to nav 1.
			if  bs[h.Offsets[types.OFFSET_SHIP]] != tables.SHIP_TARSUS {
				return false
			}

			/// actually at the derelict
			if bs[h.Offsets[types.OFFSET_SHIP]+2] == 59 {
				return true
			}

			// Anyone with the steltek gun must have been there
			guns := forms[types.OFFSET_REAL].Get("FITE", "WEAP", "GUNS")
			if guns != nil {
				for n := 0; n < len(guns.Data); n += 4 {
					if guns.Data[n] >= 8 { //8==steltek gun, 9==super steltek gun.
						return true
					}
				}
			}
			return false
		}},

		{"AID_VERY_RICH", "Almost ready to start Righteous Fire", "Possess twenty million credits", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			cur := 0
			return readers.Read_int_le(forms[types.OFFSET_REAL].Get("FITE", "CRGO", "CRGI").Data, &cur) >= 20000000
		}},

		{"AID_FIX_HUNTER_REP", "Grinder", "Recover hunter reputation to non-hostile before winning", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			// Hunters are notoriously hard to please.  The problem is that you have to kill a lot of them to win the game,
			// losing 15 rep per Demon and 20 rep per Centurion - but nothing (except the drone) will improve hunter rep by more than 1.
			// (That's right, killing a pirate talon impresses them exactly as much as killing a Kamekh)
			cur := h.Offsets[types.OFFSET_PLOT]
			str, _, _ := readers.Read_string(bs, &cur)
			flag := bs[h.Offsets[types.OFFSET_PLOT]+9]

			if len(str) != 4 {
				// Either too early or too latge
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

			// Murphy  3 not yet complete
			// Some notes:
			// The "correct" way to do this is to kill all the hunters, resulting in flag=191
			// The alternative is to just afterburn past them and land at Palan, resulting in flag=162.
			// The game is still winnable; just pick up Dr. Monkhouse and continue.
			// But if you fail the mission then go back and talk to Murphy without doing Monkhouse first she sets the plot to "failed"
			if str[0] == 's' && str[1] == '4' && str[3] == 'c' {
				if !(flag==162 || flag== 191){
					return false
				}
			}
			// If we get this far, the player has done (possibly unsuccessfully) the first 3 Palan missions and is not in an unwinnable state

			if flag == 226 {
				// failed!
				return false
			}

			if str == "s7mb" && flag == 191 {
				// won the game!
				return false
			}

			cur = 2 * tables.FACTION_HUNTERS
			return readers.Read_int16(forms[types.OFFSET_PLAY].Get("SCOR").Data, &cur) >= -25
		}},

		{"AID_CARGO_IS_TWICE_BIGGER", "How much glue do you have?", "Carry more than twice as much cargo as will fit in your ship", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			// Probably the easiest way to do this is to get a centurion without a secret compartment, buy 50T of whatever,
			// then accept 4 cargo missions.  That's how I did it, anyway.  Some savescumming required.
			info := forms[types.OFFSET_REAL].Get("FITE", "CRGO", "CRGI")
			capacity := int(info.Data[4])
			if info.Data[6] != 0 {
				capacity += 20 //secfet compartment
			}

			stored := 0
			cargo := forms[types.OFFSET_REAL].Get("FITE", "CRGO", "DATA")
			for n := 0; n < len(cargo.Data); n += 4 {
				cur := n + 1
				stored += readers.Read_int16(cargo.Data, &cur)
			}

			//fmt.Println("Stored:", stored, "Capacity:", capacity)
			return stored > capacity*2
		}},

		{"AID_INSANE_FRIENDLY", "No-one, you see, is smarter than he", "Become friendly with every real faction", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			// The problem here is that retros start out hostile, and it is not possible to improve retro rep by any means.. other than getting it below -32768,
			// causing 16-bit wraparound, flipping them to maximally friendly!  This will require about 6000 retro kills.
			// Cheev name is a reference to "Flipper", which is sort of a hint as to the only way to do this.
			rep := forms[types.OFFSET_PLAY].Get("SCOR")
			for _, f := range []int{tables.FACTION_MERCHANTS, tables.FACTION_HUNTERS, tables.FACTION_CONFEDS, tables.FACTION_KILRATHI,
				tables.FACTION_MILITIA, tables.FACTION_PIRATES, tables.FACTION_RETROS} {
				cur := 2 * f
				if readers.Read_int16(rep.Data, &cur) <= 25 {
					return false
				}
			}

			return true
		}},
	}},
}
