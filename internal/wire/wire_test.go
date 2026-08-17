// Copyright 2018 The Wire Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package wire

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"go/build"
	"go/types"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/google/go-cmp/cmp"
)

var record = flag.Bool("record", false, "whether to run tests against cloud resources and record the interactions")

func TestWire(t *testing.T) {
	const testRoot = "testdata"
	testdataEnts, err := ioutil.ReadDir(testRoot) // ReadDir sorts by name.
	if err != nil {
		t.Fatal(err)
	}
	// The marker function package source is needed to have the test cases
	// type check. loadTestCase places this file at the well-known import path.
	wireGo, err := ioutil.ReadFile(filepath.Join("..", "..", "wire.go"))
	if err != nil {
		t.Fatal(err)
	}
	tests := make([]*testCase, 0, len(testdataEnts))
	for _, ent := range testdataEnts {
		name := ent.Name()
		if !ent.IsDir() || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
			continue
		}
		test, err := loadTestCase(filepath.Join(testRoot, name), wireGo)
		if err != nil {
			t.Error(err)
			continue
		}
		tests = append(tests, test)
	}

	var goToolPath string
	if *record {
		goToolPath = filepath.Join(build.Default.GOROOT, "bin", "go")
		if _, err := os.Stat(goToolPath); err != nil {
			t.Fatal("go toolchain not available:", err)
		}
	}
	ctx := context.Background()
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// Materialize a temporary GOPATH directory.
			gopath, err := ioutil.TempDir("", "wire_test")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(gopath)
			gopath, err = filepath.EvalSymlinks(gopath)
			if err != nil {
				t.Fatal(err)
			}
			if err := test.materialize(gopath); err != nil {
				t.Fatal(err)
			}
			wd := filepath.Join(gopath, "src", "example.com")
			gens, errs := Generate(ctx, wd, append(os.Environ(), "GOPATH="+gopath), test.patterns, &GenerateOptions{Header: test.header})
			for _, gen := range gens {
				if len(gen.Errs) > 0 {
					errs = append(errs, gen.Errs...)
				}
				if len(gen.Content) > 0 {
					gen := gen
					defer t.Logf("%s:\n%s", filepath.Base(gen.OutputPath), gen.Content)
				}
			}
			if len(errs) > 0 {
				gotErrStrings := make([]string, len(errs))
				for i, e := range errs {
					t.Log(e.Error())
					gotErrStrings[i] = scrubError(gopath, e.Error())
				}
				if !test.wantWireError {
					t.Fatal("Did not expect errors. To -record an error, create want/wire_errs.txt.")
				}
				if *record {
					wireErrsFile := filepath.Join(testRoot, test.name, "want", "wire_errs.txt")
					if err := ioutil.WriteFile(wireErrsFile, []byte(strings.Join(gotErrStrings, "\n\n")), 0666); err != nil {
						t.Fatalf("failed to write wire_errs.txt file: %v", err)
					}
				} else {
					if diff := cmp.Diff(gotErrStrings, test.wantWireErrorStrings); diff != "" {
						t.Errorf("Errors didn't match expected errors from wire_errors.txt:\n%s", diff)
					}
				}
				return
			}
			if test.wantWireError {
				t.Fatal("wire succeeded; want error")
			}
			// Collect the non-empty generated files, keyed by golden path.
			got := make(map[string][]byte)
			for _, gen := range gens {
				if len(gen.Content) == 0 {
					continue
				}
				if prefix := gopath + string(os.PathSeparator) + "src" + string(os.PathSeparator); !strings.HasPrefix(gen.OutputPath, prefix) {
					t.Fatalf("suggested output path = %q; want to start with %q", gen.OutputPath, prefix)
				}
				key, err := test.goldenKey(gopath, gen.OutputPath)
				if err != nil {
					t.Fatal(err)
				}
				if _, ok := got[key]; ok {
					t.Fatalf("Generate returned duplicate outputs for %s", key)
				}
				got[key] = gen.Content
			}

			if *record {
				// Record ==> Build the generated Wire code,
				// check that the program's output matches the
				// expected output, save wire output on
				// success.
				for _, gen := range gens {
					if err := gen.Commit(); err != nil {
						t.Fatalf("failed to write %s to test GOPATH: %v", filepath.Base(gen.OutputPath), err)
					}
				}
				if err := goBuildCheck(goToolPath, gopath, test); err != nil {
					t.Fatalf("go build check failed: %v", err)
				}
				wantDir := filepath.Join(testRoot, test.name, "want")
				for key, content := range got {
					goldenPath := filepath.Join(wantDir, filepath.FromSlash(key))
					if err := os.MkdirAll(filepath.Dir(goldenPath), 0777); err != nil {
						t.Fatalf("failed to create golden dir: %v", err)
					}
					if err := ioutil.WriteFile(goldenPath, content, 0666); err != nil {
						t.Fatalf("failed to record %s to testdata: %v", key, err)
					}
				}
				// Remove stale golden .go files that Generate no longer produces.
				err := filepath.Walk(wantDir, func(src string, info os.FileInfo, err error) error {
					if err != nil {
						return err
					}
					if !info.Mode().IsRegular() || filepath.Ext(src) != ".go" {
						return nil
					}
					if info.Size() == 0 {
						// An empty golden file records that wire must
						// generate nothing for this package; keep it.
						return nil
					}
					rel, err := filepath.Rel(wantDir, src)
					if err != nil {
						return err
					}
					if _, ok := got[filepath.ToSlash(rel)]; !ok {
						return os.Remove(src)
					}
					return nil
				})
				if err != nil {
					t.Fatalf("failed to prune stale golden files: %v", err)
				}
			} else {
				// Replay ==> Load golden files and compare to
				// generated results. This check is meant to
				// detect non-deterministic behavior in the
				// Generate function.
				for key, want := range test.wantOutputs {
					content, ok := got[key]
					if !ok {
						if len(want) == 0 {
							// An empty golden file records that wire must
							// generate nothing for this package.
							continue
						}
						t.Errorf("Generate did not produce %s; golden file exists. If this change is expected, run with -record.", key)
						continue
					}
					if !bytes.Equal(content, want) {
						gotS, wantS := string(content), string(want)
						diff := cmp.Diff(strings.Split(gotS, "\n"), strings.Split(wantS, "\n"))
						t.Errorf("wire output for %s differs from golden file. If this change is expected, run with -record to update it.\n*** got:\n%s\n\n*** want:\n%s\n\n*** diff:\n%s", key, gotS, wantS, diff)
					}
				}
				for key := range got {
					if _, ok := test.wantOutputs[key]; !ok {
						t.Errorf("Generate produced %s, which has no golden file. If this change is expected, run with -record.", key)
					}
				}
			}
		})
	}
}

