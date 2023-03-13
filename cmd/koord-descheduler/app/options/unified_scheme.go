package options

import (
	kruisepolicyv1alpha1 "github.com/openkruise/kruise-api/policy/v1alpha1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

func init() {
	_ = kruisepolicyv1alpha1.AddToScheme(clientgoscheme.Scheme)
	_ = kruisepolicyv1alpha1.AddToScheme(scheme)
}
