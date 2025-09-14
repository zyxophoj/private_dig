# Privateer Save File Format #


## Disclaimer ##

As far as I am aware, no official documentation exists for this file format.  Everything in here has been determined from experimentation with save files and some guesswork.  There may be errors.


## Conventions ##

These are the types most commonly used by the savefile format.

- "int" means 16-bit signed little-endian int
- "long int" means 32-bit signed little-endian int
- "boolean" means a single byte - 1 for true, 0 for false


## Common Data structures ##

### Fixed string ###

A null-terminated byte string, with extra nulls at the end to pad it out to some fixed length.  When a length or max length is given, this is the maximum length of the bytes, so the actual length of the field is one byte more.

### Record ###

| Bytes | Content| Format |
|-------|--------|--------|
|  0-3  | Record name | 4 upper-case characters | 
|  4-7  | Data Length | 32-bit int, *big-endian* |
| 8-(7+length)|  Record data |  Depends on the record type |
| last? | Padding | trash |
 
 Notes:
 
 - "length" does not include the length of the name, the length of itself or the length of any padding
 - When the padding does exist, it is a single byte long.  Its value is the value of the next byte (does this matter?).  It appears if and only if the true record length would otherwise be odd.

### Form ###

| Bytes | Content| Format |
|-------|--------|--------|
|  0-3  | Record name |  Always "FORM" |
|  4-7  | Form length | 32-bit int, *big-endian* |
|  8-11 | Form name | 4 upper-case characters |
|  12-(7+length) | Records | records, one after the other |
|  (8+length)-??   | Footer | trash | 
 Notes:
 
 - As with records, this "length" does not include the length of the record name or the length of itself.  It does include the Form anme and the true length of each record. 
 - A form meets the definition of a record, and so a form may contain subforms, sub-sub-forms, etc.
 - A form will not ever have its own padding, because the pad bytes of the records inside it will force the whole form to have an even length. 
 - The footer is anything claimed by the form's length but not used by its records.  In practice, this only appears in Mission form chunks, is one or 2 bytes long, and overlaps the start of the next chunk.  This is probably just a case of the game calculating the form length incorrectly.
 
The FORM type and even the padding rules are very similar to IFF file format - https://en.wikipedia.org/wiki/Interchange_File_Format , which might explain the unnatural choice of big endian for the lengths.
 
 
### Blob ###

This is something of a non-definition; a blob is an array of bytes.


## File structure ## 

A savefile consists of a header followed by a number of chunks.  The header describes where to find the chunks.

### File Header ###

| Bytes | Content| Format |
|-------|--------|--------|
|  0-3  | File size | long int |
|  4-?  | Offsets | list of pointers? |

Each offset is a 4-byte "pointer" to a place within the file where a data chunk can be found.  The first 2 bytes are the actual location (int, presumably unsigned).  The last 2 bytes are always "00 E0" (maybe this is a thunk?)

The number of offsets varies because there are 2 for each non-plot mission.  (There are also 9 for the data that is always present)


### Chunk 1: Ship etc (Blob) ###

| Bytes | Content| Format |
|-------|--------|--------|
|  0    | Ship   | 0=Tarsus, 1=Orion, 2=Centurion, 3=Galaxy |
|  1    | Unused? | Always 0 |
|  2    | Location | See the BASE_ID enumeration in generated.go |
|  3-4  | Total missions accepted | int |
|  5    | Mercenaries guild member | boolean |
|  6    | Merchants guild member | boolean |
|  7-9  | Unused? | Always 0 |

Note: The length of this chunk is odd, and the length of the header (ann every other chunk) is even.  This means that every form and record in the following data starts on an odd byte, which is the exact opposite of what the IFF spec demands.


### Chunk 2: Plot status (10 bytes) ###

| Bytes | Content| Format |
|-------|--------|--------|
|  0-9  | Plot status | Fixed string |
| 10    | Flags  | Bitfield |

Notes:

 - The string  is usually "s"+(mission series number)+"m"+(mission letter).  e.g "s3mb".  However, an empty string indicates "Plot not yet started" and "FF FF FF FF FF FF FF FF" is used to indicate "generic unwinnable state".  In the very specific case of Monte's missions in RF, "Go to Drake and meet the informant" is represented by "s12mb1" and "Return to Monte" by "s12mb2"

| Series number | Fixer |
|---------------|-------|
| s0 | Sandoval |
| s1 | Tayla |
| s2 | Roman Lynch |
| s3 | Masterson |
| s4 | Lynn Murphy |
| s5 | Taryn Cross |
| s6 | Sandra Goodin |
| s7 | Admiral Terrell |
| s8 | Tayla (RF) |
| s9 | Murphy (RF) |
| s10 | Goodin (RF) |
| s11 | Masterson (RF) |
| s12 | Monte |
| s13 | Goodin 5 (RF) |
| s14 | Final |


 - Flags is a poorly-understood bitfield that contains mission status (accepted/succeeded/failed) 

| Bit | Meaning |
|-----|---------|
|  1  | Complete |
|  2  | Failed   |
| 64  | Very failed |
| 128 | Accepted  |