func goBuildCheck(goToolPath, gopath string, test *testCase) error {
	// Run `go build`.
	testExePath := filepath.Join(gopath, "bin", "testprog")
	buildCmd := []string{"build", "-o", testExePath}
	buildCmd = append(buildCmd, test.pkg)
	cmd := exec.Command(goToolPath, buildCmd...)
	cmd.Dir = filepath.Join(gopath, "src", "example.com")
	cmd.Env = append(os.Environ(), "GOPATH="+gopath)
	if buildOut, err := cmd.CombinedOutput(); err != nil {
		if len(buildOut) > 0 {
			return fmt.Errorf("build: %v; output:\n%s", err, buildOut)
		}
		return fmt.Errorf("build: %v", err)
	}

	// Run the resulting program and compare its output to the expected
	// output.
	out, err := exec.Command(testExePath).Output()
	if err != nil {
		return fmt.Errorf("run compiled program: %v", err)
	}
	if !bytes.Equal(out, test.wantProgramOutput) {
		gotS, wantS := string(out), string(test.wantProgramOutput)
		diff := cmp.Diff(strings.Split(gotS, "\n"), strings.Split(wantS, "\n"))
		return fmt.Errorf("compiled program output doesn't match:\n*** got:\n%s\n\n*** want:\n%s\n\n*** diff:\n%s", gotS, wantS, diff)
	}
	return nil
}

func TestUnexport(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"", ""},
		{"a", "a"},
		{"ab", "ab"},
		{"A", "a"},
		{"AB", "ab"},
		{"A_", "a_"},
		{"ABc", "aBc"},
		{"ABC", "abc"},
		{"AB_", "ab_"},
		{"foo", "foo"},
		{"Foo", "foo"},
		{"HTTPClient", "httpClient"},
		{"IFace", "iFace"},
		{"SNAKE_CASE", "snake_CASE"},
		{"HTTP", "http"},
	}
	for _, test := range tests {
		if got := unexport(test.name); got != test.want {
			t.Errorf("unexport(%q) = %q; want %q", test.name, got, test.want)
		}
	}
}

