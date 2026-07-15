module github.com/AndersonBargas/rainstorm/testdata/compatibility/roundtrip

go 1.24.0

replace github.com/AndersonBargas/rainstorm/v6 => ../../..

require (
	github.com/AndersonBargas/rainstorm/v5 v5.3.0
	github.com/AndersonBargas/rainstorm/v6 v6.0.0-00010101000000-000000000000
	github.com/stretchr/testify v1.11.1
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	go.etcd.io/bbolt v1.4.3 // indirect
	golang.org/x/sys v0.39.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
