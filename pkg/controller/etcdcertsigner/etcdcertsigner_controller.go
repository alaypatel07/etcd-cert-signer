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
	//TODO: think of better name
	CertificateEtcdIdentity = "auth.openshift.io/certificate-etcd-identity"
)

// Add creates a new EtcdCertSigner Controller and adds it to the Manager. The Manager will set fields on the Controller
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

	// Fetch the Pod pod
	pod := &corev1.Pod{}
	err := r.client.Get(context.TODO(), request.NamespacedName, pod)
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

	if ok := etcdPod(pod.GetLabels()); !ok {
		// Not an etcd pod, remove the key from the queue
		reqLogger.Info("Skip reconcile: Not an etcd pod", "Pod.Namespace", pod.Namespace, "Pod.Name", pod.Name)
		return reconcile.Result{}, nil
	}

	// TODO: change namespace to openshift-config-managed
	etcdCA, err := r.getSecret(etcdCASecretName, etcdCASecretNamespace)
	if err != nil {
		if errors.IsNotFound(err) {
			reqLogger.Error(err, "CA Secret does not exist", "Secret.Namespace", etcdCASecretNamespace, "Secret.Name", etcdCASecretName)
			return reconcile.Result{}, err
		} else {
			reqLogger.Error(err, "Error getting CA Secret", "Secret.Namespace ", etcdCASecretNamespace, "Secret.Name", etcdCASecretName)
		}
	}

	peerSecret, err := r.getSecret(getPeerSecretName(pod), pod.Namespace)
	fmt.Println(peerSecret)
	if err != nil {
		if errors.IsNotFound(err) {
			reqLogger.Error(err, "Peer secret does not exists", "Secret.Namespace ", pod.Namespace, "Secret.Name", getPeerSecretName(pod))
		} else {
			reqLogger.Error(err, "Error getting peer secret", "Secret.Namespace ", pod.Namespace, "Secret.Name", getPeerSecretName(pod))
		}
		return reconcile.Result{}, err
	}
	if _, ok := peerSecret.Data["tls.crt"]; !ok {
		//this controller assumes that secret for CA is populated
		// create the peer certs only if they dont exists
		pCert, pKey, err := getCerts(etcdCA, peerSecret, "system:peers")
		if err != nil {
			reqLogger.Error(err, "Error signing the certificate")
		}

		err = r.populateSecret(peerSecret, pCert, pKey)
		if err != nil {
			reqLogger.Error(err, "Unable to update peer secret", "Secret.Namespace", peerSecret.Namespace, "Secret.Name", peerSecret.Name)
		}
	}

	serverSecret, err := r.getSecret(getServerSecretName(pod), pod.Namespace)
	if err != nil {
		if errors.IsNotFound(err) {
			reqLogger.Error(err, "Server secret does not exists", "Secret.Namespace ", pod.Namespace, "Secret.Name", getServerSecretName(pod))
		} else {
			reqLogger.Error(err, "Error getting server secret", "Secret.Namespace ", pod.Namespace, "Secret.Name", getServerSecretName(pod))
		}
		return reconcile.Result{}, err
	}
	if _, ok := serverSecret.Data["tls.crt"]; !ok {
		//this controller assumes that secret for CA is populated
		// create server the certs only if they dont exists
		sCert, sKey, err := getCerts(etcdCA, peerSecret, "system:servers")
		if err != nil {
			reqLogger.Error(err, "Error signing the certificate")
		}

		err = r.populateSecret(serverSecret, sCert, sKey)
		if err != nil {
			reqLogger.Error(err, "Unable to update server secret", "Secret.Namespace", serverSecret.Namespace, "Secret.Name", serverSecret.Name)
		}
	}

	//m, err := r.getSecret(pod.Name + "-metrics", pod.Namespace)
	//if err != nil {
	//	if errors.IsNotFound(err) {
	//		reqLogger.Error(err, "Metrics secret does not exists", "Metrics Secret Namespace ", pod.Namespace, "Secret.Name", pod.Name + "-metrics")
	//	} else {
	//		reqLogger.Error(err, "Error getting metrics secret", "Secret Namespace ", pod.Namespace, "Secret.Name", pod.Name + "-metrics")
	//	}
	//	return reconcile.Result{}, err
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
		// TODO: some extensions are missing form cfssl.
		// e.g.
		//	X509v3 Subject Key Identifier:
		//		B7:30:0B:CF:47:4E:21:AE:13:60:74:42:B0:D9:C4:F3:26:69:63:03
		//	X509v3 Authority Key Identifier:
		//		keyid:9B:C0:6B:0C:8E:5C:73:6A:83:B1:E4:54:97:D3:62:18:8A:9C:BC:1E
		// TODO: Change serial number logic, to something as follows.
		// The following is taken from CFSSL library.
		// If CFSSL is providing the serial numbers, it makes
		// sense to use the max supported size.

		//	serialNumber := make([]byte, 20)
		//	_, err = io.ReadFull(rand.Reader, serialNumber)
		//	if err != nil {
		//		return err
		//	}
		//
		//	// SetBytes interprets buf as the bytes of a big-endian
		//	// unsigned integer. The leading byte should be masked
		//	// off to ensure it isn't negative.
		//	serialNumber[0] &= 0x7F
		//	cert.SerialNumber = new(big.Int).SetBytes(serialNumber)
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
	if _, ok := secret.Data["tls.key"]; !ok {
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
	//TODO: Update annotations Not Before and Not After for Cert Rotation
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

func getPeerSecretName(p *corev1.Pod) string {
	return p.Name + "-peer"
}

func getServerSecretName(p *corev1.Pod) string {
	return p.Name + "-server"
}

func getMetricsSecretName(p *corev1.Pod) string {
	return p.Name + "-metrics"
}
