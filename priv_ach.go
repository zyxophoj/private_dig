package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/ini.v1"

	"privdump/achievements"
	"privdump/readers"
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
	Unlocked map[string]map[string]bool // cheeves earned
	Visited  map[string]map[uint8]bool  // locations visited
	Secrets  map[string]*uint8          // which ships have had the secret compartment
}{map[string]map[string]bool{}, map[string]map[uint8]bool{}, map[string]*uint8{}}

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
		{"show", 1, "Show achievements for an identity"},
		{"show_missing", 1, "Show missing achievements for an identity"},
		{"run", 0, "Run and monitor achievements.  Also the default."},
	}
	
	flags:=map[string]bool{
		"--rf":false,
	}

	main_arg := ""
	subargs := []string{}
	subargs_needed := 0
	for _, arg := range os.Args[1:] {
		_, is_flag :=flags[arg]
		if is_flag {
			flags[arg]=true
			fmt.Println("fl;ags", arg)
			continue
		}
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
	state_file = filepath.Join(dir, "pracst.json")

	switch main_arg {
	case "help":
		for _, info := range arg_info {
			fmt.Println(info.arg, "-", info.desc)
		}

	case "check":
		fmt.Println("Target dir is: " + dir)

	case "list":
		load_state()
		if len(global_state.Unlocked) == 0 {
			fmt.Println("(no profiles detected")
			os.Exit(0)
		}

		for p := range global_state.Unlocked {
			fmt.Println(p)
		}

	case "show":
		fmt.Println("Showing achevements for", subargs[0])
		fmt.Println()

		load_state()
		got := global_state.Unlocked[subargs[0]]
		ttotal := 0
		for _, cat_list := range achievements.Cheev_list {
			cat_list.Cheeves = append(cat_list.Cheeves, achievements.Cheev_list_rf[cat_list.Category]...)
			total := len(cat_list.Cheeves)
			ttotal += total
			indices := []int{}
			for i, cheev := range cat_list.Cheeves {
				if got[cheev.Id] {
					indices = append(indices, i)
				}
			}
			fmt.Println(fmt.Sprintf("%v (%v/%v):", cat_list.Category, len(indices), total))
			for _, i := range indices {
				fmt.Println("   " + cat_list.Cheeves[i].Name)
				fmt.Println("   (" + cat_list.Cheeves[i].Expl + ")")
				fmt.Println()
			}
			fmt.Println()
		}
		fmt.Println(fmt.Sprintf("Overall: %v/%v", len(got), ttotal))

	case "show_missing":
		fmt.Println("Showing missing achevements for", subargs[0])
		fmt.Println()

		load_state()
		got := global_state.Unlocked[subargs[0]]

		// TODO:  a per-character "rf" flag would help here.
		is_rf := flags["--rf"]
		for cheev := range got {
			if strings.HasPrefix(cheev, "AID_RF") {
				is_rf = true
				break
			}
		}
		for _, cat_list := range achievements.Cheev_list {
			if is_rf {
				cat_list.Cheeves = append(cat_list.Cheeves, achievements.Cheev_list_rf[cat_list.Category]...)
			}
			total := len(cat_list.Cheeves)
			indices := []int{}
			for i, cheev := range cat_list.Cheeves {
				if !got[cheev.Id] {
					indices = append(indices, i)
				}
			}
			if len(indices) > 0 {
				fmt.Println(fmt.Sprintf("%v (%v/%v):", cat_list.Category, len(indices), total))
				for _, i := range indices {
					fmt.Println("   " + cat_list.Cheeves[i].Name)
					fmt.Println("   (" + cat_list.Cheeves[i].Expl + ")")

					if cat_list.Cheeves[i].Multi {
						arg := achievements.Arg{types.Header{}, nil, nil, types.GT_PRIV, global_state.Visited[subargs[0]], global_state.Secrets[subargs[0]], ""}
						cat_list.Cheeves[i].Test(&arg)
						if arg.Progress != "" {
							fmt.Println("   Progress: " + arg.Progress)
						}
					}

					fmt.Println()
				}
				fmt.Println()
			}
		}

	case "run":
		main_run(dir)
	}

	os.Exit(0)
}

