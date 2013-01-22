#!/usr/bin/env python
# Copyright (c) 2012 The Chromium Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

"""Runs the bench_pictures executable on skp files from archived webpages.

This module can be run from the command-line like this:

cd buildbot/third_party/chromium_buildbot/slave/\
Skia_Shuttle_Ubuntu12_ATI5770_Float_Release_64/build/trunk

PYTHONPATH=../../../../site_config:\
../../../../scripts \
python ../../../../../../slave/skia_slave_scripts/bench_webpage_pictures.py \
--configuration "Debug" --target_platform "" --revision 6444 \
--autogen_svn_baseurl "" --make_flags "" --test_args "" --gm_args "" \
--bench_args "" --num_cores 8 --perf_output_basedir "../../../../perfdata" \
--builder_name Skia_Shuttle_Ubuntu12_ATI5770_Float_Release_64 \
--got_revision 6444 --gm_image_subdir "" \
--do_upload_results True --dest_gsbase gs://rmistry

"""

import os
import posixpath
import sys

from build_step import BuildStep
from bench_pictures import BenchPictures
from slave import slave_utils
from utils import gs_utils
from utils import sync_bucket_subdir

import build_step
import compare_and_upload_webpage_gms


class BenchWebpagePictures(BenchPictures):
  """Runs the bench_pictures executable on skp files from archived webpages."""

  def __init__(
      self,
      timeout=(
          build_step.DEFAULT_TIMEOUT *
          compare_and_upload_webpage_gms.SKP_TIMEOUT_MULTIPLIER),
      no_output_timeout=(
          build_step.DEFAULT_NO_OUTPUT_TIMEOUT *
          compare_and_upload_webpage_gms.SKP_TIMEOUT_MULTIPLIER),
      **kwargs):
    super(BenchWebpagePictures, self).__init__(
        timeout=timeout,
        no_output_timeout=no_output_timeout,
        **kwargs)

  def _GetSkpDir(self):
    """Points to the local playback skp directory."""
    return self._local_playback_dirs.PlaybackSkpDir()

  def _GetPerfDataDir(self):
    """Points to the local playback perf data directory."""
    return self._local_playback_dirs.PlaybackPerfDataDir()

  def _PopulateSkpDir(self):
    """Copies over skp files from Google Storage if the timestamps differ."""
    dest_gsbase = (self._args.get('dest_gsbase') or
                   sync_bucket_subdir.DEFAULT_PERFDATA_GS_BASE)
    print '\n\n========Downloading skp files from Google Storage========\n\n'
    gs_utils.DownloadDirectoryContentsIfChanged(
        gs_base=dest_gsbase,
        gs_relative_dir=self._storage_playback_dirs.PlaybackSkpDir(),
        local_dir=self._local_playback_dirs.PlaybackSkpDir())


if '__main__' == __name__:
  sys.exit(BuildStep.RunBuildStep(BenchWebpagePictures))
