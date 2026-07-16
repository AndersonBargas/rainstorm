package rainstorm

// Version is the legacy metadata marker written to newly initialized databases.
// It is distinct from the Go module version; compatibility is established by
// behavioral fixture tests rather than by comparing this value during Open.
const Version = "5.3.0"
