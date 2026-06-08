module github.com/user/golovebox/orchestrator

go 1.26

require (
	github.com/google/go-github/v60 v60.0.0
	github.com/google/uuid v1.6.0
	github.com/philippgille/chromem-go v0.7.0
	github.com/user/golovebox/core v0.0.0-00010101000000-000000000000
)

require github.com/google/go-querystring v1.1.0 // indirect

replace github.com/user/golovebox/core => ../core