func TestExport(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"", ""},
		{"a", "A"},
		{"ab", "Ab"},
		{"A", "A"},
		{"AB", "AB"},
		{"A_", "A_"},
		{"ABc", "ABc"},
		{"ABC", "ABC"},
		{"AB_", "AB_"},
		{"foo", "Foo"},
		{"Foo", "Foo"},
		{"HTTPClient", "HTTPClient"},
		{"httpClient", "HttpClient"},
		{"IFace", "IFace"},
		{"iFace", "IFace"},
		{"SNAKE_CASE", "SNAKE_CASE"},
		{"HTTP", "HTTP"},
	}
	for _, test := range tests {
		if got := export(test.name); got != test.want {
			t.Errorf("export(%q) = %q; want %q", test.name, got, test.want)
		}
	}
}

func TestTypeVariableName(t *testing.T) {
	var (
		boolT           = types.Typ[types.Bool]
		stringT         = types.Typ[types.String]
		fooVarT         = types.NewNamed(types.NewTypeName(0, nil, "foo", stringT), stringT, nil)
		nonameVarT      = types.NewNamed(types.NewTypeName(0, nil, "", stringT), stringT, nil)
		barVarInFooPkgT = types.NewNamed(types.NewTypeName(0, types.NewPackage("my.example/foo", "foo"), "bar", stringT), stringT, nil)
	)
	tests := []struct {
		description     string
		typ             types.Type
		defaultName     string
		transformAppend string
		collides        map[string]bool
		want            string
	}{
		{"basic type", boolT, "", "", map[string]bool{}, "bool"},
		{"basic type with transform", boolT, "", "suffix", map[string]bool{}, "boolsuffix"},
		{"basic type with collision", boolT, "", "", map[string]bool{"bool": true}, "bool2"},
		{"basic type with transform and collision", boolT, "", "suffix", map[string]bool{"boolsuffix": true}, "boolsuffix2"},
		{"a different basic type", stringT, "", "", map[string]bool{}, "string"},
		{"named type", fooVarT, "", "", map[string]bool{}, "foo"},
		{"named type with transform", fooVarT, "", "suffix", map[string]bool{}, "foosuffix"},
		{"named type with collision", fooVarT, "", "", map[string]bool{"foo": true}, "foo2"},
		{"named type with transform and collision", fooVarT, "", "suffix", map[string]bool{"foosuffix": true}, "foosuffix2"},
		{"noname type", nonameVarT, "bar", "", map[string]bool{}, "bar"},
		{"noname type with transform", nonameVarT, "bar", "s", map[string]bool{}, "bars"},
		{"noname type with transform and collision", nonameVarT, "bar", "s", map[string]bool{"bars": true}, "bars2"},
		{"var in pkg type", barVarInFooPkgT, "", "", map[string]bool{}, "bar"},
		{"var in pkg type with collision", barVarInFooPkgT, "", "", map[string]bool{"bar": true}, "fooBar"},
		{"var in pkg type with double collision", barVarInFooPkgT, "", "", map[string]bool{"bar": true, "fooBar": true}, "bar2"},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%s: typeVariableName(%v, %q, %q, %v)", test.description, test.typ, test.defaultName, test.transformAppend, test.collides), func(t *testing.T) {
			got := typeVariableName(test.typ, test.defaultName, func(name string) string { return name + test.transformAppend }, func(name string) bool { return test.collides[name] })
			if !isIdent(got) {
				t.Errorf("%q is not an identifier", got)
			}
			if got != test.want {
				t.Errorf("got %q want %q", got, test.want)
			}
			if test.collides[got] {
				t.Errorf("%q collides", got)
			}
		})
	}
}

func TestDisambiguate(t *testing.T) {
	tests := []struct {
		name     string
		want     string
		collides map[string]bool
	}{
		{"foo", "foo", nil},
		{"foo", "foo2", map[string]bool{"foo": true}},
		{"foo", "foo3", map[string]bool{"foo": true, "foo1": true, "foo2": true}},
		{"foo1", "foo1_2", map[string]bool{"foo": true, "foo1": true, "foo2": true}},
		{"foo\u0661", "foo\u0661", map[string]bool{"foo": true, "foo1": true, "foo2": true}},
		{"foo\u0661", "foo\u06612", map[string]bool{"foo": true, "foo1": true, "foo2": true, "foo\u0661": true}},
		{"select", "select2", nil},
		{"var", "var2", nil},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("disambiguate(%q, %v)", test.name, test.collides), func(t *testing.T) {
			got := disambiguate(test.name, func(name string) bool { return test.collides[name] })
			if !isIdent(got) {
				t.Errorf("%q is not an identifier", got)
			}
			if got != test.want {
				t.Errorf("got %q want %q", got, test.want)
			}
			if test.collides[got] {
				t.Errorf("%q collides", got)
			}
		})
	}
}

func isIdent(s string) bool {
	if len(s) == 0 {
		return false
	}
	r, i := utf8.DecodeRuneInString(s)
	if !unicode.IsLetter(r) && r != '_' {
		return false
	}
	for i < len(s) {
		r, sz := utf8.DecodeRuneInString(s[i:])
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return false
		}
		i += sz
	}
	return true
}

