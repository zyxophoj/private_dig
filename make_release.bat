go build priv_ach.go
go build privedit.go
del priv_ach.zip
powershell Compress-Archive -Path priv_ach.exe,privedit.exe,priv_ach.ini -DestinationPath priv_ach.zip