package control

// TUNPreflight checks whether the host can safely start a TUN runtime.
// Keeping it injectable makes the runtime policy testable without changing
// the host network from unit tests.
type TUNPreflight interface {
	CheckTUN() error
}
