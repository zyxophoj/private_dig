## Achievements for Wing Commander: Privateer

### How to get it

Go to the releases page (https://github.com/zyxophoj/private_dig/releases) and download the zip file for the latest release.  Unzip it somewhere.

### How to run it

- Edit the priv_ach.ini file.  There is only one setting, which should be the location of your Privateer saved games.  Change it if it is wrong.
- Run priv_ach.go.
- Play Privateer!

### How it works

Every time a new save file appears, priv_ach examines the file, and awards achievements.

Achievements are awarded to an identity, which is constructed from name and callsign.  Two files with matching names and callsigns are therefore considered to belong to the same identity.  I am not sure if this is a bug or a feature, but it does make it possible to collect missed achievements - Tarsus-only ones, for example - by starting a new game with the same identity as an existing one.

Achievement state is stored in the pracst.json file; delete that file to reset all achievements.
