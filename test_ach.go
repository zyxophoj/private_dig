package main

import "fmt"
import "io/ioutil"
import "os"
import "strings"

import "gopkg.in/ini.v1"

import "privdump/achievements"
import "privdump/readers"
import "privdump/types"
import "privdump/tables"

var test_dir = "ach_test"

func read_file(filename string) (error, types.Header, []byte, map[int]*types.Form) {
	h0 := types.Header{}

	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		fmt.Println("Failed to load file", filename, "-", err)
		return err, h0, nil, nil
	}

	header := readers.Read_header(bytes)

	forms := map[int]*types.Form{}
	for _, i := range []int{types.OFFSET_PLAY, types.OFFSET_SSSS, types.OFFSET_REAL} {
		cur := header.Offsets[i]
		f, err := readers.Read_form(bytes, &cur)
		if err != nil {
			fmt.Println("Failed to load form", i, "-", err)
			return err, h0, nil, nil
		}
		forms[i] = &f
	}

	return nil, header, bytes, forms
}

func boolmap[K any](t K, f K) map[bool]K {
	return map[bool]K{true: t, false: f}
}

// real_filename constructs an actual filename from a string in the ini file.
// These strings typically don't have extensions, so guess the extension based on
// RF-ness.
// However, if the string does have an extension, use it, and that can force
// the achievement test to run in RF mode.
func real_filename(file string, RF bool) (types.Game, string) {
	game := types.GT_PRIV
	ext := ".SAV"
	if RF {
		game = types.GT_RF
		ext = ".PRS"
	}
	filename := test_dir + "/" + strings.ToUpper(strings.TrimSpace(file))
	if !strings.Contains(file, ".") {
		filename += ext
	} else {
		game = types.GT_RF
		if RF && !strings.HasSuffix(filename, ".PRS") {
			// This would be a bug in the ini file
			fmt.Println("Error: Non-RF file read in RF mode")
		}
	}

	return game, filename
}

func main() {

	cheev_map := map[string]achievements.Achievement{}
	for _, list := range achievements.Cheev_list {
		for _, cheev := range list.Cheeves {
			cheev_map[cheev.Id] = cheev
		}
	}
	for _, list := range achievements.Cheev_list_rf {
		for _, cheev := range list {
			cheev_map[cheev.Id] = cheev
		}
	}

	files, err := ini.Load(test_dir + "/files.ini")
	if err != nil {
		fmt.Println("cant' even read ini file: %v", err)
		os.Exit(1)
	}

	error_count := 0

	for _, s := range files.Sections() {
		if s.Name() != "DEFAULT" {

			cheev, exists := cheev_map[s.Name()]
			if !exists {
				fmt.Println("Error: achievment", s.Name(), "does not exist")
				error_count += 1
				continue
			}
			is_rf := strings.HasPrefix(s.Name(), "AID_RF_")
			if !cheev.Multi {
				for _, expected := range []bool{true, false} {
					for _, file := range strings.Split(s.Key(boolmap("yes", "no")[expected]).String(), ",") {

						game, filename := real_filename(file, is_rf)
						err, header, bytes, forms := read_file(filename)

						if err != nil {
							fmt.Println("While loading file:", filename, " for "+s.Name()+":", err)
							error_count += 1
							continue
						}

						if cheev.Test(&achievements.Arg{header, bytes, forms, game, nil, nil, ""}) != expected {
							fmt.Printf(boolmap("File: %s does not have achievement %s\n", "File: %s has achievement %s but should not\n")[expected], filename, s.Name())
							error_count += 1
						}
					}
				}
			} else {
				files := strings.Split(s.Key("multi").String(), ",")

				// The full list of files should get the achievement, but lists obtained by removing any one file should not.
				file_lists := [][]string{files}
				expected := []bool{true}
				for i, _ := range files {
					// This is necessary because append modifies its arguments... ouch!
					files2 := append([]string{}, files...)

					file_lists = append(file_lists, append(files2[:i], files2[i+1:]...))
					expected = append(expected, false)
				}

				for i, list := range file_lists {
					visited := map[tables.BASE_ID]bool{}
					secrets := uint8(0)

					result := false
					prog:=""
					for _, file := range list {
						game, filename := real_filename(file, is_rf)
						err, header, bytes, forms := read_file(filename)
						if err != nil {
							fmt.Println("While loading file:", filename, " for "+s.Name()+":", err)
							error_count += 1
							goto cheev_end
						}
						arg := achievements.Arg{header, bytes, forms, game, visited, &secrets, ""}
						arg.Update()

						result = cheev.Test(&arg)
						prog=arg.Progress
					}

					if result != expected[i] {
						fmt.Printf(boolmap("File list: %s does not have achievement %s (%s)\n", "File list: %s has achievement %s but should not%s\n")[expected[i]], list, s.Name(), prog)
						error_count += 1
					}
				}
			cheev_end:
			}
			delete(cheev_map, s.Name())
		}
	}

	if error_count > 0 {
		fmt.Println(error_count, "errors!!!")
	}

	if len(cheev_map) > 0 {
		fmt.Println("Untested:", len(cheev_map))
		for k := range cheev_map {
			fmt.Println(k)
		}
	}
}
