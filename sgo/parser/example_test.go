// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package parser_test

import (
	"fmt"

	"github.com/tcard/sgo/sgo/parser"
	"github.com/tcard/sgo/sgo/token"
)

func ExampleParseFile() {
	fset := token.NewFileSet() // positions are relative to fset

	// Parse the file containing this very example
	// but stop after processing the imports.
	f, err := parser.ParseFile(fset, "example_test.go", nil, parser.ImportsOnly)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Print the imports from the file's AST.
	for _, s := range f.Imports {
		fmt.Println(s.Path.Value)
	}

	// output:
	//
	// "fmt"
	// "github.com/tcard/sgo/sgo/parser"
	// "github.com/tcard/sgo/sgo/token"
}
