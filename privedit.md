## Privedit - privateer save file editor


### How to get it

Go to the releases page (https://github.com/zyxophoj/private_dig/releases) and download the zip file for the latest release.  Unzip it somewhere.


### How to run it

- Edit the priv_ach.ini file.  There is only one setting, which should be the location of your Privateer saved games.  Change it if it is wrong.
- Run privedit


### Some example commands

This should be sufficient to turn the starting Tarsus into a hilariously overpowered killing machine...

```
privedit load savefile.sav
privedit set ship Centurion
privedit set credits 10000000
privedit set location new_d
privedit set engine 5
privedit set shield 5
privedit set guns "left:Boosted Steltek gun"
privedit set guns left_outer:boo
privedit set guns right:boo
privedit set guns right_o:boo
privedit set missiles Image:32000
privedit set launchers left:miss
privedit set launchers right:miss
privedit set launchers turret_1:miss
privedit set turrets rear:present
privedit set reputation retros:100
privedit set name Filthy
privedit set callsign Cheater
privedit save
```

### Notes on saving

Privedit tries to make savefiles that will not crash immediately on load or launch.  The usual problems here are mounted equipment (guns, launchers, turrets) on mounts that don't exist, or length mismatches between equipment data and the corresponding damage data.  For example, there are actually 5 front launcher mounts: two left mounts, one centre, and two right mounts.  The Tarsus uses different left and right mounts to the Centurion and Galaxy, and so changing the ship type might result in a broken file.  This sort of thing is automatically fixed in the "sanity fix" stage at the start of saving, but it could result in equipment destruction.

With that said, the game is completely fine with other seemingly-invalid setups, including:

- Missile stack sizes up to 32767
- Missile launchers in turrets
- Missile launchers in turrets that could exist, but don't. They can even be fired from the front view.
- Level 5 engines and shields on ships that are not an Orion
- Multiple boosted steltek guns

... so privedit will let you do that.  Have fun.

### things privedit can't do

- Change plot status and flags (coming soon)
- Change a lot of equipment.  The priority is making "illegal" equipment setups possible.  For other equipment needs, just give yourself millions of credits and buy the equipment normally.