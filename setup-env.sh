#!/usr/bin/env bash

kubectl create secret tls etcd-ca --cert=./tmp/tls.crt --key=./tmp/tls.key

cat <<EOF | kubectl create -f -
apiVersion: v1
kind: Secret
metadata:
  annotations:
    auth.openshift.io/certificate-hostnames: "localhost,etcd-0.etcd.test,*.etcd.test,10.10.10.10"
    auth.openshift.io/certificate-etcd-identity: "system:peer:etcd-0.etcd.test"
  name: trial-peer
  namespace: default
EOF

cat <<EOF | kubectl create -f -
apiVersion: v1
kind: Secret
metadata:
  annotations:
    openshift.io/certificate-hostnames: "localhost,etcd-0.etcd.test,*.etcd.test,10.10.10.10"
    auth.openshift.io/certificate-etcd-identity: "system:server:etcd-0.etcd.test"
  name: trial-server
  namespace: default
EOF

cat <<EOF | kubectl create -f -
apiVersion: v1
kind: Pod
metadata:
  labels:
    k8s-app: "etcd"
  name: trial
  namespace: default
spec:
  containers:
  - image: busybox
    command:
      - sleep
      - "3600"
    imagePullPolicy: IfNotPresent
    name: busybox
  restartPolicy: Always
EOF