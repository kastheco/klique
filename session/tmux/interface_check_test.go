package tmux

// Compile-time assertion: *TmuxSession must satisfy Session interface.
var _ Session = (*TmuxSession)(nil)
