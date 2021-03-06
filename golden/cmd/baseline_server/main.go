// This program serves content that is mostly static and needs to be highly
// available. The content comes from highly available backend services like
// GCS. It needs to be deployed in a redundant way to ensure high uptime.
package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gorilla/mux"
	"go.skia.org/infra/go/auth"
	"go.skia.org/infra/go/common"
	"go.skia.org/infra/go/ds"
	"go.skia.org/infra/go/eventbus"
	"go.skia.org/infra/go/git/gitinfo"
	"go.skia.org/infra/go/gitiles"
	"go.skia.org/infra/go/gitstore"
	"go.skia.org/infra/go/gitstore/bt_gitstore"
	"go.skia.org/infra/go/httputils"
	"go.skia.org/infra/go/skiaversion"
	"go.skia.org/infra/go/sklog"
	"go.skia.org/infra/go/vcsinfo"
	"go.skia.org/infra/go/vcsinfo/bt_vcs"
	"go.skia.org/infra/golden/go/baseline/gcs_baseliner"
	"go.skia.org/infra/golden/go/expstorage/ds_expstore"
	"go.skia.org/infra/golden/go/shared"
	"go.skia.org/infra/golden/go/storage"
	"go.skia.org/infra/golden/go/tryjobstore"
	"go.skia.org/infra/golden/go/web"
	"google.golang.org/api/option"
	gstorage "google.golang.org/api/storage/v1"
)

