package main

import "fmt"
import "io/ioutil"
import "os"
import "strings"

import "gopkg.in/ini.v1"

import "privdump/achievements"
import "privdump/readers"
import "privdump/types"

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

func main() {

	cheev_map := map[string]func(a *achievements.Arg) bool{}
	for _, list := range achievements.Cheev_list {
		for _, cheev := range list.Cheeves {
			cheev_map[cheev.Id] = cheev.Test
		}
	}
	for _, list := range achievements.Cheev_list_rf {
		for _, cheev := range list {
			cheev_map[cheev.Id] = cheev.Test
		}
	}



	test_dir := "ach_test"
	files, err := ini.Load(test_dir + "/files.ini")
	if err != nil {
		fmt.Println("cant' even read ini file: %v", err)
		os.Exit(1)
	}

	error_count := 0

	for _, s := range files.Sections() {
		if s.Name() != "DEFAULT" {
			game := types.GT_PRIV
			ext := ".SAV"
			if strings.HasPrefix(s.Name(), "AID_RF_"){
				game=types.GT_RF
				ext=".PRS"
			}
			for _, expected := range []bool{true, false} {
				for _, file := range strings.Split(s.Key(boolmap("yes", "no")[expected]).String(), ",") {
					filename := test_dir + "/" + strings.ToUpper(strings.TrimSpace(file))
					if !strings.Contains(file, "."){
						filename += ext
					} else {
						game = types.GT_RF
					}

					err, header, bytes, forms := read_file(filename)

					if err != nil {
						fmt.Println("While loading file:", filename, " for "+s.Name()+":", err)
						error_count += 1
						continue
					}

					_, exists := cheev_map[s.Name()]
					if !exists {
						fmt.Println("Error: achievment", s.Name(), "does not exist")
						error_count += 1
						continue
					}

					if cheev_map[s.Name()](&achievements.Arg{header, bytes, forms, game, nil, nil, ""}) != expected {
						fmt.Printf(boolmap("File: %s does not have achievement %s\n", "File: %s has achievement %s but should not\n")[expected], filename, s.Name())
						error_count += 1
					}
				}
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
