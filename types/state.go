package types

type State int

const (
	INIT State = iota

	VERIFY
	SESSION
)
