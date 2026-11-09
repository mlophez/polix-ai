package take

import "context"

// format is the one unexported interface of the formats the entry reads that the in files may declare, besides
// Port (GOARCH-12).
type format interface {
	parse(raw string) string
}

// plain is the unexported type of the in files that implements format.
type plain struct{}

func (plain) parse(raw string) string { return raw }

// load is an unexported function of a sibling of in.go that calls the port for the shell (GOARCH-32), on
// conditions the shell may take (GOARCH-37).
func load(ctx context.Context, port Port, req Request) (string, error) {
	raw, err := port.Load(ctx, req.Shop)
	if err != nil {
		return "", err
	}
	var f format = plain{}
	return f.parse(raw), nil
}
