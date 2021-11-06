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
  node_dir = biome.path.join(biome.dirs.tools, "nodejs", "node-" + version)
  env = Environment(
    vars = {
      "NODE_PATH": biome.dirs.work,
    },
    prepend_path = [
      biome.path.join(biome.dirs.work, "node_modules", ".bin"),
      biome.path.join(node_dir, "bin"),
    ],
  )

  if biome.path.exists(node_dir):
    return env

  os = {
    "linux": "linux",
    "darwin": "darwin",
  }.get(biome.os)
  if not os:
    fail("unsupported os %s" % biome.os)
  arch = {
    "amd64": "x64",
  }.get(biome.arch)
  if not arch:
    fail("unsupported arch %s" % biome.arch)

  download_url = ("https://nodejs.org/dist/v{version}/node-v{version}-{os}-{arch}.tar.gz"
    .format(version=version, os=os, arch=arch))
  downloader.extract(biome, dst_dir=node_dir, url=download_url, mode="strip")
  return env
