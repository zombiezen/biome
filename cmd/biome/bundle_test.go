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
	tests := []struct {
		name         string
		srcs         []fs.FS
		want         []testZipFile
		wantToRemove []string
	}{
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
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			var stamps map[string]string
			for i, src := range test.srcs[:len(test.srcs)-1] {
				newStamps, _, err := bundle(ctx, io.Discard, src, nil, stamps)
				if err != nil {
					t.Fatalf("buildArchive(io.Discard, srcs[%d], %v): %v", i, stamps, err)
				}
				stamps = newStamps
			}
			buf := new(bytes.Buffer)
			_, toRemove, err := bundle(ctx, buf, test.srcs[len(test.srcs)-1], nil, stamps)
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
