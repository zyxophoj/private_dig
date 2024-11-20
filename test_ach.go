package main

import "fmt"
import "io/ioutil"
import "os"
import "strings"

import	"gopkg.in/ini.v1"

import 	"privdump/achievements"
import	"privdump/readers"
import 	"privdump/types"

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

func main() {

	cheev_map := map[string]func(types.Header, []byte, map[int]*types.Form) bool{}
	for _, list := range(achievements.Cheev_list){
		for _, cheev := range(list.Cheeves){
			cheev_map[cheev.Id] = cheev.Test
		}
	}

	test_dir := "ach_test"
	files, err := ini.Load(test_dir+"/files.ini")
	if err !=nil{
		fmt.Println("cant' even read ini file: %v", err)
		os.Exit(1)
	}
	
	error_count := 0

	for _,s := range(files.Sections()){
		if s.Name() != "DEFAULT" {
			for _,file := range(strings.Split(s.Key("yes").String(), ",")){
				filename := test_dir+"/"+strings.ToUpper(strings.TrimSpace(file))+".SAV"
				
				err, header, bytes, forms := read_file(filename)
				
				if err !=nil{
					fmt.Println("While loading file:", filename, ":", err)
					error_count+=1
					continue
				}
				
				_,exists:= cheev_map[s.Name()]
				if !exists{
					fmt.Println("Error: achievemnt", s.Name(), "does not exist")
					error_count+=1
					continue
				}
				
				result := cheev_map[s.Name()](header, bytes, forms)
				if !result{
					fmt.Println("File:", filename, "does not have achievement", s.Name())
					error_count+=1
				}
			}
			for _,file := range(strings.Split(s.Key("no").String(), ",")){
				filename := test_dir+"/"+strings.ToUpper(strings.TrimSpace(file))+".SAV"
				
				err, header, bytes, forms := read_file(filename)
				
				if err !=nil{
					fmt.Println("While loading file:", filename, ":", err)
					error_count+=1
					continue
				}
				
				_,exists:= cheev_map[s.Name()]
				if !exists{
					fmt.Println("Error: achievemnt", s.Name(), "does not exist")
					error_count+=1
					continue
				}
				
				result := cheev_map[s.Name()](header, bytes, forms)
				if result{
					fmt.Println("File:", filename, "has achievement", s.Name(), "and should not")
					error_count+=1
				}
			}
		}
	}	
}