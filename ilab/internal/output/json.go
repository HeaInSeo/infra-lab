package output

import (
	"encoding/json"
	"io"
)

func WriteJSON(w io.Writer, env Envelope) error {
	env = normalize(env)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(env)
}
