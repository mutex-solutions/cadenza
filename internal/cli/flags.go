package cli

import (
	"strings"

	"github.com/mutex-solutions/cadenza/internal/kernel"
)

// flags parses "--name value", "--name=value" and boolean "--name" anywhere
// among positional arguments (agents put flags in any order).
type flags struct {
	values map[string][]string
	bools  map[string]bool
	pos    []string
}

// parseFlags parses args. valued lists flags that take a value; boolean lists
// flags that do not. Anything else starting with "-" is an error.
func parseFlags(args []string, valued, boolean []string) (*flags, error) {
	isValued := map[string]bool{"session": true, "server": true, "agent-name": true}
	for _, v := range valued {
		isValued[v] = true
	}
	isBool := map[string]bool{}
	for _, b := range boolean {
		isBool[b] = true
	}
	f := &flags{values: map[string][]string{}, bools: map[string]bool{}}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			f.pos = append(f.pos, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			f.pos = append(f.pos, arg)
			continue
		}
		name := strings.TrimLeft(arg, "-")
		value, hasValue := "", false
		if k, v, ok := strings.Cut(name, "="); ok {
			name, value, hasValue = k, v, true
		}
		switch {
		case isBool[name]:
			f.bools[name] = !hasValue || value == "true" || value == "1"
		case isValued[name]:
			if !hasValue {
				if i+1 >= len(args) {
					return nil, kernel.Invalid("--%s needs a value", name)
				}
				i++
				value = args[i]
			}
			f.values[name] = append(f.values[name], value)
		default:
			return nil, kernel.Invalid("unknown flag --%s", name).WithFix("run `cadenza help`")
		}
	}
	return f, nil
}

func (f *flags) str(name string) string {
	if v := f.values[name]; len(v) > 0 {
		return v[len(v)-1]
	}
	return ""
}

func (f *flags) all(name string) []string { return f.values[name] }

func (f *flags) bool(name string) bool { return f.bools[name] }
