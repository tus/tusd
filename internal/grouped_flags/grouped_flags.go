// Package grouped_flags provides a small wrapper around the flag package
// from Go's standard library to allow grouping flags in the help output.
// Please see the example for more details.
package grouped_flags

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/jnovack/flag"
)

type flagGroup struct {
	name  string
	flags *flag.FlagSet
}

type FlagGroupSet struct {
	groups   []flagGroup
	allFlags *flag.FlagSet
}

func NewFlagGroupSet(errorHandling flag.ErrorHandling) *FlagGroupSet {
	f := &FlagGroupSet{
		groups:   make([]flagGroup, 0),
		allFlags: flag.NewFlagSet(os.Args[0], errorHandling),
	}

	f.allFlags.Usage = f.Usage

	return f
}

func (f *FlagGroupSet) AddGroup(name string, constructor func(*flag.FlagSet)) {
	// Construct an empty flag set
	groupFlagSet := flag.NewFlagSet("", flag.PanicOnError)

	// Pass it to the callback, which populates it with the flags for this group
	constructor(groupFlagSet)

	// Add the flags to the combined flag set, which is used for parsing
	groupFlagSet.VisitAll(func(fl *flag.Flag) {
		f.allFlags.Var(fl.Value, fl.Name, fl.Usage)
	})

	f.groups = append(f.groups, flagGroup{
		name,
		groupFlagSet,
	})
}

func (f FlagGroupSet) Parse() error {
	return f.allFlags.Parse(os.Args[1:])
}

func (f *FlagGroupSet) SetOutput(output io.Writer) {
	f.allFlags.SetOutput(output)
}

func (f *FlagGroupSet) Usage() {
	output := f.allFlags.Output()

	// Print name of program
	fmt.Fprintf(output, "Usage of %s:\n\n", f.allFlags.Name())

	for _, group := range f.groups {
		// Print name of group
		fmt.Fprintf(output, "%s:\n", group.name)

		// Write flag description into buffer and then print
		buf := new(bytes.Buffer)
		group.flags.SetOutput(buf)
		group.flags.PrintDefaults()

		fmt.Fprintln(output, buf.String())
	}
}
