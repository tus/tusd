package main

import (
	"sort"
)

// chunk holds the offsets for a partial piece of data
type chunk struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

// Size returns the number of bytes between Start and End.
func (c chunk) Size() int64 {
	return c.End - c.Start + 1
}

// chunkSet holds a set of chunks and helps with adding/merging new chunks into
// set set.
type chunkSet []chunk

// Add merges a newChunk into a chunkSet. This may lead to the chunk being
// combined with one or more adjecent chunks, possibly shrinking the chunkSet
// down to a single member.
func (c *chunkSet) Add(newChunk chunk) {
	if newChunk.Size() <= 0 {
		return
	}

	*c = append(*c, newChunk)
	sort.Sort(c)

	// merge chunks that can be combined
	for i := 0; i < len(*c)-1; i++ {
		current := (*c)[i]
		next := (*c)[i+1]

		if current.End+1 < next.Start {
			continue
		}

		*c = append((*c)[0:i], (*c)[i+1:]...)

		if current.End > next.End {
			(*c)[i].End = current.End
		}

		if current.Start < next.Start {
			(*c)[i].Start = current.Start
		}

		i--
	}
}

func (c chunkSet) Len() int {
	return len(c)
}

func (c chunkSet) Less(i, j int) bool {
	return c[i].Start < c[j].Start
}

func (c chunkSet) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}
