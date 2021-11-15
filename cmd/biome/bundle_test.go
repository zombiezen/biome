// Copyright 2021 Ross Light
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//		 https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestBuildArchive(t *testing.T) {
	type testZipFile struct {
		name    string
		mode    fs.FileMode
		content string
	}
	type buildArchiveTest struct {
		name         string
		srcs         []fs.FS
		linkRoots    []string
		want         []testZipFile
		wantToRemove []string
	}
	tests := []buildArchiveTest{
		{
			name: "Empty",
			srcs: []fs.FS{
				fstest.MapFS{},
			},
			want: []testZipFile{},
		},
		{
			name: "EmptyDirectory",
			srcs: []fs.FS{
				fstest.MapFS{
					"foo": {
						Mode: 0o755 | fs.ModeDir,
					},
				},
			},
			want: []testZipFile{
				{
					name: "foo/",
					mode: 0o755 | fs.ModeDir,
				},
			},
		},
		{
			name: "File",
			srcs: []fs.FS{
				fstest.MapFS{
					"foo": {
						Mode: 0o755 | fs.ModeDir,
					},
					"foo/bar.txt": {
						Data: []byte("Hello, World!\n"),
						Mode: 0o644,
					},
				},
			},
			want: []testZipFile{
				{
					name: "foo/",
					mode: 0o755 | fs.ModeDir,
				},
				{
					name:    "foo/bar.txt",
					mode:    0o644,
					content: "Hello, World!\n",
				},
			},
		},
		{
			name: "FileIgnored",
			srcs: []fs.FS{
				fstest.MapFS{
					"foo": {
						Mode: 0o755 | fs.ModeDir,
					},
					"foo/bar.txt": {
						Data: []byte("Hello, World!\n"),
						Mode: 0o644,
					},
					"foo/zzz.txt": {
						Data: []byte("Hello, World!\n"),
						Mode: 0o644,
					},
					ignoreFileName: {
						Data: []byte("/foo/bar.txt\n"),
						Mode: 0o644,
					},
				},
			},
			want: []testZipFile{
				{
					name: "foo/",
					mode: 0o755 | fs.ModeDir,
				},
				{
					name:    "foo/zzz.txt",
					mode:    0o644,
					content: "Hello, World!\n",
				},
			},
		},
		{
			name: "DirectoryIgnored",
			srcs: []fs.FS{
				fstest.MapFS{
					"foo": {
						Mode: 0o755 | fs.ModeDir,
					},
					"foo/bar.txt": {
						Data: []byte("Hello, World!\n"),
						Mode: 0o644,
					},
					"zzz.txt": {
						Data: []byte("Hello, World!\n"),
						Mode: 0o644,
					},
					ignoreFileName: {
						Data: []byte("/foo/\n"),
						Mode: 0o644,
					},
				},
			},
			want: []testZipFile{
				{
					name:    "zzz.txt",
					mode:    0o644,
					content: "Hello, World!\n",
				},
			},
		},
		{
			name: "FileUnchanged",
			srcs: []fs.FS{
				fstest.MapFS{
					"foo": {
						Mode: 0o755 | fs.ModeDir,
					},
					"foo/bar.txt": {
						Data: []byte("Hello, World!\n"),
						Mode: 0o644,
					},
				},
				fstest.MapFS{
					"foo": {
						Mode: 0o755 | fs.ModeDir,
					},
					"foo/bar.txt": {
						Data: []byte("Hello, World!\n"),
						Mode: 0o644,
					},
				},
			},
			want: []testZipFile{
				{
					name: "foo/",
					mode: 0o755 | fs.ModeDir,
				},
			},
		},
		{
			name: "FileChanged",
			srcs: []fs.FS{
				fstest.MapFS{
					"foo": {
						Mode: 0o755 | fs.ModeDir,
					},
					"foo/bar.txt": {
						Data: []byte("Hello, World!\n"),
						Mode: 0o644,
					},
				},
				fstest.MapFS{
					"foo": {
						Mode: 0o755 | fs.ModeDir,
					},
					"foo/bar.txt": {
						Data: []byte("foo\n"),
						Mode: 0o644,
					},
				},
			},
			want: []testZipFile{
				{
					name: "foo/",
					mode: 0o755 | fs.ModeDir,
				},
				{
					name:    "foo/bar.txt",
					mode:    0o644,
					content: "foo\n",
				},
			},
		},
		{
			name: "FileCreated",
			srcs: []fs.FS{
				fstest.MapFS{
					"foo": {
						Mode: 0o755 | fs.ModeDir,
					},
					"foo/bar.txt": {
						Data: []byte("Hello, World!\n"),
						Mode: 0o644,
					},
				},
				fstest.MapFS{
					"foo": {
						Mode: 0o755 | fs.ModeDir,
					},
					"foo/bar.txt": {
						Data: []byte("Hello, World!\n"),
						Mode: 0o644,
					},
					"baz.txt": {
						Data: []byte("Hello, World!\n"),
						Mode: 0o644,
					},
				},
			},
			want: []testZipFile{
				{
					name: "foo/",
					mode: 0o755 | fs.ModeDir,
				},
				{
					name:    "baz.txt",
					mode:    0o644,
					content: "Hello, World!\n",
				},
			},
		},
		{
			name: "FileRemoved",
			srcs: []fs.FS{
				fstest.MapFS{
					"foo": {
						Mode: 0o755 | fs.ModeDir,
					},
					"foo/bar.txt": {
						Data: []byte("Hello, World!\n"),
						Mode: 0o644,
					},
				},
				fstest.MapFS{
					"foo": {
						Mode: 0o755 | fs.ModeDir,
					},
				},
			},
			want: []testZipFile{
				{
					name: "foo/",
					mode: 0o755 | fs.ModeDir,
				},
			},
			wantToRemove: []string{"foo/bar.txt"},
		},
		{
			name: "DirectoryTurnedIntoFile",
			srcs: []fs.FS{
				fstest.MapFS{
					"foo": {
						Mode: 0o755 | fs.ModeDir,
					},
				},
				fstest.MapFS{
					"foo": {
						Data: []byte("foo\n"),
						Mode: 0o644,
					},
				},
			},
			want: []testZipFile{
				{
					name:    "foo",
					mode:    0o644,
					content: "foo\n",
				},
			},
			wantToRemove: []string{
				"foo",
			},
		},
		{
			name: "FileTurnedIntoDirectory",
			srcs: []fs.FS{
				fstest.MapFS{
					"foo": {
						Data: []byte("foo\n"),
						Mode: 0o644,
					},
				},
				fstest.MapFS{
					"foo": {
						Mode: 0o755 | fs.ModeDir,
					},
				},
			},
			want: []testZipFile{
				{
					name: "foo/",
					mode: 0o755 | fs.ModeDir,
				},
			},
			wantToRemove: []string{
				"foo",
			},
		},
	}
	if runtime.GOOS != "windows" {
		dir1 := t.TempDir()
		err := os.WriteFile(filepath.Join(dir1, "foo.txt"), []byte("Hello\n"), 0o644)
		if err != nil {
			t.Fatal(err)
		}
		err = os.Symlink("foo.txt", filepath.Join(dir1, "bar"))
		if err != nil {
			t.Fatal(err)
		}
		tests = append(tests, buildArchiveTest{
			name:      "Symlink",
			srcs:      []fs.FS{os.DirFS(dir1)},
			linkRoots: []string{dir1},
			want: []testZipFile{
				{
					name:    "foo.txt",
					mode:    0o644,
					content: "Hello\n",
				},
				{
					name:    "bar",
					mode:    0o777 | fs.ModeSymlink,
					content: "foo.txt",
				},
			},
		})

		dir2 := t.TempDir()
		err = os.WriteFile(filepath.Join(dir2, "foo.txt"), []byte("Hello\n"), 0o644)
		if err != nil {
			t.Fatal(err)
		}
		err = os.WriteFile(filepath.Join(dir2, "baz.txt"), []byte("Hello\n"), 0o644)
		if err != nil {
			t.Fatal(err)
		}
		err = os.Symlink("baz.txt", filepath.Join(dir2, "bar"))
		if err != nil {
			t.Fatal(err)
		}
		tests = append(tests, buildArchiveTest{
			name:      "ReplaceSymlink",
			srcs:      []fs.FS{os.DirFS(dir1), os.DirFS(dir2)},
			linkRoots: []string{dir1, dir2},
			want: []testZipFile{
				{
					name:    "foo.txt",
					mode:    0o644,
					content: "Hello\n",
				},
				{
					name:    "baz.txt",
					mode:    0o644,
					content: "Hello\n",
				},
				{
					name:    "bar",
					mode:    0o777 | fs.ModeSymlink,
					content: "baz.txt",
				},
			},
			wantToRemove: []string{"bar"},
		})

		dir3 := t.TempDir()
		err = os.WriteFile(filepath.Join(dir3, "foo.txt"), []byte("Hello\n"), 0o644)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(dir3, "bar"), 0o755); err != nil {
			t.Fatal(err)
		}
		err = os.Symlink(filepath.Join("..", "foo.txt"), filepath.Join(dir3, "bar", "link1"))
		if err != nil {
			t.Fatal(err)
		}
		err = os.Symlink(filepath.Join(dir3, "foo.txt"), filepath.Join(dir3, "bar", "link2"))
		if err != nil {
			t.Fatal(err)
		}
		tests = append(tests, buildArchiveTest{
			name:      "RewriteSymlink",
			srcs:      []fs.FS{os.DirFS(dir3)},
			linkRoots: []string{dir3},
			want: []testZipFile{
				{
					name:    "foo.txt",
					mode:    0o644,
					content: "Hello\n",
				},
				{
					name: "bar/",
					mode: 0o755 | fs.ModeDir,
				},
				{
					name:    "bar/link1",
					mode:    0o777 | fs.ModeSymlink,
					content: "../foo.txt",
				},
				{
					name:    "bar/link2",
					mode:    0o777 | fs.ModeSymlink,
					content: "../foo.txt",
				},
			},
		})
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			var stamps map[string]string
			for i, src := range test.srcs[:len(test.srcs)-1] {
				opts := &bundleOptions{
					prevStamps: stamps,
				}
				if i < len(test.linkRoots) {
					opts.linkRoot = test.linkRoots[i]
				}
				newStamps, _, err := bundle(ctx, io.Discard, src, opts)
				if err != nil {
					t.Fatalf("buildArchive(io.Discard, srcs[%d], %v): %v", i, stamps, err)
				}
				stamps = newStamps
			}
			buf := new(bytes.Buffer)
			opts := &bundleOptions{
				prevStamps: stamps,
			}
			if len(test.srcs)-1 < len(test.linkRoots) {
				opts.linkRoot = test.linkRoots[len(test.srcs)-1]
			}
			_, toRemove, err := bundle(ctx, buf, test.srcs[len(test.srcs)-1], opts)
			if err != nil {
				t.Errorf("buildArchive(buf, srcs[%d], %v): %v", len(test.srcs)-1, stamps, err)
			}
			toRemoveDiff := cmp.Diff(
				test.wantToRemove, toRemove,
				cmpopts.SortSlices(func(s1, s2 string) bool { return s1 < s2 }),
			)
			if toRemoveDiff != "" {
				t.Errorf("toRemove (-want +got):\n%s", toRemoveDiff)
			}
			zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
			if err != nil {
				t.Fatal(err)
			}
			var got []testZipFile
			for _, f := range zr.File {
				content := new(strings.Builder)
				r, err := f.Open()
				if err != nil {
					t.Error(err)
					break
				}
				_, err = io.Copy(content, r)
				r.Close()
				if err != nil {
					t.Error(err)
					break
				}
				got = append(got, testZipFile{
					name:    f.Name,
					mode:    f.Mode(),
					content: content.String(),
				})
			}
			diff := cmp.Diff(
				test.want, got,
				cmp.AllowUnexported(testZipFile{}),
				cmpopts.EquateEmpty(),
				cmpopts.SortSlices(func(f1, f2 testZipFile) bool { return f1.name < f2.name }),
			)
			if diff != "" {
				t.Errorf("zip archive (-want +got):\n%s", diff)
			}
		})
	}
}

