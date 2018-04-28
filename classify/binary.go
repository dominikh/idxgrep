package classify

import "bytes"

func IsBinary(b []byte) bool {
	return bytes.Contains(b, []byte{0})
}
