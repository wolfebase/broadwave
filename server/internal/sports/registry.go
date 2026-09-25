package sports

import (
	"fmt"
	"strings"
	"sync"
)

// A factory builds one Provider. Register is safe for concurrent tests.
type factory func() Provider

var (
	mu        sync.Mutex
	factories = map[string]factory{}
)

func init() {
	Register("espn", func() Provider { return NewESPN() })
}

// Register makes name select factory. A later call for the same name replaces the earlier one.
// The empty name is not a key: Open treats it as espn.
func Register(name string, build factory) {
	name = canonical(name)
	if name == "" || build == nil {
		panic("sports: Register needs a name and a factory")
	}
	mu.Lock()
	defer mu.Unlock()
	factories[name] = build
}

// Open returns the provider registered under name.
// The empty name and "espn" are the ESPN scoreboard. Any other unregistered name is an error.
func Open(name string) (Provider, error) {
	name = canonical(name)
	if name == "" {
		name = "espn"
	}
	mu.Lock()
	build, ok := factories[name]
	mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("unknown sports provider %q", name)
	}
	provider := build()
	if provider == nil {
		return nil, fmt.Errorf("sports provider %q is not available", name)
	}
	return provider, nil
}

func canonical(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
