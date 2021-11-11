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
  rust_dir = biome.path.join(biome.dirs.tools, "rust", "rust-" + version)
  rust_download_dir = rust_dir + "-download"

  env = Environment(
    vars = {
      "CARGO_HOME": biome.path.join(biome.dirs.home, "cargohome"),
    },
    prepend_path = [
      biome.path.join(rust_dir, "bin"),
    ],
  )
  if biome.path.exists(rust_dir):
    return env

  os = {
    "linux":  "unknown-linux-gnu",
    "darwin": "apple-darwin",
  }.get(biome.os)
  if not os:
    fail("unsupported os %s" % biome.os)
  arch = {
    "amd64": "x86_64",
    "386":   "i686",
  }.get(biome.arch)
  if not arch:
    fail("unsupported arch %s" % biome.arch)

  download_url = ("https://static.rust-lang.org/dist/rust-{version}-{arch}-{os}.tar.gz"
    .format(version=version, os=os, arch=arch))
  downloader.extract(biome, dst_dir=rust_download_dir, url=download_url, mode="strip")

  biome.run(
    [biome.path.join(rust_download_dir, "install.sh"), "--prefix=" + rust_dir],
    dir=rust_download_dir,
  )
  return env
