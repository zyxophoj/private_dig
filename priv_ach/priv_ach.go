package priv_ach

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	
	"privdump/achievements"
	"privdump/tables"
	"privdump/types"
)

type Achievement struct {
	Name     string
	Desc     string
	Category string
}

const (
	// a bitfield with room for expansion
	FLAG_RF = 1
)

type state_type struct {
	Unlocked map[string]map[string]bool         // cheeves earned
	Visited  map[string]map[tables.BASE_ID]bool // locations visited
	Secrets  map[string]*uint8                  // which ships have had the secret compartment
}

type Priv_ach interface {
	Start_watching(cheeves chan<- *Achievement) error
	Stop_watching()
}

func New_watcher(dir string) Priv_ach {
	return &dir_watcher{dir, "", nil, state_type{}}
}

type dir_watcher struct {
	dir           string
	last_identity string
	watcher       *fsnotify.Watcher
	state         state_type
}

func (dw *dir_watcher) Start_watching(cheeves chan<- *Achievement) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	dw.watcher = watcher
	dw.load_state()

	go func() {
		// Watcher thread - detects "file saved" events, waits for 5 seconds, then calls handle_file
		// We wait without blocking because the watcher probably wouldn't like that.
		delay := time.Second*5
		
		tasks := map[string]time.Time{}
		alert := make(chan bool)
		for {
			select {
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				// TODO: report error!
				fmt.Println(err)
			
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Write) {
					if strings.HasSuffix(strings.ToUpper(event.Name), ".SAV") || strings.HasSuffix(strings.ToUpper(event.Name), ".PRS") {
						// Wait until Privateer itself has finished writing to the file
						// This may overwrite an existing task time (if it does, that old time is no longer valid anyway)
						tasks[event.Name] = time.Now().Add(delay)
						time.AfterFunc(delay, func(){alert<-true})
					}
				}
			case <-alert:
				done := []string{}
				for filename, t := range(tasks){
					if time.Now().After(t){
						dw.handle_file(filename, cheeves)
						done=append(done, filename)
					}
				}
					
				for _,f:=range(done){
					delete(tasks, f)
				}
			}
		}
	}()
	
	err = dw.watcher.Add(dw.dir)
	if err != nil {
		dw.watcher.Close()
	}

	return err
}

func (dw *dir_watcher) Stop_watching() {
	dw.watcher.Close()
}

func (dw *dir_watcher) save_state() {
	state_file := filepath.Join(dw.dir, "pracst.json")
	b, _ := json.Marshal(dw.state)
	os.WriteFile(state_file, b, 0644)
}

func (dw *dir_watcher) load_state() {
	state_file := filepath.Join(dw.dir, "pracst.json")
	bytes, _ := os.ReadFile(state_file)
	json.Unmarshal(bytes, &dw.state)
}

func GetState(dir string) *state_type {
	state := state_type{}
	state_file := filepath.Join(dir, "pracst.json")
	bytes, _ := os.ReadFile(state_file)
	json.Unmarshal(bytes, &state)
	return &state
}

func (dw *dir_watcher) handle_file(filename string, out chan<- *Achievement) {
	// TODO : report errors better - this will have to wait until we have a real logging system.

	reader, err := os.Open(filename)
	if err != nil {
		fmt.Println("Failed to load file", filename, "-", err)
		return
	}
	savedata, err := types.Read_savedata(reader)
	if err != nil {
		fmt.Println("Failed to parse file", filename, "-", err)
		return
	}
	identity := savedata.Strings[types.OFFSET_NAME].Value + ":" + savedata.Strings[types.OFFSET_CALLSIGN].Value

	// Set up proper "uninitialised" values
	_, ok := dw.state.Visited[identity]
	if !ok {
		dw.state.Visited[identity] = map[tables.BASE_ID]bool{}
	}
	_, ok = dw.state.Secrets[identity]
	if !ok {
		dw.state.Secrets[identity] = new(uint8)
	}

	arg := achievements.Arg{*savedata, dw.state.Visited[identity], dw.state.Secrets[identity], ""}
	arg.Update()

	for _, list := range achievements.Cheev_list {

		cheeves := list.Cheeves
		if arg.Savedata.Game() == types.GT_RF {
			cheeves = append(cheeves, achievements.Cheev_list_rf[list.Category]...)
		}

		for _, cheev := range cheeves {

			if dw.last_identity != identity {
				fmt.Println("Identity is", identity)
				fmt.Println()
				dw.last_identity = identity
			}

			// Really not a fan of panic-recover, but I suppose there's a case for it here
			// Recovering will prevent a shittily-written cheev test from bringing the entire app down.
			ct_wrap := func(a *achievements.Achievement, arg achievements.Arg) bool {
				defer func() {
					if recover() != nil {
						fmt.Println("Something went *very* wrong when calculating achievement \"" + a.Name + "\":")
						debug.PrintStack()
						// If this happens, the ct_wrap function returns the default value, which is false
					}
				}()

				return a.Test(&arg)
			}

			if !dw.state.Unlocked[identity][cheev.Id] && ct_wrap(&cheev, arg) {
				out <- &Achievement{cheev.Name, cheev.Expl, list.Category}

				_, ok := dw.state.Unlocked[identity]
				if !ok {
					dw.state.Unlocked[identity] = map[string]bool{}
				}
				dw.state.Unlocked[identity][cheev.Id] = true
			}
		}
	}

	dw.save_state()
	//fmt.Println("   Finished with file", filename)
	//fmt.Println()
}
