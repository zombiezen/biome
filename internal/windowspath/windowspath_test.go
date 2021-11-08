// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//
// Copied from https://cs.opensource.google/go/go/+/refs/tags/go1.17.3:src/path/filepath/path_test.go

package windowspath

import (
	"runtime"
	"testing"
)

type PathTest struct {
	path, result string
}

var cleantests = []PathTest{
	// Already clean
	{`abc`, `abc`},
	{`abc\def`, `abc\def`},
	{`a\b\c`, `a\b\c`},
	{`.`, `.`},
	{`..`, `..`},
	{`..\..`, `..\..`},
	{`..\..\abc`, `..\..\abc`},
	{`\abc`, `\abc`},
	{`\`, `\`},

	// Empty is current dir
	{``, `.`},

	// Remove trailing slash
	{`abc\`, `abc`},
	{`abc\def\`, `abc\def`},
	{`a\b\c\`, `a\b\c`},
	{`.\`, `.`},
	{`..\`, `..`},
	{`..\..\`, `..\..`},
	{`\abc\`, `\abc`},

	// Remove doubled slash
	{`abc\\def\\ghi`, `abc\def\ghi`},
	{`\\abc`, `\abc`},
	{`\\\abc`, `\abc`},
	{`\\abc\\`, `\abc`},
	{`abc\\`, `abc`},

	// Remove . elements
	{`abc\.\def`, `abc\def`},
	{`\.\abc\def`, `\abc\def`},
	{`abc\.`, `abc`},

	// Remove .. elements
	{`abc\def\ghi\..\jkl`, `abc\def\jkl`},
	{`abc\def\..\ghi\..\jkl`, `abc\jkl`},
	{`abc\def\..`, `abc`},
	{`abc\def\..\..`, `.`},
	{`\abc\def\..\..`, `\`},
	{`abc\def\..\..\..`, `..`},
	{`\abc\def\..\..\..`, `\`},
	{`abc\def\..\..\..\ghi\jkl\..\..\..\mno`, `..\..\mno`},
	{`\..\abc`, `\abc`},

	// Combinations
	{`abc\.\..\def`, `def`},
	{`abc\\.\..\def`, `def`},
	{`abc\..\..\.\.\..\def`, `..\..\def`},

	// Windows-specific
	{`c:`, `c:.`},
	{`c:\`, `c:\`},
	{`c:\abc`, `c:\abc`},
	{`c:abc\..\..\.\.\..\def`, `c:..\..\def`},
	{`c:\abc\def\..\..`, `c:\`},
	{`c:\..\abc`, `c:\abc`},
	{`c:..\abc`, `c:..\abc`},
	{`\`, `\`},
	{`/`, `\`},
	{`\\i\..\c$`, `\c$`},
	{`\\i\..\i\c$`, `\i\c$`},
	{`\\i\..\I\c$`, `\I\c$`},
	{`\\host\share\foo\..\bar`, `\\host\share\bar`},
	{`//host/share/foo/../baz`, `\\host\share\baz`},
	{`\\a\b\..\c`, `\\a\b\c`},
	{`\\a\b`, `\\a\b`},
}

func TestClean(t *testing.T) {
	for _, test := range cleantests {
		if s := Clean(test.path); s != test.result {
			t.Errorf("Clean(%q) = %q, want %q", test.path, s, test.result)
		}
		if s := Clean(test.result); s != test.result {
			t.Errorf("Clean(%q) = %q, want %q", test.result, s, test.result)
		}
	}

	if testing.Short() {
		t.Skip("skipping malloc count in short mode")
	}
	if runtime.GOMAXPROCS(0) > 1 {
		t.Log("skipping AllocsPerRun checks; GOMAXPROCS>1")
		return
	}

	for _, test := range cleantests {
		allocs := testing.AllocsPerRun(100, func() { Clean(test.result) })
		if allocs > 0 {
			t.Errorf("Clean(%q): %v allocs, want zero", test.result, allocs)
		}
	}
}

type JoinTest struct {
	elem []string
	path string
}

var jointests = []JoinTest{
	// zero parameters
	{[]string{}, ""},

	// one parameter
	{[]string{""}, ""},
	{[]string{"/"}, "/"},
	{[]string{"a"}, "a"},

	// two parameters
	{[]string{"a", "b"}, "a/b"},
	{[]string{"a", ""}, "a"},
	{[]string{"", "b"}, "b"},
	{[]string{"/", "a"}, "/a"},
	{[]string{"/", "a/b"}, "/a/b"},
	{[]string{"/", ""}, "/"},
	{[]string{"//", "a"}, "/a"},
	{[]string{"/a", "b"}, "/a/b"},
	{[]string{"a/", "b"}, "a/b"},
	{[]string{"a/", ""}, "a"},
	{[]string{"", ""}, ""},

	// three parameters
	{[]string{"/", "a", "b"}, "/a/b"},

	// Windows
	{[]string{`directory`, `file`}, `directory\file`},
	{[]string{`C:\Windows\`, `System32`}, `C:\Windows\System32`},
	{[]string{`C:\Windows\`, ``}, `C:\Windows`},
	{[]string{`C:\`, `Windows`}, `C:\Windows`},
	{[]string{`C:`, `a`}, `C:a`},
	{[]string{`C:`, `a\b`}, `C:a\b`},
	{[]string{`C:`, `a`, `b`}, `C:a\b`},
	{[]string{`C:`, ``, `b`}, `C:b`},
	{[]string{`C:`, ``, ``, `b`}, `C:b`},
	{[]string{`C:`, ``}, `C:.`},
	{[]string{`C:`, ``, ``}, `C:.`},
	{[]string{`C:.`, `a`}, `C:a`},
	{[]string{`C:a`, `b`}, `C:a\b`},
	{[]string{`C:a`, `b`, `d`}, `C:a\b\d`},
	{[]string{`\\host\share`, `foo`}, `\\host\share\foo`},
	{[]string{`\\host\share\foo`}, `\\host\share\foo`},
	{[]string{`//host/share`, `foo/bar`}, `\\host\share\foo\bar`},
	{[]string{`\`}, `\`},
	{[]string{`\`, ``}, `\`},
	{[]string{`\`, `a`}, `\a`},
	{[]string{`\\`, `a`}, `\a`},
	{[]string{`\`, `a`, `b`}, `\a\b`},
	{[]string{`\\`, `a`, `b`}, `\a\b`},
	{[]string{`\`, `\\a\b`, `c`}, `\a\b\c`},
	{[]string{`\\a`, `b`, `c`}, `\a\b\c`},
	{[]string{`\\a\`, `b`, `c`}, `\a\b\c`},
}

func TestJoin(t *testing.T) {
	for _, test := range jointests {
		expected := FromSlash(test.path)
		if p := Join(test.elem...); p != expected {
			t.Errorf("join(%q) = %q, want %q", test.elem, p, expected)
		}
	}
}

type IsAbsTest struct {
	path  string
	isAbs bool
}

var isabstests = []IsAbsTest{
	{"", false},
	{"/", true},
	{"/usr/bin/gcc", true},
	{"..", false},
	{"/a/../bb", true},
	{".", false},
	{"./", false},
	{"lala", false},
}

var winisabstests = []IsAbsTest{
	{`C:\`, true},
	{`c\`, false},
	{`c::`, false},
	{`c:`, false},
	{`/`, false},
	{`\`, false},
	{`\Windows`, false},
	{`c:a\b`, false},
	{`c:\a\b`, true},
	{`c:/a/b`, true},
	{`\\host\share\foo`, true},
	{`//host/share/foo/bar`, true},
}

func TestIsAbs(t *testing.T) {
	var tests []IsAbsTest
	tests = append(tests, winisabstests...)
	// All non-windows tests should fail, because they have no volume letter.
	for _, test := range isabstests {
		tests = append(tests, IsAbsTest{test.path, false})
	}
	// All non-windows test should work as intended if prefixed with volume letter.
	for _, test := range isabstests {
		tests = append(tests, IsAbsTest{"c:" + test.path, test.isAbs})
	}
	// Test reserved names.
	tests = append(tests, IsAbsTest{"NUL", true})
	tests = append(tests, IsAbsTest{"nul", true})
	tests = append(tests, IsAbsTest{"CON", true})

	for _, test := range tests {
		if r := IsAbs(test.path); r != test.isAbs {
			t.Errorf("IsAbs(%q) = %v, want %v", test.path, r, test.isAbs)
		}
	}
}
