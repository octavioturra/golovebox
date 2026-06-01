module github.com/user/golovebox/sandbox

go 1.26

require (
	github.com/pkg/sftp v1.13.10
	github.com/user/golovebox/core v0.0.0
	golang.org/x/crypto v0.48.0
)

require (
	github.com/kr/fs v0.1.0 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	golang.org/x/sys v0.41.0 // indirect
)

replace github.com/user/golovebox/core => ../core