// scrubError rewrites the given string to remove occurrences of GOPATH/src,
// rewrites OS-specific path separators to slashes, and any line/column
// information to a fixed ":x:y". For example, if the gopath parameter is
// "C:\GOPATH" and running on Windows, the string
// "C:\GOPATH\src\foo\bar.go:15:4" would be rewritten to "foo/bar.go:x:y".
func scrubError(gopath string, s string) string {
	sb := new(strings.Builder)
	query := gopath + string(os.PathSeparator) + "src" + string(os.PathSeparator)
	for {
		// Find next occurrence of source root. This indicates the next path to
		// scrub.
		start := strings.Index(s, query)
		if start == -1 {
			sb.WriteString(s)
			break
		}

		// Find end of file name (extension ".go").
		fileStart := start + len(query)
		fileEnd := strings.Index(s[fileStart:], ".go")
		if fileEnd == -1 {
			// If no ".go" occurs to end of string, further searches will fail too.
			// Break the loop.
			sb.WriteString(s)
			break
		}
		fileEnd += fileStart + 3 // Advance to end of extension.

		// Write out file name and advance scrub position.
		file := s[fileStart:fileEnd]
		if os.PathSeparator != '/' {
			file = strings.Replace(file, string(os.PathSeparator), "/", -1)
		}
		sb.WriteString(s[:start])
		sb.WriteString(file)
		s = s[fileEnd:]

		// Peek past to see if there is line/column info.
		linecol, linecolLen := scrubLineColumn(s)
		sb.WriteString(linecol)
		s = s[linecolLen:]
	}
	return sb.String()
}

func scrubLineColumn(s string) (replacement string, n int) {
	if !strings.HasPrefix(s, ":") {
		return "", 0
	}
	// Skip first colon and run of digits.
	for n++; len(s) > n && '0' <= s[n] && s[n] <= '9'; {
		n++
	}
	if n == 1 {
		// No digits followed colon.
		return "", 0
	}

	// Start on column part.
	if !strings.HasPrefix(s[n:], ":") {
		return ":x", n
	}
	lineEnd := n
	// Skip second colon and run of digits.
	for n++; len(s) > n && '0' <= s[n] && s[n] <= '9'; {
		n++
	}
	if n == lineEnd+1 {
		// No digits followed second colon.
		return ":x", lineEnd
	}
	return ":x:y", n
}

type testCase struct {
	name    string
	pkg     string
	header  []byte
	goFiles map[string][]byte
	// patterns is every package pattern passed to Generate. The first
	// entry is pkg, the main package built by goBuildCheck.
	patterns          []string
	wantProgramOutput []byte
	// wantOutputs maps a golden file path relative to the test's want/
	// directory (e.g. "wire_gen.go", "bar/wire_singleton_gen.go") to its
	// expected contents.
	wantOutputs          map[string][]byte
	wantWireError        bool
	wantWireErrorStrings []string
}

// goldenKey maps a generated file path to its golden file path relative
// to the test's want/ directory. The main package's wire_gen.go keeps the
// historical top-level "wire_gen.go" name; every other generated file is
// stored under its package directory relative to example.com.
func (test *testCase) goldenKey(gopath, outputPath string) (string, error) {
	root := filepath.Join(gopath, "src", "example.com")
	rel, err := filepath.Rel(root, outputPath)
	if err != nil {
		return "", err
	}
	rel = filepath.ToSlash(rel)
	if strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("generated file %s is outside example.com", outputPath)
	}
	// The primary pattern may be an import path (example.com/foo) or a
	// path relative to the example.com directory (./foo).
	primaryDir := path.Clean(strings.TrimPrefix(test.pkg, "example.com/"))
	if rel == primaryDir+"/wire_gen.go" {
		return "wire_gen.go", nil
	}
	return rel, nil
}

