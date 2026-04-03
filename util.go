package konnektor

import (
	"encoding/json"
	"fmt"
	"strings"
)

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}

func ftoa(f float64) string {
	return fmt.Sprintf("%.2f", f)
}

func join(ss []string, sep string) string {
	return strings.Join(ss, sep)
}

func marshalJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