func TestMarshalStamp(t *testing.T) {
	tests := []struct {
		info fs.FileInfo
		want string
	}{
		{
			info: &fakeInfo{
				name:    "file.txt",
				size:    1024,
				mode:    0o644,
				modTime: time.Unix(123456, 789000),
			},
			want: "123456.000789-1024-0-420-0-0",
		},
		{
			info: &fakeInfo{
				name:    "link",
				size:    0,
				mode:    0o777 | fs.ModeSymlink,
				modTime: time.Unix(123456, 789000),
			},
			want: "123456.000789-0-0-134218239-0-0",
		},
		{
			info: &fakeInfo{
				name:    "dir",
				size:    50,
				mode:    0o755 | fs.ModeDir,
				modTime: time.Unix(123456, 789000),
			},
			want: dirStamp,
		},
	}
	for _, test := range tests {
		t.Run(test.info.Name(), func(t *testing.T) {
			if got := marshalStamp(test.info); got != test.want {
				t.Errorf("marshalStamp(...) = %q; want %q", got, test.want)
			}
		})
	}
}

type fakeInfo struct {
	name    string
	size    int64
	mode    fs.FileMode
	modTime time.Time
}

func (info *fakeInfo) Name() string       { return info.name }
func (info *fakeInfo) Size() int64        { return info.size }
func (info *fakeInfo) Mode() fs.FileMode  { return info.mode }
func (info *fakeInfo) ModTime() time.Time { return info.modTime }
func (info *fakeInfo) IsDir() bool        { return info.mode.IsDir() }
func (info *fakeInfo) Sys() interface{}   { return nil }
