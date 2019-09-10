package etcdcertsigner

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"testing"
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
