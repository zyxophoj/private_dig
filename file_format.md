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
| last? | Padding | Any extra trailing data not claimed by the "length" field. |
 
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
 
 Notes:
 
 - As with records, this "length" does not include the length of the record name or the length of itself.  It does include the Form anme and the true length of each record. 
 - A form meets the definition of a record, and so a form may contain subforms, sub-sub-forms, etc.
 - A form will not ever have its own padding, because the pad bytes of the records inside it will force the whole form to have an even length. 
 
The FORM type and even the padding rules are very similar to IFF file format - https://en.wikipedia.org/wiki/Interchange_File_Format , which might explain the unnatural choice of big endian for the lengths.
 
 
### Blob ###

This is something of a non-definition; a blob is an array of bytes.


## File structure ## 

A savefile consists of a header followed by a number of chunks.  The header describes where to find the chunks.

### File Header ###

 - Bytes 0-4: File size (long int)
 - Bytes 4-?: Offsets

Each offset is a 4-byte "pointer" to a place within the file where a data chunk can be found.  The first 2 bytes are the actual location (int, presumably unsigned)

The last 2 bytes are always "00 E0" (maybe this is a thunk?)

The number of offsets varies because there are 2 for each non-plot mission.  (There are also 9 for the data that is always present)


### Chunk 1: Ship etc (Blob) ###

 - Byte 0: Ship (0=Tarsus, 1=Orion, 2=Centurion, 3=Galaxy)
 - Byte 1: Always 0; unused?
 - Byte 2: Location (See the BASE_ID enumeration in generated.go)
 - Bytes 3-4: Total missions accepted (int)
 - Byte 5: Mercenaries guild member (boolean)
 - Byte 6: Merchants guild member (boolean) 
 - Bytes 7-9: Always 0; unused?

Note: The length of this chunk is odd, and the length of the header (adn every other chunk) is even.  This means that every form and record in the following data starts on an odd byte, which is the exact opposite of what the IFF spec demands.


### Chunk 2: Plot status (10 bytes) ###

 - Bytes 0-9: Fixed string
This is usualy "s"+(mission series number)+"m"+(mission letter).  e.g "s3mb"
However, an empty string indicates "Plot not yet started" and "FF FF FF FF FF FF FF FF" is used to indicate "unwinnable state"

 - Byte 10: Flags
A poorly-understood bitfield that contains mission status (accepted/succeeded/failed) 


### Chunk 3: Active non-plot mission count (blob) ###
 - Bytes 0-2 Mission count. (int)

### Chunk 4-?? ###
The next 0-6 offsets are for the non-plot missions.  Each mission gets a header chunk and a main chunk.

#### Mission header chunk (blob) ####
Internal name of the mission - this is a string, not null-terminated

#### Mission body chunk (form) ####
A form called MSSN

This is not well understood right now.

Note: the remaining chunks are numbered as if there were 0 missions.

### Chunk 4: Kills and reputation (form) ###
A form called PLAY, containing 2 records called SCOR and KILL
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

The first 11 bytes are not understood, and also not preserved by save-loading.  The rest is a load of boolean values, which store any plot or fixer state that can't be represented in the Plot status chunk.

 - In Privateer, that's details like temporarily rejecting a mission.
 - In RF, it's pretty much the entire plot mission state, since multiple mission chains can be active at once and a simple plot string is completely incapable of dealing with that.

There are a lot of these, but they are listed in generated.go.

### Chunk 6: Hidden jump points (Form) ### 
A form, helpfully called SSSS, with records called ORIG and SECT

ORIG contains originally hidden jump points, which is baffling (shouldn't a save file store current data, not starting data?).  (It is, however one way to tell the diffrence between an original RF file and an imported-from-Prigateer file)

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

#### FITE-AFTB ####
Afterburner.  Data length is 0

#### FITE-ECMS ####
ECM.  Data is just 1 byte, which appears to be ECM effectiveness (25, 50 or 75)

#### FITE-NAVQ ####
Quadrant maps.  Data is just 1 byte, although it is a bitfield with the following values:

 - 1: Humboldt
 - 2: Fariss
 - 4: Potter
 - 8: Clarke


#### FITE-WEAP-GUNS ####

Data is a list of 4-byte gun entries.
Each gun entry is built as follows:

 - Byte 0: Gun type
 - Byte 1: Gun mount
 - Bytes 2-3 damage? (dmage taken by the gun object, not damage done by firing it)

#### FITE-WEAP-LNCH ####

Data is a list of 4-byte launcher entries.
Each gun entry is built as follows:

 - Byte 0: Launcher type
 - Byte 1: Launcher mount
 - Bytes 2-3 damage?


#### FITE-WEAP-MISL ####

Data is a list of 3-byte missile stack entries.
Each missile stack entry is built as follows:

 - Byte 0: missile type
 - Bytes 1-2: number of missiles (int)

It is possible to have up to 32767 of each missile!  The game doesn't seem to mind, although the ship dealer displays the missile count incorrectly.


#### FITE-ENER-INFO ####

#### FITE-ENER-DAMG ####

#### FITE-SHLD-INFO ####

#### FITE-SHLD-ARMR ####

#### FITE_TRGT_INFO ####

#### FITE_TRGT_DAMG ####


### Chunk 8: Name (string) ###
Fixed string, length 17

### Chunk 9: Callsign (string) ###
Fixed string, length 14

