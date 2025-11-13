package main

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/ini.v1"

	"privdump/achievements"
	"privdump/priv_ach"
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

	flags := map[string]bool{
		"--rf": false,
	}

	main_arg := ""
	subargs := []string{}
	subargs_needed := 0
	for _, arg := range os.Args[1:] {
		_, is_flag := flags[arg]
		if is_flag {
			flags[arg] = true
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

	switch main_arg {
	case "help":
		for _, info := range arg_info {
			fmt.Println(info.arg, "-", info.desc)
		}

	case "check":
		fmt.Println("Target dir is: " + dir)

	case "list":
		state := priv_ach.GetState(dir)
		if len(state.Unlocked) == 0 {
			fmt.Println("(no profiles detected")
			os.Exit(0)
		}

		for p := range state.Unlocked {
			fmt.Println(p)
		}

	case "show":
		fmt.Println("Showing achevements for", subargs[0])
		fmt.Println()

		state := priv_ach.GetState(dir)
		got := state.Unlocked[subargs[0]]
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

		state := priv_ach.GetState(dir)
		has := state.Unlocked[subargs[0]]

		// TODO:  a per-character "rf" flag would help here.
		is_rf := flags["--rf"]
		for cheev := range has {
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
				if !has[cheev.Id] {
					indices = append(indices, i)
				}
			}
			if len(indices) > 0 {
				fmt.Println(fmt.Sprintf("%v (%v/%v):", cat_list.Category, len(indices), total))
				for _, i := range indices {
					fmt.Println("   " + cat_list.Cheeves[i].Name)
					fmt.Println("   (" + cat_list.Cheeves[i].Expl + ")")

					if cat_list.Cheeves[i].Multi {
						arg := achievements.Arg{types.Savedata{}, state.Visited[subargs[0]], state.Secrets[subargs[0]], ""}
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
		cheeves := make(chan *priv_ach.Achievement)
		watcher := priv_ach.New_watcher(dir)
		go func() {
			for {
				select {
				case cheev := <-cheeves:
					fmt.Println(cheev.Name)
					fmt.Println(cheev.Desc)
					fmt.Println("Category:", cheev.Category)
					fmt.Println()
				}
			}
		}()

		err := watcher.Start_watching(cheeves)
		if err != nil {
			fmt.Println(err)
			return
		}

		fmt.Println("Watching...", dir)
		fmt.Println()
		fmt.Println()

		// Wait forever!
		// TODO: some clean way to detect a quit key and call watcher.Stop_watching()
		<-make(chan bool)
	}

	os.Exit(0)
}
