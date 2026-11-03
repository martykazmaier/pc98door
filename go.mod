module pc98door

go 1.26.0

require github.com/kpfaulkner/jxl-go v0.0.0

require (
	github.com/sirupsen/logrus v1.9.3 // indirect
	golang.org/x/exp v0.0.0-20241004190924-225e2abe05e6 // indirect
	golang.org/x/sys v0.0.0-20220715151400-c0bba94af5f8 // indirect
)

replace github.com/kpfaulkner/jxl-go => ./third_party/jxl-go
