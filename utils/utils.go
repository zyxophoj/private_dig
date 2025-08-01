package utils

import "strconv"
import "privdump/types"

func Make_flags() map[types.Game]map[int]string {
	
	flags := map[types.Game]map[int]string{
				types.GT_PRIV: map[int]string{
					1:  "Tayla Hello",
					2:  "Tayla Goodbye",
					3:  "Masterson Hello",
					4:  "Masterson/Murphy Goodbye",
					5:  "Monkhouse Goodbye",
					6:  "Cross Goodbye",
					9:  "Current plot mission rejected",
					10: "Monkhouse Goodbye 2",
					11: "Angry Drone",
					12: "Terrel in credits mode",
				},

				// RF has offered/accepted/done flags for each mission.
				// This is needed because it's possible to do the first 4 sets of missions in any order
				// (but it's still buggy, e.g doing Murphy 1 and landing on Oxford somehow fails to set the "murphy 1 done" flag)

				//  1-24: "Mission offered" flags
				// 25-48: "Mission accepted" flags
				// 49-58 Miscellaneous
				// 59-154: unused?
				// 155-176: "Mission done" flags (Mostly, Roman Lynch's free reset is in there too, WTF?)
				types.GT_RF: map[int]string{
					49: "Roman Lynch introduced",
					//50: ????
					51: "Tayla Gone",
					52: "Murphy bounty paid",
					53: "Goodin bounty offered",
					54: "Monte unlocked",
					55: "Monte gone",
					57: "Informant gone",
					//56: ????
					58: "Terrell in credits mode",
				}}


				offered, accepted, done := 1, 25, 151
				fixers := []string{"Tayla", "Murphy", "Goodin", "Masterson"}
				add_flags := func(name string) {
					flags[types.GT_RF][offered] = name + " offered"
					flags[types.GT_RF][accepted] = name + " accepted"
					flags[types.GT_RF][done] = name + " done"
					offered, accepted, done = offered+1, accepted+1, done+1
				}
				for _, f := range fixers {
					nm := 4
					// Special case: Masterson gives 5 missions
					if f == "Masterson" {
						nm += 1
					}
					for m := range nm {
						add_flags(f + " " + strconv.Itoa(m+1))
					}
				}

				// Special case: Monte "done" list has 2a and 2b, but no 4
				// (This doesn't affect "offered" or "accepted" because the intformant sub-mission can't be refused")
				for m := range 4 {
					name := "Monte" + " " + strconv.Itoa(m+1)
					flags[types.GT_RF][offered] = name + " offered"
					flags[types.GT_RF][accepted] = name + " accepted"
					flags[types.GT_RF][done] ="Monte " + []string{"1", "2a", "2b", "3"}[m] + " done"
					offered, accepted, done = offered+1, accepted+1, done+1
				}

				// Special (as a euphemism for "retarded") case: what is this doing here???
				flags[types.GT_RF][done] = "Roman Lynch Free Reset unavailable"
				done += 1

				// last 3 missions are refreshingly normal
				for _, name := range []string{"Goodin 5", "Terrell", "Go to Gaea"} {
					add_flags(name)
				}

				// ...and one extra flag
				// (begging for forgiveness auto-accepts the final mission)
				flags[types.GT_RF][done] = "Kill Jones done"
				done += 1


	return flags
}