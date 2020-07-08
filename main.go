/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	rbacv1alpha1 "github.com/gocardless/theatre/apis/rbac/v1alpha1"
	// vaultv1alpha1 "github.com/gocardless/theatre/apis/vault/v1alpha1"
	vaultv1alpha1 "github.com/gocardless/theatre/apis/vault/v1alpha1"
	workloadsv1alpha1 "github.com/gocardless/theatre/apis/workloads/v1alpha1"
	workloadscontroller "github.com/gocardless/theatre/controllers/workloads"
	"github.com/gocardless/theatre/pkg/signals"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = rbacv1alpha1.AddToScheme(scheme)
	_ = workloadsv1alpha1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		Port:               9443,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "e34b07cb.crds.gocardless.com",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	ctx, _ := signals.SetupSignalHandler()

	// if err = (&rbaccontroller.DirectoryRoleBindingReconciler{
	// 	Client: mgr.GetClient(),
	// 	Log:    ctrl.Log.WithName("controllers").WithName("DirectoryRoleBinding"),

	// 	Scheme: mgr.GetScheme(),
	// }).SetupWithManager(ctx, mgr, nil, time.Hour); err != nil {
	// 	setupLog.Error(err, "unable to create controller", "controller", "DirectoryRoleBinding")
	// 	os.Exit(1)
	// }

	if err = (&workloadscontroller.ConsoleReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Console"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Console")
		os.Exit(1)
	}

	if err = (&workloadsv1alpha1.ConsoleTemplate{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "ConsoleTemplate")
		os.Exit(1)
	}

	// +kubebuilder:scaffold:builder

	mgr.GetWebhookServer().Register("/mutate-v1-pod-priority", &admission.Webhook{
		Handler: workloadsv1alpha1.NewPriorityInjector(
			mgr.GetClient(),
			ctrl.Log.WithName("webhooks").WithName("priority-injector"),
		),
	})
	mgr.GetWebhookServer().Register("/mutate-workloads-crd-gocardless-com-v1alpha1-console", &admission.Webhook{
		Handler: workloadsv1alpha1.NewConsoleAuthenticator(
			ctrl.Log.WithName("webhooks").WithName("console-authenticator"),
		),
	})
	mgr.GetWebhookServer().Register("/mutate-vault-crd-gocardless-com-v1alpha1-envconsulinjector", &admission.Webhook{
		Handler: vaultv1alpha1.NewEnvconsulInjector(
			mgr.GetClient(),
			ctrl.Log.WithName("webhooks").WithName("envconsul-injector"),
			vaultv1alpha1.EnvconsulInjectorOptions{},
		),
	})
	mgr.GetWebhookServer().Register("/mutate-workloads-crd-gocardless-com-v1alpha1-consoleauthorisation", &admission.Webhook{
		Handler: workloadsv1alpha1.NewConsoleAuthorisationWebhook(
			mgr.GetClient(),
			ctrl.Log.WithName("webhooks").WithName("console-authorisations"),
		),
	})

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
