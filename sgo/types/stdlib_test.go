// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file tests types.Check by using it to
// typecheck the standard library and tests.

package types_test

import (
	"fmt"
	"github.com/tcard/sgo/sgo/ast"
	"go/build"
	"github.com/tcard/sgo/sgo/importer"
	"github.com/tcard/sgo/sgo/parser"
	"github.com/tcard/sgo/sgo/scanner"
	"github.com/tcard/sgo/sgo/token"
	"github.com/tcard/sgo/sgo/internal/testenv"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	. "github.com/tcard/sgo/sgo/types"
)

var (
	pkgCount int // number of packages processed
	start    time.Time

	// Use the same importer for all std lib tests to
	// avoid repeated importing of the same packages.
	stdLibImporter = importer.Default()
)

func TestStdlib(t *testing.T) {
	testenv.MustHaveGoBuild(t)

	start = time.Now()
	walkDirs(t, filepath.Join(runtime.GOROOT(), "src"))
	if testing.Verbose() {
		fmt.Println(pkgCount, "packages typechecked in", time.Since(start))
	}
}

// firstComment returns the contents of the first comment in
// the given file, assuming there's one within the first KB.
func firstComment(filename string) string {
	f, err := os.Open(filename)
	if err != nil {
		return ""
	}
	defer f.Close()

	var src [1 << 10]byte // read at most 1KB
	n, _ := f.Read(src[:])

	var s scanner.Scanner
	s.Init(fset.AddFile("", fset.Base(), n), src[:n], nil, scanner.ScanComments)
	for {
		_, tok, lit := s.Scan()
		switch tok {
		case token.COMMENT:
			// remove trailing */ of multi-line comment
			if lit[1] == '*' {
				lit = lit[:len(lit)-2]
			}
			return strings.TrimSpace(lit[2:])
		case token.EOF:
			return ""
		}
	}
}

func testTestDir(t *testing.T, path string, ignore ...string) {
	files, err := ioutil.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}

	excluded := make(map[string]bool)
	for _, filename := range ignore {
		excluded[filename] = true
	}

	fset := token.NewFileSet()
	for _, f := range files {
		// filter directory contents
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".go") || excluded[f.Name()] {
			continue
		}

		// get per-file instructions
		expectErrors := false
		filename := filepath.Join(path, f.Name())
		if cmd := firstComment(filename); cmd != "" {
			switch cmd {
			case "skip", "compiledir":
				continue // ignore this file
			case "errorcheck":
				expectErrors = true
			}
		}

		// parse and type-check file
		file, err := parser.ParseFile(fset, filename, nil, 0)
		if err == nil {
			conf := Config{Importer: stdLibImporter}
			_, err = conf.Check(filename, fset, []*ast.File{file}, nil)
		}

		if expectErrors {
			if err == nil {
				t.Errorf("expected errors but found none in %s", filename)
			}
		} else {
			if err != nil {
				t.Error(err)
			}
		}
	}
}

func TestStdTest(t *testing.T) {
	testenv.MustHaveGoBuild(t)

	// test/recover4.go is only built for Linux and Darwin.
	// TODO(gri) Remove once tests consider +build tags (issue 10370).
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return
	}

	testTestDir(t, filepath.Join(runtime.GOROOT(), "test"),
		"cmplxdivide.go", // also needs file cmplxdivide1.go - ignore
		"sigchld.go",     // don't work on Windows; testTestDir should consult build tags
	)
}

func TestStdFixed(t *testing.T) {
	testenv.MustHaveGoBuild(t)

	testTestDir(t, filepath.Join(runtime.GOROOT(), "test", "fixedbugs"),
		"bug248.go", "bug302.go", "bug369.go", // complex test instructions - ignore
		"bug459.go",      // possibly incorrect test - see issue 6703 (pending spec clarification)
		"issue3924.go",   // possibly incorrect test - see issue 6671 (pending spec clarification)
		"issue6889.go",   // gc-specific test
		"issue7746.go",   // large constants - consumes too much memory
		"issue11326.go",  // large constants
		"issue11326b.go", // large constants
	)
}

func TestStdKen(t *testing.T) {
	testenv.MustHaveGoBuild(t)

	testTestDir(t, filepath.Join(runtime.GOROOT(), "test", "ken"))
}

// Package paths of excluded packages.
var excluded = map[string]bool{
	"builtin": true,
}

// typecheck typechecks the given package files.
func typecheck(t *testing.T, path string, filenames []string) {
	fset := token.NewFileSet()

	// parse package files
	var files []*ast.File
	for _, filename := range filenames {
		file, err := parser.ParseFile(fset, filename, nil, parser.AllErrors)
		if err != nil {
			// the parser error may be a list of individual errors; report them all
			if list, ok := err.(scanner.ErrorList); ok {
				for _, err := range list {
					t.Error(err)
				}
				return
			}
			t.Error(err)
			return
		}

		if testing.Verbose() {
			if len(files) == 0 {
				fmt.Println("package", file.Name.Name)
			}
			fmt.Println("\t", filename)
		}

		files = append(files, file)
	}

	// typecheck package files
	conf := Config{
		Error:    func(err error) { t.Error(err) },
		Importer: stdLibImporter,
	}
	info := Info{Uses: make(map[*ast.Ident]Object)}
	conf.Check(path, fset, files, &info)
	pkgCount++

	// Perform checks of API invariants.

	// All Objects have a package, except predeclared ones.
	errorError := Universe.Lookup("error").Type().Underlying().(*Interface).ExplicitMethod(0) // (error).Error
	for id, obj := range info.Uses {
		predeclared := obj == Universe.Lookup(obj.Name()) || obj == errorError
		if predeclared == (obj.Pkg() != nil) {
			posn := fset.Position(id.Pos())
			if predeclared {
				t.Errorf("%s: predeclared object with package: %s", posn, obj)
			} else {
				t.Errorf("%s: user-defined object without package: %s", posn, obj)
			}
		}
	}
}

// pkgFilenames returns the list of package filenames for the given directory.
func pkgFilenames(dir string) ([]string, error) {
	ctxt := build.Default
	ctxt.CgoEnabled = false
	pkg, err := ctxt.ImportDir(dir, 0)
	if err != nil {
		if _, nogo := err.(*build.NoGoError); nogo {
			return nil, nil // no *.go files, not an error
		}
		return nil, err
	}
	if excluded[pkg.ImportPath] {
		return nil, nil
	}
	var filenames []string
	for _, name := range pkg.GoFiles {
		filenames = append(filenames, filepath.Join(pkg.Dir, name))
	}
	for _, name := range pkg.TestGoFiles {
		filenames = append(filenames, filepath.Join(pkg.Dir, name))
	}
	return filenames, nil
}

// Note: Could use filepath.Walk instead of walkDirs but that wouldn't
//       necessarily be shorter or clearer after adding the code to
//       terminate early for -short tests.

func walkDirs(t *testing.T, dir string) {
	// limit run time for short tests
	if testing.Short() && time.Since(start) >= 750*time.Millisecond {
		return
	}

	fis, err := ioutil.ReadDir(dir)
	if err != nil {
		t.Error(err)
		return
	}

	// typecheck package in directory
	files, err := pkgFilenames(dir)
	if err != nil {
		t.Error(err)
		return
	}
	if files != nil {
		typecheck(t, dir, files)
	}

	// traverse subdirectories, but don't walk into testdata
	for _, fi := range fis {
		if fi.IsDir() && fi.Name() != "testdata" {
			walkDirs(t, filepath.Join(dir, fi.Name()))
		}
	}
}
