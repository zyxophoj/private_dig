package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"privdump/readers"
	"privdump/tables"
	"privdump/types"
)

func get_dir() string {
	// dir from command line
	if os.Args[1] == "--dir" {
		return os.Args[2]
	}

	//todo: dir from ini file

	//current dir

	wd, _ := os.Getwd()
	return wd
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
	name string
	expl string
	test func(types.Header, []byte) bool
}

func mcs_kill(name string, number int, who int) Achievement {
	return Achievement{
		name,
		fmt.Sprintf("Kill %v %v", number, tables.Factions[who]),
		func(h types.Header, bs []byte) bool {
			cur := h.Offsets[types.OFFSET_PLAY]
			form, err := readers.Read_form(bs, &cur)
			if err != nil {
				fmt.Println("Failed to read PLAY form", err)
				return false
			}

			kills := form.Get("KILL")
			cur = 2 * who
			return readers.Read_int16(kills.Data, &cur) >= number
		},
	}
}

func mcs_complete_series(name string, expl string, number uint8) Achievement {

	return Achievement{
		name,
		expl,
		func(h types.Header, bs []byte) bool {
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

var cheevz = []Achievement{
	// Progression...

	{"Cargo parasite", "Accept the first mission", func(h types.Header, bs []byte) bool {
		cur := h.Offsets[types.OFFSET_REAL]
		form, err := readers.Read_form(bs, &cur)
		if err != nil {
			fmt.Println("Failed to read REAL form", err)
			return false
		}

		cargo := form.Get("FITE", "CRGO", "DATA")
		for n := 0; n < len(cargo.Data); n += 4 {
			if cargo.Data[n] == 42 {
				return true
			}
		}

		return false
	}},

	mcs_complete_series("I'm not a pirate, I just work for them", "Complete Tayla's missions", 1),

	{"Can't you see that I am a privateer?", "Complete Roman Lynch's Missions", func(h types.Header, bs []byte) bool {
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

	mcs_complete_series("Unlocking the ancient mysteries", "Complete Masterson's missions", 3),
	mcs_complete_series("I travel the galaxy", "Complete the Palan missions", 4),
	mcs_complete_series("...and far beyond.", "Complete Taryn Cross's missions", 5),

	{"Strategically Transfer Equipment to Alternative Location", "Acquire the Steltek gun", func(h types.Header, bs []byte) bool {
		cur := h.Offsets[types.OFFSET_REAL]
		form, err := readers.Read_form(bs, &cur)
		if err != nil {
			fmt.Println("Failed to read REAL form", err)
			return false
		}

		guns := form.Get("FITE", "WEAP", "GUNS")
		for n := 0; n < len(guns.Data); n += 4 {
			if guns.Data[n] >= 8 {
				return true
			}
		}

		return false
	}},

	// Ships...
	// The idea here is one achievement per ship which exemplifies what that ship is for.

	{"Pew Pew Pew", "Mount 4 front guns and 20 warheads (on a Centurion)", func(h types.Header, bs []byte) bool {
		cur := h.Offsets[types.OFFSET_REAL]
		form, err := readers.Read_form(bs, &cur)
		if err != nil {
			fmt.Println("Failed to read REAL form", err)
			return false
		}

		count := 0
		guns := form.Get("FITE", "WEAP", "GUNS")
		for n := 1; n < len(guns.Data); n += 4 {
			if guns.Data[n] >= 1 && guns.Data[n] <= 4 {
				count += 1
			}
		}
		if count < 4 {
			return false
		}

		warheads := form.Get("FITE", "WEAP", "MISL")
		count = 0
		for n := 1; n < len(warheads.Data); n += 3 {
			count += int(warheads.Data[n])
		}

		return count == 20
	}},

	{"I'm a trader, really!", "Carry more than 240T of cargo (in a Galaxy)", func(h types.Header, bs []byte) bool {
		cur := h.Offsets[types.OFFSET_REAL]
		form, err := readers.Read_form(bs, &cur)
		if err != nil {
			fmt.Println("Failed to read REAL form", err)
			return false
		}

		// It is actually possible to be in this state in a non-Galaxy (by transfering the secret compartment from a non-galalxy to a Galaxy,
		// filling up beyond 225T, then switching to a non-Galaxy).  But since this involved having a qualifying state, we don't need to
		// check ship type.

		total := 0
		cargo := form.Get("FITE", "CRGO", "DATA")
		for n := 0; n < len(cargo.Data); n += 4 {
			cur := n + 1
			total += readers.Read_int16(cargo.Data, &cur)
		}

		return total > 240
	}},

	{"Expensive Papereweight", "Have Level 5 engines and level 5 shields (on an Orion)", func(h types.Header, bs []byte) bool {
		cur := h.Offsets[types.OFFSET_REAL]
		form, err := readers.Read_form(bs, &cur)
		if err != nil {
			fmt.Println("Failed to read REAL form", err)
			return false
		}

		engines := form.Get("FITE", "ENER", "INFO")
		if !slices.Equal(engines.Data, []byte{'E', 'N', 'E', 'R', 'G', 'Y', 0, 0, 1, 2, 2, 2, 3, 1, 4, 1, 5, 1, 6, 2}) {
			return false
		}

		shields := form.Get("FITE", "SHLD", "INFO")
		if shields == nil {
			return false
		}
		return shields.Data[8] == 89+5 //Why do we start counting at 90?  I have no clue
	}},

	{"Tarsus Gonna Tarsus", "Take damage to all four armour facings on a Tarsus", func(h types.Header, bs []byte) bool {
		if bs[h.Offsets[types.OFFSET_SHIP]] != 1 {
			return false
		}

		cur := h.Offsets[types.OFFSET_REAL]
		form, err := readers.Read_form(bs, &cur)
		if err != nil {
			fmt.Println("Failed to read REAL form", err)
			return false
		}

		armour := form.Get("FITE", "SHLD", "ARMR")
		if armour == nil {
			return false
		}

		var armours [8]int
		cur = 0
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

	// Fun?...

	// Mass-murder?  I hardly...
	mcs_kill("I like toasters", 100, tables.FACTION_RETROS),
	mcs_kill("We are not the same", 100, tables.FACTION_PIRATES),
	mcs_kill("Joan Jett mode", 100, tables.FACTION_HUNTERS),
	mcs_kill("Also Try Wing Commander", 50, tables.FACTION_KILRATHI),
	mcs_kill("Criminal", 30, tables.FACTION_MILITIA),
	mcs_kill("Traitor", 30, tables.FACTION_CONFEDS),
}

var unlocked = map[int]bool{}

func handle_file(filename string) {

	time.Sleep(5 * time.Second)

	fmt.Println("   Detected file", filename)
	fmt.Println()

	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		fmt.Println("Failed to load file", filename, "-", err)
		return
	}

	header := readers.Read_header(bytes)

	for i, cheev := range cheevz {
		if !unlocked[i] && cheev.test(header, bytes) {
			fmt.Println("Achivement!", cheev.name)
			fmt.Println(cheev.expl)
			fmt.Println()
			unlocked[i] = true
		}
	}

	fmt.Println("   Finished with file", filename)
	fmt.Println()
}
