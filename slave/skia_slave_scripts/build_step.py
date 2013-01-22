# Copyright (c) 2012 The Chromium Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

"""Base class for all slave-side build steps. """

import config
import multiprocessing
import os
import shlex
import signal
import subprocess
import sys
import threading
import time
import traceback

from playback_dirs import LocalSkpPlaybackDirs
from playback_dirs import StorageSkpPlaybackDirs
from utils import misc


DEFAULT_TIMEOUT = 2400
DEFAULT_NO_OUTPUT_TIMEOUT=3600
DEFAULT_NUM_CORES = 2


# multiprocessing.Value doesn't accept boolean types, so we have to use an int.
INT_TRUE = 1
INT_FALSE = 0
build_step_stdout_has_written = multiprocessing.Value('i', INT_FALSE)


# The canned acl to use while copying playback files to Google Storage.
PLAYBACK_CANNED_ACL = 'private'


class BuildStepWarning(Exception):
  pass


class BuildStepFailure(Exception):
  pass


class BuildStepTimeout(Exception):
  pass


class BuildStepLogger(object):
  """ Override stdout so that we can keep track of when anything has been
  logged.  This enables timeouts based on how long the process has gone without
  writing output.
  """
  def __init__(self):
    self.stdout = sys.stdout
    sys.stdout = self
    build_step_stdout_has_written.value = INT_FALSE

  def __del__(self):
    sys.stdout = self.stdout

  def fileno(self):
    return self.stdout.fileno()

  def write(self, data):
    build_step_stdout_has_written.value = INT_TRUE
    self.stdout.write(data)

  def flush(self):
    self.stdout.flush()


