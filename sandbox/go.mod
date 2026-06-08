module github.com/user/golovebox/sandbox

go 1.26

require (
	github.com/digitalocean/go-qemu v0.0.0-20250212194115-ee9b0668d242
	github.com/pkg/sftp v1.13.10
	github.com/user/golovebox/core v0.0.0-00010101000000-000000000000
	golang.org/x/crypto v0.48.0
)

require (
	github.com/digitalocean/go-libvirt v0.0.0-20220804181439-8648fbde413e // indirect
	github.com/kr/fs v0.1.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
)

replace github.com/user/golovebox/core => ../core
