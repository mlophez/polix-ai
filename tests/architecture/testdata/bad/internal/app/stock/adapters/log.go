// Package adapters is the good case of the adapters package of a second noun: one exported struct in the file named after it.
package adapters

// Log reads the log of the stock.
type Log struct{ entries []string }

// Load returns the entries of the log.
func (l *Log) Load() []string { return l.entries }
