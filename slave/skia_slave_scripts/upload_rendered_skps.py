#!/usr/bin/env python
# Copyright (c) 2014 The Chromium Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

"""Uploads the results of render_skps.py."""

import contextlib
import os
import sys
import urllib

from build_step import BuildStep
from utils import gs_utils
import skia_vars

PREFETCH_URL_FORMATTER = (
    'http://skia-tree-status.appspot.com/redirect/rebaseline-server/prefetch/'
    'setADir={set_a_dir}&setBDir={set_b_dir}')


class UploadRenderedSKPs(BuildStep):

  def __init__(self, attempts=3, **kwargs):
    super(UploadRenderedSKPs, self).__init__(
        attempts=attempts, **kwargs)
    self._gs_images_bucket = skia_vars.GetGlobalVariable('skp_images_bucket')
    self._gs_summaries_bucket = skia_vars.GetGlobalVariable(
        'skp_summaries_bucket')

  def _Run(self):
    gs = gs_utils.GSUtils()

    # Upload any new image files to Google Storage.
    # We use checksum-based filenames, so any files which have already been
    # uploaded don't need to be uploaded again.
    src_dir = os.path.abspath(self.playback_actual_images_dir)
    dest_bucket = self._gs_images_bucket
    if os.listdir(src_dir):
      print 'Uploading image files from %s to gs://%s/' % (
          src_dir, dest_bucket)
      gs.upload_dir_contents(
          source_dir=src_dir, dest_bucket=dest_bucket, dest_dir=None,
          upload_if=gs.UploadIf.IF_NEW,
          predefined_acl=gs.PLAYBACK_CANNED_ACL,
          fine_grained_acl_list=gs.PLAYBACK_FINEGRAINED_ACL_LIST)
    else:
      print ('Skipping upload to Google Storage, because no image files in %s' %
             src_dir)

    # Upload image summaries (checksums) to Google Storage.
    #
    # It's important to only upload each summary file if it has changed,
    # because we use the history of the file in Google Storage to tell us
    # when any of the results changed.
    src_dir = os.path.abspath(self.playback_actual_summaries_dir)
    dest_bucket = self._gs_summaries_bucket
    dest_dir = self._args['builder_name']
    if os.listdir(src_dir):
      print ('Uploading image summaries from %s to gs://%s/%s' % (
          src_dir, dest_bucket, dest_dir))
      gs.upload_dir_contents(
          source_dir=src_dir, dest_bucket=dest_bucket, dest_dir=dest_dir,
          upload_if=gs.UploadIf.IF_MODIFIED,
          predefined_acl=gs.PLAYBACK_CANNED_ACL,
          fine_grained_acl_list=gs.PLAYBACK_FINEGRAINED_ACL_LIST)
      # Tell rebaseline_server to prefetch latest SKP results.
      prefetch_url = PREFETCH_URL_FORMATTER.format(
          set_a_dir=urllib.quote('gs://%s/%s' % (dest_bucket, dest_dir),
                                 safe=''),
          set_b_dir=urllib.quote('repo:expectations/skp/%s' % dest_dir,
                                 safe=''))
      print ('Triggering prefetch URL %s' % prefetch_url)
      with contextlib.closing(urllib.urlopen(prefetch_url)):
        pass
    else:
      print ('Skipping upload to Google Storage, because no image summaries '
             'in %s' % src_dir)

if '__main__' == __name__:
  sys.exit(BuildStep.RunBuildStep(UploadRenderedSKPs))
