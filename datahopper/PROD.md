Datahopper Production Manual
============================

Alerts
======

job_metrics
-----------

The [job
metrics](https://skia.googlesource.com/buildbot/+/master/datahopper/go/datahopper/jobs.go)
goroutine has not successfully updated its job cache for some time.

If there are Task Scheduler alerts, resolve those first.

Otherwise, you should check the
[logs](https://console.cloud.google.com/logs/viewer?project=google.com:skia-buildbots&minLogLevel=500&expandAll=false&resource=logging_log%2Fname%2Fskia-datahopper2&logName=projects%2Fgoogle.com:skia-buildbots%2Flogs%2Fdatahopper)
to try to diagnose what's failing.


bot_coverage_metrics
--------------------

The [bot coverage
metrics](https://skia.googlesource.com/buildbot/+/master/datahopper/go/bot_metrics/bot_metrics.go)
goroutine has not successfully completed a cycle for some time. You should
check the
[logs](https://console.cloud.google.com/logs/viewer?project=google.com:skia-buildbots&minLogLevel=500&expandAll=false&resource=logging_log%2Fname%2Fskia-datahopper2&logName=projects%2Fgoogle.com:skia-buildbots%2Flogs%2Fdatahopper)
to try to diagnose what's failing.


swarming_task_metrics
--------------------

The [Swarming task
metrics](https://skia.googlesource.com/buildbot/+/master/datahopper/go/swarming_metrics/tasks.go)
goroutine has not successfully queried for Swarming tasks for some time. You should
check the
[logs](https://console.cloud.google.com/logs/viewer?project=google.com:skia-buildbots&minLogLevel=500&expandAll=false&resource=logging_log%2Fname%2Fskia-datahopper2&logName=projects%2Fgoogle.com:skia-buildbots%2Flogs%2Fdatahopper)
to try to diagnose what's failing.


event_metrics
-------------

The [event
metrics](https://skia.googlesource.com/buildbot/+/master/go/metrics2/events/events.go)
goroutine has not successfully updated metrics based on event data for some
time. You should check the logs to try to diagnose what's failing. Double-check
the instance name to verify which log stream to investigate.


swarming_bot_metrics
--------------------

The [Swarming bot
metrics](https://skia.googlesource.com/buildbot/+/master/datahopper/go/swarming_metrics/bots.go)
goroutine has not successfully queried for Swarming bots for some time. See the
alert for which pool and server is failing. You should check the
[logs](https://console.cloud.google.com/logs/viewer?project=google.com:skia-buildbots&minLogLevel=500&expandAll=false&resource=logging_log%2Fname%2Fskia-datahopper2&logName=projects%2Fgoogle.com:skia-buildbots%2Flogs%2Fdatahopper)
to try to diagnose what's failing.


firestore_backup_metrics
------------------------

The [Firestore backup
metrics](https://skia.googlesource.com/buildbot/+/master/datahopper/go/datahopper/firestore_backup_metrics.go)
goroutine has not successfully updated the metric for most recent Firestore
backup for some time.

Try running `gcloud beta firestore operations list --project=skia-firestore `. If
no output or error, check for a GCP Firestore outage.

Otherwise, you should check the
[logs](https://console.cloud.google.com/logs/viewer?project=google.com:skia-buildbots&minLogLevel=500&expandAll=false&resource=logging_log%2Fname%2Fskia-datahopper2&logName=projects%2Fgoogle.com:skia-buildbots%2Flogs%2Fdatahopper)
to try to diagnose what's failing.


firestore_nightly_backup
------------------------

The nightly backup of all Firestore collections in the skia-firestore project
has not succeeded in more than 24 hours. There are several things to check:

 - Run `gcloud beta firestore operations list --project=skia-firestore
   "--filter=metadata.outputUriPrefix~^gs://skia-firestore-backup/everything/" |
   grep -C 14 "endTime: '$(date --utc +%Y-%m-)"` (please modify if it's the
   first of the month).

   - If you see a recent endTime with "operationState: SUCCESSFUL," see below
     for diagnosing issues in Datahopper.
   - If you see a recent endTime with any other operationState, see below for
     diagnosing issues with the Firestore export.
   - If you don't see a recent endTime, see below for diagnosing issues with the
     Kubernetes CronJob.
   - If no output (without filtering through grep) or error, check for a GCP
     Firestore outage.

 - Check the [Datahopper
   logs](https://console.cloud.google.com/logs/viewer?project=google.com:skia-buildbots&minLogLevel=500&expandAll=false&resource=logging_log%2Fname%2Fskia-datahopper2&logName=projects%2Fgoogle.com:skia-buildbots%2Flogs%2Fdatahopper)
   for any warnings or errors. One likely problem is a change in the output of
   the REST API. See [the
   code](https://skia.googlesource.com/buildbot/+/master/datahopper/go/datahopper/firestore_backup_metrics.go)
   for the URL used to retrieve Firestore export operations. You can also run
   Datahopper locally using the --local flag to set up a TokenSource to
   authenticate to this URL. Add logging of the HTTP response.

 - If the export operation is in progress more than an hour after the startTime
   (remember it's UTC), it's probably stuck. You can cancel it with `gcloud beta
   firestore operations cancel --project=skia-firestore <value of name
   field>`. Then manually trigger a new export (see below).

 - If the export operation failed for any other reason, look for an error
   message in the output from `operations list` above. If the error is
   transient, manually trigger a new export (see below). Otherwise, try a Google
   search for the error.

 - Check the logs for the most recent run of the
   [firestore-export-everything-nightly](https://console.cloud.google.com/kubernetes/cronjob/us-central1-a/skia-public/default/firestore-export-everything-nightly?project=skia-public&folder&organizationId=433637338589)
   CronJob. If no recent run, check for misconfiguration. You can update the
   CronJob by running `make push` in the `firestore` directory. The
   configuration for the CronJob is
   [here](https://skia.googlesource.com/skia-public-config/+/master/firestore-export-everything-nightly.yaml).

 - To manually trigger a new export, run `gcloud beta firestore export
   --project=skia-firestore --async gs://skia-firestore-backup/everything/$(date
   --utc +%Y-%m-%dT%H:%M:%SZ)`. Alternatively, run `kubectl create job
   --from=cronjob/firestore-export-everything-nightly
   firestore-export-everything-manual`, wait for the job to finish, then run
   `kubectl delete job firestore-export-everything-manual`.
