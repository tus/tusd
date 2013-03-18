package main

import (
	"fmt"
	"testing"
)

var chunkSet_AddTests = []struct {
	Name   string
	Add    []chunk
	Expect []chunk
}{
	{
		Name:   "add one",
		Add:    []chunk{{Start: 1, End: 5}},
		Expect: []chunk{{Start: 1, End: 5}},
	},
	{
		Name:   "add twice",
		Add:    []chunk{{Start: 1, End: 5}, {Start: 1, End: 5}},
		Expect: []chunk{{Start: 1, End: 5}},
	},
	{
		Name:   "append",
		Add:    []chunk{{Start: 1, End: 5}, {Start: 7, End: 10}},
		Expect: []chunk{{Start: 1, End: 5}, {Start: 7, End: 10}},
	},
	{
		Name:   "insert",
		Add:    []chunk{{Start: 0, End: 5}, {Start: 12, End: 15}, {Start: 7, End: 10}},
		Expect: []chunk{{Start: 0, End: 5}, {Start: 7, End: 10}, {Start: 12, End: 15}},
	},
	{
		Name:   "prepend",
		Add:    []chunk{{Start: 5, End: 10}, {Start: 1, End: 3}},
		Expect: []chunk{{Start: 1, End: 3}, {Start: 5, End: 10}},
	},
	{
		Name:   "grow start",
		Add:    []chunk{{Start: 1, End: 5}, {Start: 0, End: 5}},
		Expect: []chunk{{Start: 0, End: 5}},
	},
	{
		Name:   "grow end",
		Add:    []chunk{{Start: 1, End: 5}, {Start: 1, End: 6}},
		Expect: []chunk{{Start: 1, End: 6}},
	},
	{
		Name:   "grow end with multiple items",
		Add:    []chunk{{Start: 1, End: 5}, {Start: 7, End: 10}, {Start: 8, End: 15}},
		Expect: []chunk{{Start: 1, End: 5}, {Start: 7, End: 15}},
	},
	{
		Name:   "grow exact end match",
		Add:    []chunk{{Start: 1, End: 5}, {Start: 6, End: 6}},
		Expect: []chunk{{Start: 1, End: 6}},
	},
	{
		Name:   "sink",
		Add:    []chunk{{Start: 1, End: 5}, {Start: 2, End: 3}},
		Expect: []chunk{{Start: 1, End: 5}},
	},
	{
		Name:   "swallow",
		Add:    []chunk{{Start: 1, End: 5}, {Start: 6, End: 10}, {Start: 0, End: 11}},
		Expect: []chunk{{Start: 0, End: 11}},
	},
	{
		Name:   "ignore 0 byte chunks",
		Add:    []chunk{{Start: 0, End: -1}},
		Expect: []chunk{},
	},
	{
		Name:   "ignore invalid chunks",
		Add:    []chunk{{Start: 0, End: -2}},
		Expect: []chunk{},
	},
}

func Test_chunkSet_Add(t *testing.T) {
	for _, test := range chunkSet_AddTests {
		var chunks chunkSet
		for _, chunk := range test.Add {
			chunks.Add(chunk)
		}

		expected := fmt.Sprintf("%+v", test.Expect)
		got := fmt.Sprintf("%+v", chunks)

		if got != expected {
			t.Errorf(
				"Failed test '%s':\nexpected: %s\ngot: %s",
				test.Name,
				expected,
				got,
			)
		}
	}
}