Additionally, 255 is a magical value meaning "complete".


### Chunk 3: Active non-plot mission count (blob) ###

| Bytes | Content| Format |
|-------|--------|--------|
| 0-1   | Mission count | int |

### Chunk 4-?? ###
The next 0-6 offsets are for the non-plot missions.  Each mission gets a header chunk and a main chunk.

#### Mission header chunk (blob) ####
Internal name of the mission - this is a string, not null-terminated

#### Mission body chunk (form) ####
A form called MSSN

This is not well understood right now.

Note: the remaining chunks are numbered as if there were 0 missions.

### Chunk 4: Kills and reputation (form) ###
A form called PLAY, containing 2 records called SCOR and KILL.

Each of these records contains 18 bytes of data, which is 9 int values storing reputation or kills for each faction.  The faction positions are:

 - 0-1: Merchants
 - 2-2: Hunters
 - 4-5: Confeds
 - 6-7: Kilrathi
 - 8-9: Militia
 - 10-11: Pirates
 - 12-13: Drone
 - 14-15: Steltek?
 - 16-17: Retros

12-13 representing the "drone" faction has been confirmed by killing the drone; the kill count does go up to 1 (unless the confeds steal the kill)

14-15 is speculative - nobody has managed to kill the steltek scout to confirm, and this is unlikely to change.

In the SCOR form, a reputation between -25 and 25 (inclusive) is Neutral.  Anything above 25 is Friendly; anything below -25 is Hostile.



### Chunk 5: Flags (blob) ### 

The first 11 bytes are not understood, and also not preserved by save-loading.  The rest is a list of boolean values, which store any plot or fixer state that can't be represented in the Plot status chunk.

 - In Privateer, that's a few details like temporarily rejecting a mission, or whether the Steltek drone has been angered.
 - In RF, it's pretty much the entire plot mission state, since multiple mission chains can be active at once and a simple plot string is completely incapable of dealing with that.

There are a lot of these, but they are listed in generated.go.

### Chunk 6: Hidden jump points (Form) ### 
A form, helpfully called SSSS, with records called ORIG and SECT.

ORIG contains originally hidden jump points, which is baffling - shouldn't a save file store current data, not starting data?  (It is, however one way to tell the diffrence between an original RF file and an imported-from-Privateer file)

SECT contains currently hidden jump points.

Each form's data contains an even number of bytes; each byte pair is the IDs of the two systems either side of the jump point.  Location IDs are in generated.go.



### Chunk 7: Ship equipment (Form) ###

A form called REAL.

All ship equipment, plus a few other things, is stored in here.  Many records are optional, because equipment might simply not be present.  Record order does not seem to matter.

#### FITE-CTRL ####
8 bytes, not understood

#### FITE_TRRT ####
list of installed turrets, 1 byte per turret.  Values are:

 - 1: Rear turret
 - 2: Top turret
 - 3: Bottom turret  

#### FITE_REPR ####

Repair bot.  Data is 4 bytes, which is probably a 16-bit int followed by 2 zeros.
The traditional options for the int are:

 - 400: Repair Droid
 - 200: Advanced Droid (RF only)

Other positive values work.  Repair speed is inversely proportional to the value.

#### FITE-AFTB ####
Afterburner.  Data length is 0

#### FITE-ECMS ####
ECM.  Data is just 1 byte, which appears to be ECM effectiveness (25, 50 or 75)

#### FITE-NAVQ ####
Quadrant maps.  Data is just 1 byte, although it is a bitfield with the following values:

| Bit | Meaning |
|-----|---------|
| 1   | Humboldt |
| 2   | Fariss |
| 4   | Potter |
| 8   | Clarke |


#### FITE-WEAP-GUNS ####

Data is a list of 4-byte gun entries.
Each gun entry is built as follows:

| Bytes | Content| Format |
|-------|--------|--------|
| 0     | Gun type  |    |
| 1     | Gun mount |    |
| 2-3   | damage    |  ? |

