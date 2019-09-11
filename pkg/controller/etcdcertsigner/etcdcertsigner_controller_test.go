package etcdcertsigner

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"math"
	"math/big"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"testing"
	"time"
)

func Test_etcdPod(t *testing.T) {
	type args struct {
		labels map[string]string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "Etcd pod",
			args: args{labels: map[string]string{
				"k8s-app": "etcd",
			}},
			want: true,
		},
		{
			name: "Etcd pod with other labels",
			args: args{labels: map[string]string{
				"k8s-app": "etcd",
				"foo":     "bar",
				"_":       "_",
			}},
			want: true,
		},
		{
			name: "Not an etcd pod",
			args: args{labels: map[string]string{
				"not-k8s-app": "etcd",
				"foo":         "bar",
			}},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := etcdPod(tt.args.labels); got != tt.want {
				t.Errorf("etcdPod() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEtcdCertSigner_getSecret(t *testing.T) {
	fakeSecret := &corev1.Secret{
		TypeMeta: v1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String(),
			Kind: "Secret"},
		ObjectMeta: v1.ObjectMeta{
			Name:      "etcd-0-peer",
			Namespace: "etcd-namespace",
		},
		Data:       nil,
		StringData: nil,
		Type:       corev1.SecretTypeTLS,
	}
	r := EtcdCertSigner{
		client: fake.NewFakeClient([]runtime.Object{fakeSecret}...),
		scheme: nil,
	}

	type args struct {
		name      string
		namespace string
	}
	tests := []struct {
		name    string
		args    args
		want    *corev1.Secret
		wantErr bool
	}{
		{
			name: "secret exists",
			args: args{
				name:      "etcd-0-peer",
				namespace: "etcd-namespace",
			},
			want:    fakeSecret,
			wantErr: false,
		},
		{
			name: "secret does not exists",
			args: args{
				name:      "etcd-1-peer",
				namespace: "etcd-namespace",
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := r.getSecret(tt.args.name, tt.args.namespace)
			if (err != nil) != tt.wantErr {
				t.Errorf("getSecret() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getSecret() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEtcdCertSigner_getConfigMap(t *testing.T) {
	fakeConfigMap := &corev1.ConfigMap{
		TypeMeta: v1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String(),
			Kind: "ConfigMap"},
		ObjectMeta: v1.ObjectMeta{
			Name:      "etcd-ca-configmap",
			Namespace: "etcd-namespace",
		},
		Data:       nil,
		BinaryData: nil,
	}
	fakeClient := fake.NewFakeClient([]runtime.Object{fakeConfigMap}...)
	r := EtcdCertSigner{
		client: fakeClient,
		scheme: nil,
	}

	type args struct {
		name      string
		namespace string
	}
	tests := []struct {
		name    string
		args    args
		want    *corev1.ConfigMap
		wantErr bool
	}{
		{
			name: "ConfigMap exists",
			args: args{
				name:      "etcd-ca-configmap",
				namespace: "etcd-namespace",
			},
			want:    fakeConfigMap,
			wantErr: false,
		},
		{
			name: "ConfigMap does not exists",
			args: args{
				name:      "etcd-1-peer",
				namespace: "etcd-namespace",
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := r.getConfigMap(tt.args.name, tt.args.namespace)
			if (err != nil) != tt.wantErr {
				t.Errorf("getConfigMap() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getConfigMap() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getCerts(t *testing.T) {
	serial, err := rand.Int(rand.Reader, new(big.Int).SetInt64(math.MaxInt64))
	if err != nil {
		return
	}
	ca := &x509.Certificate{
		SerialNumber: serial,
		Issuer: pkix.Name{
			OrganizationalUnit: []string{"openshift"},
			CommonName:         "etcd-signer",
		},
		Subject: pkix.Name{
			OrganizationalUnit: []string{"openshift"},
			CommonName:         "etcd-signer",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	caPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return
	}

	caBytes, err := x509.CreateCertificate(rand.Reader, ca, ca, &caPrivKey.PublicKey, caPrivKey)
	if err != nil {
		return
	}

	caPEM := new(bytes.Buffer)
	pem.Encode(caPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caBytes,
	})

	caPrivKeyPEM := new(bytes.Buffer)
	pem.Encode(caPrivKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(caPrivKey),
	})

	type args struct {
		etcdCASecret *corev1.Secret
		targetSecret *corev1.Secret
		org          string
	}

	validArgs := args{
		etcdCASecret: &corev1.Secret{
			TypeMeta: v1.TypeMeta{
				APIVersion: corev1.SchemeGroupVersion.String(),
				Kind:       "Secret",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:      "foo",
				Namespace: "bar",
			},
			Data: map[string][]byte{
				"tls.crt": caPEM.Bytes(),
				"tls.key": caPrivKeyPEM.Bytes(),
			},
			Type: corev1.SecretTypeTLS,
		},
		targetSecret: &corev1.Secret{
			TypeMeta: v1.TypeMeta{
				APIVersion: corev1.SchemeGroupVersion.String(),
				Kind:       "Secret",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:      "foo",
				Namespace: "bar",
				Annotations: map[string]string{
					CertificateHostnames:    "localhost,etcd-0.etcd.test,*.etcd.test,10.10.10.10",
					CertificateEtcdIdentity: "system:peer:etcd-0.etcd.test",
					CertificateIssuer:       "etcd-signer",
				},
			},
			Type: corev1.SecretTypeTLS,
		},
		org: "system:peers",
	}
	tests := []struct {
		name    string
		args    args
		want    *bytes.Buffer
		want1   *bytes.Buffer
		wantErr bool
	}{
		{
			name:    "Valid test",
			args:    validArgs,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1, err := getCerts(tt.args.etcdCASecret, tt.args.targetSecret, tt.args.org)
			if (err != nil) != tt.wantErr {
				t.Errorf("getCerts() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(got.Bytes()) == 0 || len(got1.Bytes()) == 0 {
					t.Errorf("getCerts() cert = %v, key %v", got.String(), got1.String())
					return
				}
				//cert, err := x509.ParseCertificate(got.Bytes())
				if err != nil {
					t.Errorf("Cannot parse created certs %v", err)
					return
				}

				fmt.Println(got.String())
				fmt.Printf("key")
				fmt.Println(got1.String())
			}
		})
	}
}
