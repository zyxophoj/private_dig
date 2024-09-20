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
	// Create new watcher.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Println(err)
		return
	}
	defer watcher.Close()

	dir := get_dir()
	state_file = dir + "\\pracst.json"
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
	{"Tarsus Grind", []Achievement{ //Because not everybody gfets their Centurion at the 3-minute mark :D

		{"AID_AFTERBURNER", "I am speed", "Equip an afterburner", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			return forms[types.OFFSET_REAL].Get("FITE", "AFTB") != nil
		}},

		{"AID_OPTIMISM", "Optimism", "Have Merchant's guild membership but no jump drive", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			if bs[h.Offsets[types.OFFSET_SHIP]+6] == 0 {
				return false
			}

			return forms[types.OFFSET_REAL].Get("FITE", "JRDV", "INFO") == nil
		}},

		{"AID_NOOBSHIELDS", "Shields to maximum!", "Equip level 2 shields!", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			shields := forms[types.OFFSET_REAL].Get("FITE", "SHLD", "INFO")
			if shields == nil {
				return false
			}
			return shields.Data[8] == 89+2 //Why do we start counting at 90?  I have no clue
		}},

		{"AID_KILL1", "It gets much easier", "Kill another person, forever destroying everything they are or could be", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			kills := forms[types.OFFSET_PLAY].Get("KILL")
			return !slices.Equal(kills.Data, make([]byte, len(kills.Data)))
		}},

		{"AID_2LAUNCHERS", "\"I am become death, destroyer of Talons\"", "Have 2 missile launchers!", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
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

		{"AID_COLOUR_SCANNER", "Taste the rainbow", "Have a colour scanner!", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			return forms[types.OFFSET_REAL].Get("FITE", "TRGT", "INFO").Data[len("TARGETNG")]-60 > 2
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

		mcs_complete_series("AID_OXFORD", "Unlocking the ancient mysteries", "Complete Masterson's missions", 3),
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

		{"AID_GALAXY", "I'm a trader, really!", "Carry more than 240T of cargo (in a Galaxy)", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			// It is actually possible to be in this state in a non-Galaxy (by transfering the secret compartment from a non-galalxy to a Galaxy,
			// filling up beyond 225T, then switching to a non-Galaxy).  But since this involved having a qualifying state, we don't need to
			// check ship type.

			total := 0
			cargo := forms[types.OFFSET_REAL].Get("FITE", "CRGO", "DATA")
			for n := 0; n < len(cargo.Data); n += 4 {
				cur := n + 1
				total += readers.Read_int16(cargo.Data, &cur)
			}

			return total > 240
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
			if bs[h.Offsets[types.OFFSET_SHIP]] != 1 {
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
			for i := 0; i < 8; i += 1 {
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

		{"AID_RICH", "Dr. Evil Pinky Finger", "Possess One Million Spacedollars", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			cur := 0
			return readers.Read_int_le(forms[types.OFFSET_REAL].Get("FITE", "CRGO", "CRGI").Data, &cur) >= 1000000
		}},

		{"AID_CARGO_IS_NIGGER", "The guild just glues it to the outside", "Carry more cargo than will fit in your ship", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
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
	}},

	{"Mostly Peaceful", []Achievement{

		mcs_kill("AID_KILL_RETROS", "Defender of toasters", 20, tables.FACTION_RETROS),
		mcs_kill("AID_KILL_PIRATES", "We are not the same", 20, tables.FACTION_PIRATES),
		mcs_kill("AID_KILL_HUNTERS", "Avril Lavigne mode", 30, tables.FACTION_HUNTERS),
		mcs_kill("AID_KILL_KILRATHI", "Also Try Wing Commander", 10, tables.FACTION_KILRATHI),
		mcs_kill("AID_KILL_MILITIA", "Criminal", 6, tables.FACTION_MILITIA),
		mcs_kill("AID_KILL_CONFEDS", "Cat lover", 6, tables.FACTION_CONFEDS),
	}},

	{"Mass-murder?  I hardly...", []Achievement{
		mcs_kill("AID_KILL_MANY_RETROS", "Guardian Angel of Toasters", 100, tables.FACTION_RETROS),
		mcs_kill("AID_KILL_MANY_PIRATES", "Your Letter of Marque is in the post", 100, tables.FACTION_PIRATES),
		mcs_kill("AID_KILL_MANY_HUNTERS", "Joan Jett mode", 100, tables.FACTION_HUNTERS),
		mcs_kill("AID_KILL_MANY_KILRATHI", "Also Try Wing Commander 3", 50, tables.FACTION_KILRATHI),
		mcs_kill("AID_KILL_MANY_MILITIA", "Menesch's Apprentice", 30, tables.FACTION_MILITIA),
		mcs_kill("AID_KILL_MANY_CONFEDS", "Traitor", 30, tables.FACTION_CONFEDS),
	}},

	{"Feats of Insanity", []Achievement{
		{"AID_TARSUS_DERELICT", "Get that trophy screenshot", "Get to the derelict in a Tarsus", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			return bs[h.Offsets[types.OFFSET_SHIP]] == 0 && bs[h.Offsets[types.OFFSET_SHIP+2]] == 59
		}},
		{"AID_VERY_RICH", "Probably sufficient to start Righteous Fire", "Possess twenty million spacedollars", func(h types.Header, bs []byte, forms map[int]*types.Form) bool {
			cur := 0
			return readers.Read_int_le(forms[types.OFFSET_REAL].Get("FITE", "CRGO", "CRGI").Data, &cur) >= 20000000
		}},
	}},
}

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