(Damage is damage taken by the gun object, not damage done by firing it.

#### FITE-WEAP-LNCH ####

Data is a list of 4-byte launcher entries.
Each gun entry is built as follows:


| Bytes | Content| Format |
|-------|--------|--------|
| 0     | Launcher type  |    |
| 1     | Launcher mount |    |
| 2-3   | damage    |  ? |

Types and mounts are in tables.go

#### FITE-WEAP-MISL ####

Data is a list of 3-byte missile stack entries.
Each missile stack entry is built as follows:

| Bytes | Content| Format |
|-------|--------|--------|
| 0     | missile type |     |
| 1-2   | stack size  | int |

It is possible to have up to 32767 of each missile!  The game doesn't seem to mind, although the ship dealer displays the missile count incorrectly.


#### FITE-ENER-INFO ####

"ENER" means engines, which are essentially the ship's power plant.

| Bytes | Content| Format |
|-------|--------|--------|
| 0-7   | Info name   | Always "ENERGY\0\0" |
| 8-??  | Engine info | Unknown |

Engine info is a list of bytes with small values - no larger than 6.  It is possible that each 2-byte chunk is a unit; for ecample "124151" may represent "2 components of type, 1 component of type 4, 1 component of type 5", although this is just guesswork. 

| Engine info | Engine |
|-------------|--------|
| 1261         | (None)  |
| 124151       | Level 1 |
| 12314151     | Level 2 |
| 123141516    | Level 3 |
| 122131415161 | Level 4 |
| 122131415162 | Level 4 |
| 122231415162 | Level 5 |
| 122331415162 | Level 6 |
| 122431415162 | Level 7 |

#### FITE-ENER-DAMG ####

#### FITE-SHLD-INFO ####

| Bytes | Content| Format |
|-------|--------|--------|
| 0-7   | Info name  | Always "SHIELDS\0" |
| 8     | Shield ID | uint8 |

Shield ID is a 89-based integer, meaning level 1 shields are represented by 90, level 2 by 91... up to 96 for level 7.  If no shields are installed, this record is simply not present.

#### FITE-SHLD-ARMR ####

Armour status.  This is always 16 bytes.

| Bytes | Content| Format |
|-------|--------|--------|
| 0-1   | Max Left  | int |
| 2-3   | Max Right | int |
| 4-5   | Max Front | int |
| 6-7   | Max Back  | int |
| 8-9   | Current Left  | int |
| 10-11 | Current Right | int |
| 12-13 | Current Front | int |
| 14-15 | Current Back  | int |

The first 4 ints will always be the same, and will be one of the armour ID values.  The second set of 4 ints represent how much armour is left, and so could be anything from 0 to the corresponding max value.

This means that the savefile is only storing armour type and armour damage as a fraction... so Tarsus armour looks exactly the same as Orion armour, even though it is about a quarter of the thickness!  (The missing thickness data is in the PRIV.TRE file)

| Armour | Armour ID |
|--------|-----------|
| None      | 0    |
| Also None | 1    |
| Plasteel  | 250  |
| Tungsten  | 500  |
| Isometal  | 3000 |

Notes:
 - Armour ID appears to be doing double duty as armour strength.  This is certainly true in RF, where a crippled Orion in Isometal armour takes more time to die than the Twelfth Doctor.  In the base game, experimentation suggests that Tungsten armour may be a scam.
 - "No armour" starts with ID 0 but, after launch-landing, has ID 1.  The current values remain 0.  This is probably because armour is displayed on screen with a thickness proportional to (current amrour)/(maximum armour), so bumping the maximum up to 1 prevents division by zero.
 
#### FITE-SHLD-DAMG ####
 2 bytes, looks like an int with 0 for "undamaged" and positive values for varying degrees of damage.

#### FITE-TRGT-INFO ####

The ship's scanner.

| Bytes | Content| Format |
|-------|--------|--------|
| 0-7   | Info name  | Always "TARGETNG" |
| 8     | Scanner ID | uint8 |

Scanner ID is a number from 60 to 68.  

Scanner locking ability is (Scanner ID-60)%3, and scanner colourfulness is (Scanner ID-60)/3 (rounded down)

#### FITE-TRGT-DAMG ####

2 bytes, looks like an int with 0 for "undamaged" and positive values for varying degrees of damage.
 
#### FITE-CRGO-INFO ####

It is far from clear what, if anything, this does.

| Bytes | Content| Format |
|-------|--------|--------|
| 0-7   | Info name  | Always "CARGO\0\0\0" |
| 8     | Always 0? | Always 0? |

#### FITE-CRGO-CRGI ####

Miscellaneous, vaguely cargo-related information.

| Bytes | Content| Format |
|-------|--------|--------|
| 0-3   | credits  | long int |
| 4-5   | capacity | int |
|  6    | secret compartment | boolean |
|  7    | expansion | boolean |

#### FITE-CRGO-DATA ####

A list of 4-0 byte cargo entries.  Each entry is built as follows:

| Bytes | Content| Format |
|-------|--------|--------|
| 0     | cargo type  | see tables.go |
| 1-2   | amount (T) | int |
| 3     | hidden     | boolean |

#### FITE-JDRV-INFO ####

Jump drive info.


| Bytes | Content| Format |
|-------|--------|--------|
| 0-1   | current fuel  | int |
| 2-3   | max fuel | int |

Notes:
 - Max fuel is always 6.  This may be a remnant from pre-relase versions where there was more than one class of jump drive.
 - Since ships are automatically refuleled when they are docked, current fuel is always 6.
 - The game will crash if the front view is shown with more than 6 units of fuel (probably because it doesn't know how to draw the fuel-o-meter).  However, if you switch to any other view immediately after launch, the game is playable (to the extent that it can be without a front view) and the extra jump capacity can be used!
 
 #### FITE-JDRV-DAMG ####
 
 2 bytes, looks like an int with 0 for "undamaged" and positive values for varying degrees of damage.
 
 #### FITE-JDRV-DAMG-DAMG ####

Sup Dawg I heard you like jump drive damage?  This really does not make sense.

### Chunk 8: Name (string) ###
Fixed string, length 17

### Chunk 9: Callsign (string) ###
Fixed string, length 14

