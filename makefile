GO=go

all: test release

test:
	$(GO) test privedit_test.go privedit.go

priv_ach.exe: priv_ach.go types/types.go
	$(GO) build priv_ach.go

privedit.exe: privedit.go types/types.go
	$(GO) build privedit.go

release: priv_ach.exe privedit.exe
	del priv_ach.zip
	powershell Compress-Archive -Path priv_ach.exe,privedit.exe,priv_ach.ini -DestinationPath priv_ach.zip