class BuildStep(multiprocessing.Process):
  def _PreRun(self):
    """ Optional preprocessing step for BuildSteps to override. """
    pass

  def __init__(self, args, attempts=1, timeout=DEFAULT_TIMEOUT,
               no_output_timeout=DEFAULT_NO_OUTPUT_TIMEOUT):
    """ Constructs a BuildStep instance.
    
    args: dictionary containing arguments to this BuildStep.
    attempts: how many times to try this BuildStep before giving up.
    timeout: maximum time allowed for this BuildStep.
    no_output_timeout: maximum time allowed for this BuildStep to run without
        any output.
    """
    multiprocessing.Process.__init__(self)
    self._args = args
    self._timeout = timeout
    self._no_output_timeout = no_output_timeout
    # Dimensions for BuildSteps which use tiling
    self.TILE_X = 256
    self.TILE_Y = 256

    self._configuration = args['configuration']
    self._gm_image_subdir = args['gm_image_subdir']
    self._builder_name = args['builder_name']
    self._target_platform = args['target_platform']
    self._revision = \
        None if args['revision'] == 'None' else int(args['revision'])
    self._got_revision = \
        None if args['got_revision'] == 'None' else int(args['got_revision'])
    self._attempts = attempts
    self._do_upload_results = (False if args['do_upload_results'] == 'None'
                               else args['do_upload_results'] == 'True')
    # Figure out where we are going to store images generated by GM.
    self._gm_actual_basedir = os.path.join(os.pardir, os.pardir, 'gm', 'actual')
    self._gm_merge_basedir = os.path.join(os.pardir, os.pardir, 'gm', 'merge')
    self._gm_expected_dir = os.path.join(os.pardir, 'gm-expected', self._gm_image_subdir)
    self._gm_actual_dir = os.path.join(self._gm_actual_basedir,
                                       self._gm_image_subdir)
    self._gm_actual_svn_baseurl = '%s/%s' % (args['autogen_svn_baseurl'],
                                             'gm-actual')
    self._skp_dir = os.path.join(os.pardir, 'skp')
    self._autogen_svn_username_file = '.autogen_svn_username'
    self._autogen_svn_password_file = '.autogen_svn_password'
    self._make_flags = shlex.split(args['make_flags'].replace('"', ''))
    self._test_args = shlex.split(args['test_args'].replace('"', ''))
    self._gm_args = shlex.split(args['gm_args'].replace('"', ''))
    self._gm_args.append('--serialize')
    self._bench_args = shlex.split(args['bench_args'].replace('"', ''))
    self._is_try = args['is_try'] == 'True'

    # Adding the playback directory transfer objects.
    self._local_playback_dirs = LocalSkpPlaybackDirs(
        self._builder_name, self._gm_image_subdir,
        None if args['perf_output_basedir'] == 'None'
            else args['perf_output_basedir'])
    self._storage_playback_dirs = StorageSkpPlaybackDirs(
        self._builder_name, self._gm_image_subdir,
        None if args['perf_output_basedir'] == 'None'
            else args['perf_output_basedir'])

    # Temporarily disable multi-process GM
    self._num_cores = 1
    #if args.get('num_cores') != 'None':
    #  self._num_cores = int(args.get('num_cores'))
    #else:
    #  self._num_cores = DEFAULT_NUM_CORES

    # Figure out where we are going to store performance output.
    if args['perf_output_basedir'] != 'None':
      self._perf_data_dir = os.path.join(args['perf_output_basedir'],
                                         self._builder_name, 'data')
      self._perf_graphs_dir = os.path.join(args['perf_output_basedir'],
                                           self._builder_name, 'graphs')
    else:
      self._perf_data_dir = None
      self._perf_graphs_dir = None

  def _PathToBinary(self, binary):
    """ Returns the path to the given built executable. """
    return os.path.join('out', self._configuration, binary)

  def _Run(self):
    """ Code to be run in a given BuildStep.  No return value; throws exception
    on failure.  Override this method in subclasses.
    """
    raise Exception('Cannot instantiate abstract BuildStep')

  def run(self):
    """ Internal method used by multiprocess.Process. _Run is provided to be
    overridden instead of this method to ensure that this implementation always
    runs.
    """
    # If a BuildStep has exceeded its allotted time, the parent process needs to
    # be able to kill the BuildStep process AND any which it has spawned,
    # without harming itself. On posix platforms, the terminate() method is
    # insufficient; it fails to kill the subprocesses launched by this process.
    # So, we use use the setpgrp() function to set a new process group for the
    # BuildStep process and its children and call os.killpg() to kill the group.
    if os.name == 'posix':
      os.setpgrp()
    try:
      self._Run()
    except BuildStepWarning as e:
      print e
      sys.exit(config.Master.retcode_warnings)

  def _WaitFunc(self, attempt):
    """ Waits a number of seconds depending upon the attempt number of a
    retry-able BuildStep before making the next attempt.  This can be overridden
    by subclasses and should be defined for attempt in [0, self._attempts - 1]

    This default implementation is exponential; we double the wait time with
    each attempt, starting with a 15-second pause between the first and second
    attempts.
    """
    base_secs = 15
    wait = base_secs * (2 ** attempt)
    print 'Retrying in %d seconds...' % wait
    time.sleep(wait)

  @staticmethod
  def KillBuildStep(step):
    """ Kills a running BuildStep.

    step: the running BuildStep instance to kill.
    """
    # On posix platforms, the terminate() method is insufficient; it fails to
    # kill the subprocesses launched by this process. So, we use use the
    # setpgrp() function to set a new process group for the BuildStep process
    # and its children and call os.killpg() to kill the group.
    if os.name == 'posix':
      os.killpg(os.getpgid(step.pid), signal.SIGTERM)
    elif os.name == 'nt':
      subprocess.call(['taskkill', '/F', '/T', '/PID', str(step.pid)])
    else:
      step.terminate()

  @staticmethod
  def RunBuildStep(StepType):
    """ Run a BuildStep, possibly making multiple attempts and handling
    timeouts.
    
    StepType: class type which subclasses BuildStep, indicating what step should
        be run. StepType should override _Run().
    """
    logger = BuildStepLogger()
    args = misc.ArgsToDict(sys.argv)
    attempt = 0
    while True:
      step = StepType(args=args)
      try:
        start_time = time.time()
        last_written_time = start_time
        step._PreRun()
        step.start()
        while step.is_alive():
          current_time = time.time()
          if current_time - start_time > step._timeout:
            BuildStep.KillBuildStep(step)
            raise BuildStepTimeout('Build step exceeded timeout of %d seconds' %
                                   step._timeout)
          elif current_time - last_written_time > step._no_output_timeout:
            BuildStep.KillBuildStep(step)
            raise BuildStepTimeout(
                'Build step exceeded %d seconds with no output' %
                step._no_output_timeout)
          time.sleep(1)
          if build_step_stdout_has_written.value == INT_TRUE:
            last_written_time = time.time()
        if step.exitcode == 0:
          return 0
        elif step.exitcode == config.Master.retcode_warnings:
          # A warning is considered to be an acceptable finishing state.
          return config.Master.retcode_warnings
        else:
          raise BuildStepFailure('Build step failed.')
      except:
        print traceback.format_exc()
        if attempt + 1 >= step._attempts:
          raise
      step._WaitFunc(attempt)
      attempt += 1
      print '**** %s, attempt %d ****' % (StepType.__name__, attempt + 1)

