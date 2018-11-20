package info

import "github.com/solo-io/supergloo/cli/pkg/cmd/options"

type Header struct {
	// Name that will be displayed when printing to terminal
	Name string

	// If true, this field will be displayed only when using the "-o wide" option
	WideOnly bool

	Multiline bool
}

// For each row, we will want to look up the values by the header name
type Data []map[string]string

func (h Header) String() string {
	return h.Name
}

type ResourceInfo struct {
	headers []Header
	data    Data
}

// Returns a slice containing header names
func (info ResourceInfo) Headers(opts options.Get) []string {
	h := make([]string, 0)
	for _, header := range info.headers {
		// if this column is wideOnly, include it only if the "-o wide" option was supplied
		if !header.WideOnly || opts.Output == "wide" {
			h = append(h, header.String())
		}
	}
	return h
}

// Returns a slice of slices, each one containing the fields corresponding to the headers
func (info ResourceInfo) Resources(opts options.Get) [][]string {
	includedHeaders := info.Headers(opts)
	result := make([][]string, len(info.data))

	// for each resource
	for i, fieldMap := range info.data {

		// for each of the columns that we want to display
		line := make([]string, len(includedHeaders))
		for j, h := range includedHeaders {
			val, ok := fieldMap[h]
			if !ok {
				val = ""
			}
			line[j] = val
		}
		result[i] = line
	}
	return result
}