// loadTestCase reads a test case from a directory.
//
// The directory structure is:
//
//	root/
//
//		pkg
//			file containing the package name containing the inject function
//			(must also be package main)
//
//		...
//			any Go files found recursively placed under GOPATH/src/...
//
//		want/
//
//			wire_errs.txt
//					Expected errors from the Wire Generate function,
//					missing if no errors expected.
//					Distinct errors are separated by a blank line,
//					and line numbers and line positions are scrubbed
//					(e.g. "$GOPATH/src/foo.go:52:8" --> "foo.go:x:y").
//
//			wire_gen.go
//					verified output of wire from a test run with
//					-record, missing if wire_errs.txt is present
//
//			program_out.txt
//					expected output from the final compiled program,
//					missing if wire_errs.txt is present
func loadTestCase(root string, wireGoSrc []byte) (*testCase, error) {
	name := filepath.Base(root)
	pkgb, err := ioutil.ReadFile(filepath.Join(root, "pkg"))
	if err != nil {
		return nil, fmt.Errorf("load test case %s: %v", name, err)
	}
	patterns := strings.Fields(string(pkgb))
	if len(patterns) == 0 {
		return nil, fmt.Errorf("load test case %s: pkg file is empty", name)
	}
	header, _ := ioutil.ReadFile(filepath.Join(root, "header"))
	var wantProgramOutput []byte
	wantOutputs := make(map[string][]byte)
	wireErrb, err := ioutil.ReadFile(filepath.Join(root, "want", "wire_errs.txt"))
	wantWireError := err == nil
	var wantWireErrorStrings []string
	if wantWireError {
		for _, errs := range strings.Split(string(wireErrb), "\n\n") {
			// Allow for trailing newlines, which can be hard to remove in some editors.
			wantWireErrorStrings = append(wantWireErrorStrings, strings.TrimRight(errs, "\n\r"))
		}
	} else {
		if !*record {
			wantDir := filepath.Join(root, "want")
			err := filepath.Walk(wantDir, func(src string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if !info.Mode().IsRegular() || filepath.Ext(src) != ".go" {
					return nil
				}
				rel, err := filepath.Rel(wantDir, src)
				if err != nil {
					return err
				}
				data, err := ioutil.ReadFile(src)
				if err != nil {
					return err
				}
				wantOutputs[filepath.ToSlash(rel)] = data
				return nil
			})
			if err != nil {
				return nil, fmt.Errorf("load test case %s: %v", name, err)
			}
			if len(wantOutputs) == 0 {
				return nil, fmt.Errorf("load test case %s: no golden .go files under want/; if this is a new testcase, run with -record to generate them", name)
			}
		}
		wantProgramOutput, err = ioutil.ReadFile(filepath.Join(root, "want", "program_out.txt"))
		if err != nil {
			return nil, fmt.Errorf("load test case %s: %v", name, err)
		}
	}
	goFiles := map[string][]byte{
		"github.com/avianlabs/wire/wire.go": wireGoSrc,
	}
	err = filepath.Walk(root, func(src string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, src)
		if err != nil {
			return err // unlikely
		}
		if info.Mode().IsDir() && rel == "want" {
			// The "want" directory should not be included in goFiles.
			return filepath.SkipDir
		}
		if !info.Mode().IsRegular() || filepath.Ext(src) != ".go" {
			return nil
		}
		data, err := ioutil.ReadFile(src)
		if err != nil {
			return err
		}
		goFiles["example.com/"+filepath.ToSlash(rel)] = data
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("load test case %s: %v", name, err)
	}
	return &testCase{
		name:                 name,
		pkg:                  patterns[0],
		patterns:             patterns,
		header:               header,
		goFiles:              goFiles,
		wantOutputs:          wantOutputs,
		wantProgramOutput:    wantProgramOutput,
		wantWireError:        wantWireError,
		wantWireErrorStrings: wantWireErrorStrings,
	}, nil
}

// materialize creates a new GOPATH at the given directory, which may or
// may not exist.
func (test *testCase) materialize(gopath string) error {
	for name, content := range test.goFiles {
		dst := filepath.Join(gopath, "src", filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(dst), 0777); err != nil {
			return fmt.Errorf("materialize GOPATH: %v", err)
		}
		if err := ioutil.WriteFile(dst, content, 0666); err != nil {
			return fmt.Errorf("materialize GOPATH: %v", err)
		}
	}

	// Add go.mod files to example.com and github.com/google/wire.
	const importPath = "example.com"
	const depPath = "github.com/avianlabs/wire"
	depLoc := filepath.Join(gopath, "src", filepath.FromSlash(depPath))
	example := fmt.Sprintf("module %s\n\nrequire %s v0.1.0\nreplace %s => %s\n", importPath, depPath, depPath, depLoc)
	gomod := filepath.Join(gopath, "src", filepath.FromSlash(importPath), "go.mod")
	if err := ioutil.WriteFile(gomod, []byte(example), 0666); err != nil {
		return fmt.Errorf("generate go.mod for %s: %v", gomod, err)
	}
	if err := ioutil.WriteFile(filepath.Join(depLoc, "go.mod"), []byte("module "+depPath+"\n"), 0666); err != nil {
		return fmt.Errorf("generate go.mod for %s: %v", depPath, err)
	}
	return nil
}
