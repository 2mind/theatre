PROJECT=github.com/lawrencejones/operators

.PHONY: codegen

codegen:
	vendor/k8s.io/code-generator/generate-groups.sh all \
		$(PROJECT)/pkg/client \
		$(PROJECT)/pkg/apis \
		rbac:v1alpha1
