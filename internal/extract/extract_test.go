// Copyright 2021 Ross Light
// Copyright 2020 YourBase Inc.
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

package extract

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/yourbase/commons/http/headers"
	"zombiezen.com/go/biome"
	"zombiezen.com/go/biome/downloader"
	"zombiezen.com/go/log/testlog"
)

const extractContent = "Hello, World!\n"

func TestExtract(t *testing.T) {
	tests := []struct {
		name        string
		archive     []byte
		ext         string
		contentType string
		mode        bool
	}{
		{
			name:        "Zip",
			archive:     makeZip("root/foo/bar.txt"),
			ext:         ".zip",
			contentType: "application/zip",
			mode:        StripTopDirectory,
		},
		{
			name:        "GzipTar",
			mode:        StripTopDirectory,
			ext:         ".tar.gz",
			archive:     makeGzipTar("root/foo/bar.txt"),
			contentType: "application/gzip",
		},
		{
			name:        "ZipBomb",
			archive:     makeZip("foo/bar.txt"),
			ext:         ".zip",
			contentType: "application/zip",
			mode:        Tarbomb,
		},
		{
			name:        "GzipTarBomb",
			archive:     makeGzipTar("foo/bar.txt"),
			ext:         ".tar.gz",
			contentType: "application/gzip",
			mode:        Tarbomb,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wantPath := "/archive" + test.ext
			f := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != wantPath {
					http.NotFound(w, r)
					return
				}
				w.Header().Set(headers.ContentType, test.contentType)
				w.Header().Set(headers.ContentLength, strconv.Itoa(len(test.archive)))
				w.Write(test.archive)
			})
			srv := httptest.NewServer(f)
			t.Cleanup(srv.Close)

			ctx := testlog.WithTB(context.Background(), t)
			bio := biome.Local{
				WorkDir: t.TempDir(),
				HomeDir: t.TempDir(),
			}
			output := new(strings.Builder)
			opts := &Options{
				URL:            srv.URL + wantPath,
				DestinationDir: bio.JoinPath(bio.HomeDir, "extractpoint"),
				Biome:          bio,
				Output:         output,
				Downloader:     downloader.New(t.TempDir()),
				ExtractMode:    test.mode,
			}
			opts.Downloader.Client = srv.Client()

			if err := Extract(ctx, opts); err != nil {
				t.Error("extract:", err)
			}

			outPath := bio.JoinPath(opts.DestinationDir, "foo", "bar.txt")
			got, err := ioutil.ReadFile(outPath)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != extractContent {
				t.Errorf("%s content = %q; want %q", outPath, got, extractContent)
			}
		})
	}
}

func TestTopLevelZipFilenames(t *testing.T) {
	tests := []struct {
		name  string
		files []*zip.File

		root      string
		want      []string
		wantError bool
	}{
		{
			name:  "Empty",
			files: nil,
			want:  nil,
		},
		{
			name: "RootDirOnly",
			files: []*zip.File{
				{
					FileHeader: zip.FileHeader{
						Name: "foo/",
					},
				},
			},
			root: "foo",
			want: nil,
		},
		{
			name: "SingleFile",
			files: []*zip.File{
				{FileHeader: zip.FileHeader{Name: "foo/"}},
				{FileHeader: zip.FileHeader{Name: "foo/bar.txt"}},
			},
			want: []string{"bar.txt"},
			root: "foo",
		},
		{
			name: "FileWithoutRootEntry",
			files: []*zip.File{
				{FileHeader: zip.FileHeader{Name: "foo/bar.txt"}},
			},
			want: []string{"bar.txt"},
			root: "foo",
		},
		{
			name: "Subdirectory",
			files: []*zip.File{
				{FileHeader: zip.FileHeader{Name: "foo/"}},
				{FileHeader: zip.FileHeader{Name: "foo/bar/"}},
				{FileHeader: zip.FileHeader{Name: "foo/bar/baz.txt"}},
			},
			want: []string{"bar"},
			root: "foo",
		},
		{
			name: "ComplexTree",
			files: []*zip.File{
				{FileHeader: zip.FileHeader{Name: "foo/"}},
				{FileHeader: zip.FileHeader{Name: "foo/bar/"}},
				{FileHeader: zip.FileHeader{Name: "foo/bar/baz.txt"}},
				{FileHeader: zip.FileHeader{Name: "foo/quux/"}},
				{FileHeader: zip.FileHeader{Name: "foo/quux/spam.txt"}},
				{FileHeader: zip.FileHeader{Name: "foo/quux/eggs.txt"}},
				{FileHeader: zip.FileHeader{Name: "foo/myfile.dat"}},
			},
			want: []string{"bar", "quux", "myfile.dat"},
			root: "foo",
		},
		{
			name: "RootFile",
			files: []*zip.File{
				{FileHeader: zip.FileHeader{Name: "foo.txt"}},
			},
			wantError: true,
		},
		{
			name: "ChildMatchesRoot",
			files: []*zip.File{
				{FileHeader: zip.FileHeader{Name: "foo/foo"}},
			},
			wantError: true,
		},
		{
			name: "LowerDownMatchesRoot",
			files: []*zip.File{
				{FileHeader: zip.FileHeader{Name: "foo/bar/foo"}},
			},
			want: []string{"bar"},
			root: "foo",
		},
		{
			name: "DifferingRoots",
			files: []*zip.File{
				{FileHeader: zip.FileHeader{Name: "foo/bar"}},
				{FileHeader: zip.FileHeader{Name: "baz/quux"}},
			},
			wantError: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, got, err := topLevelZipFilenames(test.files)
			if err != nil {
				if test.wantError {
					t.Logf("Got expected error: %v", err)
				} else {
					t.Errorf("Unexpected error: %v", err)
				}
				return
			}
			if err == nil && test.wantError {
				t.Error("topLevelZipFilenames did not return an error")
				return
			}
			if root != test.root {
				t.Errorf("root = %q; want %q", root, test.root)
			}
			diff := cmp.Diff(
				test.want, got,
				cmpopts.EquateEmpty(),
				cmpopts.SortSlices(func(s1, s2 string) bool { return s1 < s2 }),
			)
			if diff != "" {
				t.Errorf("filenames (-want +got):\n%s", diff)
			}
		})
	}
}

func makeZip(fname string) []byte {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	f, err := zw.Create(fname)
	if err != nil {
		panic(err)
	}
	if _, err := io.WriteString(f, extractContent); err != nil {
		panic(err)
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func makeGzipTar(fname string) []byte {
	buf := new(bytes.Buffer)
	zw := gzip.NewWriter(buf)
	tw := tar.NewWriter(zw)
	err := tw.WriteHeader(&tar.Header{
		Name:     fname,
		Mode:     0644,
		Size:     int64(len(extractContent)),
		Typeflag: tar.TypeReg,
	})
	if err != nil {
		panic(err)
	}
	if _, err := io.WriteString(tw, extractContent); err != nil {
		panic(err)
	}
	if err := tw.Close(); err != nil {
		panic(err)
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func TestMain(m *testing.M) {
	testlog.Main(nil)
	os.Exit(m.Run())
}
