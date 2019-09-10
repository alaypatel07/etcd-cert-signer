package etcdcertsigner

import (
	"bytes"
	"context"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"strings"
	"time"

	"github.com/openshift/library-go/pkg/crypto"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_certificatesigningrequest")
var etcdCASecretName = "etcd-ca"
var etcdCASecretNamespace = "openshift-etcd"

const EtcdCertValidity = 3 * 365 * 24 * time.Hour
const (
	// CertificateNotBeforeAnnotation contains the certificate expiration date in RFC3339 format.
	CertificateNotBeforeAnnotation = "auth.openshift.io/certificate-not-before"
	// CertificateNotAfterAnnotation contains the certificate expiration date in RFC3339 format.
	CertificateNotAfterAnnotation = "auth.openshift.io/certificate-not-after"
	// CertificateIssuer contains the common name of the certificate that signed another certificate.
	CertificateIssuer = "auth.openshift.io/certificate-issuer"
	// CertificateHostnames contains the hostnames used by a signer.
	CertificateHostnames = "auth.openshift.io/certificate-hostnames"
	//Todo: think of better name
	CertificateEtcdIdentity = "auth.openshift.io/certificate-etcd-identity"
)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new CertificateSigningRequest Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &EtcdCertSigner{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("certificatesigningrequest-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource CertificateSigningRequest
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	//	// Watch for changes to secondary resource Pods and requeue the owner CertificateSigningRequest
	//	//err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
	//	//	IsController: true,
	//	//	OwnerType:    &certificatesv1beta1.CertificateSigningRequest{},
	//	//})
	//	//if err != nil {
	//	//	return err
	//	//}

	return nil
}

// blank assignment to verify that EtcdCertSigner implements reconcile.Reconciler
var _ reconcile.Reconciler = &EtcdCertSigner{}

// EtcdCertSigner reconciles a CertificateSigningRequest object
type EtcdCertSigner struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile watches on etcd cluster pods and checks if secrets for their certs are appropriately created.
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *EtcdCertSigner) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling CertificateSigningRequest")

	// Fetch the Pod instance
	instance := &corev1.Pod{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Error(err, "Skip reconcile: Pod not found", "Pod.Namespace", request.Namespace, "Pod.Name", request.Name)
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		reqLogger.Error(err, "Skip reconcile: Error getting pod", "Pod.Namespace", request.Namespace, "Pod.Name", request.Name)
		return reconcile.Result{}, err
	}

	if ok := etcdPod(instance.GetLabels()); !ok {
		// Not an etcd pod, remove the key from the queue
		reqLogger.Info("Skip reconcile: Not an etcd pod", "Pod.Namespace", instance.Namespace, "Pod.Name", instance.Name)
		return reconcile.Result{}, nil
	}

	// Todo: change namespace to openshift-config-managed
	etcdCA, err := r.getSecret(etcdCASecretName, etcdCASecretNamespace)
	if err != nil {
		if errors.IsNotFound(err) {
			reqLogger.Error(err, "CA Secret does not exist", "Secret.Namespace", etcdCASecretNamespace, "Secret.Name", etcdCASecretName)
			return reconcile.Result{Requeue: true}, err
		} else {
			reqLogger.Error(err, "Error getting CA Secret", "Secret.Namespace ", etcdCASecretNamespace, "Secret.Name", etcdCASecretName)
		}

	}

	p, err := r.getSecret(instance.Name+"-peer", instance.Namespace)
	fmt.Println(p)
	if err != nil {
		if errors.IsNotFound(err) {
			reqLogger.Error(err, "Peer secret does not exists", "Secret.Namespace ", instance.Namespace, "Secret.Name", instance.Name+"-peer")
		} else {
			reqLogger.Error(err, "Error getting peer secret", "Secret.Namespace ", instance.Namespace, "Secret.Name", instance.Name+"-peer")
		}
		return reconcile.Result{Requeue: true}, err
	}

	//this controller assumes that secret for CA is populated
	pCert, pKey, err := getCerts(etcdCA, p, "system:peers")
	if err != nil {
		reqLogger.Error(err, "Error signing the certificate")
	}

	err = r.populateSecret(p, pCert, pKey)
	if err != nil {
		reqLogger.Error(err, "Unable to update peer secret", "Secret.Namespace", p.Namespace, "Secret.Name", p.Name)
	}

	s, err := r.getSecret(instance.Name+"-server", instance.Namespace)
	if err != nil {
		if errors.IsNotFound(err) {
			reqLogger.Error(err, "Server secret does not exists", "Secret.Namespace ", instance.Namespace, "Secret.Name", instance.Name+"-server")
		} else {
			reqLogger.Error(err, "Error getting server secret", "Secret.Namespace ", instance.Namespace, "Secret.Name", instance.Name+"-server")
		}
		return reconcile.Result{Requeue: true}, err
	}
	//this controller assumes that secret for CA is populated
	sCert, sKey, err := getCerts(etcdCA, p, "system:servers")
	if err != nil {
		reqLogger.Error(err, "Error signing the certificate")
	}

	err = r.populateSecret(s, sCert, sKey)
	if err != nil {
		reqLogger.Error(err, "Unable to update server secret", "Secret.Namespace", s.Namespace, "Secret.Name", s.Name)
	}

	//m, err := r.getSecret(instance.Name + "-metrics", instance.Namespace)
	//if err != nil {
	//	if errors.IsNotFound(err) {
	//		reqLogger.Error(err, "Metrics secret does not exists", "Metrics Secret Namespace ", instance.Namespace, "Secret.Name", instance.Name + "-metrics")
	//	} else {
	//		reqLogger.Error(err, "Error getting metrics secret", "Secret Namespace ", instance.Namespace, "Secret.Name", instance.Name + "-metrics")
	//	}
	//	return reconcile.Result{Requeue: true}, err
	//}

	//TODO populate metrics secret with signed certificate
	return reconcile.Result{}, nil
}

