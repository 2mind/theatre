package acceptance

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	kitlog "github.com/go-kit/kit/log"
	"github.com/hashicorp/vault/api"
	"github.com/pkg/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// This should be in testdata, but right now our test runner doesn't support relative file
// access. We should aim to bring back the ability to run acceptance tests from the ginkgo
// wrapper.
const podFixtureYAML = `
---
apiVersion: v1
kind: Pod
metadata:
  generateName: read-a-secret-
  namespace: staging # provisioned by the acceptance kustomize overlay
spec:
  serviceAccountName: secret-reader
  restartPolicy: Never
  containers:
    - name: print-env
      image: theatre:latest
      imagePullPolicy: Never
      env:
        - name: VAULT_RESOLVED_KEY
          value: vault:secret/data/kubernetes/staging/secret-reader/jimmy
      command:
        - /usr/local/bin/theatre-envconsul
      args:
        - exec
        - --vault-address=http://vault.vault.svc.cluster.local:8200
        - --no-vault-use-tls
        - --install-path=/usr/local/bin
        - --
        - env
`

const (
	AuthBackendMountPath = "kubernetes"
	AuthBackendRole      = "default"
	SentinelSecretValue  = "eats-the-world"
)

type Runner struct{}

// Prepare is used for configuring a Vault server in our acceptance tests to provide
// Kubernetes authentication via service account.
//
// It does several things:
//
// - Mounts a kv2 secrets engine at secret/
// - Creates a Kubernetes auth backend mounted at auth/kubernetes
// - Configures the Kubernetes backend to authenticate against the currently detected
//   Kubernetes API server (the current cluster, if run from within)
// - For all successful Kubernetes logins, the user is assigned a token that maps to a
//   cluster-reader policy, which permits reading of secrets from:
//
//   secret/data/kubernetes/{namespace}/{service-account-name}/*
//
func (r *Runner) Prepare(logger kitlog.Logger, config *rest.Config) error {
	cfg := api.DefaultConfig()
	cfg.Address = "http://localhost:8200"

	transport := cfg.HttpClient.Transport.(*http.Transport)
	transport.TLSClientConfig = nil

	client, err := api.NewClient(cfg)
	if err != nil {
		return errors.Wrap(err, "failed to configure vault client")
	}

	client.SetToken("vault-token") // set in the acceptance overlay (config/overlays/acceptance)

	mountPath := "secret"
	mountOptions := &api.MountInput{
		Type:        "kv",
		Description: "Generic Vault kv mount",
		Options: map[string]string{
			"version": "2",
		},
	}

	logger.Log("msg", "mounting secret engine", "path", mountPath, "options", mountOptions)
	client.Sys().Unmount(mountPath)
	if err := client.Sys().Mount(mountPath, mountOptions); err != nil {
		return err
	}

	enableOptions := &api.EnableAuthOptions{
		Type:        "kubernetes",
		Description: "Permit authentication by Kubernetes service accounts",
	}

	logger.Log("msg", "enabling auth mount", "path", AuthBackendMountPath, "options", enableOptions)
	client.Sys().DisableAuth(AuthBackendMountPath)
	if err := client.Sys().EnableAuthWithOptions(AuthBackendMountPath, enableOptions); err != nil {
		return err
	}

	var ca []byte = config.CAData

	if len(ca) == 0 {
		ca, err = ioutil.ReadFile(config.CAFile)
		if err != nil {
			return errors.Wrap(err, "could not parse certificate for kubernetes")
		}
	}

	// We'll be running the acceptance tests from outside the kubernetes cluster, where the
	// API server will have an IP address that is relative to the host machine. When we're
	// within the cluster, like Vault, we want to talk to kubernetes.default.svc to ensure
	// we're tapping the host IP address.
	backendConfigPath := fmt.Sprintf("auth/%s/config", AuthBackendMountPath)
	backendConfig := map[string]interface{}{
		"kubernetes_host":    "https://kubernetes.default.svc",
		"kubernetes_ca_cert": string(ca),
	}

	logger.Log("msg", "writing auth backend config", "path", backendConfigPath, "config", backendConfig)
	if _, err := client.Logical().Write(backendConfigPath, backendConfig); err != nil {
		return err
	}

	backendRolePath := fmt.Sprintf("auth/%s/role/default", AuthBackendMountPath)
	backendRoleConfig := map[string]interface{}{
		// https://github.com/hashicorp/vault-plugin-auth-kubernetes/pull/66
		"bound_service_account_names": strings.Split(
			"a*,b*,c*,d*,e*,f*,h*,i*,j*,k*,l*,m*,n*,o*,p*,q*,r*,s*,t*,u*,v*,w*,x*,y*,z*,1*,2*,3*,4*,5*,6*,7*,8*,9*,0*", ",",
		),
		"bound_service_account_namespaces": []string{"*"},
		"token_policies":                   []string{"default", "cluster-reader"},
		"token_ttl":                        600,
	}

	logger.Log("msg", "creating default backend role", "path", backendRolePath)
	if _, err := client.Logical().Write(backendRolePath, backendRoleConfig); err != nil {
		return err
	}

	auths, err := client.Sys().ListAuth()
	if err != nil {
		return errors.Wrap(err, "could not list auth backends which prevents linking roles against a backend")
	}

	backend := auths[fmt.Sprintf("%s/", AuthBackendMountPath)]
	readerPathTemplate :=
		"{{identity.entity.aliases.%s.metadata.service_account_namespace}}/" +
			"{{identity.entity.aliases.%s.metadata.service_account_name}}/" +
			"*"

	policyRules := fmt.Sprintf(
		`path "secret/data/kubernetes/%s" { capabilities = ["read"] }`,
		fmt.Sprintf(readerPathTemplate, backend.Accessor, backend.Accessor),
	)

	logger.Log("msg", "creating cluster-reader policy to permit kubernetes service accounts to read secrets")
	if err := client.Sys().PutPolicy("cluster-reader", policyRules); err != nil {
		return err
	}

	secretPath := "secret/data/kubernetes/staging/secret-reader/jimmy"
	secretData := map[string]interface{}{"data": map[string]interface{}{"data": SentinelSecretValue}}

	logger.Log("msg", "writing sentinel secret value", "path", secretPath)
	if _, err := client.Logical().Write(secretPath, secretData); err != nil {
		return err
	}

	return nil
}