func main() {
	// Command line flags.
	var (
		baselineGSPath     = flag.String("baseline_gs_path", "", "GS path, where the baseline file are stored. This should match the same flag in skiacorrectness which writes the baselines. Format: <bucket>/<path>.")
		dsNamespace        = flag.String("ds_namespace", "", "Cloud datastore namespace to be used by this instance.")
		gitBTInstanceID    = flag.String("git_bt_instance", "", "ID of the BigTable instance that contains Git metadata")
		gitBTTableID       = flag.String("git_bt_table", "", "ID of the BigTable table that contains Git metadata")
		gitRepoDir         = flag.String("git_repo_dir", "", "Directory location for the Skia repo.")
		gitRepoURL         = flag.String("git_repo_url", "https://skia.googlesource.com/skia", "The URL to pass to git clone for the source repository.")
		hashesGSPath       = flag.String("hashes_gs_path", "", "GS path, where the known hashes file should be stored. This should match the same flag in skiacorrectness which writes the hashes. Format: <bucket>/<path>.")
		noCloudLog         = flag.Bool("no_cloud_log", false, "Disables cloud logging. Primarily for running locally and in K8s.")
		port               = flag.String("port", ":9000", "HTTP service address (e.g., ':9000')")
		projectID          = flag.String("project_id", common.PROJECT_ID, "GCP project ID.")
		promPort           = flag.String("prom_port", ":20000", "Metrics service address (e.g., ':10110')")
		serviceAccountFile = flag.String("service_account_file", "", "Credentials file for service account.")
	)

	// Parse the options. So we can configure logging.
	flag.Parse()

	// Set up the logging options.
	logOpts := []common.Opt{
		common.PrometheusOpt(promPort),
	}

	// Should we disable cloud logging.
	if !*noCloudLog {
		logOpts = append(logOpts, common.CloudLoggingOpt())
	}
	_, appName := filepath.Split(os.Args[0])
	common.InitWithMust(appName, logOpts...)
	skiaversion.MustLogVersion()

	// Get the client to be used to access GCS and the Monorail issue tracker.
	tokenSource, err := auth.NewJWTServiceAccountTokenSource("", *serviceAccountFile, gstorage.CloudPlatformScope, "https://www.googleapis.com/auth/userinfo.email")
	if err != nil {
		sklog.Fatalf("Failed to authenticate service account: %s", err)
	}

	// TODO(dogben): Ok to add request/dial timeouts?
	client := httputils.DefaultClientConfig().WithTokenSource(tokenSource).WithoutRetries().Client()
	evt := eventbus.New()
	ctx := context.Background()

	// TODO(stephana): There is a lot of overlap with code in skiacorrectness/main.go. This should
	// be factored out into a common function.
	gsClientOpt := storage.GCSClientOptions{
		HashesGSPath:   *hashesGSPath,
		BaselineGSPath: *baselineGSPath,
	}

	gsClient, err := storage.NewGCSClient(client, gsClientOpt)
	if err != nil {
		sklog.Fatalf("Unable to create GCSClient: %s", err)
	}

	if err := ds.InitWithOpt(*projectID, *dsNamespace, option.WithTokenSource(tokenSource)); err != nil {
		sklog.Fatalf("Unable to configure cloud datastore: %s", err)
	}

	// Set up the cloud expectations store
	expStore, issueExpStoreFactory, err := ds_expstore.New(ds.DS, evt)
	if err != nil {
		sklog.Fatalf("Unable to configure cloud expectations store: %s", err)
	}

	tryjobStore, err := tryjobstore.NewCloudTryjobStore(ds.DS, issueExpStoreFactory, evt)
	if err != nil {
		sklog.Fatalf("Unable to instantiate tryjob store: %s", err)
	}

	var vcs vcsinfo.VCS
	if *gitBTInstanceID != "" && *gitBTTableID != "" {
		btConf := &bt_gitstore.BTConfig{
			ProjectID:  *projectID,
			InstanceID: *gitBTInstanceID,
			TableID:    *gitBTTableID,
		}
		var gitStore gitstore.GitStore
		gitStore, err = bt_gitstore.New(ctx, btConf, *gitRepoURL)
		if err != nil {
			sklog.Fatalf("Error instantiating gitstore: %s", err)
		}
		gitilesRepo := gitiles.NewRepo("", "", nil)
		vcs, err = bt_vcs.New(gitStore, "master", gitilesRepo, nil, 0)
	} else {
		vcs, err = gitinfo.CloneOrUpdate(ctx, *gitRepoURL, *gitRepoDir, false)
	}
	if err != nil {
		sklog.Fatalf("Error creating VCS instance: %s", err)
	}

	// Initialize the Baseliner instance from the values set above.
	baseliner, err := gcs_baseliner.New(gsClient, expStore, issueExpStoreFactory, tryjobStore, vcs)
	if err != nil {
		sklog.Fatalf("Error initializing baseliner: %s", err)
	}

	// We only need to fill in the Storage struct with the following subset, since the baseline
	// server only supplies a subset of the functionality.
	storages := &storage.Storage{
		GCSClient: gsClient,
		Baseliner: baseliner,
	}

	handlers := web.WebHandlers{
		Storages: storages,
	}

	// Set up a router for all the application endpoints which are part of the Gold API.
	appRouter := mux.NewRouter()

	// Serve the known hashes from GCS.
	appRouter.HandleFunc(shared.KNOWN_HASHES_ROUTE, handlers.TextKnownHashesProxy).Methods("GET")
	appRouter.HandleFunc(shared.LEGACY_KNOWN_HASHES_ROUTE, handlers.TextKnownHashesProxy).Methods("GET")

	// Serve the expectations for the master branch and for CLs in progress.
	appRouter.HandleFunc(shared.EXPECTATIONS_ROUTE, handlers.JsonBaselineHandler).Methods("GET")
	appRouter.HandleFunc(shared.EXPECTATIONS_ISSUE_ROUTE, handlers.JsonBaselineHandler).Methods("GET")

	// Only log and compress the app routes, but not the health check.
	router := mux.NewRouter()
	router.HandleFunc("/healthz", httputils.ReadyHandleFunc)
	router.PathPrefix("/").Handler(httputils.LoggingGzipRequestResponse(appRouter))

	// Start the server
	sklog.Infof("Serving on http://127.0.0.1" + *port)
	sklog.Fatal(http.ListenAndServe(*port, router))
}