func getCerts(etcdCASecret *corev1.Secret, targetSecret *corev1.Secret, org string) (*bytes.Buffer, *bytes.Buffer, error) {
	err := ensureCASecret(etcdCASecret)
	if err != nil {
		return nil, nil, err
	}
	hostnames, err := getHostNamesFromSecret(targetSecret)
	if err != nil {
		return nil, nil, err
	}
	cn, err := getCommonNameFromSecret(targetSecret)
	etcdCAKeyPair, err := crypto.GetCAFromBytes(etcdCASecret.Data["tls.crt"], etcdCASecret.Data["tls.key"])
	if err != nil {
		return nil, nil, err
	}

	identity, ok := targetSecret.GetAnnotations()[CertificateEtcdIdentity]
	if !ok {
		return nil, nil, errors.NewBadRequest("Etcd Identity not found")
	}

	certConfig, err := etcdCAKeyPair.MakeServerCertForDuration(sets.NewString(hostnames...), EtcdCertValidity, func(cert *x509.Certificate) error {

		cert.Issuer = pkix.Name{
			OrganizationalUnit: []string{"openshift"},
			CommonName:         cn,
		}
		cert.Subject = pkix.Name{
			Organization: []string{org},
			CommonName:   identity,
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	certBytes := &bytes.Buffer{}
	keyBytes := &bytes.Buffer{}
	if err := certConfig.WriteCertConfig(certBytes, keyBytes); err != nil {
		return nil, nil, err
	}
	return certBytes, keyBytes, nil
}

func ensureCASecret(secret *corev1.Secret) error {
	if _, ok := secret.Data["tls.crt"]; !ok {
		return errors.NewBadRequest("CA Cert not found")
	}
	if _, ok := secret.Data["tls.pem"]; !ok {
		return errors.NewBadRequest("CA Pem not found")
	}
	return nil
}

func getCommonNameFromSecret(secret *corev1.Secret) (string, error) {
	if strings.Contains(secret.Name, "peer") || strings.Contains(secret.Name, "server") {
		return "etcd-signer", nil
	}
	if strings.Contains(secret.Name, "metric") {
		return "etcd-metric-signer", nil
	}
	return "", errors.NewBadRequest("Unable to recognise secret name")
}

func getHostNamesFromSecret(secret *corev1.Secret) ([]string, error) {
	hostnames, ok := secret.GetAnnotations()[CertificateHostnames]
	if !ok {
		return nil, errors.NewBadRequest("Hostnames not found")
	}
	return strings.Split(hostnames, ","), nil
}

func getEtcdIdentityFromSecret(secret *corev1.Secret) (string, error) {
	identity, ok := secret.GetAnnotations()[CertificateEtcdIdentity]
	if !ok {
		return "", errors.NewBadRequest("Etcd Identity not found")
	}
	return identity, nil
}

func (r EtcdCertSigner) getSecret(name string, namespace string) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	if err := r.client.Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: name}, secret); err != nil {
		return nil, err
	}
	return secret, nil
}

func (r *EtcdCertSigner) getConfigMap(name string, namespace string) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{}
	if err := r.client.Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: name}, cm); err != nil {
		return nil, err
	}
	return cm, nil
}

func (r *EtcdCertSigner) populateSecret(secret *corev1.Secret, cert *bytes.Buffer, key *bytes.Buffer) error {
	//Todo: Update annotations Not Before and Not After for Cert Rotation
	secret.Data["tls.crt"] = cert.Bytes()
	secret.Data["tls.key"] = key.Bytes()
	return r.client.Update(context.Background(), secret)
}

func etcdPod(labels map[string]string) bool {
	if l, ok := labels["k8s-app"]; ok && l == "etcd" {
		return true
	}
	return false
}