func (r *Runner) Run(logger kitlog.Logger, config *rest.Config) {
	var (
		clientset *kubernetes.Clientset
	)

	BeforeEach(func() {
		var err error
		clientset, err = kubernetes.NewForConfig(config)
		Expect(err).NotTo(HaveOccurred(), "failed to create kubernetes clientset")
	})

	Describe("theatre-envconsul", func() {
		It("Resolves env variables into the pod command", func() {
			decoder := scheme.Codecs.UniversalDeserializer()
			obj, _, err := decoder.Decode([]byte(podFixtureYAML), nil, nil)
			podFixture := obj.(*corev1.Pod)

			podsClient := clientset.CoreV1().Pods("staging")
			pod, err := podsClient.Create(podFixture)
			Expect(err).NotTo(HaveOccurred(), "failed to create pod")

			getPodPhase := func() corev1.PodPhase {
				pod, err := podsClient.Get(pod.Name, metav1.GetOptions{})
				if err != nil {
					return ""
				}

				return pod.Status.Phase
			}

			Eventually(getPodPhase).Should(
				Equal(corev1.PodSucceeded),
			)

			req := podsClient.GetLogs(pod.Name, &corev1.PodLogOptions{})
			logs, err := req.Stream()
			Expect(err).NotTo(HaveOccurred())
			defer logs.Close()

			var buffer bytes.Buffer
			_, err = io.Copy(&buffer, logs)

			Expect(err).NotTo(HaveOccurred())
			Expect(buffer.String()).To(ContainSubstring(fmt.Sprintf("VAULT_RESOLVED_KEY=%s", SentinelSecretValue)))
		})
	})
}
