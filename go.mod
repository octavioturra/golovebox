module github.com/user/golovebox

go 1.26

require (
	github.com/user/golovebox/core v0.0.0-00010101000000-000000000000
	github.com/user/golovebox/promptlang v0.0.0-00010101000000-000000000000
	github.com/user/golovebox/sandbox v0.0.0-00010101000000-000000000000
	github.com/BurntSushi/toml v1.6.0
	github.com/digitalocean/go-qemu v0.0.0-20250212194115-ee9b0668d242
	github.com/go-chi/chi/v5 v5.3.0
	github.com/go-telegram-bot-api/telegram-bot-api/v5 v5.5.1
	github.com/google/go-github/v60 v60.0.0
	github.com/google/uuid v1.6.0
	github.com/gorilla/websocket v1.5.3
	github.com/kdomanski/iso9660 v0.4.0
	github.com/philippgille/chromem-go v0.7.0
	github.com/pkg/sftp v1.13.10
	github.com/spf13/cobra v1.10.2
	golang.org/x/crypto v0.48.0
)

require (
	github.com/digitalocean/go-libvirt v0.0.0-20220804181439-8648fbde413e // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/kr/fs v0.1.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	golang.org/x/sys v0.41.0 // indirect
)

replace (
	github.com/user/golovebox/core => ./core
	github.com/user/golovebox/promptlang => ./promptlang
	github.com/user/golovebox/sandbox => ./sandbox
)
