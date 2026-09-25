package web

import (
	"fmt"
	"slices"
)

// FilterBar is a list page's filters, which the "filters" template renders as a GET form
// to Action so that the URL holds the list state.
type FilterBar struct {
	Action  string
	Filters []Filter
}

// Filter is one select in a FilterBar.
type Filter struct {
	Name    string
	Label   string
	Options []Option
}

type Option struct {
	Value    string
	Label    string
	Selected bool
}

type Direction string

const (
	Desc Direction = "desc"
	Asc  Direction = "asc"
)

var Directions = []Direction{Desc, Asc}

func (d Direction) Label() string {
	switch d {
	case Desc:
		return "Descending"
	case Asc:
		return "Ascending"
	}
	return string(d)
}

func Options[T ~string](values []T, label func(T) string, selected T) []Option {
	options := make([]Option, len(values))
	for i, v := range values {
		options[i] = Option{Value: string(v), Label: label(v), Selected: v == selected}
	}
	return options
}

// ParseChoice returns def for an empty value and an error for a value outside allowed.
func ParseChoice[T ~string](name, value string, allowed []T, def T) (T, error) {
	if value == "" {
		return def, nil
	}
	if !slices.Contains(allowed, T(value)) {
		return def, fmt.Errorf("invalid %s %q", name, value)
	}
	return T(value), nil
}
