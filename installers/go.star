# Copyright 2021 Ross Light
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
# SPDX-License-Identifier: Apache-2.0

def install(biome, version, downloader, **_):
  golang_root = biome.path.join(biome.dirs.tools, "go")
  golang_dir = biome.path.join(golang_root, "go"+version)
  gopath_dir = biome.path.join(golang_root, "gopath")

  env = Environment(
    vars = {
      "GOROOT": golang_dir,
      "GOPATH": gopath_dir + ":" + biome.dirs.work,
    },
    prepend_path = [
      biome.path.join(gopath_dir, "bin"),
      biome.path.join(golang_dir, "bin"),
    ],
  )
  if biome.path.exists(golang_dir):
    return env

  download_url = "https://dl.google.com/go/go%s.%s-%s.tar.gz" % (version, biome.os, biome.arch)
  downloader.extract(biome, dst_dir=golang_dir, url=download_url, mode="strip")
  return env
