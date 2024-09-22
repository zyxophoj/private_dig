package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/ini.v1"

	"privdump/readers"
	"privdump/tables"
	"privdump/types"
)

func get_dir() string {
	// dir from command line
	if len(os.Args) > 1 && os.Args[1] == "--dir" {
		return os.Args[2]
	}

	//dir from ini file
	cfg, err := ini.Load("priv_ach.ini")
	if err == nil {
		// Classic read of values, default section can be represented as empty string
		dir := cfg.Section("").Key("dir").String()
		if dir != "" {
			return dir
		}
	}

	/*bytes, err := ioutil.ReadFile("priv_ach.cfg")
	if err == nil {
		cfg := map[string]string{}
		_ = json.Unmarshal(bytes, &cfg)
		if cfg["dir"] != "" {
			return cfg["dir"]
		}
	}*/

	//current dir

	wd, _ := os.Getwd()
	return wd
}

var global_state = struct {
	Unlocked map[string]map[string]bool
}{map[string]map[string]bool{}}

var state_file = ""

func save_state() {
	b, _ := json.Marshal(global_state)
	ioutil.WriteFile(state_file, b, 0644)
}

func load_state() {
	bytes, _ := ioutil.ReadFile(state_file)
	json.Unmarshal(bytes, &global_state)
}

func main() {
	// Deal with args

	arg_info := []struct {
		arg     string
		subargs int
		desc    string
	}{
		{"help", 0, "Display this possibly helpful info"},
		{"check", 0, "Sanity check"},
		{"list", 0, "List identities"},
		{"show", 1, "Show achievemnts for an identity"},
		{"show_missing", 1, "Show missing achievemnts for an identity"},
		{"run", 0, "Run and monitor achievements.  Also the default."},
	}

	main_arg := ""
	subargs := []string{}
	subargs_needed := 0
	for _, arg := range os.Args[1:] {
		if main_arg == "" {
			for _, info := range arg_info {
				if info.arg == arg {
					main_arg = arg
					subargs_needed = info.subargs
					break
				}
			}
			if main_arg == "" {
				fmt.Println("Unexpected extra argument:", arg)
				os.Exit(1)
			}
		} else if len(subargs) < subargs_needed {
			subargs = append(subargs, arg)
		} else {
			fmt.Println("Unexpected extra argument:", arg)
			os.Exit(1)
		}
	}
	if main_arg == "" {
		main_arg = "run"
	}

	if len(subargs) != subargs_needed {
		fmt.Println(fmt.Sprintf("Expected %v extra arguments; got %v:", subargs_needed, len(subargs)))
		os.Exit(1)
	}

	dir := get_dir()
	state_file = dir + "\\pracst.json"

	switch main_arg {
	case "help":
		for _, info := range arg_info {
			fmt.Println(info.arg, "-", info.desc)
		}
		os.Exit(0)

	case "check":
		fmt.Println("Target dir is: " + dir)
		os.Exit(0)

	case "list":
		load_state()
		if len(global_state.Unlocked) == 0 {
			fmt.Println("(no profiles detected")
			os.Exit(0)
		}

		for p := range global_state.Unlocked {
			fmt.Println(p)
		}
		os.Exit(0)

	case "show":
		fmt.Println("Showing achevements for", subargs[0])
		fmt.Println()

		load_state()
		got := global_state.Unlocked[subargs[0]]
		ttotal := 0
		for _, cat_list := range cheev_list {
			total := len(cat_list.cheeves)
			ttotal += total
			indices := []int{}
			for i, cheev := range cat_list.cheeves {
				if got[cheev.id] {
					indices = append(indices, i)
				}
			}
			fmt.Println(fmt.Sprintf("%v (%v/%v):", cat_list.category, len(indices), total))
			for _, i := range indices {
				fmt.Println("   " + cat_list.cheeves[i].name)
				fmt.Println("   (" + cat_list.cheeves[i].expl + ")")
				fmt.Println()
			}
			fmt.Println()
		}
		fmt.Println(fmt.Sprintf("Overall: %v/%v", len(got), ttotal))
		os.Exit(0)

	case "show_missing":
		fmt.Println("Showing missing achevements for", subargs[0])
		fmt.Println()

		load_state()
		got := global_state.Unlocked[subargs[0]]
		for _, cat_list := range cheev_list {
			total := len(cat_list.cheeves)
			indices := []int{}
			for i, cheev := range cat_list.cheeves {
				if !got[cheev.id] {
					indices = append(indices, i)
				}
			}
			if len(indices) > 0 {
				fmt.Println(fmt.Sprintf("%v (%v/%v):", cat_list.category, len(indices), total))
				for _, i := range indices {
					fmt.Println("   " + cat_list.cheeves[i].name)
					fmt.Println("   (" + cat_list.cheeves[i].expl + ")")
					fmt.Println()
				}
				fmt.Println()
			}
		}
		os.Exit(0)

	case "run":
		break
	}

	// Create new watcher.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Println(err)
		return
	}
	defer watcher.Close()

	load_state()

	// Start listening for events.
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Write) {
					if strings.HasSuffix(event.Name, ".SAV") {
						handle_file(event.Name)
					}

				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				fmt.Println("error:", err)
			}
		}
	}()

	fmt.Println("Watching...", dir)
	fmt.Println()
	fmt.Println()
	err = watcher.Add(dir)

	// Wait forever!
	<-make(chan bool)
}

