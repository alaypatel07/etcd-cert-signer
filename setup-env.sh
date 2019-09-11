#!/usr/bin/env bash

kubectl create secret tls etcd-ca -n default --cert=./tmp/tls.crt --key=./tmp/tls.key

cat <<EOF | kubectl create -f -
apiVersion: v1
kind: Secret
metadata:
  annotations:
    auth.openshift.io/certificate-hostnames: "localhost,etcd-1,127.0.0.1,172.30.66.10"
    auth.openshift.io/certificate-etcd-identity: "system:peer:etcd-1"
  name: etcd-1-peer
  namespace: default
EOF

cat <<EOF | kubectl create -f -
apiVersion: v1
kind: Secret
metadata:
  annotations:
    openshift.io/certificate-hostnames: "localhost,etcd-1,127.0.0.1,172.30.66.10"
    auth.openshift.io/certificate-etcd-identity: "system:server:etcd-1"
  name: etcd-1-server
  namespace: default
EOF

cat <<EOF | kubectl create -f -
apiVersion: v1
kind: Secret
metadata:
  annotations:
    auth.openshift.io/certificate-hostnames: "localhost,etcd-2,127.0.0.1,172.30.66.11"
    auth.openshift.io/certificate-etcd-identity: "system:peer:etcd-2"
  name: etcd-2-peer
  namespace: default
EOF

cat <<EOF | kubectl create -f -
apiVersion: v1
kind: Secret
metadata:
  annotations:
    openshift.io/certificate-hostnames: "localhost,etcd-2,127.0.0.1,172.30.66.11"
    auth.openshift.io/certificate-etcd-identity: "system:server:etcd-2"
  name: etcd-2-server
  namespace: default
EOF


cat <<EOF | kubectl create -f -
apiVersion: v1
kind: Secret
metadata:
  annotations:
    auth.openshift.io/certificate-hostnames: "localhost,etcd-3,127.0.0.1,172.30.66.12"
    auth.openshift.io/certificate-etcd-identity: "system:peer:etcd-3"
  name: etcd-3-peer
  namespace: default
EOF

cat <<EOF | kubectl create -f -
apiVersion: v1
kind: Secret
metadata:
  annotations:
    openshift.io/certificate-hostnames: "localhost,etcd-3,127.0.0.1,172.30.66.12"
    auth.openshift.io/certificate-etcd-identity: "system:server:etcd-3"
  name: etcd-3-server
  namespace: default
EOF

cat <<EOF | kubectl create -f -
apiVersion: v1
kind: Pod
metadata:
  labels:
    k8s-app: "etcd"
  name: etcd-1
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

cat <<EOF | kubectl create -f -
apiVersion: v1
kind: Pod
metadata:
  labels:
    k8s-app: "etcd"
  name: etcd-2
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

cat <<EOF | kubectl create -f -
apiVersion: v1
kind: Pod
metadata:
  labels:
    k8s-app: "etcd"
  name: etcd-3
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

sleep 10

cat <<EOF | kubectl create -f -
apiVersion: v1
kind: Pod
metadata:
  name: etcd-certs
  namespace: default
spec:
  containers:
  - image: busybox
    command:
      - sleep
      - "3600"
    imagePullPolicy: IfNotPresent
    name: busybox
    volumeMounts:
    - name: server-1-certs
      mountPath: "/etc/ssl/certs1/server"
    - name: peer-1-certs
      mountPath: "/etc/ssl/certs1/peer"
    - name: server-2-certs
      mountPath: "/etc/ssl/certs2/server"
    - name: peer-2-certs
      mountPath: "/etc/ssl/certs2/peer"
    - name: server-3-certs
      mountPath: "/etc/ssl/certs3/server"
    - name: peer-3-certs
      mountPath: "/etc/ssl/certs3/peer"
  volumes:
  - name: server-1-certs
    secret:
      secretName: etcd-1-server
  - name: peer-1-certs
    secret:
      secretName: etcd-1-peer
  - name: server-2-certs
    secret:
      secretName: etcd-2-server
  - name: peer-2-certs
    secret:
      secretName: etcd-2-peer
  - name: server-3-certs
    secret:
      secretName: etcd-3-server
  - name: peer-3-certs
    secret:
      secretName: etcd-3-peer
  restartPolicy: Always
EOF