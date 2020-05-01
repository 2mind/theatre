package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/alecthomas/kingpin"

	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp" // this is required to auth against GCP

	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/gocardless/theatre/cmd"
	"github.com/gocardless/theatre/pkg/apis"
	"github.com/gocardless/theatre/pkg/apis/workloads"
	"github.com/gocardless/theatre/pkg/signals"
	"github.com/gocardless/theatre/pkg/workloads/console"
	"github.com/gocardless/theatre/pkg/workloads/priority"
)

// Set by goreleaser
var (
	// Version is set at compile time
	Version   = "dev"
	Commit    = "none"
	Date      = "unknown"
	BuiltBy   = "unknown"
	GoVersion = runtime.Version()
)

var (
	app         = kingpin.New("workloads-manager", "Manages workloads.crd.gocardless.com resources").Version(versionStanza())
	webhookName = app.Flag("webhook-name", "Kubernetes mutating webhook name").Default("theatre-workloads").String()
	namespace   = app.Flag("namespace", "Kubernetes webhook service namespace").Default("theatre-system").String()
	serviceName = app.Flag("service-name", "Kubernetes webhook service name").Default("theatre-workloads-manager").String()

	commonOpts = cmd.NewCommonOptions(app).WithMetrics(app)
)

func main() {
	kingpin.MustParse(app.Parse(os.Args[1:]))
	logger := commonOpts.Logger()

	if err := apis.AddToScheme(scheme.Scheme); err != nil {
		app.Fatalf("failed to add schemes: %v", err)
	}

	go func() {
		commonOpts.ListenAndServeMetrics(logger)
	}()

	ctx, cancel := signals.SetupSignalHandler()
	defer cancel()

	mgr, err := manager.New(config.GetConfigOrDie(), manager.Options{})
	if err != nil {
		app.Fatalf("failed to create manager: %v", err)
	}

	opts := webhook.ServerOptions{
		CertDir: "/tmp/theatre-workloads",
		BootstrapOptions: &webhook.BootstrapOptions{
			MutatingWebhookConfigName: *webhookName,
			Service: &webhook.Service{
				Namespace: *namespace,
				Name:      *serviceName,
				Selectors: map[string]string{
					"app":   "theatre",
					"group": workloads.GroupName,
				},
			},
		},
	}

	svr, err := webhook.NewServer("workloads", mgr, opts)
	if err != nil {
		app.Fatalf("failed to create admission server: %v", err)
	}

	// Console controller
	if _, err = console.Add(ctx, logger, mgr); err != nil {
		app.Fatalf(err.Error())
	}

	// Console authenticator webhook
	consoleAuthenticatorWh, err := console.NewAuthenticatorWebhook(logger, mgr)
	if err != nil {
		app.Fatalf(err.Error())
	}

	// Console authorisation webhook
	consoleAuthorisationWh, err := console.NewAuthorisationWebhook(logger, mgr)
	if err != nil {
		app.Fatalf(err.Error())
	}

	// Console template webhook
	consoleTemplateWh, err := console.NewTemplateValidationWebhook(logger, mgr)
	if err != nil {
		app.Fatalf(err.Error())
	}

	priorityWh, err := priority.NewWebhook(logger, mgr, priority.InjectorOptions{})
	if err != nil {
		app.Fatalf(err.Error())
	}

	if err := svr.Register(consoleAuthenticatorWh, consoleAuthorisationWh, consoleTemplateWh, priorityWh); err != nil {
		app.Fatalf(err.Error())
	}

	if err := mgr.Start(ctx.Done()); err != nil {
		app.Fatalf("failed to run manager: %v", err)
	}
}

func versionStanza() string {
	return fmt.Sprintf(
		"Version: %v\nGit SHA: %v\nGo Version: %v\nGo OS/Arch: %v/%v\nBuilt at: %v",
		Version, Commit, GoVersion, runtime.GOOS, runtime.GOARCH, Date,
	)
}