type Achievement struct {
	id   string
	name string
	expl string
	test func(types.Header, []byte, map[int]*types.Form) bool
}

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
// (especially if "we" is a pseedrunner who can't remember how to play the game normally)
//
// Check for typos before pushing!
var cheev_list = []struct {
	category string
	cheeves  []Achievement
}{
	{"Tarsus Grind", []Achievement{ //Because not everybody gets their Centurion at the 3-minute mark :D

		{"AID_AFTERBURNER", "I am speed", "Equip an afterburner", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			return forms[types.OFFSET_REAL].Get("FITE", "AFTB") != nil
		}},

		{"AID_OPTIMISM", "Optimism", "Have Merchant's guild membership but no jump drive", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			if bs[h.Offsets[types.OFFSET_SHIP]+6] == 0 {
				return false
			}

			return forms[types.OFFSET_REAL].Get("FITE", "JRDV", "INFO") == nil
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
			for i := 0; i < len(launchers.Data); i += 4 {
				if launchers.Data[i] == 50 {
					count += 1
				}
			}
			return count == 2
		}},

		{"AID_TACHYON", "Now witness the firepower", "Equip a Tachyon Cannon", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			guns := forms[types.OFFSET_REAL].Get("FITE", "WEAP", "GUNS")
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
			return forms[types.OFFSET_REAL].Get("FITE", "TRGT", "INFO").Data[len("TARGETNG")]-60 > 2
		}},

		{"AID_SCANNER_DAMAGE", "Crackle crackle", "Forget to repair your scanner", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			armour := forms[types.OFFSET_REAL].Get("FITE", "SHLD", "ARMR")

			// Scanner damage, no armour damage, no repair bot.  This is a very easy mistake to make due to scanner repair being
			// only available in the "Software" store.
			return forms[types.OFFSET_REAL].Get("FITE", "REPR") != nil &&
				!is_all_zero(forms[types.OFFSET_REAL].Get("FITE", "TRGT", "DAMG").Data) &&
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
			if str == "s2md" && (flag == 162 || flag == 226) {
				return true
			}

			return false
		}},

		mcs_complete_series("AID_OXFORD", "Unlocking the greatest mysteries", "Complete Masterson's missions", 3),
		mcs_complete_series("AID_PALAN", "I travel the galaxy", "Complete the Palan missions", 4),
		mcs_complete_series("AID_RYGANNON", "...and far beyond", "Complete Taryn Cross's missions", 5),

		{"AID_STELTEK_GUN", "Strategically Transfer Equipment to Alternative Location", "Acquire the Steltek gun", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			guns := forms[types.OFFSET_REAL].Get("FITE", "WEAP", "GUNS")
			for n := 0; n < len(guns.Data); n += 4 {
				if guns.Data[n] >= 8 { //8==steltek gun, 9==super steltek gun.
					return true
				}
			}

			return false
		}},

		{"AID_WON", "That'll be 30000 credits", "Win the game (and get paid for it)", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			cur := h.Offsets[types.OFFSET_PLOT]
			str, _, _ := readers.Read_string(bs, &cur)
			flag := bs[h.Offsets[types.OFFSET_PLOT]+9]

			return str == "s7mb" && flag == 191
		}},
	}},

	{"Ships", []Achievement{ // The idea here is one achievement per ship which exemplifies what that ship is for.

		{"AID_CENTURION", "Pew Pew Pew", "Mount 4 front guns and 20 warheads (on a Centurion)", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			count := 0
			guns := forms[types.OFFSET_REAL].Get("FITE", "WEAP", "GUNS")
			for n := 1; n < len(guns.Data); n += 4 {
				if guns.Data[n] >= 1 && guns.Data[n] <= 4 {
					count += 1
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

		{"AID_GALAXY", "I'm a trader, really!", "Carry more than 200T of cargo in a Galaxy", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
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
				if armours[i] == armours[i+4] {
					return false
				}
			}
			return true
		}},
	}},

	{"Random", []Achievement{
		{"AID_DUPER", "I know what you did", "Equip multiple tractor beams", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			launchers := forms[types.OFFSET_REAL].Get("FITE", "WEAP", "LNCH")
			count := 0
			for i := 0; i < len(launchers.Data); i += 4 {
				if launchers.Data[i] == 52 {
					count += 1
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

		{"AID_BAD_FRIENDLY", "Questonable Morailty", "Become friendly with Pirates and Kilrathi", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
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

		{"AID_FAIL_ESCORT", "Wing Commander Nostalgia", "Fail a Drayman escort mission", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
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
			for n := 0; n < len(guns.Data); n += 4 {
				if guns.Data[n] >= 8 && (guns.Data[n+1] == 2 || guns.Data[n+1] == 3) {
					return true
				}
			}

			return false
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
		mcs_kill("AID_KILL_MANY_RETROS", "Guardian Angel of Toasters", 100, tables.FACTION_RETROS),
		mcs_kill("AID_KILL_MANY_PIRATES", "Your Letter of Marque is in the post", 100, tables.FACTION_PIRATES),
		mcs_kill("AID_KILL_MANY_HUNTERS", "Joan Jett mode", 100, tables.FACTION_HUNTERS),
		mcs_kill("AID_KILL_MANY_KILRATHI", "Also Try Wing Commander 3", 50, tables.FACTION_KILRATHI),
		mcs_kill("AID_KILL_MANY_MILITIA", "Menesch's Apprentice", 30, tables.FACTION_MILITIA),
		mcs_kill("AID_KILL_MANY_CONFEDS", "Arch-Traitor", 30, tables.FACTION_CONFEDS),
	}},

	{"Feats of Insanity", []Achievement{
		{"AID_TARSUS_DERELICT", "Get that trophy screenshot", "Get to the derelict in a Tarsus", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			return bs[h.Offsets[types.OFFSET_SHIP]] == tables.SHIP_TARSUS && bs[h.Offsets[types.OFFSET_SHIP+2]] == 59
		}},
		{"AID_VERY_RICH", "Probably sufficient to start Righteous Fire", "Possess twenty million credits", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			cur := 0
			return readers.Read_int_le(forms[types.OFFSET_REAL].Get("FITE", "CRGO", "CRGI").Data, &cur) >= 20000000
		}},

		{"AID_FIX_HUNTER_REP", "Grinder", "Recover hunter reputation to non-hostile before winning", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
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

			// Not onto Murphy 3 (we need to get at least this far to have the bad rep
			if str[0] == 's' && str[1] == '4' && str[3] < 'd' {
				return false
			}

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

var last_identity = ""

func handle_file(filename string) {

	time.Sleep(5 * time.Second)

	//fmt.Println("   Detected file", filename)
	//fmt.Println()

	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		fmt.Println("Failed to load file", filename, "-", err)
		return
	}

	header := readers.Read_header(bytes)

	forms := map[int]*types.Form{}
	for _, i := range []int{types.OFFSET_PLAY, types.OFFSET_SSSS, types.OFFSET_REAL} {
		cur := header.Offsets[i]
		f, err := readers.Read_form(bytes, &cur)
		if err != nil {
			fmt.Println("Failed to load form", i, "-", err)
			return
		}
		forms[i] = &f
	}

	cur := header.Offsets[types.OFFSET_NAME]
	name, _, err := readers.Read_string(bytes, &cur)
	if err != nil {
		fmt.Println("Failed to read name", err)
		return
	}
	cur = header.Offsets[types.OFFSET_CALLSIGN]
	callsign, _, err := readers.Read_string(bytes, &cur)
	if err != nil {
		fmt.Println("Failed to read callsign", err)
		return
	}

	for _, list := range cheev_list {
		for _, cheev := range list.cheeves {
			identity := name + ":" + callsign
			if last_identity != identity {
				fmt.Println("Identity is", identity)
				fmt.Println()
				last_identity = identity
			}
			if !global_state.Unlocked[identity][cheev.id] && cheev.test(header, bytes, forms) {
				fmt.Println(cheev.name)
				fmt.Println(cheev.expl)
				fmt.Println("Category:", list.category)
				fmt.Println()

				_, ok := global_state.Unlocked[identity]
				if !ok {
					global_state.Unlocked[identity] = map[string]bool{}
				}
				global_state.Unlocked[identity][cheev.id] = true
			}
		}
	}

	save_state()
	//fmt.Println("   Finished with file", filename)
	//fmt.Println()
}
