## Achievements for Wing Commander: Privateer

### How to get it

Go to the releases page (https://github.com/zyxophoj/private_dig/releases) and download the zip file for the latest release.  Unzip it somewhere.

### How to run it

- Edit the priv_ach.ini file.  There is only one setting, which should be the location of your Privateer saved games.  Change it if it is wrong.
- Run priv_ach.go.
- Play Privateer!





## Fictitious, Unasked, Questions

### How does it work?

Every time a new save file appears, priv_ach examines the file, and awards achievements.

Achievements are awarded to an identity, which is constructed from name and callsign.  Two files with matching names and callsigns are therefore considered to belong to the same identity.  I am not sure if this is a bug or a feature, but it does make it possible to collect missed achievements - Tarsus-only ones, for example - by starting a new game with the same identity as an existing one.

Achievement state is stored in the pracst.json file; delete that file to reset all achievements.


### Windows Defender is complaing about viruses.  Are you trying to sneak a trojan onto my PC?

Probably not.  https://go.dev/doc/faq#virus

Unfortunately, Microsoft's virus scanner on Windows 11 seems to think that anything written in Go - a language made by one of Microsoft's competitors - has a trojan.
Even a "Hello World" program written in Go will sometimes get flagged as malicious.
The reported "wacatac" trojan is one of the most common false positives coming out of Windows Security.

Unfortunately, if I really was trying to trojan you, I'd probably do something sneaky like use a trojan that is one of the most common false positives.  How certain are you that the exe file available for download really was made by compiling the source code?  If you don't trust some random guy on the internet, you could compile the code yourself.


### Ok, how do I compile it myself then?

Get the latest code - https://github.com/zyxophoj/private_dig/archive/refs/heads/main.zip - and unzip it somewhere.

Download and install the go compiler if you don't already have one.  https://go.dev/dl/  is a good place to start. (Just in case it matters, I'm currenlty using go version 1.23.1)

On the command line, go to wherever you unzipped the source code to (the dir containing priv_ach.go), and do:  ```go build priv_ach.go``` 


This is a bit more effort than clicking a link, and I'm sorry that Microsoft's virus detection is a steaming pile of machine learning, but there is a silver lining - Downloaders get some bonus extra features:

* privdump.go - a save file dumping program. 
* ach_test.go - a test program for the acheivement code 
* and the source code for priv_ach

### "Fictitious, Unasked, Questions" is a bit of a mouthful.  Is there an abbreviation?

Uhh...