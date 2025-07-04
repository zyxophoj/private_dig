go build priv_ach.go
del priv_ach.zip
powershell Compress-Archive -Path priv_ach.exe,priv_ach.ini -DestinationPath priv_ach.zip