func main_run(dir string) {
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
					if strings.HasSuffix(event.Name, ".SAV") || strings.HasSuffix(event.Name, ".PRS") {
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

var last_identity = ""

func handle_file(filename string) {
	// Wait for Privateer itself to finish with the file
	time.Sleep(5 * time.Second)

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
	identity := name + ":" + callsign

	// Set up proper "uninitialised" values
	_, ok := global_state.Visited[identity]
	if !ok {
		global_state.Visited[identity] = map[uint8]bool{}
	}
	_, ok = global_state.Secrets[identity]
	if !ok {
		global_state.Secrets[identity] = new(uint8)
	}

	// We're dealing with RF iff the Valhalla<->Gaea jump point was originally hidden.
	game := types.GT_PRIV
	hidden := forms[types.OFFSET_SSSS].Get("ORIG").Data
	if hidden[len(hidden)-1] == 68 {
		game = types.GT_RF
		//fmt.Printf("RF mode engaged!")
	}

	arg := achievements.Arg{header, bytes, forms, game, global_state.Visited[identity], global_state.Secrets[identity], ""}

	update_visited(&arg)

	// update secret compartment status
	if arg.Forms[types.OFFSET_REAL].Get("FITE", "CRGO", "CRGI").Data[6] != 0 {
		*arg.Secrets = *arg.Secrets | (1 << arg.Offset(types.OFFSET_SHIP)[0])
	}

	for _, list := range achievements.Cheev_list {

		cheeves := list.Cheeves
		if arg.Game == types.GT_RF {
			cheeves = append(cheeves, achievements.Cheev_list_rf[list.Category]...)
		}

		for _, cheev := range cheeves {

			if last_identity != identity {
				fmt.Println("Identity is", identity)
				fmt.Println()
				last_identity = identity
			}

			// Really not a fan of panic-recover, but I suppose there's a case for it here
			// Recovering will prevent a shittily-written cheev test from bringing the entire app down.
			ct_wrap := func(a *achievements.Achievement, header types.Header, bytes []byte, form map[int]*types.Form) bool {
				defer func() {
					if recover() != nil {
						fmt.Println("Something went *very* wrong when calculating achievement \"" + a.Name + "\":")
						debug.PrintStack()
						// If this happens, the ct_wrap function returns the default value, which is false
					}
				}()

				return a.Test(&arg)
			}

			if !global_state.Unlocked[identity][cheev.Id] && ct_wrap(&cheev, header, bytes, forms) {
				fmt.Println(cheev.Name)
				fmt.Println(cheev.Expl)
				fmt.Println("Category:", list.Category)
				fmt.Println()

				_, ok := global_state.Unlocked[identity]
				if !ok {
					global_state.Unlocked[identity] = map[string]bool{}
				}
				global_state.Unlocked[identity][cheev.Id] = true
			}
		}
	}

	save_state()
	//fmt.Println("   Finished with file", filename)
	//fmt.Println()
}

// update_visited updates the visited list based on file state.
// In addition to the obvious current_location, any base the player has provably been to
// should be added to the Visited list.
//
// TODO: is this actually the constructor for achievements.Arg?
func update_visited(arg *achievements.Arg) {
	arg.Visited[arg.Location()] = true // current location

	switch arg.Game {
	case types.GT_PRIV:

		arg.Visited[0] = true // Achilles, the starting location

		// Locations that must have been visited to advance the plot.
		//
		// The best we can do without using the poorly understood flag byte is to detect if a mission has been accepted.
		// This doesn't work perfectly - for example, New Constantinople is provably visited on completion of Tayla 3,
		// but we only acknowledge the start of Tayla 4.
		infos := []struct {
			plot     string
			location uint8
		}{
			{"s0ma", 32}, // New Detroit
			{"s1mb", 36}, // Oakham
			{"s1mc", 15}, // Hector
			{"s1md", 31}, // New Constantinople
			{"s2mc", 48}, // Siva
			{"s2md", 42}, // Remus
			{"s3ma", 39}, // Oxford
			{"s4ma", 3},  // Basra
			{"s4md", 40}, // Palan
			{"s5ma", 46}, // Rygannon
			{"s6ma", 59}, // Derelict
			{"s7mb", 41}, // Perry
		}

		str, _ := arg.Plot_info()
		if len(str) == 4 {
			for _, info := range infos {
				if str >= info.plot {
					arg.Visited[info.location] = true
				}
			}
		}

		// Either Steltek gun proves the player got to the derelict
		guns := arg.Forms[types.OFFSET_REAL].Get("FITE", "WEAP", "GUNS")
		if guns != nil {
			for n := 0; n < len(guns.Data); n += 4 {
				if guns.Data[n] >= 8 {
					for _, info := range infos {
						arg.Visited[info.location] = true
						if info.location == 59 && guns.Data[n] == 9 {
							break
						}
					}
				}
			}
		}

		// TODO: theoretically, the player could have visited the derelict and not picked up a gun
		// Ths should in principle be detectable by the "angry drone" state, but where is that stored in the save file?
		// Also, who does that?

	case types.GT_RF:
		arg.Visited[18] = true // Jolson, the starting location
	}
}
